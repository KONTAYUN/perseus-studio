package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Conversation struct {
	ID                      string    `json:"id"`
	UserID                  string    `json:"user_id"`
	Title                   string    `json:"title"`
	SelectedModelProviderID string    `json:"selected_model_provider_id"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

type ConversationSummary struct {
	UserID         string    `json:"user_id"`
	ConversationID string    `json:"conversation_id"`
	Summary        string    `json:"summary"`
	UntilMessageID string    `json:"until_message_id"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Message struct {
	ID              string          `json:"id"`
	UserID          string          `json:"user_id"`
	ConversationID  string          `json:"conversation_id"`
	Role            string          `json:"role"`
	Content         string          `json:"content"`
	AttachmentsJSON string          `json:"-"`
	Status          string          `json:"status,omitempty"`
	SegmentIndex    int             `json:"segment_index,omitempty"`
	MetaJSON        string          `json:"-"`
	UpdatedAt       sql.NullTime    `json:"-"`
	CreatedAt       time.Time       `json:"created_at"`
	Attachments     json.RawMessage `json:"attachments,omitempty"`
	Meta            json.RawMessage `json:"meta,omitempty"`
}

type CreateConversationInput struct {
	UserID                  string
	Title                   string
	SelectedModelProviderID string
}

type MessageDraftInput struct {
	UserID          string
	ConversationID  string
	Role            string
	Content         string
	AttachmentsJSON string
	Status          string
	SegmentIndex    int
	MetaJSON        string
}

type ToolCall struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	ConversationID string    `json:"conversation_id"`
	MessageID      string    `json:"message_id,omitempty"`
	TaskID         string    `json:"task_id,omitempty"`
	ToolName       string    `json:"tool_name"`
	ArgumentsJSON  string    `json:"arguments_json"`
	ResultSummary  string    `json:"result_summary"`
	Status         string    `json:"status"`
	Error          string    `json:"error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type ToolCallInput struct {
	ID             string
	UserID         string
	ConversationID string
	MessageID      string
	TaskID         string
	ToolName       string
	ArgumentsJSON  string
	ResultSummary  string
	Status         string
	Error          string
}

type ConversationStore struct {
	db *sql.DB
}

func NewConversationStore(db *sql.DB) *ConversationStore {
	return &ConversationStore{db: db}
}

func (s *ConversationStore) Migrate(ctx context.Context) error {
	tables := `
CREATE TABLE IF NOT EXISTS conversations (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL,
  selected_model_provider_id TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS messages (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL DEFAULT '',
  conversation_id TEXT NOT NULL,
  role TEXT NOT NULL,
  content TEXT NOT NULL,
  attachments_json TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'final',
  segment_index INTEGER NOT NULL DEFAULT 0,
  meta_json TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  updated_at DATETIME,
  FOREIGN KEY(conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS tool_calls (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL DEFAULT '',
  conversation_id TEXT,
  message_id TEXT,
  task_id TEXT,
  tool_name TEXT NOT NULL,
  arguments_json TEXT NOT NULL,
  result_summary TEXT NOT NULL,
  status TEXT NOT NULL,
  error TEXT NOT NULL,
  created_at DATETIME NOT NULL
);
`
	if _, err := s.db.ExecContext(ctx, tables); err != nil {
		return err
	}
	for _, tbl := range []string{"conversations", "messages", "tool_calls"} {
		if err := AddColumnIfNotExists(ctx, s.db, tbl, "user_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
	}
	if err := AddColumnIfNotExists(ctx, s.db, "messages", "status", "TEXT NOT NULL DEFAULT 'final'"); err != nil {
		return err
	}
	if err := AddColumnIfNotExists(ctx, s.db, "messages", "segment_index", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := AddColumnIfNotExists(ctx, s.db, "messages", "meta_json", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := AddColumnIfNotExists(ctx, s.db, "messages", "updated_at", "DATETIME"); err != nil {
		return err
	}
	if err := AddColumnIfNotExists(ctx, s.db, "conversations", "summary_text", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := AddColumnIfNotExists(ctx, s.db, "conversations", "summary_until_message_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := AddColumnIfNotExists(ctx, s.db, "conversations", "summary_updated_at", "DATETIME"); err != nil {
		return err
	}
	indexes := `
CREATE INDEX IF NOT EXISTS idx_conversations_updated_at ON conversations(updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_conversations_user_id ON conversations(user_id);
CREATE INDEX IF NOT EXISTS idx_messages_conversation_created_at ON messages(conversation_id, created_at ASC);
CREATE INDEX IF NOT EXISTS idx_messages_conversation_segment ON messages(conversation_id, segment_index, created_at ASC);
CREATE INDEX IF NOT EXISTS idx_messages_user_id ON messages(user_id);
CREATE INDEX IF NOT EXISTS idx_tool_calls_conversation_created_at ON tool_calls(conversation_id, created_at ASC);
CREATE INDEX IF NOT EXISTS idx_tool_calls_user_id ON tool_calls(user_id);
`
	_, err := s.db.ExecContext(ctx, indexes)
	return err
}

func (s *ConversationStore) ListConversations(ctx context.Context, userID string) ([]Conversation, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, title, selected_model_provider_id, created_at, updated_at
FROM conversations
WHERE user_id = ?
ORDER BY updated_at DESC
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Conversation
	for rows.Next() {
		item, err := scanConversation(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *ConversationStore) CreateConversation(ctx context.Context, input CreateConversationInput) (Conversation, error) {
	now := time.Now().UTC()
	id := nextStoreID("conv", now)
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = "新对话"
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO conversations (id, user_id, title, selected_model_provider_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
`, id, input.UserID, title, input.SelectedModelProviderID, now, now)
	if err != nil {
		return Conversation{}, err
	}
	return s.GetConversation(ctx, input.UserID, id)
}

func (s *ConversationStore) GetConversation(ctx context.Context, userID, id string) (Conversation, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, user_id, title, selected_model_provider_id, created_at, updated_at
FROM conversations
WHERE id = ? AND user_id = ?
`, id, userID)
	return scanConversation(row)
}

func (s *ConversationStore) GetConversationSummary(ctx context.Context, userID, conversationID string) (ConversationSummary, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT user_id, id, summary_text, summary_until_message_id, updated_at
FROM conversations
WHERE id = ? AND user_id = ?
`, conversationID, userID)
	var item ConversationSummary
	if err := row.Scan(&item.UserID, &item.ConversationID, &item.Summary, &item.UntilMessageID, &item.UpdatedAt); err != nil {
		return ConversationSummary{}, err
	}
	return item, nil
}

func (s *ConversationStore) UpdateConversationSummary(ctx context.Context, userID, conversationID, summary, untilMessageID string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE conversations
SET summary_text = ?, summary_until_message_id = ?, summary_updated_at = ?
WHERE id = ? AND user_id = ?
`, strings.TrimSpace(summary), untilMessageID, time.Now().UTC(), conversationID, userID)
	return err
}

func (s *ConversationStore) DeleteConversation(ctx context.Context, userID, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM conversations WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

func (s *ConversationStore) BranchConversation(ctx context.Context, userID, conversationID, messageID string) (Conversation, []Message, error) {
	source, err := s.GetConversation(ctx, userID, conversationID)
	if err != nil {
		return Conversation{}, nil, err
	}
	messages, err := s.ListMessages(ctx, userID, conversationID)
	if err != nil {
		return Conversation{}, nil, err
	}
	var selected []Message
	for _, item := range messages {
		selected = append(selected, item)
		if item.ID == messageID {
			break
		}
	}
	if len(selected) == 0 || selected[len(selected)-1].ID != messageID {
		return Conversation{}, nil, sql.ErrNoRows
	}

	now := time.Now().UTC()
	newID := nextStoreID("conv", now)
	title := strings.TrimSpace(source.Title)
	if title == "" {
		title = "新对话"
	}
	title += " - 分支"

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Conversation{}, nil, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
INSERT INTO conversations (id, user_id, title, selected_model_provider_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
`, newID, userID, title, source.SelectedModelProviderID, now, now); err != nil {
		return Conversation{}, nil, err
	}

	for index, item := range selected {
		messageID := fmt.Sprintf("%s_%d", nextStoreID("msg", now), index)
		createdAt := now.Add(time.Duration(index) * time.Nanosecond)
		if _, err := tx.ExecContext(ctx, `
INSERT INTO messages (id, user_id, conversation_id, role, content, attachments_json, status, segment_index, meta_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, messageID, userID, newID, item.Role, item.Content, item.AttachmentsJSON, firstNonEmptyString(item.Status, "final"), item.SegmentIndex, item.MetaJSON, createdAt, item.UpdatedAt); err != nil {
			return Conversation{}, nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Conversation{}, nil, err
	}
	created, err := s.GetConversation(ctx, userID, newID)
	if err != nil {
		return Conversation{}, nil, err
	}
	createdMessages, err := s.ListMessages(ctx, userID, newID)
	if err != nil {
		return Conversation{}, nil, err
	}
	return created, createdMessages, nil
}

func (s *ConversationStore) ListMessages(ctx context.Context, userID, conversationID string) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, conversation_id, role, content, attachments_json, status, segment_index, meta_json, created_at, updated_at
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
	if attachmentsJSON == "" {
		attachmentsJSON = "[]"
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
VALUES (?, ?, ?, ?, ?, ?, 'final', 0, '', ?, ?)
`, id, userID, conversationID, role, content, attachmentsJSON, now, now)
	if err != nil {
		return Message{}, err
	}
	_, err = tx.ExecContext(ctx, `
UPDATE conversations
SET updated_at = ?
WHERE id = ? AND user_id = ?
`, now, conversationID, userID)
	if err != nil {
		return Message{}, err
	}
	if err := tx.Commit(); err != nil {
		return Message{}, err
	}
	return s.GetMessage(ctx, userID, id)
}

func (s *ConversationStore) GetMessage(ctx context.Context, userID, id string) (Message, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, user_id, conversation_id, role, content, attachments_json, status, segment_index, meta_json, created_at, updated_at
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

func (s *ConversationStore) UpdateConversationModel(ctx context.Context, userID, id, modelProviderID string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE conversations
SET selected_model_provider_id = ?, updated_at = ?
WHERE id = ? AND user_id = ?
`, modelProviderID, time.Now().UTC(), id, userID)
	return err
}

func (s *ConversationStore) UpdateConversationTitle(ctx context.Context, userID, id, title string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE conversations
SET title = ?
WHERE id = ? AND user_id = ?
`, strings.TrimSpace(title), id, userID)
	return err
}

func (s *ConversationStore) RecordToolCall(ctx context.Context, input ToolCallInput) (ToolCall, error) {
	now := time.Now().UTC()
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = nextStoreID("tc", now)
	}
	if input.Status == "" {
		input.Status = "success"
	}
	if input.ArgumentsJSON == "" {
		input.ArgumentsJSON = "{}"
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO tool_calls (id, user_id, conversation_id, message_id, task_id, tool_name, arguments_json, result_summary, status, error, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, id, input.UserID, input.ConversationID, input.MessageID, input.TaskID, input.ToolName, input.ArgumentsJSON, input.ResultSummary, input.Status, input.Error, now)
	if err != nil {
		return ToolCall{}, err
	}
	return ToolCall{
		ID:             id,
		UserID:         input.UserID,
		ConversationID: input.ConversationID,
		MessageID:      input.MessageID,
		TaskID:         input.TaskID,
		ToolName:       input.ToolName,
		ArgumentsJSON:  input.ArgumentsJSON,
		ResultSummary:  input.ResultSummary,
		Status:         input.Status,
		Error:          input.Error,
		CreatedAt:      now,
	}, nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func scanConversation(s scanner) (Conversation, error) {
	var item Conversation
	err := s.Scan(&item.ID, &item.UserID, &item.Title, &item.SelectedModelProviderID, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func scanMessage(s scanner) (Message, error) {
	var item Message
	err := s.Scan(&item.ID, &item.UserID, &item.ConversationID, &item.Role, &item.Content, &item.AttachmentsJSON, &item.Status, &item.SegmentIndex, &item.MetaJSON, &item.CreatedAt, &item.UpdatedAt)
	item.Attachments = json.RawMessage(item.AttachmentsJSON)
	if len(item.Attachments) == 0 {
		item.Attachments = json.RawMessage("[]")
	}
	item.Meta = json.RawMessage(item.MetaJSON)
	if len(item.Meta) == 0 {
		item.Meta = json.RawMessage("{}")
	}
	if item.Status == "" {
		item.Status = "final"
	}
	return item, err
}
