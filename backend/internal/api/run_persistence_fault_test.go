package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"webide-backend/internal/store"
	"webide-backend/internal/tasks"
)

func TestEventPersistenceFailureStopsNextMutationAndPreservesFailureReason(t *testing.T) {
	env := newAPITestAIEnv(t)
	runs := store.NewAgentRunStore(env.db)
	if err := runs.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	env.ai.AttachRunStore(runs)
	requests := 0
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var calls []map[string]any
		for _, name := range []string{"first.md", "second.md"} {
			args, _ := json.Marshal(map[string]any{"patch": "*** Begin Patch\n*** Add File: " + name + "\n+test\n*** End Patch"})
			calls = append(calls, map[string]any{"id": name, "type": "function", "function": map[string]any{"name": "apply_workspace_patch", "arguments": string(args)}})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "tool_calls": calls}, "finish_reason": "tool_calls"}}})
	}))
	defer providerServer.Close()
	provider, err := env.configs.CreateModelProvider(t.Context(), env.admin.ID, store.ModelProviderInput{Name: "test", Protocol: "openai", BaseURL: providerServer.URL, APIKey: "test", ModelID: "mock", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := env.conversations.CreateConversation(t.Context(), store.CreateConversationInput{UserID: env.admin.ID, Title: "test", SelectedModelProviderID: provider.ID})
	if err != nil {
		t.Fatal(err)
	}
	message, err := env.conversations.AppendMessage(t.Context(), env.admin.ID, conv.ID, "user", "create two files", "[]")
	if err != nil {
		t.Fatal(err)
	}
	run, err := runs.CreateRun(t.Context(), store.CreateAgentRunInput{UserID: env.admin.ID, ConversationID: conv.ID, UserMessageID: message.ID, ModelProviderID: provider.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = env.db.Exec(`CREATE TRIGGER reject_tool_event BEFORE INSERT ON agent_events WHEN NEW.type='tool_call_completed' BEGIN SELECT RAISE(FAIL,'injected event storage failure'); END;`)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{ai: env.ai, runs: runs, configs: env.configs, conversations: env.conversations, factory: env.factory, runBroker: tasks.NewBroker()}
	s.executeConversationRun(t.Context(), env.admin.ID, "admin", conv.ID, message.ID, provider.ID, message.Content, "[]", nil, "standard", run.ID)
	final, err := runs.GetRun(t.Context(), env.admin.ID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != store.AgentRunFailed || !strings.Contains(final.ErrorMessage.String, "injected event storage failure") {
		t.Fatalf("wrong failure/cancellation: %s %s", final.Status, final.ErrorMessage.String)
	}
	files, err := env.factory.Workspace(env.admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = files.ReadFile("first.md"); err != nil {
		t.Fatal("first mutation did not run", err)
	}
	if _, err = files.StatPath("second.md"); err == nil {
		t.Fatal("next mutation executed after persistence failure")
	}
	if requests != 1 {
		t.Fatalf("continued provider requests: %d", requests)
	}
	var count int
	if err = env.db.QueryRow(`SELECT COUNT(*) FROM tool_calls WHERE run_id=? AND status='success'`, run.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("ledger should remain usable: count=%d err=%v", count, err)
	}
}
