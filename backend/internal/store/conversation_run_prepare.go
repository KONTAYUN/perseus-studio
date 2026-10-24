package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrConversationBusy = errors.New("会话已有任务运行，请停止或等待后再试")
	ErrRequestConflict  = errors.New("请求标识已用于其他内容")
)

type PrepareConversationRunInput struct {
	UserID, ConversationID, ModelProviderID string
	Content, AttachmentsJSON, ThinkingMode  string
	RegenerateMessageID, RequestID          string
	PendingMessageID                        string `json:",omitempty"`
	AttachmentPayloadsJSON                  string `json:",omitempty"`
	MessageMetaJSON                         string `json:",omitempty"`
}

type PreparedConversationRun struct {
	Run      AgentRun
	Message  Message
	Replayed bool
}

// PrepareConversationRun commits the input, run and initial events together.
// Request identities are scoped to a user; a replay never schedules work twice.
func (s *AgentRunStore) PrepareConversationRun(ctx context.Context, in PrepareConversationRunInput) (PreparedConversationRun, error) {
	if len(in.RequestID) > 128 {
		return PreparedConversationRun{}, fmt.Errorf("request id is too long")
	}
	identity := in
	identity.RequestID = ""
	raw, err := json.Marshal(identity)
	if err != nil {
		return PreparedConversationRun{}, err
	}
	sum := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(sum[:])
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PreparedConversationRun{}, err
	}
	defer tx.Rollback()
	var runID, messageID string
	if in.RequestID != "" {
		var existingHash string
		err = tx.QueryRowContext(ctx, `SELECT request_hash, run_id FROM conversation_run_requests WHERE user_id = ? AND request_id = ?`, in.UserID, in.RequestID).Scan(&existingHash, &runID)
		if err == nil {
			if existingHash != fingerprint {
				return PreparedConversationRun{}, ErrRequestConflict
			}
			if err := tx.Commit(); err != nil {
				return PreparedConversationRun{}, err
			}
			return s.preparedConversationRun(ctx, in.UserID, runID, true)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return PreparedConversationRun{}, err
		}
	}
	// Check ownership inside the same transaction as admission.
	var conversation string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM conversations WHERE user_id = ? AND id = ?`, in.UserID, in.ConversationID).Scan(&conversation); err != nil {
		return PreparedConversationRun{}, err
	}
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_runs WHERE user_id = ? AND conversation_id = ? AND status IN ('queued','running')`, in.UserID, in.ConversationID).Scan(&active); err != nil {
		return PreparedConversationRun{}, err
	}
	if active != 0 {
		return PreparedConversationRun{}, ErrConversationBusy
	}
	if in.PendingMessageID != "" {
		if err := consumePendingMessage(ctx, tx, in); err != nil {
			return PreparedConversationRun{}, err
		}
	}
	now := time.Now().UTC()
	if in.RegenerateMessageID != "" {
		messageID, err = regenerationUserMessage(ctx, tx, in.UserID, in.ConversationID, in.RegenerateMessageID)
		if err != nil {
			return PreparedConversationRun{}, err
		}
	} else {
		messageID = nextStoreID("msg", now)
		if in.AttachmentsJSON == "" {
			in.AttachmentsJSON = "[]"
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO messages (id,user_id,conversation_id,role,content,attachments_json,status,segment_index,meta_json,created_at,updated_at) VALUES (?,?,?,'user',?,?,'final',0,?,?,?)`, messageID, in.UserID, in.ConversationID, strings.TrimSpace(in.Content), in.AttachmentsJSON, in.MessageMetaJSON, now, now)
		if err != nil {
			return PreparedConversationRun{}, err
		}
		if err := saveMessageAttachmentPayloads(ctx, tx, in.UserID, messageID, in.AttachmentPayloadsJSON); err != nil {
			return PreparedConversationRun{}, err
		}
	}
	runID = nextStoreID("run", now)
	_, err = tx.ExecContext(ctx, `INSERT INTO agent_runs (id,user_id,conversation_id,user_message_id,model_provider_id,status,stage,created_at,updated_at) VALUES (?,?,?,?,?,'queued','queued',?,?)`, runID, in.UserID, in.ConversationID, messageID, in.ModelProviderID, now, now)
	if err != nil {
		return PreparedConversationRun{}, err
	}
	if in.RequestID != "" {
		_, err = tx.ExecContext(ctx, `INSERT INTO conversation_run_requests (user_id,request_id,request_hash,run_id) VALUES (?,?,?,?)`, in.UserID, in.RequestID, fingerprint, runID)
		if err != nil {
			return PreparedConversationRun{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE conversations SET updated_at = ?, selected_model_provider_id = ? WHERE user_id = ? AND id = ?`, now, in.ModelProviderID, in.UserID, in.ConversationID); err != nil {
		return PreparedConversationRun{}, err
	}
	seq := 1
	if in.RegenerateMessageID == "" {
		payload, err := json.Marshal(map[string]any{"user_message": map[string]any{"id": messageID, "conversation_id": in.ConversationID, "role": "user", "content": strings.TrimSpace(in.Content), "attachments": json.RawMessage(in.AttachmentsJSON), "created_at": now}})
		if err != nil {
			return PreparedConversationRun{}, err
		}
		if err := insertPreparedEvent(ctx, tx, in, runID, messageID, seq, "user_message_committed", string(payload), now); err != nil {
			return PreparedConversationRun{}, err
		}
		seq++
	}
	queuedPayload, _ := json.Marshal(map[string]string{"message": "run queued", "regenerate_message_id": in.RegenerateMessageID, "thinking_mode": in.ThinkingMode})
	if err := insertPreparedEvent(ctx, tx, in, runID, messageID, seq, "run_queued", string(queuedPayload), now); err != nil {
		return PreparedConversationRun{}, err
	}
	if err := tx.Commit(); err != nil {
		return PreparedConversationRun{}, err
	}
	return s.preparedConversationRun(ctx, in.UserID, runID, false)
}

