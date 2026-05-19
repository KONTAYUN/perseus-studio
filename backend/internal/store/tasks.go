package store

import (
	"context"
	"database/sql"
	"time"
)

type TaskStatus string

const (
	TaskPending  TaskStatus = "pending"
	TaskRunning  TaskStatus = "running"
	TaskDone     TaskStatus = "done"
	TaskFailed   TaskStatus = "failed"
	TaskCanceled TaskStatus = "canceled"
)

type Task struct {
	ID                string         `json:"id"`
	UserID            string         `json:"user_id"`
	Type              string         `json:"type"`
	Status            TaskStatus     `json:"status"`
	Title             string         `json:"title"`
	PayloadJSON       string         `json:"payload_json"`
	ResultJSON        sql.NullString `json:"-"`
	ErrorMessage      sql.NullString `json:"error_message"`
	WorkerID          sql.NullString `json:"worker_id"`
	RetryCount        int            `json:"retry_count"`
	ProgressStage     sql.NullString `json:"progress_stage"`
	ProgressMessage   sql.NullString `json:"progress_message"`
	CreatedAt         time.Time      `json:"created_at"`
	StartedAt         sql.NullTime   `json:"started_at"`
	FinishedAt        sql.NullTime   `json:"finished_at"`
	CancelRequestedAt sql.NullTime   `json:"cancel_requested_at"`
}

type TaskLog struct {
	ID        int64     `json:"id"`
	UserID    string    `json:"user_id"`
	TaskID    string    `json:"task_id"`
	Seq       int64     `json:"seq"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateTaskInput struct {
	ID          string
	UserID      string
	Type        string
	Title       string
	PayloadJSON string
}

type UpdateTaskStateInput struct {
	Status          TaskStatus
	ProgressStage   string
	ProgressMessage string
	ErrorMessage    *string
	ResultJSON      *string
	SetStartedAt    bool
	SetFinishedAt   bool
	WorkerID        *string
}

type TaskStore struct {
	db *sql.DB
}

func NewTaskStore(db *sql.DB) *TaskStore {
	return &TaskStore{db: db}
}

func (s *TaskStore) Migrate(ctx context.Context) error {
	tables := `
CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL DEFAULT '',
  type TEXT NOT NULL,
  status TEXT NOT NULL,
  title TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  result_json TEXT,
  error_message TEXT,
  worker_id TEXT,
  retry_count INTEGER NOT NULL DEFAULT 0,
  progress_stage TEXT,
  progress_message TEXT,
  created_at DATETIME NOT NULL,
  started_at DATETIME,
  finished_at DATETIME,
  cancel_requested_at DATETIME
);

CREATE TABLE IF NOT EXISTS task_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id TEXT NOT NULL DEFAULT '',
  task_id TEXT NOT NULL,
  seq INTEGER NOT NULL,
  level TEXT NOT NULL,
  message TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE
);
`
	if _, err := s.db.ExecContext(ctx, tables); err != nil {
		return err
	}
	for _, tbl := range []string{"tasks", "task_logs"} {
		if err := AddColumnIfNotExists(ctx, s.db, tbl, "user_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
	}
	indexes := `
CREATE INDEX IF NOT EXISTS idx_tasks_status_created_at ON tasks(status, created_at);
CREATE INDEX IF NOT EXISTS idx_tasks_user_id ON tasks(user_id);
CREATE INDEX IF NOT EXISTS idx_task_logs_task_id_seq ON task_logs(task_id, seq);
CREATE INDEX IF NOT EXISTS idx_task_logs_user_id ON task_logs(user_id);
`
	_, err := s.db.ExecContext(ctx, indexes)
	return err
}
