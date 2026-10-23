package store

import "context"

func (s *AgentRunStore) Ready(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *ConversationStore) ListRunToolCalls(ctx context.Context, userID, runID string) ([]ToolCall, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, run_id, conversation_id, message_id, task_id, tool_name, operation_hash,
       arguments_json, result_summary, status, error, started_at, completed_at, duration_ms,
       result_json, changed_paths_json, version_id, content_hash, created_at
FROM tool_calls WHERE user_id = ? AND run_id = ? ORDER BY created_at ASC, id ASC`, userID, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ToolCall
	for rows.Next() {
		item, err := scanToolCall(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
