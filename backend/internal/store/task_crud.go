package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

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
