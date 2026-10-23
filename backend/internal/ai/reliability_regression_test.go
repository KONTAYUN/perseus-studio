package ai

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"webide-backend/internal/store"
	"webide-backend/internal/workspace"
)

func TestPatchDeleteConflictRollsBackEarlierWrites(t *testing.T) {
	env := newTestAIEnv(t)
	before, err := env.files.CreateFile(t.Context(), "a.txt", "before")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.files.WriteFile(t.Context(), "a.txt", "user changed", "test"); err != nil {
		t.Fatal(err)
	}
	_, err = commitWorkspacePatch(t.Context(), env.files,
		[]plannedWorkspacePatchWrite{{path: "created.txt", content: "agent", create: true}}, nil,
		[]plannedWorkspacePatchDelete{{path: "a.txt", hash: before.Hash}}, 1, 0, 1)
	if !errors.Is(err, workspace.ErrFileConflict) {
		t.Fatalf("stale deletion not rejected: %v", err)
	}
	if read, err := env.files.ReadFile("a.txt"); err != nil || read.Content != "user changed" {
		t.Fatalf("user edit lost: %+v %v", read, err)
	}
	if _, err := env.files.ReadFile("created.txt"); err == nil {
		t.Fatal("earlier create not rolled back")
	}
}

func TestCanceledPatchDoesNotMoveOrDelete(t *testing.T) {
	for _, op := range []string{"move", "delete"} {
		t.Run(op, func(t *testing.T) {
			env := newTestAIEnv(t)
			file, err := env.files.CreateFile(t.Context(), "a.txt", "keep")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			var moves []plannedWorkspacePatchMove
			var deletes []plannedWorkspacePatchDelete
			if op == "move" {
				moves = []plannedWorkspacePatchMove{{oldPath: "a.txt", newPath: "b.txt", hash: file.Hash}}
			} else {
				deletes = []plannedWorkspacePatchDelete{{path: "a.txt", hash: file.Hash}}
			}
			_, err = commitWorkspacePatch(ctx, env.files, nil, moves, deletes, 0, 0, 0)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled operation executed: %v", err)
			}
			if read, err := env.files.ReadFile("a.txt"); err != nil || read.Content != "keep" {
				t.Fatalf("file changed after cancel: %+v %v", read, err)
			}
		})
	}
}

func TestFreshRunRecoveryAndTerminalState(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := t.Context()
	conversation, err := env.conversations.CreateConversation(ctx, store.CreateConversationInput{UserID: testUserID})
	if err != nil {
		t.Fatal(err)
	}
	run, err := env.runs.CreateRun(ctx, store.CreateAgentRunInput{UserID: testUserID, ConversationID: conversation.ID, ModelProviderID: "model"})
	if err != nil {
		t.Fatal(err)
	}
	count, err := env.runs.FailStaleRunningRuns(ctx, 0, "restart")
	if err != nil || count != 1 {
		t.Fatalf("fresh run recovery: %d %v", count, err)
	}
	late, err := env.runs.UpdateRun(ctx, testUserID, run.ID, store.UpdateAgentRunInput{Status: store.AgentRunSucceeded, Stage: "completed"})
	if err != nil || late.Status != store.AgentRunFailed {
		t.Fatalf("late completion overwrote terminal state: %+v %v", late, err)
	}
	var winners atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := env.runs.ResumeRun(ctx, testUserID, run.ID); err == nil {
				winners.Add(1)
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("resume admitted %d times", winners.Load())
	}
}

func TestDurableLedgerGuardsInterruptedSideEffects(t *testing.T) {
	for _, tool := range []string{"apply_workspace_patch", "run_workspace_command", "start_background_task"} {
		t.Run(tool, func(t *testing.T) {
			env := newTestAIEnv(t)
			_, err := env.conversations.StartToolCall(t.Context(), store.ToolCallInput{UserID: testUserID, RunID: "run", ConversationID: "conv", ToolName: tool})
			if err != nil {
				t.Fatal(err)
			}
			r := &agentRuntime{service: env.service, opts: RunOptions{UserID: testUserID, RunID: "run"}, appliedMutations: map[string]ToolEvent{}}
			if err := r.restoreDurableToolLedger(t.Context()); err == nil || !strings.Contains(err.Error(), "不会盲目重试") {
				t.Fatalf("unsafe recovery: %v", err)
			}
		})
	}
}

