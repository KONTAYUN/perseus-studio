package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Truncate only after the queue has admitted the run. An admission failure
// therefore keeps the previous assistant response intact.
func (s *AgentRunStore) PrepareRegenerationHistory(ctx context.Context, userID, conversationID, selectedID, messageID, runID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status AgentRunStatus
	if err := tx.QueryRowContext(ctx, `SELECT status FROM agent_runs WHERE id = ? AND user_id = ? AND conversation_id = ?`, runID, userID, conversationID).Scan(&status); err != nil {
		return err
	}
	if status != AgentRunQueued {
		return fmt.Errorf("regeneration is no longer queued")
	}
	lastUser, err := regenerationUserMessage(ctx, tx, userID, conversationID, selectedID)
	if err != nil {
		return err
	}
	if lastUser != messageID {
		return fmt.Errorf("conversation changed before regeneration")
	}
	var cutoff int64
	if err := tx.QueryRowContext(ctx, `SELECT rowid FROM messages WHERE user_id = ? AND conversation_id = ? AND id = ?`, userID, conversationID, messageID).Scan(&cutoff); err != nil {
		return err
	}
	// Ordering is created_at + insertion order. Do not compare random ID suffixes.
	var createdAt time.Time
	if err := tx.QueryRowContext(ctx, `SELECT created_at FROM messages WHERE rowid = ?`, cutoff).Scan(&createdAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE user_id = ? AND conversation_id = ? AND (created_at > ? OR (created_at = ? AND rowid > ?))`, userID, conversationID, createdAt, createdAt, cutoff); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE conversations SET summary_text = '', summary_until_message_id = '', updated_at = ? WHERE user_id = ? AND id = ?`, time.Now().UTC(), userID, conversationID); err != nil {
		return err
	}
	var seq int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0)+1 FROM agent_events WHERE run_id = ?`, runID).Scan(&seq); err != nil {
		return err
	}
	if err := insertPreparedEvent(ctx, tx, PrepareConversationRunInput{UserID: userID, ConversationID: conversationID}, runID, messageID, seq, "regeneration_prepared", `{}`, time.Now().UTC()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *AgentRunStore) PendingRegeneration(ctx context.Context, userID, runID string) (string, error) {
	var payload string
	err := s.db.QueryRowContext(ctx, `SELECT payload_json FROM agent_events WHERE user_id = ? AND run_id = ? AND type = 'run_queued' AND NOT EXISTS (SELECT 1 FROM agent_events WHERE user_id = ? AND run_id = ? AND type = 'regeneration_prepared') ORDER BY seq ASC LIMIT 1`, userID, runID, userID, runID).Scan(&payload)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var meta struct {
		SelectedID string `json:"regenerate_message_id"`
	}
	if err := json.Unmarshal([]byte(payload), &meta); err != nil {
		return "", err
	}
	return meta.SelectedID, nil
}

func (s *AgentRunStore) RunByRequestID(ctx context.Context, userID, requestID string) (AgentRun, error) {
	var id string
	if requestID == "" {
		return AgentRun{}, sql.ErrNoRows
	}
	if err := s.db.QueryRowContext(ctx, `SELECT run_id FROM conversation_run_requests WHERE user_id = ? AND request_id = ?`, userID, requestID).Scan(&id); err != nil {
		return AgentRun{}, err
	}
	return s.GetRun(ctx, userID, id)
}
