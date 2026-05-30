package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"webide-backend/internal/idgen"
)

type DiagnosticLog struct {
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"`
	ConversationID  string    `json:"conversation_id,omitempty"`
	MessageID       string    `json:"message_id,omitempty"`
	TaskID          string    `json:"task_id,omitempty"`
	ModelProviderID string    `json:"model_provider_id,omitempty"`
	ModelName       string    `json:"model_name,omitempty"`
	ModelID         string    `json:"model_id,omitempty"`
	ProviderName    string    `json:"provider_name,omitempty"`
	Protocol        string    `json:"protocol,omitempty"`
	BaseURLHost     string    `json:"base_url_host,omitempty"`
	FinalURLPath    string    `json:"final_url_path,omitempty"`
	Stage           string    `json:"stage"`
	Level           string    `json:"level"`
	HTTPStatus      int       `json:"http_status,omitempty"`
	ContentType     string    `json:"content_type,omitempty"`
	ErrorMessage    string    `json:"error_message,omitempty"`
	ResponseSnippet string    `json:"response_snippet,omitempty"`
	RequestSummary  string    `json:"request_summary,omitempty"`
	DurationMS      int64     `json:"duration_ms,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type DiagnosticLogInput struct {
	UserID          string
	ConversationID  string
	MessageID       string
	TaskID          string
	ModelProviderID string
	ModelName       string
	ModelID         string
	ProviderName    string
	Protocol        string
	BaseURLHost     string
	FinalURLPath    string
	Stage           string
	Level           string
	HTTPStatus      int
	ContentType     string
	ErrorMessage    string
	ResponseSnippet string
	RequestSummary  string
	DurationMS      int64
}

type DiagnosticLogQuery struct {
	UserID   string
	AllUsers bool
	Limit    int
	Level    string
	Since    *time.Time
}

type DiagnosticStore struct {
	db *sql.DB
}

func NewDiagnosticStore(db *sql.DB) *DiagnosticStore {
	return &DiagnosticStore{db: db}
}

func (s *DiagnosticStore) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS diagnostic_logs (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  conversation_id TEXT NOT NULL DEFAULT '',
  message_id TEXT NOT NULL DEFAULT '',
  task_id TEXT NOT NULL DEFAULT '',
  model_provider_id TEXT NOT NULL DEFAULT '',
  model_name TEXT NOT NULL DEFAULT '',
  model_id TEXT NOT NULL DEFAULT '',
  provider_name TEXT NOT NULL DEFAULT '',
  protocol TEXT NOT NULL DEFAULT '',
  base_url_host TEXT NOT NULL DEFAULT '',
  final_url_path TEXT NOT NULL DEFAULT '',
  stage TEXT NOT NULL,
  level TEXT NOT NULL,
  http_status INTEGER NOT NULL DEFAULT 0,
  content_type TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  response_snippet TEXT NOT NULL DEFAULT '',
  request_summary TEXT NOT NULL DEFAULT '',
  duration_ms INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_diagnostic_logs_user_created ON diagnostic_logs(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_diagnostic_logs_user_level ON diagnostic_logs(user_id, level);
`)
	if err != nil {
		return err
	}
	return AddColumnIfNotExists(ctx, s.db, "diagnostic_logs", "request_summary", "TEXT NOT NULL DEFAULT ''")
}

func (s *DiagnosticStore) Append(ctx context.Context, input DiagnosticLogInput) (DiagnosticLog, error) {
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		return DiagnosticLog{}, fmt.Errorf("user_id is required")
	}
	level := normalizeDiagnosticLevel(input.Level)
	stage := strings.TrimSpace(input.Stage)
	if stage == "" {
		stage = "unknown"
	}
	now := time.Now().UTC()
	item := DiagnosticLog{
		ID:              newDiagnosticID(now),
		UserID:          userID,
		ConversationID:  trimDiagnosticField(input.ConversationID, 200),
		MessageID:       trimDiagnosticField(input.MessageID, 200),
		TaskID:          trimDiagnosticField(input.TaskID, 200),
		ModelProviderID: trimDiagnosticField(input.ModelProviderID, 200),
		ModelName:       trimDiagnosticField(input.ModelName, 200),
		ModelID:         trimDiagnosticField(input.ModelID, 200),
		ProviderName:    trimDiagnosticField(input.ProviderName, 200),
		Protocol:        trimDiagnosticField(input.Protocol, 32),
		BaseURLHost:     trimDiagnosticField(input.BaseURLHost, 255),
		FinalURLPath:    trimDiagnosticField(input.FinalURLPath, 512),
		Stage:           trimDiagnosticField(stage, 200),
		Level:           level,
		HTTPStatus:      input.HTTPStatus,
		ContentType:     trimDiagnosticField(input.ContentType, 200),
		ErrorMessage:    trimDiagnosticField(input.ErrorMessage, 1000),
		ResponseSnippet: trimDiagnosticField(input.ResponseSnippet, 500),
		RequestSummary:  trimDiagnosticField(input.RequestSummary, 8000),
		DurationMS:      input.DurationMS,
		CreatedAt:       now,
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO diagnostic_logs (
  id, user_id, conversation_id, message_id, task_id, model_provider_id, model_name, model_id,
  provider_name, protocol, base_url_host, final_url_path, stage, level, http_status,
  content_type, error_message, response_snippet, request_summary, duration_ms, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.UserID, item.ConversationID, item.MessageID, item.TaskID, item.ModelProviderID, item.ModelName, item.ModelID,
		item.ProviderName, item.Protocol, item.BaseURLHost, item.FinalURLPath, item.Stage, item.Level, item.HTTPStatus,
		item.ContentType, item.ErrorMessage, item.ResponseSnippet, item.RequestSummary, item.DurationMS, item.CreatedAt)
	if err != nil {
		return DiagnosticLog{}, err
	}
	_ = s.CleanupUser(ctx, userID)
	return item, nil
}

