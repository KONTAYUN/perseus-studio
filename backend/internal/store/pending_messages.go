package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"webide-backend/internal/document"
)

// One replaceable follow-up per conversation, matching the composer contract.
// It is consumed in the same transaction that commits its message and run.
type PendingMessage struct {
	DocumentAction  *document.Action `json:"documentAction,omitempty"`
	ID              string           `json:"id"`
	ConversationID  string           `json:"conversationId"`
	Content         string           `json:"content"`
	ModelProviderID string           `json:"modelProviderId"`
	ThinkingMode    string           `json:"thinkingMode"`
	CreatedAt       time.Time        `json:"createdAt"`
	LatestStatus    string           `json:"latestStatus"`
}

const pendingColumns = `p.id,p.conversation_id,p.content,p.model_provider_id,p.thinking_mode,p.created_at,p.document_action_json,
 COALESCE((SELECT status FROM agent_runs WHERE user_id=p.user_id AND conversation_id=p.conversation_id ORDER BY created_at DESC,id DESC LIMIT 1),'')`

func scanPending(row interface{ Scan(...any) error }) (PendingMessage, error) {
	var p PendingMessage
	var raw string
	err := row.Scan(&p.ID, &p.ConversationID, &p.Content, &p.ModelProviderID, &p.ThinkingMode, &p.CreatedAt, &raw, &p.LatestStatus)
	if err == nil {
		p.DocumentAction, err = document.ActionFromMeta(raw)
	}
	return p, err
}

func (s *AgentRunStore) ListPendingMessages(ctx context.Context, userID string) ([]PendingMessage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+pendingColumns+` FROM pending_conversation_messages p WHERE p.user_id=? ORDER BY p.created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []PendingMessage{}
	for rows.Next() {
		p, err := scanPending(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, rows.Err()
}

func (s *AgentRunStore) GetPendingMessage(ctx context.Context, userID, id string) (PendingMessage, error) {
	return scanPending(s.db.QueryRowContext(ctx, `SELECT `+pendingColumns+` FROM pending_conversation_messages p WHERE p.user_id=? AND p.id=?`, userID, id))
}

func (s *AgentRunStore) SavePendingMessage(ctx context.Context, userID string, p PendingMessage) (PendingMessage, error) {
	p.Content = strings.TrimSpace(p.Content)
	if err := p.DocumentAction.Validate(); err != nil {
		return PendingMessage{}, err
	}
	if p.ID == "" || len(p.ID) > 100 || p.ConversationID == "" || p.ModelProviderID == "" || p.Content == "" || len(p.Content) > 1<<20 || len(p.ThinkingMode) > 128 {
		return PendingMessage{}, fmt.Errorf("invalid pending message")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PendingMessage{}, err
	}
	defer tx.Rollback()
	var owned string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM conversations WHERE user_id=? AND id=?`, userID, p.ConversationID).Scan(&owned); err != nil {
		return PendingMessage{}, err
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM conversation_run_requests WHERE user_id=? AND request_id=?) OR EXISTS(SELECT 1 FROM agent_events WHERE user_id=? AND type='agent_steer_received' AND CASE WHEN json_valid(payload_json) THEN json_extract(payload_json,'$.pending_message_id') END=?)`, userID, "pending:"+p.ID, userID, p.ID).Scan(&exists); err != nil {
		return PendingMessage{}, err
	}
	if exists {
		return PendingMessage{}, ErrRequestConflict
	}
	existing, err := scanPending(tx.QueryRowContext(ctx, `SELECT `+pendingColumns+` FROM pending_conversation_messages p WHERE p.user_id=? AND p.id=?`, userID, p.ID))
	if err == nil {
		if existing.Content != p.Content || existing.ConversationID != p.ConversationID || existing.ModelProviderID != p.ModelProviderID || existing.ThinkingMode != p.ThinkingMode || existing.DocumentAction.MetaJSON() != p.DocumentAction.MetaJSON() {
			return PendingMessage{}, ErrRequestConflict
		}
		return existing, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return PendingMessage{}, err
	}
	if p.DocumentAction != nil {
		var occupied bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pending_conversation_messages WHERE user_id=? AND conversation_id=?)`, userID, p.ConversationID).Scan(&occupied); err != nil {
			return PendingMessage{}, err
		}
		if occupied {
			return PendingMessage{}, fmt.Errorf("%w: 当前对话已有挂起消息，格式修复不能替换它", ErrRequestConflict)
		}
	}
	p.CreatedAt = time.Now().UTC()
	_, err = tx.ExecContext(ctx, `INSERT INTO pending_conversation_messages (id,user_id,conversation_id,content,model_provider_id,thinking_mode,created_at,document_action_json) VALUES (?,?,?,?,?,?,?,?) ON CONFLICT(user_id,conversation_id) DO UPDATE SET id=excluded.id,content=excluded.content,model_provider_id=excluded.model_provider_id,thinking_mode=excluded.thinking_mode,created_at=excluded.created_at,document_action_json=excluded.document_action_json`, p.ID, userID, p.ConversationID, p.Content, p.ModelProviderID, p.ThinkingMode, p.CreatedAt, p.DocumentAction.MetaJSON())
	if err != nil {
		return PendingMessage{}, err
	}
	p, err = scanPending(tx.QueryRowContext(ctx, `SELECT `+pendingColumns+` FROM pending_conversation_messages p WHERE p.user_id=? AND p.id=?`, userID, p.ID))
	if err != nil {
		return PendingMessage{}, err
	}
	if err := tx.Commit(); err != nil {
		return PendingMessage{}, err
	}
	return p, nil
}

