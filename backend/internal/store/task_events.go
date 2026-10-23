package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type TaskEvent struct {
	ID          string          `json:"id"`
	UserID      string          `json:"user_id"`
	TaskID      string          `json:"task_id"`
	ExecutionID string          `json:"execution_id,omitempty"`
	Type        string          `json:"type"`
	Level       string          `json:"level"`
	Seq         int64           `json:"seq"`
	Payload     json.RawMessage `json:"payload"`
	CreatedAt   time.Time       `json:"created_at"`
}

type TaskEventInput struct {
	UserID, TaskID, ExecutionID string
	Type, Level                 string
	Payload                     any
}

type taskEventQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *TaskStore) AppendEvent(ctx context.Context, input TaskEventInput) (TaskEvent, error) {
	return appendTaskEvent(ctx, s.db, input)
}

func appendTaskEvent(ctx context.Context, db taskEventQuerier, input TaskEventInput) (TaskEvent, error) {
	if strings.TrimSpace(input.Type) == "" {
		return TaskEvent{}, fmt.Errorf("task event type is required")
	}
	raw, err := json.Marshal(input.Payload)
	if err != nil {
		return TaskEvent{}, err
	}
	level := strings.ToLower(strings.TrimSpace(input.Level))
	if level == "" {
		level = "info"
	}
	now := time.Now().UTC()
	return scanTaskEvent(db.QueryRowContext(ctx, `
INSERT INTO task_events(id,user_id,task_id,execution_id,type,level,seq,payload_json,created_at)
SELECT ?,?,?,?,?,?,COALESCE((SELECT MAX(seq)+1 FROM task_events WHERE task_id=?),1),?,?
WHERE EXISTS(SELECT 1 FROM tasks WHERE id=? AND user_id=?)
RETURNING id,user_id,task_id,execution_id,type,level,seq,payload_json,created_at`,
		nextStoreID("tevt", now), input.UserID, input.TaskID, input.ExecutionID, input.Type, level,
		input.TaskID, string(raw), now, input.TaskID, input.UserID))
}

func scanTaskEvent(row interface{ Scan(...any) error }) (TaskEvent, error) {
	var event TaskEvent
	var raw string
	err := row.Scan(&event.ID, &event.UserID, &event.TaskID, &event.ExecutionID, &event.Type, &event.Level, &event.Seq, &raw, &event.CreatedAt)
	event.Payload = json.RawMessage(raw)
	return event, err
}

// A bounded ascending cursor avoids silently truncating long task histories.
func (s *TaskStore) ListEventsAfter(ctx context.Context, userID, taskID string, after int64, limit int) ([]TaskEvent, error) {
	if after < 0 {
		return nil, fmt.Errorf("event cursor must be non-negative")
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id,user_id,task_id,execution_id,type,level,seq,payload_json,created_at
FROM task_events WHERE user_id=? AND task_id=? AND seq>? ORDER BY seq LIMIT ?`, userID, taskID, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]TaskEvent, 0)
	for rows.Next() {
		event, err := scanTaskEvent(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, event)
	}
	return items, rows.Err()
}

// AI task termination and its durable event are one commit. Repeated finishes
// (for example worker cancellation racing its scheduler) do not rewrite history.
func (s *TaskStore) FinishWithEvent(ctx context.Context, taskID string, input UpdateTaskStateInput) (Task, *TaskEvent, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, nil, err
	}
	defer tx.Rollback()
	task, err := scanTask(tx.QueryRowContext(ctx, `SELECT id,user_id,type,status,title,payload_json,result_json,error_message,worker_id,retry_count,
progress_stage,progress_message,created_at,started_at,finished_at,cancel_requested_at FROM tasks WHERE id=?`, taskID))
	if err != nil {
		return Task{}, nil, err
	}
	if task.Status != TaskRunning && task.Status != TaskPending {
		return task, nil, nil
	}
	if input.Status != TaskDone && input.Status != TaskFailed && input.Status != TaskCanceled {
		return Task{}, nil, fmt.Errorf("invalid terminal task status: %s", input.Status)
	}
	var event *TaskEvent
	if task.Type == "ai_agent" {
		payload := map[string]any{"status": input.Status, "stage": input.ProgressStage}
		if input.ResultJSON != nil {
			var result any
			if err := json.Unmarshal([]byte(*input.ResultJSON), &result); err != nil {
				return Task{}, nil, err
			}
			payload["result"] = result
		}
		if input.ErrorMessage != nil {
			payload["error"] = *input.ErrorMessage
		}
		item, err := appendTaskEvent(ctx, tx, TaskEventInput{UserID: task.UserID, TaskID: task.ID,
			Type: "task_" + string(input.Status), Payload: payload})
		if err != nil {
			return Task{}, nil, err
		}
		event = &item
	}
	_, err = tx.ExecContext(ctx, `UPDATE tasks SET status=?,progress_stage=?,progress_message=?,error_message=?,
result_json=COALESCE(?,result_json),finished_at=? WHERE id=?`, input.Status, input.ProgressStage,
		input.ProgressMessage, input.ErrorMessage, input.ResultJSON, time.Now().UTC(), taskID)
	if err != nil {
		return Task{}, nil, err
	}
	if err := tx.Commit(); err != nil {
		return Task{}, nil, err
	}
	task, err = s.GetTaskByID(ctx, taskID)
	return task, event, err
}