func newDiagnosticID(now time.Time) string {
	return idgen.NewAt("diag", now)
}

func (s *DiagnosticStore) List(ctx context.Context, query DiagnosticLogQuery) ([]DiagnosticLog, error) {
	limit := normalizeDiagnosticLimit(query.Limit, 50)
	level := strings.TrimSpace(strings.ToLower(query.Level))
	var args []any
	var filters []string
	if !query.AllUsers {
		userID := strings.TrimSpace(query.UserID)
		if userID == "" {
			return nil, fmt.Errorf("user_id is required")
		}
		filters = append(filters, "user_id = ?")
		args = append(args, userID)
	}
	if level == "info" || level == "warn" || level == "error" {
		filters = append(filters, "level = ?")
		args = append(args, level)
	}
	if query.Since != nil {
		filters = append(filters, "created_at >= ?")
		args = append(args, query.Since.UTC())
	}
	where := ""
	if len(filters) > 0 {
		where = "WHERE " + strings.Join(filters, " AND ")
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, conversation_id, message_id, task_id, model_provider_id, model_name, model_id,
  provider_name, protocol, base_url_host, final_url_path, stage, level, http_status,
  content_type, error_message, response_snippet, request_summary, duration_ms, created_at
FROM diagnostic_logs
`+where+`
ORDER BY created_at DESC
LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []DiagnosticLog
	for rows.Next() {
		item, err := scanDiagnosticLog(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *DiagnosticStore) ClearUser(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM diagnostic_logs WHERE user_id = ?`, userID)
	return err
}

func (s *DiagnosticStore) Delete(ctx context.Context, userID, id string) error {
	userID = strings.TrimSpace(userID)
	id = strings.TrimSpace(id)
	if userID == "" {
		return fmt.Errorf("user_id is required")
	}
	if id == "" {
		return fmt.Errorf("id is required")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM diagnostic_logs WHERE user_id = ? AND id = ?`, userID, id)
	return err
}

func (s *DiagnosticStore) DeleteAny(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("id is required")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM diagnostic_logs WHERE id = ?`, id)
	return err
}

func (s *DiagnosticStore) ClearAll(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM diagnostic_logs`)
	return err
}

func (s *DiagnosticStore) CleanupUser(ctx context.Context, userID string) error {
	cutoff := time.Now().UTC().Add(-14 * 24 * time.Hour)
	if _, err := s.db.ExecContext(ctx, `DELETE FROM diagnostic_logs WHERE user_id = ? AND created_at < ?`, userID, cutoff); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
DELETE FROM diagnostic_logs
WHERE user_id = ?
  AND id NOT IN (
    SELECT id FROM diagnostic_logs WHERE user_id = ? ORDER BY created_at DESC LIMIT 200
  )`, userID, userID)
	return err
}

func normalizeDiagnosticLimit(limit, defaultLimit int) int {
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func normalizeDiagnosticLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "info", "warn", "error":
		return strings.ToLower(strings.TrimSpace(level))
	default:
		return "error"
	}
}

func trimDiagnosticField(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

func scanDiagnosticLog(s scanner) (DiagnosticLog, error) {
	var item DiagnosticLog
	err := s.Scan(
		&item.ID, &item.UserID, &item.ConversationID, &item.MessageID, &item.TaskID, &item.ModelProviderID, &item.ModelName, &item.ModelID,
		&item.ProviderName, &item.Protocol, &item.BaseURLHost, &item.FinalURLPath, &item.Stage, &item.Level, &item.HTTPStatus,
		&item.ContentType, &item.ErrorMessage, &item.ResponseSnippet, &item.RequestSummary, &item.DurationMS, &item.CreatedAt,
	)
	return item, err
}
