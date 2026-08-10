package store

import (
	"context"
	"time"
)

func (s *TaskStore) AppendLog(ctx context.Context, userID, taskID, level, message string) (TaskLog, error) {
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TaskLog{}, err
	}
	defer tx.Rollback()
	var seq int64
	if err := tx.QueryRowContext(ctx, `
UPDATE tasks
SET event_head_seq = event_head_seq + 1
WHERE id = ? AND user_id = ?
RETURNING event_head_seq
`, taskID, userID).Scan(&seq); err != nil {
		return TaskLog{}, err
	}
	row := tx.QueryRowContext(ctx, `
INSERT INTO task_logs (user_id, task_id, seq, level, message, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING id, user_id, task_id, seq, level, message, created_at
`, userID, taskID, seq, level, message, now)

	var logItem TaskLog
	if err := row.Scan(&logItem.ID, &logItem.UserID, &logItem.TaskID, &logItem.Seq, &logItem.Level, &logItem.Message, &logItem.CreatedAt); err != nil {
		return TaskLog{}, err
	}
	if err := tx.Commit(); err != nil {
		return TaskLog{}, err
	}
	return logItem, nil
}

func (s *TaskStore) GetLogs(ctx context.Context, userID, taskID string) ([]TaskLog, error) {
	return s.GetLogsAfter(ctx, userID, taskID, 0)
}

func (s *TaskStore) GetLogsAfter(ctx context.Context, userID, taskID string, afterSeq int64) ([]TaskLog, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, task_id, seq, level, message, created_at
FROM task_logs
WHERE task_id = ? AND user_id = ? AND seq > ?
ORDER BY seq ASC
`, taskID, userID, afterSeq)
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

func (s *TaskStore) normalizeTaskLogSequences(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
WITH ranked AS (
  SELECT id, ROW_NUMBER() OVER (
    PARTITION BY task_id ORDER BY seq ASC, created_at ASC, id ASC
  ) AS normalized_seq
  FROM task_logs
)
UPDATE task_logs
SET seq = (SELECT normalized_seq FROM ranked WHERE ranked.id = task_logs.id)
WHERE seq <> (SELECT normalized_seq FROM ranked WHERE ranked.id = task_logs.id)
`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE tasks
SET event_head_seq = COALESCE((SELECT MAX(seq) FROM task_logs WHERE task_id = tasks.id), 0),
    final_seq = CASE
      WHEN status IN (?, ?, ?)
      THEN COALESCE((SELECT MAX(seq) FROM task_logs WHERE task_id = tasks.id), 0)
      ELSE 0
    END
`, TaskDone, TaskFailed, TaskCanceled); err != nil {
		return err
	}
	return tx.Commit()
}
