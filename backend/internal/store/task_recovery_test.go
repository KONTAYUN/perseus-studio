package store

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRestartNeverReplaysInterruptedBackgroundAI(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewTaskStore(db)
	if err := s.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		id, kind string
		canceled bool
	}{{"ai", "ai_agent", false}, {"mock", "mock", false}, {"cancel", "ai_agent", true}} {
		if _, err := db.Exec(`INSERT INTO tasks (id,user_id,type,status,title,payload_json,created_at) VALUES (?,'u',?,'running','task','{}',CURRENT_TIMESTAMP)`, item.id, item.kind); err != nil {
			t.Fatal(err)
		}
		if item.canceled {
			if _, err := s.RequestCancel(t.Context(), "u", item.id); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := s.RequeueRunningTasks(t.Context()); err != nil {
		t.Fatal(err)
	}
	for id, expected := range map[string]TaskStatus{"ai": TaskFailed, "mock": TaskPending, "cancel": TaskCanceled} {
		task, err := s.GetTask(t.Context(), "u", id)
		if err != nil || task.Status != expected {
			t.Fatalf("%s status=%s expected=%s err=%v", id, task.Status, expected, err)
		}
		if id == "ai" && (!task.FinishedAt.Valid || !strings.Contains(task.ErrorMessage.String, "核验")) {
			t.Fatalf("ambiguous result not explained: %+v", task)
		}
		logs, err := s.GetLogs(t.Context(), "u", id)
		if err != nil || len(logs) != 1 {
			t.Fatalf("missing recovery log: %s %+v %v", id, logs, err)
		}
	}
	if err := s.RequeueRunningTasks(t.Context()); err != nil {
		t.Fatal(err)
	}
	logs, _ := s.GetLogs(t.Context(), "u", "ai")
	if len(logs) != 1 {
		t.Fatal("recovery was not idempotent")
	}
}

func TestRestartRecoveryLogFailureRollsBackTaskState(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewTaskStore(db)
	if err := s.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO tasks (id,user_id,type,status,title,payload_json,created_at) VALUES ('ai','u','ai_agent','running','task','{}',CURRENT_TIMESTAMP); CREATE TRIGGER reject_recovery_log BEFORE INSERT ON task_logs BEGIN SELECT RAISE(ABORT, 'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.RequeueRunningTasks(t.Context()); err == nil {
		t.Fatal("expected injected failure")
	}
	task, err := s.GetTask(t.Context(), "u", "ai")
	if err != nil || task.Status != TaskRunning {
		t.Fatalf("partial recovery: %+v %v", task, err)
	}
}
