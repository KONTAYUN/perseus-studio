package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type AgentRunStatus string

const (
	AgentRunQueued    AgentRunStatus = "queued"
	AgentRunRunning   AgentRunStatus = "running"
	AgentRunSucceeded AgentRunStatus = "succeeded"
	AgentRunWaiting   AgentRunStatus = "waiting_for_user"
	AgentRunDelegated AgentRunStatus = "delegated"
	AgentRunFailed    AgentRunStatus = "failed"
	AgentRunCanceled  AgentRunStatus = "canceled"
)

func (s *AgentRunStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

type AgentRun struct {
	ID                 string         `json:"id"`
	UserID             string         `json:"user_id"`
	ConversationID     string         `json:"conversation_id"`
	UserMessageID      sql.NullString `json:"-"`
	AssistantMessageID sql.NullString `json:"-"`
	ModelProviderID    string         `json:"model_provider_id"`
	ThinkingMode       string         `json:"thinking_mode,omitempty"`
	Status             AgentRunStatus `json:"status"`
	Stage              sql.NullString `json:"-"`
	ErrorMessage       sql.NullString `json:"-"`
	DiagnosticLogID    sql.NullString `json:"-"`
	FailureJSON        sql.NullString `json:"-"`
	EventHeadSeq       int64          `json:"event_head_seq"`
	FinalSeq           int64          `json:"final_seq"`
	CreatedAt          time.Time      `json:"created_at"`
	QueuedAt           sql.NullTime   `json:"-"`
	QueuePosition      int            `json:"queue_position,omitempty"`
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
	ThinkingMode    string
}

type UpdateAgentRunInput struct {
	Status             AgentRunStatus
	Stage              string
	ErrorMessage       *string
	DiagnosticLogID    string
	FailureJSON        *string
	UserMessageID      string
	AssistantMessageID string
	SetStartedAt       bool
	SetCompletedAt     bool
}

type AgentEventInput struct {
	ID             string
	RunID          string
	UserID         string
	ConversationID string
	MessageID      string
	TaskID         string
	Type           string
	Level          string
	Payload        any
}

type FinalizeAgentRunInput struct {
	RunID  string
	UserID string
	Status AgentRunStatus
	Stage  string
	Update UpdateAgentRunInput
	Event  AgentEventInput
}

type ResumeAgentRunInput struct {
	RunID  string
	UserID string
	Event  AgentEventInput
}

type AgentEventFilter struct {
	Types []string
	Limit int
}

type AgentRunStore struct {
	db *sql.DB
}

const agentRunSelectColumns = `
id, user_id, conversation_id, user_message_id, assistant_message_id, model_provider_id, thinking_mode,
status, stage, error_message, diagnostic_log_id, failure_json, event_head_seq, final_seq,
created_at, queued_at, started_at, completed_at, updated_at`

type AgentQueueFullError struct {
	Scope string
	Limit int
}

func (e *AgentQueueFullError) Error() string {
	return fmt.Sprintf("agent %s queue limit reached (%d)", e.Scope, e.Limit)
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
	thinking_mode TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  stage TEXT,
  error_message TEXT,
  diagnostic_log_id TEXT,
  created_at DATETIME NOT NULL,
	queued_at DATETIME,
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

CREATE TABLE IF NOT EXISTS agent_followup_answers (
  id TEXT PRIMARY KEY,
  answer_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  run_id TEXT NOT NULL,
  tool_call_id TEXT NOT NULL,
  answer TEXT NOT NULL,
  answer_message_id TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  UNIQUE(user_id, run_id, answer_id),
  UNIQUE(user_id, run_id, tool_call_id),
  FOREIGN KEY(run_id) REFERENCES agent_runs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS agent_edit_previews (
  preview_id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  tool_call_id TEXT NOT NULL,
  path TEXT NOT NULL,
  base_hash TEXT NOT NULL DEFAULT '',
  revision INTEGER NOT NULL,
  snapshot_json TEXT NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY(run_id) REFERENCES agent_runs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS agent_data_migrations (
  name TEXT PRIMARY KEY,
  details_json TEXT NOT NULL DEFAULT '{}',
  completed_at DATETIME NOT NULL
);
`
	if _, err := s.db.ExecContext(ctx, tables); err != nil {
		return err
	}
	if err := AddColumnIfNotExists(ctx, s.db, "agent_runs", "failure_json", "TEXT"); err != nil {
		return err
	}
	if err := AddColumnIfNotExists(ctx, s.db, "agent_runs", "event_head_seq", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := AddColumnIfNotExists(ctx, s.db, "agent_runs", "final_seq", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := AddColumnIfNotExists(ctx, s.db, "agent_runs", "thinking_mode", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := AddColumnIfNotExists(ctx, s.db, "agent_runs", "queued_at", "DATETIME"); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE agent_runs SET queued_at = created_at WHERE status = ? AND queued_at IS NULL`, AgentRunQueued); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `
UPDATE agent_runs
SET event_head_seq = COALESCE((SELECT MAX(seq) FROM agent_events WHERE run_id = agent_runs.id), 0)
WHERE event_head_seq = 0
`); err != nil {
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
CREATE INDEX IF NOT EXISTS idx_agent_followup_answers_run ON agent_followup_answers(user_id, run_id, created_at);
CREATE INDEX IF NOT EXISTS idx_agent_edit_previews_run ON agent_edit_previews(user_id, run_id, updated_at);
`
	if _, err := s.db.ExecContext(ctx, indexes); err != nil {
		return err
	}
	if err := s.cleanupLegacyPreviewEvents(ctx); err != nil {
		return err
	}
	if err := s.normalizeAgentEventSequences(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_events_run_seq_unique ON agent_events(run_id, seq)
`)
	return err
}

func (s *AgentRunStore) CreateRun(ctx context.Context, input CreateAgentRunInput) (AgentRun, error) {
	return s.CreateRunWithQueueLimits(ctx, input, 0, 0)
}

func (s *AgentRunStore) CreateRunWithQueueLimits(ctx context.Context, input CreateAgentRunInput, maxGlobal, maxPerUser int) (AgentRun, error) {
	now := time.Now().UTC()
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = nextStoreID("run", now)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AgentRun{}, err
	}
	defer tx.Rollback()
	if maxGlobal > 0 {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_runs WHERE status = ?`, AgentRunQueued).Scan(&count); err != nil {
			return AgentRun{}, err
		}
		if count >= maxGlobal {
			return AgentRun{}, &AgentQueueFullError{Scope: "global", Limit: maxGlobal}
		}
	}
	if maxPerUser > 0 {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_runs WHERE status = ? AND user_id = ?`, AgentRunQueued, input.UserID).Scan(&count); err != nil {
			return AgentRun{}, err
		}
		if count >= maxPerUser {
			return AgentRun{}, &AgentQueueFullError{Scope: "user", Limit: maxPerUser}
		}
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO agent_runs (
  id, user_id, conversation_id, user_message_id, model_provider_id, thinking_mode, status, stage, created_at, queued_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, id, input.UserID, input.ConversationID, nullIfEmpty(input.UserMessageID), input.ModelProviderID, input.ThinkingMode, AgentRunQueued, "queued", now, now, now)
	if err != nil {
		return AgentRun{}, err
	}
	if err := tx.Commit(); err != nil {
		return AgentRun{}, err
	}
	return s.GetRun(ctx, input.UserID, id)
}

