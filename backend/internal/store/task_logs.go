package store

import (
	"context"
	"time"
)

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
