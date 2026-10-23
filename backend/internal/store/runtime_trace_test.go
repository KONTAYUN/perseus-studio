package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestRunCompletionEventAndStateAreAtomic(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewAgentRunStore(db)
	if err := s.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	run, err := s.CreateRun(t.Context(), CreateAgentRunInput{UserID: "owner", ConversationID: "conv"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveCheckpoint(t.Context(), "owner", run.ID, `{}`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_terminal BEFORE INSERT ON agent_events WHEN NEW.type='run_completed' BEGIN SELECT RAISE(FAIL,'injected terminal failure'); END`); err != nil {
		t.Fatal(err)
	}
	input := CompleteAgentRunInput{AssistantMessageID: "answer", TaskID: "task", Outcome: "success"}
	if _, _, err := s.CompleteRun(t.Context(), "owner", run.ID, input); err == nil {
		t.Fatal("accepted terminal with missing event")
	}
	current, _ := s.GetRun(t.Context(), "owner", run.ID)
	if current.Status != AgentRunQueued || current.CompletedAt.Valid {
		t.Fatalf("partial commit: %#v", current)
	}
	if _, err := s.LatestCheckpoint(t.Context(), "owner", run.ID); err != nil {
		t.Fatalf("failed completion lost recovery checkpoint: %v", err)
	}
	if _, err := db.Exec(`DROP TRIGGER reject_terminal`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CompleteRun(t.Context(), "other", run.ID, input); err != sql.ErrNoRows {
		t.Fatalf("ownership: %v", err)
	}
	for range 2 {
		current, event, err := s.CompleteRun(t.Context(), "owner", run.ID, input)
		if err != nil || current.Status != AgentRunSucceeded || event.TaskID != "task" {
			t.Fatalf("completion: %#v %#v %v", current, event, err)
		}
	}
	events, err := s.ListEvents(t.Context(), "owner", run.ID)
	if err != nil || len(events) != 1 {
		t.Fatalf("duplicate or missing event: %d %v", len(events), err)
	}
	if _, err := s.LatestCheckpoint(t.Context(), "owner", run.ID); err != sql.ErrNoRows {
		t.Fatalf("successful completion retained checkpoint: %v", err)
	}
}

func TestTaskTraceCursorOwnershipAndTerminalAtomicity(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewTaskStore(db)
	if err := s.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	// Insert a standalone task fixture without requiring unrelated conversation schema.
	if _, err := db.Exec(`INSERT INTO tasks(id,user_id,type,status,title,payload_json,created_at) VALUES ('task','owner','ai_agent','running','test','{}',CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	for i := range 205 {
		if _, err := s.AppendEvent(t.Context(), TaskEventInput{UserID: "owner", TaskID: "task", ExecutionID: "execution", Type: "model_delta", Payload: map[string]any{"text": "正文", "index": i}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.AppendEvent(t.Context(), TaskEventInput{UserID: "other", TaskID: "task", Type: "forged"}); err != sql.ErrNoRows {
		t.Fatalf("append ownership: %v", err)
	}
	other, _ := s.ListEventsAfter(t.Context(), "other", "task", 0, 200)
	if len(other) != 0 {
		t.Fatal("leaked trace")
	}
	first, err := s.ListEventsAfter(t.Context(), "owner", "task", 0, 200)
	if err != nil || len(first) != 200 {
		t.Fatalf("page1: %d %v", len(first), err)
	}
	last, err := s.ListEventsAfter(t.Context(), "owner", "task", first[199].Seq, 200)
	if err != nil || len(last) != 5 || last[0].Seq != 201 {
		t.Fatalf("page2: %#v %v", last, err)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_terminal BEFORE INSERT ON task_events WHEN NEW.type='task_done' BEGIN SELECT RAISE(FAIL,'injected terminal failure'); END`); err != nil {
		t.Fatal(err)
	}
	input := UpdateTaskStateInput{Status: TaskDone, ProgressStage: "done"}
	if _, _, err := s.FinishWithEvent(t.Context(), "task", input); err == nil {
		t.Fatal("accepted missing task terminal")
	}
	task, _ := s.GetTaskByID(t.Context(), "task")
	if task.Status != TaskRunning {
		t.Fatal("task committed without terminal event")
	}
	if _, err := db.Exec(`DROP TRIGGER reject_terminal`); err != nil {
		t.Fatal(err)
	}
	_, event, err := s.FinishWithEvent(t.Context(), "task", input)
	if err != nil || event == nil || event.Seq != 206 {
		t.Fatalf("finish: %#v %v", event, err)
	}
	task, event, err = s.FinishWithEvent(context.Background(), "task", UpdateTaskStateInput{Status: TaskCanceled})
	if err != nil || event != nil || task.Status != TaskDone {
		t.Fatal("late cancel rewrote terminal history")
	}
}

func TestTaskRestartTraceAndStateRollbackTogether(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewTaskStore(db)
	if err := s.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO tasks(id,user_id,type,status,title,payload_json,created_at) VALUES ('task','owner','ai_agent','running','test','{}',CURRENT_TIMESTAMP);
CREATE TRIGGER reject_restart BEFORE INSERT ON task_events WHEN NEW.type='task_failed' BEGIN SELECT RAISE(FAIL,'injected recovery trace failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.RequeueRunningTasks(t.Context()); err == nil {
		t.Fatal("accepted recovery without trace")
	}
	task, _ := s.GetTaskByID(t.Context(), "task")
	logs, _ := s.GetLogs(t.Context(), "owner", "task")
	if task.Status != TaskRunning || len(logs) != 0 {
		t.Fatal("partial recovery commit")
	}
	if _, err := db.Exec(`DROP TRIGGER reject_restart`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := s.RequeueRunningTasks(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	events, err := s.ListEventsAfter(t.Context(), "owner", "task", 0, 200)
	if err != nil || len(events) != 1 || events[0].Type != "task_failed" {
		t.Fatalf("restart terminal: %#v %v", events, err)
	}
}