func TestDurableLedgerRestoresMutationWithoutDuplicate(t *testing.T) {
	env := newTestAIEnv(t)
	result, _, _, _, execErr := env.service.executeTool(t.Context(), "apply_workspace_patch", mustJSON(map[string]any{"patch": "*** Begin Patch\n*** Add File: a.txt\n+hello\n*** End Patch"}), RunOptions{UserID: testUserID})
	if execErr != nil {
		t.Fatal(execErr)
	}
	_, err := env.conversations.RecordToolCall(t.Context(), store.ToolCallInput{UserID: testUserID, RunID: "run", ConversationID: "conv", ToolName: "apply_workspace_patch", OperationHash: "hash", ResultJSON: result, ChangedPathsJSON: `["a.txt"]`})
	if err != nil {
		t.Fatal(err)
	}
	r := &agentRuntime{service: env.service, opts: RunOptions{UserID: testUserID, RunID: "run"}, appliedMutations: map[string]ToolEvent{}}
	for range 2 {
		if err := r.restoreDurableToolLedger(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.events) != 1 || len(r.appliedMutations) != 1 || !r.performedMutation || !r.requiresCompletion {
		t.Fatalf("ledger not restored exactly once: %+v", r.events)
	}
	if _, err := env.files.WriteFile(t.Context(), "a.txt", "newer user content", "test"); err != nil {
		t.Fatal(err)
	}
	fresh := &agentRuntime{service: env.service, opts: r.opts, appliedMutations: map[string]ToolEvent{}}
	if err := fresh.restoreDurableToolLedger(t.Context()); err == nil {
		t.Fatal("restored a stale write result after user modification")
	}
}

func TestCompletedCommandWithoutCheckpointIsNotBlindlyReplayed(t *testing.T) {
	env := newTestAIEnv(t)
	call, err := env.conversations.RecordToolCall(t.Context(), store.ToolCallInput{UserID: testUserID, RunID: "run", ConversationID: "conv", ToolName: "run_workspace_command", Status: "success", ResultJSON: `{}`})
	if err != nil {
		t.Fatal(err)
	}
	r := &agentRuntime{service: env.service, opts: RunOptions{UserID: testUserID, RunID: "run"}, appliedMutations: map[string]ToolEvent{}}
	if err := r.restoreDurableToolLedger(t.Context()); err == nil {
		t.Fatal("uncheckpointed command was allowed to replay")
	}
	r.events = []ToolEvent{{ToolCallID: call.ID, ToolName: call.ToolName, Status: "success"}}
	if err := r.restoreDurableToolLedger(t.Context()); err != nil {
		t.Fatalf("known checkpointed command blocked: %v", err)
	}
}

func TestCorruptCheckpointStopsRecovery(t *testing.T) {
	env := newTestAIEnv(t)
	run, err := env.runs.CreateRun(t.Context(), store.CreateAgentRunInput{UserID: testUserID, ConversationID: "conv", ModelProviderID: "model"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.runs.SaveCheckpoint(t.Context(), testUserID, run.ID, "broken-json"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := env.service.loadRunCheckpointStrict(t.Context(), RunOptions{UserID: testUserID, RunID: run.ID}); err == nil {
		t.Fatal("corrupt checkpoint treated as missing")
	}
}

func TestPatchVerifiesEveryFile(t *testing.T) {
	env := newTestAIEnv(t)
	opts := RunOptions{UserID: testUserID}
	result, _, _, _, err := env.service.executeTool(t.Context(), "apply_workspace_patch", mustJSON(map[string]any{"patch": "*** Begin Patch\n*** Add File: a.txt\n+first\n*** Add File: b.txt\n+second\n*** End Patch"}), opts)
	if err != nil {
		t.Fatal(err)
	}
	if v := env.service.verifyToolResult(t.Context(), "apply_workspace_patch", result, opts); v.Status != "verified" {
		t.Fatal(v)
	}
	if _, err := env.files.WriteFile(t.Context(), "b.txt", "user changed", "test"); err != nil {
		t.Fatal(err)
	}
	if v := env.service.verifyToolResult(t.Context(), "apply_workspace_patch", result, opts); v.Status != "failed" {
		t.Fatalf("second file was ignored: %+v", v)
	}
}

func TestPatchMoveAndDeleteVerification(t *testing.T) {
	for _, patch := range []string{
		"*** Begin Patch\n*** Update File: a.txt\n*** Move to: b.txt\n@@\n-old\n+new\n*** End Patch",
		"*** Begin Patch\n*** Delete File: a.txt\n*** End Patch",
	} {
		env := newTestAIEnv(t)
		if _, err := env.files.CreateTextFile(t.Context(), "a.txt", "old\n", "test"); err != nil {
			t.Fatal(err)
		}
		opts := RunOptions{UserID: testUserID}
		result, _, _, _, err := env.service.executeTool(t.Context(), "apply_workspace_patch", mustJSON(map[string]any{"patch": patch}), opts)
		if err != nil {
			t.Fatal(err)
		}
		if v := env.service.verifyToolResult(t.Context(), "apply_workspace_patch", result, opts); v.Status != "verified" {
			t.Fatalf("move/delete: %+v; %s", v, result)
		}
	}
}

func TestPatchRollbackPreservesNewerUserContent(t *testing.T) {
	env := newTestAIEnv(t)
	if _, err := env.files.CreateTextFile(t.Context(), "a.txt", "user content", "test"); err != nil {
		t.Fatal(err)
	}
	err := rollbackWorkspacePatchWrites(context.Background(), env.files, []plannedWorkspacePatchWrite{{path: "a.txt", content: "agent content", create: true}})
	if err == nil {
		t.Fatal("expected conflict")
	}
	read, err := env.files.ReadFile("a.txt")
	if err != nil || read.Content != "user content" {
		t.Fatalf("lost newer content: %+v %v", read, err)
	}
}
