package ai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"webide-backend/internal/document"
	"webide-backend/internal/store"
)

func TestDocumentRepairRuntimeStagesWithoutPublishingAndRejectsOtherCapabilities(t *testing.T) {
	env := newTestAIEnv(t)
	head, err := env.files.CreateFile(t.Context(), "draft.md", "_正文 123_\n")
	if err != nil {
		t.Fatal(err)
	}
	action := &document.Action{Type: "format_repair", Path: head.Path, BaseHash: head.Hash, Profile: document.Version, Diagnostics: []document.Issue{{Code: "conversion", Message: "测试转换诊断"}}}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var p map[string]any
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			t.Error(err)
			return
		}
		for _, raw := range p["tools"].([]any) {
			tool := raw.(map[string]any)
			name := tool["function"].(map[string]any)["name"].(string)
			if !action.Allows(name) {
				t.Errorf("forbidden advertised tool: %s", name)
			}
		}
		requests++
		switch requests {
		case 1:
			writeOpenAIToolCall(t, w, "read", "read_workspace_text", `{"path":"draft.md"}`)
		case 2:
			writeOpenAIToolCall(t, w, "stage", "apply_workspace_patch", mustJSON(map[string]any{"patch": "*** Begin Patch\n*** Update File: draft.md\n@@\n-_正文 123_\n+*正文 123*\n*** End Patch"}))
		default:
			writeOpenAIToolCall(t, w, "done", "attempt_completion", `{"outcome":"success","result":"格式建议已暂存，原稿未修改。","changed_paths":[]}`)
		}
	}))
	defer server.Close()
	p, c, m, messages := prepareOpenAIConversation(t, t.Context(), env, server.URL, "提供格式候选")
	run, err := env.runs.CreateRun(t.Context(), store.CreateAgentRunInput{UserID: testUserID, ConversationID: c.ID, UserMessageID: m.ID, ModelProviderID: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	opts := RunOptions{UserID: testUserID, RunID: run.ID, DocumentAction: action, MaxProviderRequests: 5}
	opts.Event = func(kind, level string, payload map[string]any) {
		if kind == "completion_rejected" {
			t.Logf("completion rejected: %v", payload)
		}
	}
	reply, err := env.service.GenerateReply(t.Context(), c.ID, m.ID, p.ID, messages, nil, opts)
	if err != nil || reply.Outcome != "success" || requests != 3 {
		t.Fatal("runtime did not finish", reply.Outcome, requests, err)
	}
	actual, _ := env.files.ReadFile(head.Path)
	if actual.Hash != head.Hash {
		t.Fatal("staging published a change")
	}
	candidates, err := env.files.DocumentRepairCandidates(t.Context(), head.Path)
	if err != nil || len(candidates) != 1 {
		t.Fatal("missing durable candidate", err, candidates)
	}
	restored := &agentRuntime{service: env.service, opts: opts, transcript: newAgentTranscript(), appliedMutations: map[string]ToolEvent{}}
	if err = restored.restoreDurableToolLedger(t.Context()); err != nil {
		t.Fatal(err)
	}
	if restored.performedMutation || len(restored.appliedMutations) != 0 {
		t.Fatal("staged advice restored as applied mutation")
	}
	for _, call := range []struct{ name, args string }{
		{"read_workspace_text", `{"path":"other.md"}`}, {"run_workspace_command", `{"command":"echo bad"}`},
		{"save_workspace_file", `{"path":"draft.md","content":"bad","mode":"overwrite"}`},
		{"start_background_task", `{"goal":"bypass"}`}, {"perseus_search_web", `{"query":"bypass"}`},
		{"apply_workspace_patch", `{"patch":"*** Begin Patch\n*** Delete File: draft.md\n*** End Patch"}`},
	} {
		if _, _, _, _, err := env.service.executeTool(t.Context(), call.name, call.args, opts); err == nil {
			t.Fatalf("scope bypass %s", call.name)
		}
	}
	checkpoint := restored.checkpoint()
	resumed := &agentRuntime{}
	resumed.restoreCheckpoint(checkpoint)
	if resumed.opts.DocumentAction == nil || resumed.opts.DocumentAction.Allows("save_workspace_file") {
		t.Fatal("repair policy disappeared on resume")
	}
}
