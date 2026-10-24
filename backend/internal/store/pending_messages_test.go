package store

import (
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"webide-backend/internal/document"
)

func TestQueuedDocumentRepairCannotReplaceOrdinaryPendingMessage(t *testing.T) {
	_, runs, _, conversation := runPreparationEnv(t)
	p := PendingMessage{ID: "ordinary", ConversationID: conversation.ID, Content: "用户已挂起的消息", ModelProviderID: "model"}
	if _, err := runs.SavePendingMessage(t.Context(), "u", p); err != nil {
		t.Fatal(err)
	}
	repair := p
	repair.ID = "repair"
	repair.Content = "repair"
	repair.DocumentAction = &document.Action{Type: "format_repair", Path: "a.md", BaseHash: strings.Repeat("a", 64), Profile: document.Version, Diagnostics: []document.Issue{{Code: "test", Message: "test"}}}
	if _, err := runs.SavePendingMessage(t.Context(), "u", repair); !errors.Is(err, ErrRequestConflict) {
		t.Fatal("pending overwritten", err)
	}
	got, err := runs.GetPendingMessage(t.Context(), "u", p.ID)
	if err != nil || got.Content != p.Content {
		t.Fatal("user queue lost", got, err)
	}
}

func TestPendingMessagesSurviveReloadAndAdmitExactlyOnce(t *testing.T) {
	db, runs, conversations, conversation := runPreparationEnv(t)
	p := PendingMessage{ID: "follow-up", ConversationID: conversation.ID, Content: "保留排队改稿意见", ModelProviderID: "model", ThinkingMode: "standard"}
	if _, err := runs.SavePendingMessage(t.Context(), "u", p); err != nil {
		t.Fatal(err)
	}
	// A fresh store models another HTTP client/process reading durable state.
	reloaded := NewAgentRunStore(db)
	items, err := reloaded.ListPendingMessages(t.Context(), "u")
	if err != nil || len(items) != 1 || items[0].Content != p.Content {
		t.Fatalf("lost queue: %+v %v", items, err)
	}
	in := PrepareConversationRunInput{UserID: "u", ConversationID: p.ConversationID, Content: p.Content, AttachmentsJSON: "[]", ModelProviderID: p.ModelProviderID, ThinkingMode: p.ThinkingMode, RequestID: "pending:" + p.ID, PendingMessageID: p.ID}
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := reloaded.PrepareConversationRun(t.Context(), in)
			if err != nil {
				failures <- err
				return
			}
			ids <- result.Run.ID
		}()
	}
	wg.Wait()
	close(ids)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	unique := map[string]bool{}
	for id := range ids {
		unique[id] = true
	}
	if len(unique) != 1 {
		t.Fatalf("duplicate runs: %v", unique)
	}
	items, err = runs.ListPendingMessages(t.Context(), "u")
	if err != nil || len(items) != 0 {
		t.Fatalf("accepted queue not consumed: %v %v", items, err)
	}
	messages, err := conversations.ListMessages(t.Context(), "u", conversation.ID)
	if err != nil || len(messages) != 1 || messages[0].Content != p.Content {
		t.Fatalf("duplicate/lost message: %v %v", messages, err)
	}
	if _, err := runs.SavePendingMessage(t.Context(), "u", p); !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("accepted identity was reused: %v", err)
	}
	if err := runs.DeletePendingMessage(t.Context(), "u", p.ID); !errors.Is(err, ErrPendingAlreadySent) {
		t.Fatal("accepted message falsely reported canceled", err)
	}
}

