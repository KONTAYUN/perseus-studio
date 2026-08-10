package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrAgentRunFinalized = errors.New("agent run is already finalized")
var ErrAgentEventIntegrity = errors.New("agent event integrity error")

func (s *AgentRunStore) AppendEvent(ctx context.Context, input AgentEventInput) (AgentEvent, error) {
	now := time.Now().UTC()
	level := strings.TrimSpace(strings.ToLower(input.Level))
	if level == "" {
		level = "info"
	}
	eventType := strings.TrimSpace(input.Type)
	if eventType == "" {
		eventType = "event"
	}
	raw, err := json.Marshal(input.Payload)
	if err != nil {
		raw = []byte(`{"error":"failed to encode event payload"}`)
	}
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = nextStoreID("evt", now)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AgentEvent{}, err
	}
	defer tx.Rollback()
	if input.ID != "" {
		existing, lookupErr := scanAgentEvent(tx.QueryRowContext(ctx, `
SELECT id, run_id, user_id, conversation_id, message_id, task_id, type, level, seq, payload_json, created_at
FROM agent_events WHERE id = ?
`, id))
		if lookupErr == nil {
			if existing.RunID == input.RunID && existing.UserID == input.UserID &&
				existing.ConversationID == input.ConversationID && existing.MessageID == input.MessageID &&
				existing.TaskID == input.TaskID && existing.Type == eventType &&
				existing.Level == level && existing.PayloadJSON == string(raw) {
				return existing, nil
			}
			return AgentEvent{}, fmt.Errorf("%w: agent event id %s was replayed with different content", ErrAgentEventIntegrity, id)
		}
		if !errors.Is(lookupErr, sql.ErrNoRows) {
			return AgentEvent{}, lookupErr
		}
	}
	var seq int64
	if err := tx.QueryRowContext(ctx, `
UPDATE agent_runs
SET event_head_seq = event_head_seq + 1, updated_at = ?
WHERE id = ? AND user_id = ? AND final_seq = 0
RETURNING event_head_seq
`, now, input.RunID, input.UserID).Scan(&seq); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AgentEvent{}, ErrAgentRunFinalized
		}
		return AgentEvent{}, err
	}
	row := tx.QueryRowContext(ctx, `
INSERT INTO agent_events (
  id, run_id, user_id, conversation_id, message_id, task_id, type, level, seq, payload_json, created_at
) VALUES (
  ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING id, run_id, user_id, conversation_id, message_id, task_id, type, level, seq, payload_json, created_at
`, id, input.RunID, input.UserID, input.ConversationID, input.MessageID, input.TaskID, eventType, level, seq, string(raw), now)
	item, err := scanAgentEvent(row)
	if err != nil {
		return AgentEvent{}, err
	}
	if err := tx.Commit(); err != nil {
		return AgentEvent{}, err
	}
	return item, nil
}