func (s *AgentRunStore) GetRun(ctx context.Context, userID, id string) (AgentRun, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+agentRunSelectColumns+`
FROM agent_runs
WHERE id = ? AND user_id = ?`, id, userID)
	return scanAgentRun(row)
}

func (s *AgentRunStore) LatestConversationRun(ctx context.Context, userID, conversationID string) (AgentRun, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+agentRunSelectColumns+`
FROM agent_runs
WHERE user_id = ? AND conversation_id = ?
ORDER BY created_at DESC, id DESC
LIMIT 1`, userID, conversationID)
	return scanAgentRun(row)
}

func (s *AgentRunStore) UpdateRun(ctx context.Context, userID, id string, input UpdateAgentRunInput) (AgentRun, error) {
	if isTerminalAgentRunStatus(input.Status) {
		return AgentRun{}, fmt.Errorf("terminal agent run status %q must use FinalizeRunWithEvent", input.Status)
	}
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
	 failure_json = COALESCE(?, failure_json),
    user_message_id = COALESCE(NULLIF(?, ''), user_message_id),
    assistant_message_id = COALESCE(NULLIF(?, ''), assistant_message_id),
    started_at = COALESCE(?, started_at),
    completed_at = COALESCE(?, completed_at),
    updated_at = ?
WHERE id = ? AND user_id = ? AND status NOT IN (?, ?, ?, ?, ?)
`, input.Status, input.Stage, input.ErrorMessage, input.DiagnosticLogID, input.FailureJSON, input.UserMessageID, input.AssistantMessageID,
		startedAt, completedAt, now, id, userID, AgentRunSucceeded, AgentRunWaiting, AgentRunDelegated, AgentRunFailed, AgentRunCanceled)
	if err != nil {
		return AgentRun{}, err
	}
	return s.GetRun(ctx, userID, id)
}

func isTerminalAgentRunStatus(status AgentRunStatus) bool {
	switch status {
	case AgentRunSucceeded, AgentRunWaiting, AgentRunDelegated, AgentRunFailed, AgentRunCanceled:
		return true
	default:
		return false
	}
}

func (s *AgentRunStore) QueueCounts(ctx context.Context, userID string) (global, user int, err error) {
	if err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_runs WHERE status = ?`, AgentRunQueued).Scan(&global); err != nil {
		return 0, 0, err
	}
	if err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_runs WHERE status = ? AND user_id = ?`, AgentRunQueued, userID).Scan(&user); err != nil {
		return 0, 0, err
	}
	return global, user, nil
}

func (s *AgentRunStore) ListQueuedRuns(ctx context.Context, limit int) ([]AgentRun, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+agentRunSelectColumns+`
FROM agent_runs
WHERE status = ?
ORDER BY COALESCE(queued_at, created_at) ASC, created_at ASC, id ASC
LIMIT ?`, AgentRunQueued, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]AgentRun, 0)
	for rows.Next() {
		item, scanErr := scanAgentRun(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *AgentRunStore) ListRunningRuns(ctx context.Context, limit int) ([]AgentRun, error) {
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+agentRunSelectColumns+`
FROM agent_runs
WHERE status = ?
ORDER BY updated_at ASC, id ASC
LIMIT ?`, AgentRunRunning, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]AgentRun, 0)
	for rows.Next() {
		item, scanErr := scanAgentRun(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *AgentRunStore) ClaimQueuedRun(ctx context.Context, userID, id string) (AgentRun, bool, error) {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
UPDATE agent_runs
SET status = ?, stage = ?, started_at = COALESCE(started_at, ?), updated_at = ?
WHERE id = ? AND user_id = ? AND status = ?
`, AgentRunRunning, "calling_model", now, now, id, userID, AgentRunQueued)
	if err != nil {
		return AgentRun{}, false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return AgentRun{}, false, err
	}
	if count == 0 {
		return AgentRun{}, false, nil
	}
	run, err := s.GetRun(ctx, userID, id)
	return run, err == nil, err
}

