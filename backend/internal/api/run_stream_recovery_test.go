package api

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"webide-backend/internal/store"
)

func TestRunStreamReplaysDurableEventsBeyondNotificationBuffer(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	runs := store.NewAgentRunStore(db)
	if err := runs.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	run, err := runs.CreateRun(t.Context(), store.CreateAgentRunInput{UserID: "user", ConversationID: "conv", ModelProviderID: "model"})
	if err != nil {
		t.Fatal(err)
	}
	for range 65 {
		if _, err := runs.AppendEvent(t.Context(), store.AgentEventInput{UserID: "user", RunID: run.ID, ConversationID: "conv", Type: "test"}); err != nil {
			t.Fatal(err)
		}
	}
	s := &Server{runs: runs}
	var cursor int64
	response := httptest.NewRecorder()
	if err := s.writeAgentRunEvents(t.Context(), response, "user", run.ID, &cursor); err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(response.Body.String(), "event: event"); count != 65 {
		t.Fatalf("lost durable events: %d", count)
	}
	response = httptest.NewRecorder()
	if err := s.writeAgentRunEvents(t.Context(), response, "user", run.ID, &cursor); err != nil {
		t.Fatal(err)
	}
	if response.Body.Len() != 0 {
		t.Fatal("replayed already delivered events")
	}
}
