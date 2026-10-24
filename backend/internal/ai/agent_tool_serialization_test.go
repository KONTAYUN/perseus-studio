package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"webide-backend/internal/store"
)

func newSerializationTestRuntime(t *testing.T, env *testAIEnv, id string) *agentRuntime {
	t.Helper()
	run, err := env.runs.CreateRun(context.Background(), store.CreateAgentRunInput{
		ID:              "run_" + id,
		UserID:          testUserID,
		ConversationID:  "conv_" + id,
		UserMessageID:   "msg_" + id,
		ModelProviderID: "provider_" + id,
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	return &agentRuntime{
		service:          env.service,
		protocol:         agentProtocolOpenAI,
		conversationID:   "conv_" + id,
		messageID:        "msg_" + id,
		opts:             RunOptions{UserID: testUserID, RunID: run.ID},
		allowTask:        true,
		loadedToolsets:   allAgentToolsets(true),
		appliedMutations: map[string]ToolEvent{},
		toolLoopActive:   true,
		transcript:       newAgentTranscript(),
		contextManager:   newAgentContextManager(),
		currentRound:     1,
	}
}

func patchToolCall(t *testing.T, id, patch string) agentToolCall {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"patch": patch})
	if err != nil {
		t.Fatal(err)
	}
	return agentToolCall{ID: id, Name: "apply_workspace_patch", ArgumentsJSON: string(raw)}
}

// Regression: a model that "rewrites" a file by returning a delete call and a
// re-create call in one turn must not have only the delete executed.
func TestDeleteThenRecreateBatchIsHeldWithoutTouchingFile(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	const path = "ROSE/script.md"
	const original = "第一场 外景\n亚瑟：你好。\n"
	if _, err := env.files.CreateTextFile(ctx, path, original, "test"); err != nil {
		t.Fatalf("CreateTextFile returned error: %v", err)
	}
	runtime := newSerializationTestRuntime(t, env, "held_batch")
	turn := agentModelTurn{ToolCalls: []agentToolCall{
		patchToolCall(t, "call_delete", "*** Begin Patch\n*** Delete File: "+path+"\n*** End Patch"),
		patchToolCall(t, "call_add", "*** Begin Patch\n*** Add File: "+path+"\n+## 1. 外景\n+**亚瑟**：你好。\n*** End Patch"),
	}}
	turn, skipped := prepareToolTurnForExecution(turn)
	if len(turn.ToolCalls) != 1 || len(skipped) != 1 || skipped[0].ID != "call_add" {
		t.Fatalf("expected first call kept and second returned as skipped, got kept=%#v skipped=%#v", turn.ToolCalls, skipped)
	}
	runtime.appendAssistantTurn(turn)

	if request, completed, err := runtime.processSerializedToolCalls(ctx, turn.ToolCalls, skipped); err != nil || completed || request != nil {
		t.Fatalf("processSerializedToolCalls returned request=%v completed=%v err=%v", request, completed, err)
	}

	read, err := env.files.ReadFile(path)
	if err != nil {
		t.Fatalf("file must still exist after a held batch: %v", err)
	}
	if read.Content != original {
		t.Fatalf("file content changed: %q", read.Content)
	}
	if items, err := env.files.TrashItems(); err != nil || len(items) != 0 {
		t.Fatalf("expected empty trash, got %#v err=%v", items, err)
	}
	last := runtime.transcript.Entries[len(runtime.transcript.Entries)-1]
	if len(last.ToolResults) != 1 || last.ToolResults[0].ToolCallID != "call_delete" || !last.ToolResults[0].IsError {
		t.Fatalf("expected one error result paired with the kept call, got %#v", last.ToolResults)
	}
	content := last.ToolResults[0].Content
	for _, want := range []string{"not_executed", "Replace File", "apply_workspace_patch", "never delete a file"} {
		if !strings.Contains(content, want) {
			t.Fatalf("held result must mention %q so the model can recover, got %s", want, content)
		}
	}
	if len(runtime.events) != 1 || runtime.events[0].Status != "failed" {
		t.Fatalf("expected one not-executed event, got %#v", runtime.events)
	}
}