func (s *AgentRunStore) QueuePosition(ctx context.Context, userID, id string) (int, error) {
	var position int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*) + 1
FROM agent_runs candidate, agent_runs target
WHERE target.id = ? AND target.user_id = ?
  AND candidate.status = ?
  AND (
    COALESCE(candidate.queued_at, candidate.created_at) < COALESCE(target.queued_at, target.created_at)
    OR (COALESCE(candidate.queued_at, candidate.created_at) = COALESCE(target.queued_at, target.created_at) AND candidate.id < target.id)
  )
`, id, userID, AgentRunQueued).Scan(&position)
	return position, err
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
		&item.ThinkingMode,
		&item.Status,
		&item.Stage,
		&item.ErrorMessage,
		&item.DiagnosticLogID,
		&item.FailureJSON,
		&item.EventHeadSeq,
		&item.FinalSeq,
		&item.CreatedAt,
		&item.QueuedAt,
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
	type failureDTO struct {
		FailureClass      string     `json:"failure_class,omitempty"`
		RetryAfterMS      int64      `json:"retry_after_ms,omitempty"`
		ResetAt           *time.Time `json:"reset_at,omitempty"`
		ProviderID        string     `json:"provider_id,omitempty"`
		ModelID           string     `json:"model_id,omitempty"`
		CanResumeNow      *bool      `json:"can_resume_now,omitempty"`
		RecommendedAction string     `json:"recommended_action,omitempty"`
	}
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
		failureDTO
		CreatedAt    time.Time  `json:"created_at"`
		StartedAt    *time.Time `json:"started_at"`
		CompletedAt  *time.Time `json:"completed_at"`
		UpdatedAt    time.Time  `json:"updated_at"`
		EventHeadSeq int64      `json:"event_head_seq"`
		FinalSeq     int64      `json:"final_seq"`
	}
	failure := failureDTO{}
	if r.FailureJSON.Valid && strings.TrimSpace(r.FailureJSON.String) != "" {
		_ = json.Unmarshal([]byte(r.FailureJSON.String), &failure)
		if failure.ResetAt != nil {
			remaining := time.Until(*failure.ResetAt)
			canResume := remaining <= 0
			failure.CanResumeNow = &canResume
			if remaining > 0 {
				failure.RetryAfterMS = remaining.Milliseconds()
			} else {
				failure.RetryAfterMS = 0
			}
		}
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
		failureDTO:         failure,
		EventHeadSeq:       r.EventHeadSeq,
		FinalSeq:           r.FinalSeq,
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
