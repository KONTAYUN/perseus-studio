package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestConversationDeletionProtectsActiveRunAndRejectsOrphanResume(t *testing.T) {
	db, runs, conversations, conversation := runPreparationEnv(t)
	if err := NewTaskStore(db).Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	prepared, err := runs.PrepareConversationRun(t.Context(), PrepareConversationRunInput{UserID: "u", ConversationID: conversation.ID, ModelProviderID: "model", Content: "keep me"})
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []AgentRunStatus{AgentRunQueued, AgentRunRunning} {
		if _, err := runs.UpdateRun(t.Context(), "u", prepared.Run.ID, UpdateAgentRunInput{Status: status}); err != nil {
			t.Fatal(err)
		}
		if err := conversations.DeleteConversation(t.Context(), "u", conversation.ID); !errors.Is(err, ErrConversationBusy) {
			t.Fatalf("deleted %s conversation: %v", status, err)
		}
		if _, err := conversations.GetMessage(t.Context(), "u", prepared.Message.ID); err != nil {
			t.Fatal("active input lost:", err)
		}
	}
	if _, err := runs.UpdateRun(t.Context(), "u", prepared.Run.ID, UpdateAgentRunInput{Status: AgentRunFailed}); err != nil {
		t.Fatal(err)
	}
	if err := conversations.DeleteConversation(t.Context(), "u", conversation.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runs.ResumeRunWithEvent(t.Context(), "u", prepared.Run.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("orphan run resumed: %v", err)
	}
	if run, err := runs.GetRun(t.Context(), "u", prepared.Run.ID); err != nil || run.Status != AgentRunFailed {
		t.Fatalf("orphan state changed: %+v %v", run, err)
	}
}

func TestConversationDeletionProtectsBackgroundTask(t *testing.T) {
	db, _, conversations, conversation := runPreparationEnv(t)
	tasks := NewTaskStore(db)
	if err := tasks.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	task, err := tasks.CreateTask(t.Context(), CreateTaskInput{ID: "task", UserID: "u", Type: "ai_agent", PayloadJSON: fmt.Sprintf(`{"conversation_id":%q}`, conversation.ID)})
	if err != nil {
		t.Fatal(err)
	}
	if err := conversations.DeleteConversation(t.Context(), "other", conversation.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	for _, status := range []TaskStatus{TaskPending, TaskRunning} {
		if _, err := tasks.UpdateTaskState(t.Context(), task.ID, UpdateTaskStateInput{Status: status}); err != nil {
			t.Fatal(err)
		}
		if err := conversations.DeleteConversation(t.Context(), "u", conversation.ID); !errors.Is(err, ErrConversationBusy) {
			t.Fatalf("deleted background %s: %v", status, err)
		}
	}
	if _, err := tasks.UpdateTaskState(t.Context(), task.ID, UpdateTaskStateInput{Status: TaskDone}); err != nil {
		t.Fatal(err)
	}
	if err := conversations.DeleteConversation(t.Context(), "u", conversation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.CreateTask(t.Context(), CreateTaskInput{ID: "late", UserID: "u", Type: "ai_agent", PayloadJSON: task.PayloadJSON}); err == nil {
		t.Fatal("background task admitted after deletion")
	}
}

func TestConversationDeletionRacingAdmissionNeverOrphansActiveRun(t *testing.T) {
	for range 20 {
		db, runs, conversations, conversation := runPreparationEnv(t)
		if err := NewTaskStore(db).Migrate(t.Context()); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _ = conversations.DeleteConversation(t.Context(), "u", conversation.ID) }()
		go func() {
			defer wg.Done()
			_, _ = runs.PrepareConversationRun(t.Context(), PrepareConversationRunInput{UserID: "u", ConversationID: conversation.ID, Content: "hello"})
		}()
		wg.Wait()
		var orphaned int
		if err := db.QueryRow(`SELECT COUNT(*) FROM agent_runs r WHERE status IN ('queued','running') AND NOT EXISTS (SELECT 1 FROM conversations c WHERE c.id=r.conversation_id AND c.user_id=r.user_id)`).Scan(&orphaned); err != nil || orphaned != 0 {
			t.Fatalf("orphaned=%d err=%v", orphaned, err)
		}
	}
}

func TestConversationDeletionRollsBackOnStorageFailure(t *testing.T) {
	db, _, conversations, conversation := runPreparationEnv(t)
	if err := NewTaskStore(db).Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	message, err := conversations.AppendMessage(t.Context(), "u", conversation.ID, "user", "keep", "[]")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_delete BEFORE DELETE ON conversations BEGIN SELECT RAISE(ABORT, 'injected deletion failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := conversations.DeleteConversation(t.Context(), "u", conversation.ID); err == nil {
		t.Fatal("expected injected failure")
	}
	if _, err := conversations.GetConversation(t.Context(), "u", conversation.ID); err != nil {
		t.Fatal("conversation lost:", err)
	}
	if _, err := conversations.GetMessage(t.Context(), "u", message.ID); err != nil {
		t.Fatal("message lost:", err)
	}
}

func TestConversationDeletionRacingBackgroundAdmission(t *testing.T) {
	for range 12 {
		db, _, conversations, conversation := runPreparationEnv(t)
		tasks := NewTaskStore(db)
		if err := tasks.Migrate(t.Context()); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _ = conversations.DeleteConversation(t.Context(), "u", conversation.ID) }()
		go func() {
			defer wg.Done()
			_, _ = tasks.CreateTask(t.Context(), CreateTaskInput{ID: "task", UserID: "u", Type: "ai_agent", PayloadJSON: fmt.Sprintf(`{"conversation_id":%q}`, conversation.ID)})
		}()
		wg.Wait()
		var orphaned int
		if err := db.QueryRow(`SELECT COUNT(*) FROM tasks t WHERE status IN ('pending','running') AND NOT EXISTS (SELECT 1 FROM conversations c WHERE c.id=json_extract(t.payload_json,'$.conversation_id') AND c.user_id=t.user_id)`).Scan(&orphaned); err != nil || orphaned != 0 {
			t.Fatalf("orphaned=%d err=%v", orphaned, err)
		}
	}
}
