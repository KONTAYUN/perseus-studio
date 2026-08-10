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
			return AgentEvent{}, fmt.Errorf("agent event id %s was replayed with different content", id)
		}
		if !errors.Is(lookupErr, sql.ErrNoRows) {
			return AgentEvent{}, lookupErr
		}
	}
	var seq int64
	if err := tx.QueryRowContext(ctx, `
UPDATE agent_runs
SET event_head_seq = event_head_seq + 1, updated_at = ?
WHERE id = ? AND user_id = ?
RETURNING event_head_seq
`, now, input.RunID, input.UserID).Scan(&seq); err != nil {
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