var ErrPendingAlreadySent = errors.New("这条消息已经发送，请在对应任务中停止；不能再删除排队消息")

func (s *AgentRunStore) DeletePendingMessage(ctx context.Context, userID, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM pending_conversation_messages WHERE user_id=? AND id=?`, userID, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil || count > 0 {
		return err
	}
	var sent bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM conversation_run_requests WHERE user_id=? AND request_id=?) OR EXISTS(SELECT 1 FROM agent_events WHERE user_id=? AND type='agent_steer_received' AND CASE WHEN json_valid(payload_json) THEN json_extract(payload_json,'$.pending_message_id') END=?)`, userID, "pending:"+id, userID, id).Scan(&sent); err != nil {
		return err
	}
	if sent {
		return ErrPendingAlreadySent
	}
	return nil
}

func consumePendingMessage(ctx context.Context, tx *sql.Tx, in PrepareConversationRunInput) error {
	p, err := scanPending(tx.QueryRowContext(ctx, `SELECT `+pendingColumns+` FROM pending_conversation_messages p WHERE p.user_id=? AND p.id=?`, in.UserID, in.PendingMessageID))
	if err != nil {
		return err
	}
	if in.RequestID != "pending:"+p.ID || in.ConversationID != p.ConversationID || in.Content != p.Content || in.ModelProviderID != p.ModelProviderID || in.ThinkingMode != p.ThinkingMode || in.MessageMetaJSON != p.DocumentAction.MetaJSON() {
		return ErrRequestConflict
	}
	if p.LatestStatus != "" && p.LatestStatus != string(AgentRunSucceeded) {
		return ErrConversationBusy
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM pending_conversation_messages WHERE user_id=? AND id=?`, in.UserID, p.ID)
	return err
}

func (s *AgentRunStore) PendingRunThinkingMode(ctx context.Context, userID, runID string) (string, error) {
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT payload_json FROM agent_events WHERE user_id=? AND run_id=? AND type='run_queued' ORDER BY seq LIMIT 1`, userID, runID).Scan(&raw); err != nil {
		return "", err
	}
	var payload struct {
		ThinkingMode string `json:"thinking_mode"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return "", err
	}
	return payload.ThinkingMode, nil
}
