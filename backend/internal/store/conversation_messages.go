package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

func (s *ConversationStore) ListMessages(ctx context.Context, userID, conversationID string) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, conversation_id, role, content, attachments_json, status, segment_index, meta_json, created_at, updated_at,
COALESCE((SELECT payload_json FROM message_attachment_payloads p WHERE p.message_id=messages.id AND p.user_id=messages.user_id),'')
FROM messages
WHERE conversation_id = ? AND user_id = ?
ORDER BY created_at ASC
`, conversationID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Message
	for rows.Next() {
		item, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *ConversationStore) AppendMessage(ctx context.Context, userID, conversationID, role, content, attachmentsJSON string) (Message, error) {
	return s.AppendMessageWithMeta(ctx, MessageInput{
		UserID:          userID,
		ConversationID:  conversationID,
		Role:            role,
		Content:         content,
		AttachmentsJSON: attachmentsJSON,
		Status:          "final",
	})
}

func (s *ConversationStore) AppendMessageWithMeta(ctx context.Context, input MessageInput) (Message, error) {
	if input.AttachmentsJSON == "" {
		input.AttachmentsJSON = "[]"
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "final"
	}
	metaJSON := strings.TrimSpace(input.MetaJSON)
	operationNow := time.Now().UTC()
	createdAt := input.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = operationNow
	}
	id := nextStoreID("msg", createdAt)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Message{}, err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
INSERT INTO messages (id, user_id, conversation_id, role, content, attachments_json, status, segment_index, meta_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, id, input.UserID, input.ConversationID, input.Role, input.Content, input.AttachmentsJSON, status, input.SegmentIndex, metaJSON, createdAt, operationNow)
	if err != nil {
		return Message{}, err
	}
	_, err = tx.ExecContext(ctx, `
UPDATE conversations
SET updated_at = ?
WHERE id = ? AND user_id = ?
`, operationNow, input.ConversationID, input.UserID)
	if err != nil {
		return Message{}, err
	}
	if err := tx.Commit(); err != nil {
		return Message{}, err
	}
	return s.GetMessage(ctx, input.UserID, id)
}

func (s *ConversationStore) GetMessage(ctx context.Context, userID, id string) (Message, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, user_id, conversation_id, role, content, attachments_json, status, segment_index, meta_json, created_at, updated_at,
COALESCE((SELECT payload_json FROM message_attachment_payloads p WHERE p.message_id=messages.id AND p.user_id=messages.user_id),'')
FROM messages
WHERE id = ? AND user_id = ?
`, id, userID)
	return scanMessage(row)
}

func (s *ConversationStore) CreateMessageDraft(ctx context.Context, input MessageDraftInput) (Message, error) {
	if input.AttachmentsJSON == "" {
		input.AttachmentsJSON = "[]"
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "streaming"
	}
	now := time.Now().UTC()
	id := nextStoreID("msg", now)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Message{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
INSERT INTO messages (id, user_id, conversation_id, role, content, attachments_json, status, segment_index, meta_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, id, input.UserID, input.ConversationID, input.Role, input.Content, input.AttachmentsJSON, status, input.SegmentIndex, input.MetaJSON, now, now)
	if err != nil {
		return Message{}, err
	}
	_, err = tx.ExecContext(ctx, `
UPDATE conversations
SET updated_at = ?
WHERE id = ? AND user_id = ?
`, now, input.ConversationID, input.UserID)
	if err != nil {
		return Message{}, err
	}
	if err := tx.Commit(); err != nil {
		return Message{}, err
	}
	return s.GetMessage(ctx, input.UserID, id)
}

func (s *ConversationStore) AppendMessageContent(ctx context.Context, userID, messageID, delta string) (Message, error) {
	_, err := s.db.ExecContext(ctx, `
UPDATE messages
SET content = content || ?, updated_at = ?
WHERE id = ? AND user_id = ?
`, delta, time.Now().UTC(), messageID, userID)
	if err != nil {
		return Message{}, err
	}
	return s.GetMessage(ctx, userID, messageID)
}

func (s *ConversationStore) PatchMessageMeta(ctx context.Context, userID, messageID, metaJSON, status string) (Message, error) {
	_, err := s.db.ExecContext(ctx, `
UPDATE messages
SET meta_json = COALESCE(NULLIF(?, ''), meta_json),
    status = COALESCE(NULLIF(?, ''), status),
    updated_at = ?
WHERE id = ? AND user_id = ?
`, metaJSON, status, time.Now().UTC(), messageID, userID)
	if err != nil {
		return Message{}, err
	}
	return s.GetMessage(ctx, userID, messageID)
}

func (s *ConversationStore) FinalizeMessage(ctx context.Context, userID, messageID, finalContent, metaJSON string) (Message, error) {
	_, err := s.db.ExecContext(ctx, `
UPDATE messages
SET content = ?,
    meta_json = COALESCE(NULLIF(?, ''), meta_json),
    status = 'final',
    updated_at = ?
WHERE id = ? AND user_id = ?
`, finalContent, metaJSON, time.Now().UTC(), messageID, userID)
	if err != nil {
		return Message{}, err
	}
	return s.GetMessage(ctx, userID, messageID)
}

func (s *ConversationStore) UpdateMessageAttachments(ctx context.Context, userID, messageID, attachmentsJSON string) (Message, error) {
	if attachmentsJSON == "" {
		attachmentsJSON = "[]"
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE messages
SET attachments_json = ?,
    updated_at = ?
WHERE id = ? AND user_id = ?
`, attachmentsJSON, time.Now().UTC(), messageID, userID)
	if err != nil {
		return Message{}, err
	}
	return s.GetMessage(ctx, userID, messageID)
}

func (s *ConversationStore) DeleteMessage(ctx context.Context, userID, conversationID, messageID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
DELETE FROM messages
WHERE id = ? AND conversation_id = ? AND user_id = ?
`, messageID, conversationID, userID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	_, err = tx.ExecContext(ctx, `
UPDATE conversations
SET updated_at = ?
WHERE id = ? AND user_id = ?
`, time.Now().UTC(), conversationID, userID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *ConversationStore) DeleteMessagesAfter(ctx context.Context, userID, conversationID, afterMessageID string) error {
	messages, err := s.ListMessages(ctx, userID, conversationID)
	if err != nil {
		return err
	}
	cutIndex := -1
	for index, item := range messages {
		if item.ID == afterMessageID {
			cutIndex = index
			break
		}
	}
	if cutIndex == -1 {
		return sql.ErrNoRows
	}
	if cutIndex == len(messages)-1 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, item := range messages[cutIndex+1:] {
		if _, err := tx.ExecContext(ctx, `
DELETE FROM messages
WHERE id = ? AND conversation_id = ? AND user_id = ?
`, item.ID, conversationID, userID); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `
UPDATE conversations
SET updated_at = ?
WHERE id = ? AND user_id = ?
`, time.Now().UTC(), conversationID, userID)
	if err != nil {
		return err
	}
	return tx.Commit()
}
