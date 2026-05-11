package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

func (s *TaskStore) RequeueRunningTasks(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id FROM tasks WHERE status = ?`, TaskRunning)
	if err != nil {
		return err
	}

	type taskRef struct {
		id     string
		userID string
	}
	var refs []taskRef
	for rows.Next() {
		var ref taskRef
		if err := rows.Scan(&ref.id, &ref.userID); err != nil {
			return err
		}
		refs = append(refs, ref)
	}
	rows.Close()

	_, err = s.db.ExecContext(ctx, `
UPDATE tasks
SET status = ?, worker_id = NULL, started_at = NULL, progress_stage = 'requeued', progress_message = 'service restarted, task requeued'
WHERE status = ?
`, TaskPending, TaskRunning)
	if err != nil {
		return err
	}

	for _, ref := range refs {
		if _, err := s.AppendLog(ctx, ref.userID, ref.id, "system", "service restarted, task requeued"); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *TaskStore) CreateTask(ctx context.Context, input CreateTaskInput) (Task, error) {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO tasks (
  id, user_id, type, status, title, payload_json, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?)
`, input.ID, input.UserID, input.Type, TaskPending, input.Title, input.PayloadJSON, now)
	if err != nil {
		return Task{}, err
	}
	return s.GetTask(ctx, input.UserID, input.ID)
}

func (s *TaskStore) ListTasks(ctx context.Context, userID string, limit int) ([]Task, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, type, status, title, payload_json, result_json, error_message, worker_id, retry_count,
       progress_stage, progress_message, created_at, started_at, finished_at, cancel_requested_at
FROM tasks
WHERE user_id = ?
ORDER BY created_at DESC
LIMIT ?
`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, task)
	}
	return items, rows.Err()
}

func (s *TaskStore) GetTask(ctx context.Context, userID, id string) (Task, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, user_id, type, status, title, payload_json, result_json, error_message, worker_id, retry_count,
       progress_stage, progress_message, created_at, started_at, finished_at, cancel_requested_at
FROM tasks
WHERE id = ? AND user_id = ?
`, id, userID)
	return scanTask(row)
}

func (s *TaskStore) GetTaskByID(ctx context.Context, id string) (Task, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, user_id, type, status, title, payload_json, result_json, error_message, worker_id, retry_count,
       progress_stage, progress_message, created_at, started_at, finished_at, cancel_requested_at
FROM tasks
WHERE id = ?
`, id)
	return scanTask(row)
}

func (s *TaskStore) DeleteTask(ctx context.Context, userID, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

func (s *TaskStore) ClaimNextTask(ctx context.Context, workerID string) (*Task, error) {
	row := s.db.QueryRowContext(ctx, `
UPDATE tasks
SET status = ?, worker_id = ?, started_at = ?, progress_stage = 'claimed', progress_message = 'task claimed by worker'
WHERE id = (
  SELECT id FROM tasks
  WHERE status = ?
  ORDER BY created_at ASC
  LIMIT 1
)
RETURNING id, user_id, type, status, title, payload_json, result_json, error_message, worker_id, retry_count,
          progress_stage, progress_message, created_at, started_at, finished_at, cancel_requested_at
`, TaskRunning, workerID, time.Now().UTC(), TaskPending)

	task, err := scanTask(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &task, nil
}

func (s *TaskStore) UpdateTaskState(ctx context.Context, taskID string, input UpdateTaskStateInput) (Task, error) {
	var startedAt any
	var finishedAt any
	if input.SetStartedAt {
		startedAt = time.Now().UTC()
	}
	if input.SetFinishedAt {
		finishedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx, `
UPDATE tasks
SET status = ?,
    progress_stage = ?,
    progress_message = ?,
    error_message = ?,
    result_json = COALESCE(?, result_json),
    worker_id = COALESCE(?, worker_id),
    started_at = COALESCE(?, started_at),
    finished_at = COALESCE(?, finished_at)
WHERE id = ?
`,
		input.Status,
		nullIfEmpty(input.ProgressStage),
		nullIfEmpty(input.ProgressMessage),
		input.ErrorMessage,
		input.ResultJSON,
		input.WorkerID,
		startedAt,
		finishedAt,
		taskID,
	)
	if err != nil {
		return Task{}, err
	}
	return s.GetTaskByID(ctx, taskID)
}

func (s *TaskStore) RequestCancel(ctx context.Context, userID, taskID string) (Task, error) {
	_, err := s.db.ExecContext(ctx, `
UPDATE tasks
SET cancel_requested_at = ?
WHERE id = ? AND user_id = ? AND cancel_requested_at IS NULL
`, time.Now().UTC(), taskID, userID)
	if err != nil {
		return Task{}, err
	}
	return s.GetTask(ctx, userID, taskID)
}

func (s *TaskStore) IsCancelRequested(ctx context.Context, taskID string) (bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT cancel_requested_at FROM tasks WHERE id = ?`, taskID)
	var cancelRequestedAt sql.NullTime
	if err := row.Scan(&cancelRequestedAt); err != nil {
		return false, err
	}
	return cancelRequestedAt.Valid, nil
}

