package api

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"webide-backend/internal/failure"
	"webide-backend/internal/store"
	"webide-backend/internal/tasks"
)

func TestRunPersistenceDiagnosticsTreatContextCanceledAsExpectedCancellation(t *testing.T) {
	if !isExpectedRunCancellation(context.Canceled) {
		t.Fatal("expected context.Canceled to be treated as expected run cancellation")
	}
}

func TestRunPersistenceDiagnosticSummaryIncludesCancelContext(t *testing.T) {
	server := &Server{
		runCancels: map[string]*runCancelEntry{
			"run-1": {
				source:      "stop_button",
				reason:      "user_requested_stop",
				requestedAt: time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC),
			},
		},
	}
	raw := server.runPersistenceDiagnosticSummary("user-1", "run-1", "assistant_message_delta_failed", time.Now().Add(-time.Second))
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("invalid summary json: %v", err)
	}
	if payload["failure_class"] != "local_run_persistence" || payload["cancel_source"] != "stop_button" || payload["cancel_reason"] != "user_requested_stop" {
		t.Fatalf("unexpected summary: %#v", payload)
	}
}

func TestCancelAgentRunRecordsSourceAndReason(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	runs := store.NewAgentRunStore(db)
	if err := runs.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	run, err := runs.CreateRun(context.Background(), store.CreateAgentRunInput{
		ID:              "run-1",
		UserID:          "user-1",
		ConversationID:  "conv-1",
		UserMessageID:   "msg-1",
		ModelProviderID: "mp-1",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	if _, err := runs.UpdateRun(context.Background(), "user-1", run.ID, store.UpdateAgentRunInput{Status: store.AgentRunRunning, Stage: "calling_model"}); err != nil {
		t.Fatalf("UpdateRun returned error: %v", err)
	}
	canceled := false
	server := &Server{
		runs:       runs,
		runBroker:  tasks.NewBroker(),
		runCancels: map[string]*runCancelEntry{run.ID: {cancel: func() { canceled = true }}},
	}
	req := cancelAgentRunRequest{Source: "stop_button", Reason: "user_requested_stop"}
	if _, err := server.cancelAgentRun(context.Background(), "user-1", run.ID, req); err != nil {
		t.Fatalf("cancelAgentRun returned error: %v", err)
	}
	if !canceled {
		t.Fatal("expected cancel func to be called")
	}
	events, err := runs.ListEvents(context.Background(), "user-1", run.ID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected cancel event")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(events[0].PayloadJSON), &payload); err != nil {
		t.Fatalf("invalid event payload: %v", err)
	}
	if payload["source"] != "stop_button" || payload["reason"] != "user_requested_stop" {
		t.Fatalf("unexpected cancel payload: %#v", payload)
	}
}

func TestFailConversationRunCreatesDiagnosticForLocalRunFailure(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	configs := store.NewConfigStore(db)
	if err := configs.Migrate(context.Background()); err != nil {
		t.Fatalf("configs.Migrate returned error: %v", err)
	}
	provider, err := configs.CreateModelProvider(context.Background(), "admin-1", store.ModelProviderInput{
		Name:       "GLM",
		Visibility: store.ResourceVisibilityPublic,
		Protocol:   "openai",
		BaseURL:    "https://api.example.test/v1",
		APIKey:     "secret-key",
		ModelID:    "glm-test",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("CreateModelProvider returned error: %v", err)
	}
	runs := store.NewAgentRunStore(db)
	if err := runs.Migrate(context.Background()); err != nil {
		t.Fatalf("runs.Migrate returned error: %v", err)
	}
	run, err := runs.CreateRun(context.Background(), store.CreateAgentRunInput{
		ID:              "run-local-fail",
		UserID:          "user-1",
		ConversationID:  "conv-1",
		UserMessageID:   "msg-1",
		ModelProviderID: provider.ID,
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	if _, err := runs.UpdateRun(context.Background(), "user-1", run.ID, store.UpdateAgentRunInput{Status: store.AgentRunRunning, Stage: "calling_model", SetStartedAt: true}); err != nil {
		t.Fatalf("UpdateRun returned error: %v", err)
	}
	diagnostics := store.NewDiagnosticStore(db)
	if err := diagnostics.Migrate(context.Background()); err != nil {
		t.Fatalf("diagnostics.Migrate returned error: %v", err)
	}
	server := &Server{
		configs:     configs,
		diagnostics: diagnostics,
		runs:        runs,
		runBroker:   tasks.NewBroker(),
	}

	server.failConversationRunStartedAt(context.Background(), "user-1", "conv-1", "msg-1", run.ID, failure.New("agent_runtime_action_failed", "模型没有返回可执行工具调用或 attempt_completion，workspace 任务未执行完成", nil), time.Now().Add(-2*time.Second))

	updated, err := runs.GetRun(context.Background(), "user-1", run.ID)
	if err != nil {
		t.Fatalf("GetRun returned error: %v", err)
	}
	if !updated.DiagnosticLogID.Valid || updated.DiagnosticLogID.String == "" {
		t.Fatalf("expected run diagnostic id, got %#v", updated.DiagnosticLogID)
	}
	logs, err := diagnostics.List(context.Background(), store.DiagnosticLogQuery{UserID: "user-1", Limit: 10})
	if err != nil {
		t.Fatalf("diagnostics.List returned error: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected one diagnostic log, got %#v", logs)
	}
	if logs[0].Stage != "run_failed:calling_model" {
		t.Fatalf("unexpected stage: %q", logs[0].Stage)
	}
	if logs[0].ModelProviderID != provider.ID || logs[0].ModelID != "glm-test" || logs[0].Protocol != "openai" || logs[0].BaseURLHost != "api.example.test" {
		t.Fatalf("provider metadata missing from diagnostic: %#v", logs[0])
	}
	if strings.Contains(logs[0].ErrorMessage, "secret-key") || strings.Contains(logs[0].RequestSummary, "secret-key") {
		t.Fatalf("diagnostic leaked API key: %#v", logs[0])
	}
	var summary map[string]any
	if err := json.Unmarshal([]byte(logs[0].RequestSummary), &summary); err != nil {
		t.Fatalf("invalid request summary: %v", err)
	}
	if summary["failure_class"] != "agent_runtime_action_failed" || summary["run_stage"] != "calling_model" {
		t.Fatalf("unexpected summary: %#v", summary)
	}
}

func TestFailConversationRunDoesNotCreateDiagnosticForCanceledRun(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	runs := store.NewAgentRunStore(db)
	if err := runs.Migrate(context.Background()); err != nil {
		t.Fatalf("runs.Migrate returned error: %v", err)
	}
	run, err := runs.CreateRun(context.Background(), store.CreateAgentRunInput{
		ID:              "run-canceled",
		UserID:          "user-1",
		ConversationID:  "conv-1",
		UserMessageID:   "msg-1",
		ModelProviderID: "provider-1",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	diagnostics := store.NewDiagnosticStore(db)
	if err := diagnostics.Migrate(context.Background()); err != nil {
		t.Fatalf("diagnostics.Migrate returned error: %v", err)
	}
	server := &Server{
		diagnostics: diagnostics,
		runs:        runs,
		runBroker:   tasks.NewBroker(),
	}

	server.failConversationRun(context.Background(), "user-1", "conv-1", "msg-1", run.ID, context.Canceled)

	logs, err := diagnostics.List(context.Background(), store.DiagnosticLogQuery{UserID: "user-1", Limit: 10})
	if err != nil {
		t.Fatalf("diagnostics.List returned error: %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("expected no diagnostic for user cancellation, got %#v", logs)
	}
	updated, err := runs.GetRun(context.Background(), "user-1", run.ID)
	if err != nil {
		t.Fatalf("GetRun returned error: %v", err)
	}
	if updated.DiagnosticLogID.Valid {
		t.Fatalf("expected no diagnostic id for cancellation, got %#v", updated.DiagnosticLogID)
	}
}
