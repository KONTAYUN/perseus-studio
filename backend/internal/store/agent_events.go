package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

func (s *AgentRunStore) AppendEvent(ctx context.Context, input AgentEventInput) (AgentEvent, error) {
	if input.PendingMessageID != "" {
		return s.appendPendingSteer(ctx, input)
	}
	return appendAgentEvent(ctx, s.db, input)
}

func appendAgentEvent(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, input AgentEventInput) (AgentEvent, error) {
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
	id := nextStoreID("evt", now)
	row := db.QueryRowContext(ctx, `
INSERT INTO agent_events (
  id, run_id, user_id, conversation_id, message_id, task_id, type, level, seq, payload_json, created_at
) VALUES (
  ?, ?, ?, ?, ?, ?, ?, ?,
  COALESCE((SELECT MAX(seq) + 1 FROM agent_events WHERE run_id = ?), 1),
  ?, ?
)
RETURNING id, run_id, user_id, conversation_id, message_id, task_id, type, level, seq, payload_json, created_at
`, id, input.RunID, input.UserID, input.ConversationID, input.MessageID, input.TaskID, eventType, level, input.RunID, string(raw), now)
	return scanAgentEvent(row)
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
