package store

import (
	"database/sql"
	"encoding/json"
	"time"
)

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
		&task.EventHeadSeq,
		&task.FinalSeq,
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
		EventHeadSeq      int64      `json:"event_head_seq"`
		FinalSeq          int64      `json:"final_seq"`
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
		EventHeadSeq:      t.EventHeadSeq,
		FinalSeq:          t.FinalSeq,
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
