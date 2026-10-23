package store

import (
	"context"
	"database/sql"
	"errors"
)

// Serialize history removal with run/task admission and recovery. A handler-only
// check leaves a gap in which a new worker can acquire history about to disappear.
func (s *ConversationStore) DeleteConversation(ctx context.Context, userID, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var owned string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM conversations WHERE id = ? AND user_id = ?`, id, userID).Scan(&owned); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sql.ErrNoRows
		}
		return err
	}
	var active bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_runs WHERE user_id = ? AND conversation_id = ? AND status IN ('queued','running'))`, userID, id).Scan(&active); err != nil {
		return err
	}
	if active {
		return ErrConversationBusy
	}
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM tasks WHERE user_id = ? AND type = 'ai_agent' AND status IN ('pending','running') AND CASE WHEN json_valid(payload_json) THEN json_extract(payload_json, '$.conversation_id') END = ?)`, userID, id).Scan(&active); err != nil {
		return err
	}
	if active {
		return ErrConversationBusy
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM conversations WHERE id = ? AND user_id = ?`, id, userID); err != nil {
		return err
	}
	return tx.Commit()
}
