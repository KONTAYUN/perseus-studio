package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
)

func runPreparationEnv(t *testing.T) (*sql.DB, *AgentRunStore, *ConversationStore, Conversation) {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	runs, conversations := NewAgentRunStore(db), NewConversationStore(db)
	if err := conversations.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := runs.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	conversation, err := conversations.CreateConversation(t.Context(), CreateConversationInput{UserID: "u", SelectedModelProviderID: "model"})
	if err != nil {
		t.Fatal(err)
	}
	return db, runs, conversations, conversation
}

func TestRunPreparationIsAtomicOnEventFailure(t *testing.T) {
	db, runs, conversations, conversation := runPreparationEnv(t)
	if _, err := db.Exec(`CREATE TRIGGER reject_event BEFORE INSERT ON agent_events BEGIN SELECT RAISE(ABORT, 'injected event failure'); END`); err != nil {
		t.Fatal(err)
	}
	_, err := runs.PrepareConversationRun(t.Context(), PrepareConversationRunInput{UserID: "u", ConversationID: conversation.ID, ModelProviderID: "model", Content: "hello", RequestID: "request"})
	if err == nil {
		t.Fatal("expected injected persistence failure")
	}
	items, err := conversations.ListMessages(t.Context(), "u", conversation.ID)
	if err != nil || len(items) != 0 {
		t.Fatalf("orphaned input: %+v %v", items, err)
	}
	for _, table := range []string{"agent_runs", "agent_events", "conversation_run_requests"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("partial commit in %s: %d %v", table, count, err)
		}
	}
}

func TestConcurrentRequestReplayCreatesExactlyOneRunAndInput(t *testing.T) {
	_, runs, conversations, conversation := runPreparationEnv(t)
	in := PrepareConversationRunInput{UserID: "u", ConversationID: conversation.ID, ModelProviderID: "model", Content: "hello", RequestID: "request"}
	var wg sync.WaitGroup
	results := make(chan PreparedConversationRun, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := runs.PrepareConversationRun(t.Context(), in)
			if err != nil {
				t.Error(err)
				return
			}
			results <- result
		}()
	}
	wg.Wait()
	close(results)
	id, newCount := "", 0
	for result := range results {
		if id == "" {
			id = result.Run.ID
		}
		if result.Run.ID != id {
			t.Fatal("duplicate run")
		}
		if !result.Replayed {
			newCount++
		}
	}
	if newCount != 1 {
		t.Fatalf("created %d runs", newCount)
	}
	items, err := conversations.ListMessages(t.Context(), "u", conversation.ID)
	if err != nil || len(items) != 1 {
		t.Fatalf("duplicate input: %+v %v", items, err)
	}
	if _, err := runs.RunByRequestID(t.Context(), "other-user", "request"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("request identity crossed users: %v", err)
	}
	in.Content = "different"
	if _, err := runs.PrepareConversationRun(t.Context(), in); !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("identity reused for changed content: %v", err)
	}
	in.RequestID = "another-request"
	if _, err := runs.PrepareConversationRun(t.Context(), in); !errors.Is(err, ErrConversationBusy) {
		t.Fatalf("overlapping conversation admitted: %v", err)
	}
}

func TestRegenerationKeepsHistoryUntilAdmittedAndPreparesOnce(t *testing.T) {
	db, runs, conversations, conversation := runPreparationEnv(t)
	user, err := conversations.AppendMessage(t.Context(), "u", conversation.ID, "user", "hello", "[]")
	if err != nil {
		t.Fatal(err)
	}
	assistant, err := conversations.AppendMessage(t.Context(), "u", conversation.ID, "assistant", "previous answer", "[]")
	if err != nil {
		t.Fatal(err)
	}
	result, err := runs.PrepareConversationRun(t.Context(), PrepareConversationRunInput{UserID: "u", ConversationID: conversation.ID, ModelProviderID: "model", RegenerateMessageID: assistant.ID})
	if err != nil {
		t.Fatal(err)
	}
	if messages, err := conversations.ListMessages(t.Context(), "u", conversation.ID); err != nil || len(messages) != 2 {
		t.Fatalf("history lost before admission: %+v %v", messages, err)
	}
	if selected, err := runs.PendingRegeneration(t.Context(), "u", result.Run.ID); err != nil || selected != assistant.ID {
		t.Fatalf("regeneration not durable: %s %v", selected, err)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_regeneration BEFORE INSERT ON agent_events WHEN NEW.type = 'regeneration_prepared' BEGIN SELECT RAISE(ABORT, 'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := runs.PrepareRegenerationHistory(t.Context(), "u", conversation.ID, assistant.ID, user.ID, result.Run.ID); err == nil {
		t.Fatal("expected injected failure")
	}
	if messages, err := conversations.ListMessages(t.Context(), "u", conversation.ID); err != nil || len(messages) != 2 {
		t.Fatalf("truncation did not roll back: %+v %v", messages, err)
	}
	if _, err := db.Exec(`DROP TRIGGER reject_regeneration`); err != nil {
		t.Fatal(err)
	}
	if err := runs.PrepareRegenerationHistory(t.Context(), "u", conversation.ID, assistant.ID, user.ID, result.Run.ID); err != nil {
		t.Fatal(err)
	}
	if selected, err := runs.PendingRegeneration(t.Context(), "u", result.Run.ID); err != nil || selected != "" {
		t.Fatalf("history would truncate again on resume: %s %v", selected, err)
	}
	if messages, err := conversations.ListMessages(t.Context(), "u", conversation.ID); err != nil || len(messages) != 1 || messages[0].ID != user.ID {
		t.Fatalf("wrong history after preparation: %+v %v", messages, err)
	}
}

func TestResumeCannotReviveOlderRunOrOverlapLatestRun(t *testing.T) {
	_, runs, _, conversation := runPreparationEnv(t)
	in := CreateAgentRunInput{UserID: "u", ConversationID: conversation.ID, ModelProviderID: "model"}
	old, err := runs.CreateRun(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runs.UpdateRun(t.Context(), "u", old.ID, UpdateAgentRunInput{Status: AgentRunFailed}); err != nil {
		t.Fatal(err)
	}
	latest, err := runs.CreateRun(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runs.ResumeRun(t.Context(), "u", old.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("old checkpoint revived alongside newer input: %v", err)
	}
	if _, err := runs.UpdateRun(t.Context(), "u", latest.ID, UpdateAgentRunInput{Status: AgentRunFailed}); err != nil {
		t.Fatal(err)
	}
	if _, err := runs.ResumeRun(t.Context(), "u", old.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("old checkpoint revived after newer run: %v", err)
	}
	if _, err := runs.ResumeRun(t.Context(), "u", latest.ID); err != nil {
		t.Fatal(err)
	}
}