func (s *TaskStore) AppendLog(ctx context.Context, userID, taskID, level, message string) (TaskLog, error) {
	now := time.Now().UTC()
	row := s.db.QueryRowContext(ctx, `
INSERT INTO task_logs (user_id, task_id, seq, level, message, created_at)
VALUES (
  ?,
  ?,
  COALESCE((SELECT MAX(seq) + 1 FROM task_logs WHERE task_id = ?), 1),
  ?, ?, ?
)
RETURNING id, user_id, task_id, seq, level, message, created_at
`, userID, taskID, taskID, level, message, now)

	var logItem TaskLog
	if err := row.Scan(&logItem.ID, &logItem.UserID, &logItem.TaskID, &logItem.Seq, &logItem.Level, &logItem.Message, &logItem.CreatedAt); err != nil {
		return TaskLog{}, err
	}
	return logItem, nil
}

func (s *TaskStore) GetLogs(ctx context.Context, userID, taskID string) ([]TaskLog, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, task_id, seq, level, message, created_at
FROM task_logs
WHERE task_id = ? AND user_id = ?
ORDER BY seq ASC
`, taskID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []TaskLog
	for rows.Next() {
		var item TaskLog
		if err := rows.Scan(&item.ID, &item.UserID, &item.TaskID, &item.Seq, &item.Level, &item.Message, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *TaskStore) MarshalResult(value any) *string {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	text := string(raw)
	return &text
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTask(s scanner) (Task, error) {
	var task Task
	err := s.Scan(
		&task.ID,
		&task.UserID,
		&task.Type,
		&task.Status,
		&task.Title,
		&task.PayloadJSON,
		&task.ResultJSON,
		&task.ErrorMessage,
		&task.WorkerID,
		&task.RetryCount,
		&task.ProgressStage,
		&task.ProgressMessage,
		&task.CreatedAt,
		&task.StartedAt,
		&task.FinishedAt,
		&task.CancelRequestedAt,
	)
	return task, err
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (t Task) MarshalJSON() ([]byte, error) {
	type taskDTO struct {
		ID                string     `json:"id"`
		UserID            string     `json:"user_id"`
		Type              string     `json:"type"`
		Status            TaskStatus `json:"status"`
		Title             string     `json:"title"`
		PayloadJSON       string     `json:"payload_json"`
		ResultJSON        any        `json:"result_json"`
		ErrorMessage      *string    `json:"error_message"`
		WorkerID          *string    `json:"worker_id"`
		RetryCount        int        `json:"retry_count"`
		ProgressStage     *string    `json:"progress_stage"`
		ProgressMessage   *string    `json:"progress_message"`
		CreatedAt         time.Time  `json:"created_at"`
		StartedAt         *time.Time `json:"started_at"`
		FinishedAt        *time.Time `json:"finished_at"`
		CancelRequestedAt *time.Time `json:"cancel_requested_at"`
	}

	dto := taskDTO{
		ID:                t.ID,
		UserID:            t.UserID,
		Type:              t.Type,
		Status:            t.Status,
		Title:             t.Title,
		PayloadJSON:       t.PayloadJSON,
		ResultJSON:        parseResultJSON(t.ResultJSON),
		ErrorMessage:      nullableString(t.ErrorMessage),
		WorkerID:          nullableString(t.WorkerID),
		RetryCount:        t.RetryCount,
		ProgressStage:     nullableString(t.ProgressStage),
		ProgressMessage:   nullableString(t.ProgressMessage),
		CreatedAt:         t.CreatedAt,
		StartedAt:         nullableTime(t.StartedAt),
		FinishedAt:        nullableTime(t.FinishedAt),
		CancelRequestedAt: nullableTime(t.CancelRequestedAt),
	}

	return json.Marshal(dto)
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}

func nullableTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	timestamp := value.Time
	return &timestamp
}

func parseResultJSON(value sql.NullString) any {
	if !value.Valid || value.String == "" {
		return nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(value.String), &decoded); err == nil {
		return decoded
	}
	return value.String
}
