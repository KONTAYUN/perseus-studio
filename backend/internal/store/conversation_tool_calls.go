package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func (s *ConversationStore) RecordToolCall(ctx context.Context, input ToolCallInput) (ToolCall, error) {
	now := time.Now().UTC()
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = nextStoreID("tc", now)
	}
	if input.Status == "" {
		input.Status = "success"
	}
	if input.ArgumentsJSON == "" {
		input.ArgumentsJSON = "{}"
	}
	startedAt := input.StartedAt
	if startedAt.IsZero() {
		startedAt = now
	}
	completedAt := input.CompletedAt
	if completedAt.IsZero() && input.Status != "running" {
		completedAt = now
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO tool_calls (
  id, user_id, run_id, conversation_id, message_id, task_id, tool_name, operation_hash,
  arguments_json, result_summary, status, error, started_at, completed_at, duration_ms,
  result_json, changed_paths_json, version_id, content_hash, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, id, input.UserID, input.RunID, input.ConversationID, input.MessageID, input.TaskID, input.ToolName, input.OperationHash,
		input.ArgumentsJSON, input.ResultSummary, input.Status, input.Error, nullTime(startedAt), nullTime(completedAt), input.DurationMS,
		input.ResultJSON, input.ChangedPathsJSON, input.VersionID, input.ContentHash, now)
	if err != nil {
		return ToolCall{}, err
	}
	return s.GetToolCallForRun(ctx, input.UserID, input.RunID, id)
}

func (s *ConversationStore) StartToolCall(ctx context.Context, input ToolCallInput) (ToolCall, error) {
	now := time.Now().UTC()
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = nextStoreID("tc", now)
	}
	if input.ArgumentsJSON == "" {
		input.ArgumentsJSON = "{}"
	}
	startedAt := input.StartedAt
	if startedAt.IsZero() {
		startedAt = now
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO tool_calls (
  id, user_id, run_id, conversation_id, message_id, task_id, tool_name, operation_hash,
  arguments_json, result_summary, status, error, started_at, duration_ms,
  result_json, changed_paths_json, version_id, content_hash, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '', 'running', '', ?, 0, '', '', '', '', ?)
`, id, input.UserID, input.RunID, input.ConversationID, input.MessageID, input.TaskID, input.ToolName, input.OperationHash,
		input.ArgumentsJSON, startedAt, now)
	if err != nil {
		return ToolCall{}, err
	}
	return s.GetToolCallForRun(ctx, input.UserID, input.RunID, id)
}

func (s *ConversationStore) CompleteToolCall(ctx context.Context, input ToolCallInput) (ToolCall, error) {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return ToolCall{}, fmt.Errorf("tool call id is required")
	}
	completedAt := input.CompletedAt
	if completedAt.IsZero() {
		completedAt = time.Now().UTC()
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "success"
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE tool_calls
SET result_summary = ?,
    status = ?,
    error = ?,
    completed_at = ?,
    duration_ms = ?,
    result_json = ?,
    changed_paths_json = ?,
    version_id = ?,
    content_hash = ?
WHERE id = ? AND user_id = ? AND (? = '' OR run_id = ?)
`, input.ResultSummary, status, input.Error, completedAt, input.DurationMS, input.ResultJSON, input.ChangedPathsJSON, input.VersionID, input.ContentHash, id, input.UserID, input.RunID, input.RunID)
	if err != nil {
		return ToolCall{}, err
	}
	if rows, rowErr := toolCallRowsAffected(ctx, s.db, id, input.UserID, input.RunID); rowErr != nil {
		return ToolCall{}, rowErr
	} else if rows == 0 {
		return ToolCall{}, sql.ErrNoRows
	}
	return s.GetToolCallForRun(ctx, input.UserID, input.RunID, id)
}

func toolCallRowsAffected(ctx context.Context, db *sql.DB, id, userID, runID string) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM tool_calls
WHERE id = ? AND user_id = ? AND (? = '' OR run_id = ?)
`, id, userID, runID, runID).Scan(&count)
	return count, err
}

func (s *ConversationStore) GetToolCall(ctx context.Context, userID, id string) (ToolCall, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, user_id, run_id, conversation_id, message_id, task_id, tool_name, operation_hash,
       arguments_json, result_summary, status, error, started_at, completed_at, duration_ms,
       result_json, changed_paths_json, version_id, content_hash, created_at
FROM tool_calls
WHERE id = ? AND user_id = ?
ORDER BY created_at DESC
LIMIT 1
`, id, userID)
	return scanToolCall(row)
}

func (s *ConversationStore) GetToolCallForRun(ctx context.Context, userID, runID, id string) (ToolCall, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, user_id, run_id, conversation_id, message_id, task_id, tool_name, operation_hash,
       arguments_json, result_summary, status, error, started_at, completed_at, duration_ms,
       result_json, changed_paths_json, version_id, content_hash, created_at
FROM tool_calls
WHERE id = ? AND user_id = ? AND run_id = ?
`, id, userID, runID)
	return scanToolCall(row)
}

func (s *ConversationStore) ListRecentToolCalls(ctx context.Context, userID, conversationID string, limit int) ([]ToolCall, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, run_id, conversation_id, message_id, task_id, tool_name, operation_hash,
       arguments_json, result_summary, status, error, started_at, completed_at, duration_ms,
       result_json, changed_paths_json, version_id, content_hash, created_at
FROM tool_calls
WHERE user_id = ? AND conversation_id = ?
ORDER BY created_at DESC
LIMIT ?
`, userID, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ToolCall
	for rows.Next() {
		item, scanErr := scanToolCall(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanToolCall(s scanner) (ToolCall, error) {
	var item ToolCall
	var conversationID, messageID, taskID sql.NullString
	err := s.Scan(
		&item.ID,
		&item.UserID,
		&item.RunID,
		&conversationID,
		&messageID,
		&taskID,
		&item.ToolName,
		&item.OperationHash,
		&item.ArgumentsJSON,
		&item.ResultSummary,
		&item.Status,
		&item.Error,
		&item.StartedAt,
		&item.CompletedAt,
		&item.DurationMS,
		&item.ResultJSON,
		&item.ChangedPathsJSON,
		&item.VersionID,
		&item.ContentHash,
		&item.CreatedAt,
	)
	if item.ArgumentsJSON == "" {
		item.ArgumentsJSON = "{}"
	}
	item.ConversationID = conversationID.String
	item.MessageID = messageID.String
	item.TaskID = taskID.String
	return ToolCall{
		ID:               item.ID,
		UserID:           item.UserID,
		RunID:            item.RunID,
		ConversationID:   item.ConversationID,
		MessageID:        item.MessageID,
		TaskID:           item.TaskID,
		ToolName:         item.ToolName,
		OperationHash:    item.OperationHash,
		ArgumentsJSON:    item.ArgumentsJSON,
		ResultSummary:    item.ResultSummary,
		Status:           item.Status,
		Error:            item.Error,
		StartedAt:        item.StartedAt,
		CompletedAt:      item.CompletedAt,
		DurationMS:       item.DurationMS,
		ResultJSON:       item.ResultJSON,
		ChangedPathsJSON: item.ChangedPathsJSON,
		VersionID:        item.VersionID,
		ContentHash:      item.ContentHash,
		CreatedAt:        item.CreatedAt,
	}, err
}