func insertPreparedEvent(ctx context.Context, tx *sql.Tx, in PrepareConversationRunInput, runID, messageID string, seq int, kind, payload string, now time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO agent_events (id,run_id,user_id,conversation_id,message_id,task_id,type,level,seq,payload_json,created_at) VALUES (?,?,?,?,?,'',?,'info',?,?,?)`, nextStoreID("event", now), runID, in.UserID, in.ConversationID, messageID, kind, seq, payload, now)
	return err
}

func (s *AgentRunStore) preparedConversationRun(ctx context.Context, userID, runID string, replayed bool) (PreparedConversationRun, error) {
	// Once committed, HTTP disconnect must not prevent scheduling the run.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	run, err := s.GetRun(ctx, userID, runID)
	if err != nil {
		return PreparedConversationRun{}, err
	}
	message, err := NewConversationStore(s.db).GetMessage(ctx, userID, run.UserMessageID.String)
	return PreparedConversationRun{Run: run, Message: message, Replayed: replayed}, err
}

func regenerationUserMessage(ctx context.Context, tx *sql.Tx, userID, conversationID, selectedID string) (string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, role FROM messages WHERE user_id = ? AND conversation_id = ? ORDER BY created_at ASC, rowid ASC`, userID, conversationID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	lastUserID, selectedAfterLastUser := "", false
	for rows.Next() {
		var id, role string
		if err := rows.Scan(&id, &role); err != nil {
			return "", err
		}
		if role == "user" {
			lastUserID, selectedAfterLastUser = id, false
		}
		if id == selectedID && role == "assistant" && lastUserID != "" {
			selectedAfterLastUser = true
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if !selectedAfterLastUser {
		return "", fmt.Errorf("only the latest assistant tail can be regenerated")
	}
	return lastUserID, nil
}
