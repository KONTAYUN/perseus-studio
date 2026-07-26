package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const (
	ChatAttachmentStatusStaged   = "staged"
	ChatAttachmentStatusAttached = "attached"
	MaxChatAttachmentsPerMessage = 8
	MaxChatAttachmentTotalBytes  = 20 << 20
)

type ChatAttachment struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id,omitempty"`
	OriginalName string    `json:"name"`
	MIMEType     string    `json:"mime_type"`
	SizeBytes    int64     `json:"size"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	SHA256       string    `json:"sha256,omitempty"`
	StorageKey   string    `json:"-"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Ordinal      int       `json:"-"`
	Included     bool      `json:"included_in_context"`
	MessageID    string    `json:"-"`
}

type CreateChatAttachmentInput struct {
	ID           string
	UserID       string
	OriginalName string
	MIMEType     string
	SizeBytes    int64
	Width        int
	Height       int
	SHA256       string
	StorageKey   string
}

func (s *ConversationStore) migrateChatAttachments(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS chat_attachments (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  original_name TEXT NOT NULL,
  mime_type TEXT NOT NULL,
  size_bytes INTEGER NOT NULL,
  width INTEGER NOT NULL DEFAULT 0,
  height INTEGER NOT NULL DEFAULT 0,
  sha256 TEXT NOT NULL,
  storage_key TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'staged',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS message_attachments (
  message_id TEXT NOT NULL,
  attachment_id TEXT NOT NULL,
  ordinal INTEGER NOT NULL DEFAULT 0,
  included_in_context INTEGER NOT NULL DEFAULT 1,
  PRIMARY KEY(message_id, attachment_id),
  FOREIGN KEY(message_id) REFERENCES messages(id) ON DELETE CASCADE,
  FOREIGN KEY(attachment_id) REFERENCES chat_attachments(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_chat_attachments_user_status_created
  ON chat_attachments(user_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_message_attachments_attachment
  ON message_attachments(attachment_id);
`)
	return err
}

func (s *ConversationStore) CreateChatAttachment(ctx context.Context, input CreateChatAttachmentInput) (ChatAttachment, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(input.ID) == "" || strings.TrimSpace(input.UserID) == "" {
		return ChatAttachment{}, fmt.Errorf("attachment id and user id are required")
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO chat_attachments
  (id, user_id, original_name, mime_type, size_bytes, width, height, sha256, storage_key, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, input.ID, input.UserID, input.OriginalName, input.MIMEType, input.SizeBytes, input.Width, input.Height,
		input.SHA256, input.StorageKey, ChatAttachmentStatusStaged, now, now)
	if err != nil {
		return ChatAttachment{}, err
	}
	return s.GetChatAttachment(ctx, input.ID)
}

func (s *ConversationStore) GetChatAttachment(ctx context.Context, id string) (ChatAttachment, error) {
	return scanChatAttachment(s.db.QueryRowContext(ctx, `
SELECT id, user_id, original_name, mime_type, size_bytes, width, height, sha256, storage_key, status, created_at, updated_at
FROM chat_attachments
WHERE id = ?
`, id))
}

func (s *ConversationStore) ListMessageChatAttachments(ctx context.Context, userID, messageID string) ([]ChatAttachment, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT a.id, a.user_id, a.original_name, a.mime_type, a.size_bytes, a.width, a.height,
       a.sha256, a.storage_key, a.status, a.created_at, a.updated_at,
       ma.ordinal, ma.included_in_context
FROM message_attachments ma
JOIN chat_attachments a ON a.id = ma.attachment_id
JOIN messages m ON m.id = ma.message_id
WHERE ma.message_id = ? AND m.user_id = ? AND a.user_id = ?
ORDER BY ma.ordinal ASC
`, messageID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ChatAttachment
	for rows.Next() {
		var item ChatAttachment
		if err := rows.Scan(&item.ID, &item.UserID, &item.OriginalName, &item.MIMEType, &item.SizeBytes,
			&item.Width, &item.Height, &item.SHA256, &item.StorageKey, &item.Status, &item.CreatedAt,
			&item.UpdatedAt, &item.Ordinal, &item.Included); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *ConversationStore) ListConversationChatAttachments(ctx context.Context, userID, conversationID string) ([]ChatAttachment, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT DISTINCT a.id, a.user_id, a.original_name, a.mime_type, a.size_bytes, a.width, a.height,
       a.sha256, a.storage_key, a.status, a.created_at, a.updated_at
FROM chat_attachments a
JOIN message_attachments ma ON ma.attachment_id = a.id
JOIN messages m ON m.id = ma.message_id
WHERE m.conversation_id = ? AND m.user_id = ? AND a.user_id = ?
`, conversationID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ChatAttachment
	for rows.Next() {
		item, err := scanChatAttachment(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *ConversationStore) ListConversationMessageChatAttachments(ctx context.Context, userID, conversationID string) ([]ChatAttachment, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT a.id, a.user_id, a.original_name, a.mime_type, a.size_bytes, a.width, a.height,
       a.sha256, a.storage_key, a.status, a.created_at, a.updated_at,
       ma.ordinal, ma.included_in_context, m.id
FROM message_attachments ma
JOIN chat_attachments a ON a.id = ma.attachment_id
JOIN messages m ON m.id = ma.message_id
WHERE m.conversation_id = ? AND m.user_id = ? AND a.user_id = ?
ORDER BY m.created_at ASC, ma.ordinal ASC
`, conversationID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ChatAttachment
	for rows.Next() {
		var item ChatAttachment
		if err := rows.Scan(&item.ID, &item.UserID, &item.OriginalName, &item.MIMEType, &item.SizeBytes,
			&item.Width, &item.Height, &item.SHA256, &item.StorageKey, &item.Status, &item.CreatedAt,
			&item.UpdatedAt, &item.Ordinal, &item.Included, &item.MessageID); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *ConversationStore) ListExpiredStagedChatAttachments(ctx context.Context, before time.Time) ([]ChatAttachment, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, original_name, mime_type, size_bytes, width, height, sha256, storage_key, status, created_at, updated_at
FROM chat_attachments
WHERE status = ? AND created_at < ?
ORDER BY created_at ASC
`, ChatAttachmentStatusStaged, before.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ChatAttachment
	for rows.Next() {
		item, err := scanChatAttachment(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *ConversationStore) DeleteStagedChatAttachment(ctx context.Context, userID, id string) (ChatAttachment, error) {
	item, err := s.GetChatAttachment(ctx, id)
	if err != nil || item.UserID != userID || item.Status != ChatAttachmentStatusStaged {
		return ChatAttachment{}, sql.ErrNoRows
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM chat_attachments WHERE id = ? AND user_id = ? AND status = ?`,
		id, userID, ChatAttachmentStatusStaged)
	if err != nil {
		return ChatAttachment{}, err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ChatAttachment{}, sql.ErrNoRows
	}
	return item, nil
}

func (s *ConversationStore) DeleteExpiredStagedChatAttachment(ctx context.Context, id string, before time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM chat_attachments WHERE id = ? AND status = ? AND created_at < ?`,
		id, ChatAttachmentStatusStaged, before.UTC())
	return err
}

func (s *ConversationStore) AppendMessageWithAttachmentIDs(ctx context.Context, input MessageInput, attachmentIDs []string) (Message, error) {
	if len(attachmentIDs) == 0 {
		return s.AppendMessageWithMeta(ctx, input)
	}
	if len(attachmentIDs) > MaxChatAttachmentsPerMessage {
		return Message{}, fmt.Errorf("a message can contain at most %d images", MaxChatAttachmentsPerMessage)
	}
	if input.AttachmentsJSON == "" {
		input.AttachmentsJSON = "[]"
	}
	status := firstNonEmptyString(input.Status, "final")
	operationNow := time.Now().UTC()
	createdAt := input.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = operationNow
	}
	messageID := nextStoreID("msg", createdAt)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Message{}, err
	}
	defer tx.Rollback()

	totalBytes := int64(0)
	seen := map[string]bool{}
	for _, id := range attachmentIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return Message{}, fmt.Errorf("attachment ids must be non-empty and unique")
		}
		seen[id] = true
		var owner, attachmentStatus string
		var size int64
		if err := tx.QueryRowContext(ctx, `SELECT user_id, status, size_bytes FROM chat_attachments WHERE id = ?`, id).
			Scan(&owner, &attachmentStatus, &size); err != nil {
			return Message{}, fmt.Errorf("attachment is unavailable: %w", err)
		}
		if owner != input.UserID || attachmentStatus != ChatAttachmentStatusStaged {
			return Message{}, fmt.Errorf("attachment is unavailable")
		}
		totalBytes += size
	}
	if totalBytes > MaxChatAttachmentTotalBytes {
		return Message{}, fmt.Errorf("message images exceed the %d MB total limit", MaxChatAttachmentTotalBytes>>20)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO messages (id, user_id, conversation_id, role, content, attachments_json, status, segment_index, meta_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, messageID, input.UserID, input.ConversationID, input.Role, input.Content, input.AttachmentsJSON,
		status, input.SegmentIndex, strings.TrimSpace(input.MetaJSON), createdAt, operationNow); err != nil {
		return Message{}, err
	}
	for ordinal, id := range attachmentIDs {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO message_attachments (message_id, attachment_id, ordinal, included_in_context)
VALUES (?, ?, ?, 1)
`, messageID, id, ordinal); err != nil {
			return Message{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE chat_attachments SET status = ?, updated_at = ? WHERE id = ?`,
			ChatAttachmentStatusAttached, operationNow, id); err != nil {
			return Message{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE conversations SET updated_at = ? WHERE id = ? AND user_id = ?`,
		operationNow, input.ConversationID, input.UserID); err != nil {
		return Message{}, err
	}
	if err := tx.Commit(); err != nil {
		return Message{}, err
	}
	return s.GetMessage(ctx, input.UserID, messageID)
}

func (s *ConversationStore) DeleteConversationWithAttachments(ctx context.Context, userID, conversationID string) ([]ChatAttachment, error) {
	candidates, err := s.ListConversationChatAttachments(ctx, userID, conversationID)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM message_attachments WHERE message_id IN (SELECT id FROM messages WHERE conversation_id = ? AND user_id = ?)`, conversationID, userID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM conversations WHERE id = ? AND user_id = ?`, conversationID, userID); err != nil {
		return nil, err
	}
	var deleted []ChatAttachment
	for _, item := range candidates {
		var links int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM message_attachments WHERE attachment_id = ?`, item.ID).Scan(&links); err != nil {
			return nil, err
		}
		if links == 0 {
			if _, err := tx.ExecContext(ctx, `DELETE FROM chat_attachments WHERE id = ?`, item.ID); err != nil {
				return nil, err
			}
			deleted = append(deleted, item)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return deleted, nil
}

func listMessageChatAttachmentsTx(ctx context.Context, tx *sql.Tx, userID, messageID string) ([]ChatAttachment, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT a.id, a.user_id, a.original_name, a.mime_type, a.size_bytes, a.width, a.height,
       a.sha256, a.storage_key, a.status, a.created_at, a.updated_at
FROM message_attachments ma
JOIN chat_attachments a ON a.id = ma.attachment_id
JOIN messages m ON m.id = ma.message_id
WHERE ma.message_id = ? AND m.user_id = ? AND a.user_id = ?
`, messageID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ChatAttachment
	for rows.Next() {
		item, err := scanChatAttachment(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func deleteUnlinkedChatAttachmentsTx(ctx context.Context, tx *sql.Tx, candidates []ChatAttachment) ([]ChatAttachment, error) {
	var deleted []ChatAttachment
	seen := map[string]bool{}
	for _, item := range candidates {
		if seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		var links int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM message_attachments WHERE attachment_id = ?`, item.ID).Scan(&links); err != nil {
			return nil, err
		}
		if links != 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM chat_attachments WHERE id = ?`, item.ID); err != nil {
			return nil, err
		}
		deleted = append(deleted, item)
	}
	return deleted, nil
}

func scanChatAttachment(s scanner) (ChatAttachment, error) {
	var item ChatAttachment
	err := s.Scan(&item.ID, &item.UserID, &item.OriginalName, &item.MIMEType, &item.SizeBytes,
		&item.Width, &item.Height, &item.SHA256, &item.StorageKey, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}
