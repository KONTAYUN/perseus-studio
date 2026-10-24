package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

type AgentRunStatus string

const (
	AgentRunQueued    AgentRunStatus = "queued"
	AgentRunRunning   AgentRunStatus = "running"
	AgentRunSucceeded AgentRunStatus = "succeeded"
	AgentRunFailed    AgentRunStatus = "failed"
	AgentRunCanceled  AgentRunStatus = "canceled"
)

type AgentRun struct {
	ID                 string         `json:"id"`
	UserID             string         `json:"user_id"`
	ConversationID     string         `json:"conversation_id"`
	UserMessageID      sql.NullString `json:"-"`
	AssistantMessageID sql.NullString `json:"-"`
	ModelProviderID    string         `json:"model_provider_id"`
	Status             AgentRunStatus `json:"status"`
	Stage              sql.NullString `json:"-"`
	ErrorMessage       sql.NullString `json:"-"`
	DiagnosticLogID    sql.NullString `json:"-"`
	CreatedAt          time.Time      `json:"created_at"`
	StartedAt          sql.NullTime   `json:"-"`
	CompletedAt        sql.NullTime   `json:"-"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

type AgentEvent struct {
	ID             string    `json:"id"`
	RunID          string    `json:"run_id"`
	UserID         string    `json:"user_id"`
	ConversationID string    `json:"conversation_id,omitempty"`
	MessageID      string    `json:"message_id,omitempty"`
	TaskID         string    `json:"task_id,omitempty"`
	Type           string    `json:"type"`
	Level          string    `json:"level"`
	Seq            int64     `json:"seq"`
	PayloadJSON    string    `json:"payload_json"`
	CreatedAt      time.Time `json:"created_at"`
}

type CreateAgentRunInput struct {
	ID              string
	UserID          string
	ConversationID  string
	UserMessageID   string
	ModelProviderID string
}

type UpdateAgentRunInput struct {
	Status             AgentRunStatus
	Stage              string
	ErrorMessage       *string
	DiagnosticLogID    string
	UserMessageID      string
	AssistantMessageID string
	SetStartedAt       bool
	SetCompletedAt     bool
}

type AgentEventInput struct {
	PendingMessageID string
	RunID            string
	UserID           string
	ConversationID   string
	MessageID        string
	TaskID           string
	Type             string
	Level            string
	Payload          any
}

type AgentEventFilter struct {
	Types []string
	Limit int
}

type AgentRunStore struct {
	db *sql.DB
}

func NewAgentRunStore(db *sql.DB) *AgentRunStore {
	return &AgentRunStore{db: db}
}

func (s *AgentRunStore) Migrate(ctx context.Context) error {
	tables := `
CREATE TABLE IF NOT EXISTS agent_runs (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  conversation_id TEXT NOT NULL,
  user_message_id TEXT,
  assistant_message_id TEXT,
  model_provider_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  stage TEXT,
  error_message TEXT,
  diagnostic_log_id TEXT,
  created_at DATETIME NOT NULL,
  started_at DATETIME,
  completed_at DATETIME,
  updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS agent_events (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  conversation_id TEXT NOT NULL DEFAULT '',
  message_id TEXT NOT NULL DEFAULT '',
  task_id TEXT NOT NULL DEFAULT '',
  type TEXT NOT NULL,
  level TEXT NOT NULL,
  seq INTEGER NOT NULL,
  payload_json TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  FOREIGN KEY(run_id) REFERENCES agent_runs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS agent_run_checkpoints (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  seq INTEGER NOT NULL,
  state_json TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  FOREIGN KEY(run_id) REFERENCES agent_runs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS conversation_run_requests (
  user_id TEXT NOT NULL,
  request_id TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  run_id TEXT NOT NULL,
  PRIMARY KEY(user_id, request_id),
  FOREIGN KEY(run_id) REFERENCES agent_runs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS pending_conversation_messages (
  id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  conversation_id TEXT NOT NULL,
  content TEXT NOT NULL,
  model_provider_id TEXT NOT NULL,
  thinking_mode TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  PRIMARY KEY(user_id, id),
  UNIQUE(user_id, conversation_id),
  FOREIGN KEY(conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
);
`
	if _, err := s.db.ExecContext(ctx, tables); err != nil {
		return err
	}
	if err := AddColumnIfNotExists(ctx, s.db, "pending_conversation_messages", "document_action_json", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	indexes := `
CREATE INDEX IF NOT EXISTS idx_agent_runs_user_created ON agent_runs(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_runs_conversation ON agent_runs(user_id, conversation_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_runs_status ON agent_runs(status, created_at);
CREATE INDEX IF NOT EXISTS idx_agent_events_run_seq ON agent_events(run_id, seq);
CREATE INDEX IF NOT EXISTS idx_agent_events_user_created ON agent_events(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_events_conversation_type ON agent_events(user_id, conversation_id, type, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_run_checkpoints_run_seq ON agent_run_checkpoints(run_id, seq);
`
	_, err := s.db.ExecContext(ctx, indexes)
	return err
}

func (s *AgentRunStore) CreateRun(ctx context.Context, input CreateAgentRunInput) (AgentRun, error) {
	now := time.Now().UTC()
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = nextStoreID("run", now)
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO agent_runs (
  id, user_id, conversation_id, user_message_id, model_provider_id, status, stage, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
`, id, input.UserID, input.ConversationID, nullIfEmpty(input.UserMessageID), input.ModelProviderID, AgentRunQueued, "queued", now, now)
	if err != nil {
		return AgentRun{}, err
	}
	return s.GetRun(ctx, input.UserID, id)
}

func (s *AgentRunStore) GetRun(ctx context.Context, userID, id string) (AgentRun, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, user_id, conversation_id, user_message_id, assistant_message_id, model_provider_id,
       status, stage, error_message, diagnostic_log_id, created_at, started_at, completed_at, updated_at
FROM agent_runs
WHERE id = ? AND user_id = ?
`, id, userID)
	return scanAgentRun(row)
}

func (s *AgentRunStore) LatestConversationRun(ctx context.Context, userID, conversationID string) (AgentRun, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, user_id, conversation_id, user_message_id, assistant_message_id, model_provider_id,
       status, stage, error_message, diagnostic_log_id, created_at, started_at, completed_at, updated_at
FROM agent_runs
WHERE user_id = ? AND conversation_id = ?
ORDER BY created_at DESC, id DESC
LIMIT 1
`, userID, conversationID)
	return scanAgentRun(row)
}

func (s *AgentRunStore) UpdateRun(ctx context.Context, userID, id string, input UpdateAgentRunInput) (AgentRun, error) {
	now := time.Now().UTC()
	var startedAt any
	var completedAt any
	if input.SetStartedAt {
		startedAt = now
	}
	if input.SetCompletedAt {
		completedAt = now
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE agent_runs
SET status = COALESCE(NULLIF(?, ''), status),
    stage = COALESCE(NULLIF(?, ''), stage),
    error_message = COALESCE(?, error_message),
    diagnostic_log_id = COALESCE(NULLIF(?, ''), diagnostic_log_id),
    user_message_id = COALESCE(NULLIF(?, ''), user_message_id),
    assistant_message_id = COALESCE(NULLIF(?, ''), assistant_message_id),
    started_at = COALESCE(?, started_at),
    completed_at = COALESCE(?, completed_at),
    updated_at = ?
WHERE id = ? AND user_id = ? AND status IN ('queued', 'running')
`, input.Status, input.Stage, input.ErrorMessage, input.DiagnosticLogID, input.UserMessageID, input.AssistantMessageID, startedAt, completedAt, now, id, userID)
	if err != nil {
		return AgentRun{}, err
	}
	return s.GetRun(ctx, userID, id)
}

func (s *AgentRunStore) ResumeRun(ctx context.Context, userID, id string) (AgentRun, error) {
	run, _, err := s.resumeRun(ctx, userID, id, false, "")
	return run, err
}

func (s *AgentRunStore) FailStaleRunningRuns(ctx context.Context, olderThan time.Duration, message string) (int64, error) {
	if olderThan < 0 {
		olderThan = 2 * time.Minute
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "上次服务停止或本地状态保存中断，本轮已暂停，可点击继续本轮。"
	}
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
UPDATE agent_runs
SET status = ?,
    stage = ?,
    error_message = ?,
    completed_at = ?,
    updated_at = ?
WHERE status IN (?, ?)
  AND updated_at < ?
`, AgentRunFailed, "recoverable", message, now, now, AgentRunRunning, AgentRunQueued, now.Add(-olderThan))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func scanAgentRun(s scanner) (AgentRun, error) {
	var item AgentRun
	err := s.Scan(
		&item.ID,
		&item.UserID,
		&item.ConversationID,
		&item.UserMessageID,
		&item.AssistantMessageID,
		&item.ModelProviderID,
		&item.Status,
		&item.Stage,
		&item.ErrorMessage,
		&item.DiagnosticLogID,
		&item.CreatedAt,
		&item.StartedAt,
		&item.CompletedAt,
		&item.UpdatedAt,
	)
	return item, err
}

func scanAgentEvent(s scanner) (AgentEvent, error) {
	var item AgentEvent
	err := s.Scan(
		&item.ID,
		&item.RunID,
		&item.UserID,
		&item.ConversationID,
		&item.MessageID,
		&item.TaskID,
		&item.Type,
		&item.Level,
		&item.Seq,
		&item.PayloadJSON,
		&item.CreatedAt,
	)
	return item, err
}

func (r AgentRun) MarshalJSON() ([]byte, error) {
	type runDTO struct {
		ID                 string         `json:"id"`
		UserID             string         `json:"user_id"`
		ConversationID     string         `json:"conversation_id"`
		UserMessageID      *string        `json:"user_message_id"`
		AssistantMessageID *string        `json:"assistant_message_id"`
		ModelProviderID    string         `json:"model_provider_id"`
		Status             AgentRunStatus `json:"status"`
		Stage              *string        `json:"stage"`
		ErrorMessage       *string        `json:"error_message"`
		DiagnosticLogID    *string        `json:"diagnostic_log_id"`
		CreatedAt          time.Time      `json:"created_at"`
		StartedAt          *time.Time     `json:"started_at"`
		CompletedAt        *time.Time     `json:"completed_at"`
		UpdatedAt          time.Time      `json:"updated_at"`
	}
	return json.Marshal(runDTO{
		ID:                 r.ID,
		UserID:             r.UserID,
		ConversationID:     r.ConversationID,
		UserMessageID:      nullableString(r.UserMessageID),
		AssistantMessageID: nullableString(r.AssistantMessageID),
		ModelProviderID:    r.ModelProviderID,
		Status:             r.Status,
		Stage:              nullableString(r.Stage),
		ErrorMessage:       nullableString(r.ErrorMessage),
		DiagnosticLogID:    nullableString(r.DiagnosticLogID),
		CreatedAt:          r.CreatedAt,
		StartedAt:          nullableTime(r.StartedAt),
		CompletedAt:        nullableTime(r.CompletedAt),
		UpdatedAt:          r.UpdatedAt,
	})
}

func (e AgentEvent) MarshalJSON() ([]byte, error) {
	type eventDTO struct {
		ID             string    `json:"id"`
		RunID          string    `json:"run_id"`
		UserID         string    `json:"user_id"`
		ConversationID string    `json:"conversation_id,omitempty"`
		MessageID      string    `json:"message_id,omitempty"`
		TaskID         string    `json:"task_id,omitempty"`
		Type           string    `json:"type"`
		Level          string    `json:"level"`
		Seq            int64     `json:"seq"`
		Payload        any       `json:"payload"`
		CreatedAt      time.Time `json:"created_at"`
	}
	var payload any
	if err := json.Unmarshal([]byte(e.PayloadJSON), &payload); err != nil {
		payload = e.PayloadJSON
	}
	return json.Marshal(eventDTO{
		ID:             e.ID,
		RunID:          e.RunID,
		UserID:         e.UserID,
		ConversationID: e.ConversationID,
		MessageID:      e.MessageID,
		TaskID:         e.TaskID,
		Type:           e.Type,
		Level:          e.Level,
		Seq:            e.Seq,
		Payload:        payload,
		CreatedAt:      e.CreatedAt,
	})
}