func TestReadOnlyBatchRunsFirstCallAndReportsTheRest(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	for _, path := range []string{"a.md", "b.md"} {
		if _, err := env.files.CreateTextFile(ctx, path, "text "+path+"\n", "test"); err != nil {
			t.Fatalf("CreateTextFile returned error: %v", err)
		}
	}
	runtime := newSerializationTestRuntime(t, env, "read_batch")
	turn := agentModelTurn{ToolCalls: []agentToolCall{
		{ID: "call_a", Name: "read_workspace_text", ArgumentsJSON: `{"path":"a.md"}`},
		{ID: "call_b", Name: "read_workspace_text", ArgumentsJSON: `{"path":"b.md"}`},
	}}
	turn, skipped := prepareToolTurnForExecution(turn)
	runtime.appendAssistantTurn(turn)

	if _, completed, err := runtime.processSerializedToolCalls(ctx, turn.ToolCalls, skipped); err != nil || completed {
		t.Fatalf("processSerializedToolCalls returned completed=%v err=%v", completed, err)
	}
	if len(runtime.events) != 1 || runtime.events[0].Path != "a.md" || runtime.events[0].Status != "success" {
		t.Fatalf("expected only the first read to run, got %#v", runtime.events)
	}
	last := runtime.transcript.Entries[len(runtime.transcript.Entries)-1]
	if last.Kind != transcriptEntryRuntimeObservation {
		t.Fatalf("expected a runtime observation reporting skipped calls, got %#v", last)
	}
	if !strings.Contains(last.Text, "NOT executed") || !strings.Contains(last.Text, "b.md") {
		t.Fatalf("observation must name the skipped call, got %q", last.Text)
	}
}

func TestReplaceFilePatchRewritesExistingFileInPlace(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	const path = "ROSE/script.md"
	if _, err := env.files.CreateTextFile(ctx, path, "旧内容\n第二行\n", "test"); err != nil {
		t.Fatalf("CreateTextFile returned error: %v", err)
	}

	result, err := applyWorkspacePatch(ctx, env.files, "*** Begin Patch\n*** Replace File: "+path+"\n+## 1. 外景\n+\n+**亚瑟**：你好。\n*** End Patch")
	if err != nil {
		t.Fatalf("Replace File returned error: %v", err)
	}
	read, err := env.files.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if read.Content != "## 1. 外景\n\n**亚瑟**：你好。\n" {
		t.Fatalf("unexpected replaced content: %q", read.Content)
	}
	if result.Updated != 1 || result.Deleted != 0 || len(result.Changes) != 1 || result.Changes[0].Operation != "update" {
		t.Fatalf("replace must be recorded as a single update, got %#v", result)
	}
	if items, err := env.files.TrashItems(); err != nil || len(items) != 0 {
		t.Fatalf("replace must not use the trash, got %#v err=%v", items, err)
	}
}

func TestReplaceFilePatchRejectsUnsafeInput(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	const path = "keep.md"
	const original = "保留\n"
	if _, err := env.files.CreateTextFile(ctx, path, original, "test"); err != nil {
		t.Fatalf("CreateTextFile returned error: %v", err)
	}
	cases := map[string]struct {
		patch string
		want  string
	}{
		"missing file":          {"*** Begin Patch\n*** Replace File: missing.md\n+x\n*** End Patch", "replace file not readable"},
		"empty content":         {"*** Begin Patch\n*** Replace File: " + path + "\n*** End Patch", "no content lines"},
		"unprefixed line":       {"*** Begin Patch\n*** Replace File: " + path + "\nx\n*** End Patch", "replace file lines must start with +"},
		"delete and add in one": {"*** Begin Patch\n*** Delete File: " + path + "\n*** Add File: " + path + "\n+x\n*** End Patch", "Replace File"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := applyWorkspacePatch(ctx, env.files, tc.patch)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
			read, readErr := env.files.ReadFile(path)
			if readErr != nil || read.Content != original {
				t.Fatalf("rejected patch must leave %s unchanged, got %q err=%v", path, read.Content, readErr)
			}
		})
	}
}

func TestShouldHoldToolBatchOnlyWhenTheExecutedCallDeletes(t *testing.T) {
	deletePatch := patchToolCall(t, "d", "*** Begin Patch\n*** Delete File: a.md\n*** End Patch")
	addA := patchToolCall(t, "a", "*** Begin Patch\n*** Add File: a.md\n+x\n*** End Patch")
	addB := patchToolCall(t, "b", "*** Begin Patch\n*** Add File: b.md\n+x\n*** End Patch")
	trash := agentToolCall{ID: "t", Name: "manage_workspace_path", ArgumentsJSON: `{"action":"trash","path":"a.md"}`}
	read := agentToolCall{ID: "r", Name: "read_workspace_text", ArgumentsJSON: `{"path":"a.md"}`}
	cases := []struct {
		name    string
		first   agentToolCall
		skipped []agentToolCall
		hold    bool
	}{
		{"patch delete then recreate", deletePatch, []agentToolCall{addA}, true},
		{"trash then recreate", trash, []agentToolCall{addA}, true},
		{"single delete is not a batch", deletePatch, nil, false},
		{"independent creations", addA, []agentToolCall{addB}, false},
		{"read then delete runs the read", read, []agentToolCall{deletePatch}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldHoldToolBatch([]agentToolCall{tc.first}, tc.skipped); got != tc.hold {
				t.Fatalf("shouldHoldToolBatch = %v, want %v", got, tc.hold)
			}
		})
	}
}