func TestPendingAdmissionFailureKeepsInputAndCancelReplaceAreSafe(t *testing.T) {
	db, runs, _, conversation := runPreparationEnv(t)
	p := PendingMessage{ID: "old", ConversationID: conversation.ID, Content: "original", ModelProviderID: "model", ThinkingMode: "standard"}
	if _, err := runs.SavePendingMessage(t.Context(), "u", p); err != nil {
		t.Fatal(err)
	}
	in := PrepareConversationRunInput{UserID: "u", ConversationID: p.ConversationID, Content: p.Content, AttachmentsJSON: "[]", ModelProviderID: p.ModelProviderID, ThinkingMode: p.ThinkingMode, RequestID: "pending:" + p.ID, PendingMessageID: p.ID}
	if _, err := db.Exec(`CREATE TRIGGER reject_pending_event BEFORE INSERT ON agent_events BEGIN SELECT RAISE(ABORT,'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := runs.PrepareConversationRun(t.Context(), in); err == nil {
		t.Fatal("fault did not trigger")
	}
	if _, err := runs.GetPendingMessage(t.Context(), "u", p.ID); err != nil {
		t.Fatal("failed admission consumed input", err)
	}
	if _, err := db.Exec(`DROP TRIGGER reject_pending_event`); err != nil {
		t.Fatal(err)
	}
	p.ID = "replacement"
	p.Content = "new input"
	if _, err := runs.SavePendingMessage(t.Context(), "u", p); err != nil {
		t.Fatal(err)
	}
	if _, err := runs.PrepareConversationRun(t.Context(), in); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale replacement sent: %v", err)
	}
	if err := runs.DeletePendingMessage(t.Context(), "u", "old"); err != nil {
		t.Fatal(err)
	}
	if _, err := runs.GetPendingMessage(t.Context(), "u", p.ID); err != nil {
		t.Fatal("stale cancellation deleted replacement", err)
	}
	if err := runs.DeletePendingMessage(t.Context(), "u", p.ID); err != nil {
		t.Fatal(err)
	}
	in.PendingMessageID = p.ID
	in.RequestID = "pending:" + p.ID
	in.Content = p.Content
	if _, err := runs.PrepareConversationRun(t.Context(), in); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("canceled input sent: %v", err)
	}
}

func TestPendingMessagesAreScopedAndPauseOnUnfinishedRuns(t *testing.T) {
	db, runs, conversations, conversation := runPreparationEnv(t)
	if err := NewTaskStore(db).Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	p := PendingMessage{ID: "same-id", ConversationID: conversation.ID, Content: "private", ModelProviderID: "model"}
	if _, err := runs.SavePendingMessage(t.Context(), "intruder", p); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("cross-user queue write", err)
	}
	if _, err := runs.SavePendingMessage(t.Context(), "u", p); err != nil {
		t.Fatal(err)
	}
	if items, err := runs.ListPendingMessages(t.Context(), "intruder"); err != nil || len(items) != 0 {
		t.Fatal("queue disclosure", items, err)
	}
	if err := runs.DeletePendingMessage(t.Context(), "intruder", p.ID); err != nil {
		t.Fatal(err)
	}
	initial, err := runs.PrepareConversationRun(t.Context(), PrepareConversationRunInput{UserID: "u", ConversationID: conversation.ID, ModelProviderID: "model", Content: "first"})
	if err != nil {
		t.Fatal(err)
	}
	in := PrepareConversationRunInput{UserID: "u", ConversationID: p.ConversationID, Content: p.Content, AttachmentsJSON: "[]", ModelProviderID: p.ModelProviderID, RequestID: "pending:" + p.ID, PendingMessageID: p.ID}
	for _, status := range []AgentRunStatus{AgentRunQueued, AgentRunRunning, AgentRunFailed, AgentRunCanceled} {
		if _, err := runs.UpdateRun(t.Context(), "u", initial.Run.ID, UpdateAgentRunInput{Status: status}); err != nil {
			t.Fatal(err)
		}
		if _, err := runs.PrepareConversationRun(t.Context(), in); !errors.Is(err, ErrConversationBusy) {
			t.Fatalf("dispatched after %s: %v", status, err)
		}
		if _, err := runs.GetPendingMessage(t.Context(), "u", p.ID); err != nil {
			t.Fatal("paused input lost", err)
		}
	}
	if _, err := runs.UpdateRun(t.Context(), "u", initial.Run.ID, UpdateAgentRunInput{Status: AgentRunSucceeded}); err != nil {
		t.Fatal(err)
	}
	if err := conversations.DeleteConversation(t.Context(), "u", conversation.ID); err != nil {
		t.Fatal(err)
	}
	if items, err := runs.ListPendingMessages(t.Context(), "u"); err != nil || len(items) != 0 {
		t.Fatal("orphan queue", items, err)
	}
}
