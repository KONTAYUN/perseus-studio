package api

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"webide-backend/internal/store"
)

func TestDeleteActiveConversationReturnsConflictAndPreservesHistory(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	conversations, runs := store.NewConversationStore(db), store.NewAgentRunStore(db)
	if err := conversations.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := runs.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.NewTaskStore(db).Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	conversation, err := conversations.CreateConversation(t.Context(), store.CreateConversationInput{UserID: "u"})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := runs.PrepareConversationRun(t.Context(), store.PrepareConversationRunInput{UserID: "u", ConversationID: conversation.ID, Content: "keep"})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{conversations: conversations, runs: runs}
	req := httptest.NewRequest("DELETE", "/api/conversations/"+conversation.ID, nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxSessionKey, &SessionInfo{UserID: "u"}))
	rec := httptest.NewRecorder()
	s.handleConversationRoutes(rec, req)
	if rec.Code != 409 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := conversations.GetMessage(t.Context(), "u", prepared.Message.ID); err != nil {
		t.Fatal("input lost:", err)
	}
	if _, err := runs.UpdateRun(t.Context(), "u", prepared.Run.ID, store.UpdateAgentRunInput{Status: store.AgentRunCanceled}); err != nil {
		t.Fatal(err)
	}
	s.runCancels = map[string]*runCancelEntry{prepared.Run.ID: {}}
	if err := s.deleteConversation(t.Context(), "u", conversation.ID); err != store.ErrConversationBusy {
		t.Fatalf("history removed before cancellation drained: %v", err)
	}
	s.unregisterRunCancel(prepared.Run.ID)
	if err := s.deleteConversation(t.Context(), "u", conversation.ID); err != nil {
		t.Fatal(err)
	}
}
