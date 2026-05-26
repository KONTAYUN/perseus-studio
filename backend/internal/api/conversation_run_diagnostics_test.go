package api

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

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