// FinalizeRunWithEvent appends the terminal event and seals the run at that
// exact sequence in one transaction. A terminal state must never become
// visible before its final durable event exists.
func (s *AgentRunStore) FinalizeRunWithEvent(ctx context.Context, input FinalizeAgentRunInput) (AgentRun, AgentEvent, error) {
	if !isTerminalAgentRunStatus(input.Status) {
		return AgentRun{}, AgentEvent{}, fmt.Errorf("agent run status %q is not terminal", input.Status)
	}
	if strings.TrimSpace(input.RunID) == "" || strings.TrimSpace(input.UserID) == "" {
		return AgentRun{}, AgentEvent{}, fmt.Errorf("terminal run id and user id are required")
	}
	eventInput := input.Event
	if eventInput.RunID == "" {
		eventInput.RunID = input.RunID
	}
	if eventInput.UserID == "" {
		eventInput.UserID = input.UserID
	}
	if eventInput.RunID != input.RunID || eventInput.UserID != input.UserID {
		return AgentRun{}, AgentEvent{}, fmt.Errorf("terminal event ownership does not match run")
	}

	now := time.Now().UTC()
	level := strings.TrimSpace(strings.ToLower(eventInput.Level))
	if level == "" {
		level = "info"
	}
	eventType := strings.TrimSpace(eventInput.Type)
	if eventType == "" {
		return AgentRun{}, AgentEvent{}, fmt.Errorf("terminal event type is required")
	}
	raw, err := json.Marshal(eventInput.Payload)
	if err != nil {
		return AgentRun{}, AgentEvent{}, fmt.Errorf("encode terminal event payload: %w", err)
	}
	eventID := strings.TrimSpace(eventInput.ID)
	if eventID == "" {
		eventID = nextStoreID("evt", now)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AgentRun{}, AgentEvent{}, err
	}
	defer tx.Rollback()

	if eventInput.ID != "" {
		existing, lookupErr := scanAgentEvent(tx.QueryRowContext(ctx, `
SELECT id, run_id, user_id, conversation_id, message_id, task_id, type, level, seq, payload_json, created_at
FROM agent_events WHERE id = ?
`, eventID))
		if lookupErr == nil {
			if existing.RunID != eventInput.RunID || existing.UserID != eventInput.UserID ||
				existing.ConversationID != eventInput.ConversationID || existing.MessageID != eventInput.MessageID ||
				existing.TaskID != eventInput.TaskID || existing.Type != eventType || existing.Level != level ||
				existing.PayloadJSON != string(raw) {
				return AgentRun{}, AgentEvent{}, fmt.Errorf("%w: agent event id %s was replayed with different content", ErrAgentEventIntegrity, eventID)
			}
			run, runErr := scanAgentRun(tx.QueryRowContext(ctx, `SELECT `+agentRunSelectColumns+`
FROM agent_runs WHERE id = ? AND user_id = ?`, input.RunID, input.UserID))
			if runErr != nil {
				return AgentRun{}, AgentEvent{}, runErr
			}
			if run.Status != input.Status || run.FinalSeq != existing.Seq {
				return AgentRun{}, AgentEvent{}, fmt.Errorf("%w: terminal event %s exists but run state is inconsistent", ErrAgentEventIntegrity, eventID)
			}
			return run, existing, nil
		}
		if !errors.Is(lookupErr, sql.ErrNoRows) {
			return AgentRun{}, AgentEvent{}, lookupErr
		}
	}

	var seq int64
	if err := tx.QueryRowContext(ctx, `
UPDATE agent_runs
SET event_head_seq = event_head_seq + 1, updated_at = ?
WHERE id = ? AND user_id = ? AND final_seq = 0
RETURNING event_head_seq
`, now, input.RunID, input.UserID).Scan(&seq); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AgentRun{}, AgentEvent{}, ErrAgentRunFinalized
		}
		return AgentRun{}, AgentEvent{}, err
	}
	event, err := scanAgentEvent(tx.QueryRowContext(ctx, `
INSERT INTO agent_events (
  id, run_id, user_id, conversation_id, message_id, task_id, type, level, seq, payload_json, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id, run_id, user_id, conversation_id, message_id, task_id, type, level, seq, payload_json, created_at
`, eventID, eventInput.RunID, eventInput.UserID, eventInput.ConversationID, eventInput.MessageID,
		eventInput.TaskID, eventType, level, seq, string(raw), now))
	if err != nil {
		return AgentRun{}, AgentEvent{}, err
	}

	update := input.Update
	var startedAt any
	var completedAt any
	if update.SetStartedAt {
		startedAt = now
	}
	if update.SetCompletedAt {
		completedAt = now
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE agent_runs
SET status = ?,
    stage = ?,
    error_message = COALESCE(?, error_message),
    diagnostic_log_id = COALESCE(NULLIF(?, ''), diagnostic_log_id),
    failure_json = COALESCE(?, failure_json),
    user_message_id = COALESCE(NULLIF(?, ''), user_message_id),
    assistant_message_id = COALESCE(NULLIF(?, ''), assistant_message_id),
    started_at = COALESCE(?, started_at),
    completed_at = COALESCE(?, completed_at),
    final_seq = ?,
    updated_at = ?
WHERE id = ? AND user_id = ?
`, input.Status, input.Stage, update.ErrorMessage, update.DiagnosticLogID, update.FailureJSON,
		update.UserMessageID, update.AssistantMessageID, startedAt, completedAt, seq, now, input.RunID, input.UserID); err != nil {
		return AgentRun{}, AgentEvent{}, err
	}
	run, err := scanAgentRun(tx.QueryRowContext(ctx, `SELECT `+agentRunSelectColumns+`
FROM agent_runs WHERE id = ? AND user_id = ?`, input.RunID, input.UserID))
	if err != nil {
		return AgentRun{}, AgentEvent{}, err
	}
	if err := tx.Commit(); err != nil {
		return AgentRun{}, AgentEvent{}, err
	}
	return run, event, nil
}

// ResumeRunWithEvent reopens a failed or canceled run and appends its resume
// event in the same transaction. A queued run is therefore never observable
// without the durable lifecycle item that reopened it.
func (s *AgentRunStore) ResumeRunWithEvent(ctx context.Context, input ResumeAgentRunInput) (AgentRun, AgentEvent, error) {
	if strings.TrimSpace(input.RunID) == "" || strings.TrimSpace(input.UserID) == "" {
		return AgentRun{}, AgentEvent{}, fmt.Errorf("resume run id and user id are required")
	}
	eventInput := input.Event
	if eventInput.RunID == "" {
		eventInput.RunID = input.RunID
	}
	if eventInput.UserID == "" {
		eventInput.UserID = input.UserID
	}
	if eventInput.RunID != input.RunID || eventInput.UserID != input.UserID {
		return AgentRun{}, AgentEvent{}, fmt.Errorf("resume event ownership does not match run")
	}
	eventType := strings.TrimSpace(eventInput.Type)
	if eventType == "" {
		return AgentRun{}, AgentEvent{}, fmt.Errorf("resume event type is required")
	}
	level := strings.TrimSpace(strings.ToLower(eventInput.Level))
	if level == "" {
		level = "info"
	}
	raw, err := json.Marshal(eventInput.Payload)
	if err != nil {
		return AgentRun{}, AgentEvent{}, fmt.Errorf("encode resume event payload: %w", err)
	}
	now := time.Now().UTC()
	eventID := strings.TrimSpace(eventInput.ID)
	if eventID == "" {
		eventID = nextStoreID("evt", now)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AgentRun{}, AgentEvent{}, err
	}
	defer tx.Rollback()
	if eventInput.ID != "" {
		existing, lookupErr := scanAgentEvent(tx.QueryRowContext(ctx, `
SELECT id, run_id, user_id, conversation_id, message_id, task_id, type, level, seq, payload_json, created_at
FROM agent_events WHERE id = ?
`, eventID))
		if lookupErr == nil {
			if existing.RunID != eventInput.RunID || existing.UserID != eventInput.UserID ||
				existing.ConversationID != eventInput.ConversationID || existing.MessageID != eventInput.MessageID ||
				existing.TaskID != eventInput.TaskID || existing.Type != eventType || existing.Level != level ||
				existing.PayloadJSON != string(raw) {
				return AgentRun{}, AgentEvent{}, fmt.Errorf("%w: resume event id %s was replayed with different content", ErrAgentEventIntegrity, eventID)
			}
			run, runErr := scanAgentRun(tx.QueryRowContext(ctx, `SELECT `+agentRunSelectColumns+`
FROM agent_runs WHERE id = ? AND user_id = ?`, input.RunID, input.UserID))
			return run, existing, runErr
		}
		if !errors.Is(lookupErr, sql.ErrNoRows) {
			return AgentRun{}, AgentEvent{}, lookupErr
		}
	}

	var seq int64
	if err := tx.QueryRowContext(ctx, `
UPDATE agent_runs
SET status = ?, stage = ?, error_message = NULL, diagnostic_log_id = NULL,
    failure_json = NULL, started_at = NULL, completed_at = NULL, final_seq = 0,
    queued_at = ?, event_head_seq = event_head_seq + 1, updated_at = ?
WHERE id = ? AND user_id = ? AND status IN (?, ?) AND final_seq > 0
RETURNING event_head_seq
`, AgentRunQueued, "queued", now, now, input.RunID, input.UserID, AgentRunFailed, AgentRunCanceled).Scan(&seq); err != nil {
		return AgentRun{}, AgentEvent{}, err
	}
	event, err := scanAgentEvent(tx.QueryRowContext(ctx, `
INSERT INTO agent_events (
  id, run_id, user_id, conversation_id, message_id, task_id, type, level, seq, payload_json, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id, run_id, user_id, conversation_id, message_id, task_id, type, level, seq, payload_json, created_at
`, eventID, eventInput.RunID, eventInput.UserID, eventInput.ConversationID, eventInput.MessageID,
		eventInput.TaskID, eventType, level, seq, string(raw), now))
	if err != nil {
		return AgentRun{}, AgentEvent{}, err
	}
	run, err := scanAgentRun(tx.QueryRowContext(ctx, `SELECT `+agentRunSelectColumns+`
FROM agent_runs WHERE id = ? AND user_id = ?`, input.RunID, input.UserID))
	if err != nil {
		return AgentRun{}, AgentEvent{}, err
	}
	if err := tx.Commit(); err != nil {
		return AgentRun{}, AgentEvent{}, err
	}
	return run, event, nil
}

func (s *AgentRunStore) EventHeadSeq(ctx context.Context, userID, runID string) (int64, error) {
	var seq int64
	err := s.db.QueryRowContext(ctx, `
SELECT event_head_seq FROM agent_runs WHERE id = ? AND user_id = ?
`, runID, userID).Scan(&seq)
	return seq, err
}

func (s *AgentRunStore) normalizeAgentEventSequences(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
WITH ranked AS (
  SELECT id, ROW_NUMBER() OVER (
    PARTITION BY run_id ORDER BY seq ASC, created_at ASC, id ASC
  ) AS normalized_seq
  FROM agent_events
)
UPDATE agent_events
SET seq = (SELECT normalized_seq FROM ranked WHERE ranked.id = agent_events.id)
WHERE seq <> (SELECT normalized_seq FROM ranked WHERE ranked.id = agent_events.id)
`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE agent_runs
SET event_head_seq = COALESCE((SELECT MAX(seq) FROM agent_events WHERE run_id = agent_runs.id), 0),
    final_seq = CASE
      WHEN status IN (?, ?, ?, ?, ?)
      THEN COALESCE((SELECT MAX(seq) FROM agent_events WHERE run_id = agent_runs.id), 0)
      ELSE 0
    END
`, AgentRunSucceeded, AgentRunWaiting, AgentRunDelegated, AgentRunFailed, AgentRunCanceled); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *AgentRunStore) ListEvents(ctx context.Context, userID, runID string) ([]AgentEvent, error) {
	return s.ListEventsAfter(ctx, userID, runID, 0)
}

func (s *AgentRunStore) ListEventsFiltered(ctx context.Context, userID, runID string, filter AgentEventFilter) ([]AgentEvent, error) {
	types := normalizedEventTypes(filter.Types)
	if len(types) == 0 {
		return s.ListEvents(ctx, userID, runID)
	}
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	placeholders := make([]string, len(types))
	args := make([]any, 0, len(types)+3)
	args = append(args, runID, userID)
	for index, eventType := range types {
		placeholders[index] = "?"
		args = append(args, eventType)
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `
SELECT id, run_id, user_id, conversation_id, message_id, task_id, type, level, seq, payload_json, created_at
FROM (
  SELECT id, run_id, user_id, conversation_id, message_id, task_id, type, level, seq, payload_json, created_at
  FROM agent_events
  WHERE run_id = ? AND user_id = ? AND type IN (`+strings.Join(placeholders, ",")+`)
  ORDER BY seq DESC
  LIMIT ?
)
ORDER BY seq ASC
`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []AgentEvent
	for rows.Next() {
		item, err := scanAgentEvent(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *AgentRunStore) ListEventsAfter(ctx context.Context, userID, runID string, afterSeq int64) ([]AgentEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, run_id, user_id, conversation_id, message_id, task_id, type, level, seq, payload_json, created_at
FROM agent_events
WHERE run_id = ? AND user_id = ? AND seq > ?
ORDER BY seq ASC
`, runID, userID, afterSeq)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []AgentEvent
	for rows.Next() {
		item, err := scanAgentEvent(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *AgentRunStore) LatestConversationEventByType(ctx context.Context, userID, conversationID, eventType string) (AgentEvent, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, run_id, user_id, conversation_id, message_id, task_id, type, level, seq, payload_json, created_at
FROM agent_events
WHERE user_id = ? AND conversation_id = ? AND type = ?
ORDER BY created_at DESC, id DESC
LIMIT 1
`, userID, conversationID, eventType)
	return scanAgentEvent(row)
}

func normalizedEventTypes(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
