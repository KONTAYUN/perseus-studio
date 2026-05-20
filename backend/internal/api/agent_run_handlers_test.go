package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"webide-backend/internal/store"
	"webide-backend/internal/tasks"
)

func TestAgentRunSteerAcceptsRunningRunAndValidatesExpectedRun(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	users := store.NewUserStore(db)
	if err := users.Migrate(t.Context()); err != nil {
		t.Fatalf("users.Migrate returned error: %v", err)
	}
	user, err := users.CreateUser(t.Context(), "user-a", "password", "user")
	if err != nil {
		t.Fatalf("CreateUser returned error: %v", err)
	}
	sessions := store.NewSessionStore(db)
	if err := sessions.Migrate(t.Context()); err != nil {
		t.Fatalf("sessions.Migrate returned error: %v", err)
	}
	runs := store.NewAgentRunStore(db)
	if err := runs.Migrate(t.Context()); err != nil {
		t.Fatalf("runs.Migrate returned error: %v", err)
	}
	run, err := runs.CreateRun(t.Context(), store.CreateAgentRunInput{
		ID:              "run-steer-api",
		UserID:          user.ID,
		ConversationID:  "conv-steer-api",
		UserMessageID:   "msg-steer-api",
		ModelProviderID: "provider-steer-api",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	if _, err := runs.UpdateRun(t.Context(), user.ID, run.ID, store.UpdateAgentRunInput{Status: store.AgentRunRunning, Stage: "calling_model"}); err != nil {
		t.Fatalf("UpdateRun returned error: %v", err)
	}
	server := &Server{
		runs:      runs,
		runBroker: tasks.NewBroker(),
		mux:       http.NewServeMux(),
		sessions:  sessions,
	}
	server.register()

	body := bytes.NewBufferString(`{"content":"只处理我选中的文件","expected_run_id":"run-steer-api","client_steer_id":"client-1"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/agent-runs/run-steer-api/steer", body)
	attachTestSession(t, server, req, "token-a", user)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	events, err := runs.ListEvents(t.Context(), user.ID, run.ID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if len(events) != 1 || events[0].Type != "agent_steer_received" || !strings.Contains(events[0].PayloadJSON, "只处理我选中的文件") {
		t.Fatalf("unexpected steer events: %#v", events)
	}
	var response struct {
		Event struct {
			Payload map[string]any `json:"payload"`
		} `json:"event"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode steer response: %v", err)
	}
	if response.Event.Payload["client_steer_id"] != "client-1" {
		t.Fatalf("expected client_steer_id in response payload, got %#v", response.Event.Payload)
	}
	steerID, _ := response.Event.Payload["steer_id"].(string)
	if strings.TrimSpace(steerID) == "" {
		t.Fatalf("expected steer_id in response payload, got %#v", response.Event.Payload)
	}
	if response.Event.Payload["content_preview"] != "只处理我选中的文件" {
		t.Fatalf("expected content_preview in response payload, got %#v", response.Event.Payload)
	}

	wrongReq := httptest.NewRequest(http.MethodPost, "/api/agent-runs/run-steer-api/steer", bytes.NewBufferString(`{"content":"x","expected_run_id":"other"}`))
	attachTestSession(t, server, wrongReq, "token-b", user)
	wrongRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(wrongRec, wrongReq)
	if wrongRec.Code != http.StatusBadRequest {
		t.Fatalf("expected expected_run_id mismatch to fail, got %d", wrongRec.Code)
	}
}

func TestAgentRunSteerRejectsCompletedRun(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	users := store.NewUserStore(db)
	if err := users.Migrate(t.Context()); err != nil {
		t.Fatalf("users.Migrate returned error: %v", err)
	}
	user, err := users.CreateUser(t.Context(), "user-a", "password", "user")
	if err != nil {
		t.Fatalf("CreateUser returned error: %v", err)
	}
	sessions := store.NewSessionStore(db)
	if err := sessions.Migrate(t.Context()); err != nil {
		t.Fatalf("sessions.Migrate returned error: %v", err)
	}
	runs := store.NewAgentRunStore(db)
	if err := runs.Migrate(t.Context()); err != nil {
		t.Fatalf("runs.Migrate returned error: %v", err)
	}
	run, err := runs.CreateRun(t.Context(), store.CreateAgentRunInput{
		ID:              "run-steer-done",
		UserID:          user.ID,
		ConversationID:  "conv-steer-done",
		UserMessageID:   "msg-steer-done",
		ModelProviderID: "provider-steer-done",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	if _, err := runs.UpdateRun(t.Context(), user.ID, run.ID, store.UpdateAgentRunInput{Status: store.AgentRunSucceeded, Stage: "completed", SetCompletedAt: true}); err != nil {
		t.Fatalf("UpdateRun returned error: %v", err)
	}
	server := &Server{
		runs:      runs,
		runBroker: tasks.NewBroker(),
		mux:       http.NewServeMux(),
		sessions:  sessions,
	}
	server.register()

	req := httptest.NewRequest(http.MethodPost, "/api/agent-runs/run-steer-done/steer", bytes.NewBufferString(`{"content":"x","expected_run_id":"run-steer-done"}`))
	attachTestSession(t, server, req, "token-a", user)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected completed run to reject steer, got %d", rec.Code)
	}
}
