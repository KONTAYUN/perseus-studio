package store

import (
	"context"
	"database/sql"
	"errors"
)

const pendingSteerSelect = `SELECT id,run_id,user_id,conversation_id,message_id,task_id,type,level,seq,payload_json,created_at FROM agent_events WHERE user_id=? AND type='agent_steer_received' AND CASE WHEN json_valid(payload_json) THEN json_extract(payload_json,'$.pending_message_id') END=? LIMIT 1`

func (s *AgentRunStore) PendingSteerEvent(ctx context.Context, userID, id string) (AgentEvent, error) {
	return scanAgentEvent(s.db.QueryRowContext(ctx, pendingSteerSelect, userID, id))
}

func (s *AgentRunStore) appendPendingSteer(ctx context.Context, in AgentEventInput) (AgentEvent, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AgentEvent{}, err
	}
	defer tx.Rollback()
	existing, err := scanAgentEvent(tx.QueryRowContext(ctx, pendingSteerSelect, in.UserID, in.PendingMessageID))
	if err == nil {
		if existing.RunID != in.RunID {
			return AgentEvent{}, ErrRequestConflict
		}
		return existing, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return AgentEvent{}, err
	}
	p, err := scanPending(tx.QueryRowContext(ctx, `SELECT `+pendingColumns+` FROM pending_conversation_messages p WHERE p.user_id=? AND p.id=?`, in.UserID, in.PendingMessageID))
	if err != nil {
		return AgentEvent{}, err
	}
	payload, ok := in.Payload.(map[string]any)
	if !ok || in.Type != "agent_steer_received" || p.ConversationID != in.ConversationID || payload["content"] != p.Content || payload["client_steer_id"] != "pending:"+p.ID {
		return AgentEvent{}, ErrRequestConflict
	}
	var active bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_runs WHERE user_id=? AND id=? AND conversation_id=? AND status IN ('queued','running'))`, in.UserID, in.RunID, in.ConversationID).Scan(&active); err != nil {
		return AgentEvent{}, err
	}
	if !active {
		return AgentEvent{}, ErrConversationBusy
	}
	event, err := appendAgentEvent(ctx, tx, in)
	if err != nil {
		return AgentEvent{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM pending_conversation_messages WHERE user_id=? AND id=?`, in.UserID, p.ID); err != nil {
		return AgentEvent{}, err
	}
	return event, tx.Commit()
}
