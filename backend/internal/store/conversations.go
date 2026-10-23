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
	TitleSource             string    `json:"title_source"`
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
	ID                     string          `json:"id"`
	UserID                 string          `json:"user_id"`
	ConversationID         string          `json:"conversation_id"`
	Role                   string          `json:"role"`
	Content                string          `json:"content"`
	AttachmentsJSON        string          `json:"-"`
	AttachmentPayloadsJSON string          `json:"-"`
	Status                 string          `json:"status,omitempty"`
	SegmentIndex           int             `json:"segment_index,omitempty"`
	MetaJSON               string          `json:"-"`
	UpdatedAt              sql.NullTime    `json:"-"`
	CreatedAt              time.Time       `json:"created_at"`
	Attachments            json.RawMessage `json:"attachments,omitempty"`
	Meta                   json.RawMessage `json:"meta,omitempty"`
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

type MessageInput struct {
	UserID          string
	ConversationID  string
	Role            string
	Content         string
	AttachmentsJSON string
	Status          string
	SegmentIndex    int
	MetaJSON        string
	CreatedAt       time.Time
}

type ToolCall struct {
	ID               string       `json:"id"`
	UserID           string       `json:"user_id"`
	RunID            string       `json:"run_id,omitempty"`
	ConversationID   string       `json:"conversation_id"`
	MessageID        string       `json:"message_id,omitempty"`
	TaskID           string       `json:"task_id,omitempty"`
	ToolName         string       `json:"tool_name"`
	OperationHash    string       `json:"operation_hash,omitempty"`
	ArgumentsJSON    string       `json:"arguments_json"`
	ResultSummary    string       `json:"result_summary"`
	Status           string       `json:"status"`
	Error            string       `json:"error,omitempty"`
	StartedAt        sql.NullTime `json:"-"`
	CompletedAt      sql.NullTime `json:"-"`
	DurationMS       int64        `json:"duration_ms,omitempty"`
	ResultJSON       string       `json:"result_json,omitempty"`
	ChangedPathsJSON string       `json:"changed_paths_json,omitempty"`
	VersionID        string       `json:"version_id,omitempty"`
	ContentHash      string       `json:"content_hash,omitempty"`
	CreatedAt        time.Time    `json:"created_at"`
}

type ToolCallInput struct {
	ID               string
	UserID           string
	RunID            string
	ConversationID   string
	MessageID        string
	TaskID           string
	ToolName         string
	OperationHash    string
	ArgumentsJSON    string
	ResultSummary    string
	Status           string
	Error            string
	StartedAt        time.Time
	CompletedAt      time.Time
	DurationMS       int64
	ResultJSON       string
	ChangedPathsJSON string
	VersionID        string
	ContentHash      string
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
  title_source TEXT NOT NULL DEFAULT 'default',
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
  run_id TEXT NOT NULL DEFAULT '',
  conversation_id TEXT,
  message_id TEXT,
  task_id TEXT,
  tool_name TEXT NOT NULL,
  operation_hash TEXT NOT NULL DEFAULT '',
  arguments_json TEXT NOT NULL,
  result_summary TEXT NOT NULL,
  status TEXT NOT NULL,
  error TEXT NOT NULL,
  started_at DATETIME,
  completed_at DATETIME,
  duration_ms INTEGER NOT NULL DEFAULT 0,
  result_json TEXT NOT NULL DEFAULT '',
  changed_paths_json TEXT NOT NULL DEFAULT '',
  version_id TEXT NOT NULL DEFAULT '',
  content_hash TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS message_attachment_payloads (
  message_id TEXT PRIMARY KEY REFERENCES messages(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL,
  payload_json TEXT NOT NULL CHECK(json_valid(payload_json))
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
	if err := AddColumnIfNotExists(ctx, s.db, "conversations", "title_source", "TEXT NOT NULL DEFAULT 'default'"); err != nil {
		return err
	}
	if err := AddColumnIfNotExists(ctx, s.db, "conversations", "summary_until_message_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := AddColumnIfNotExists(ctx, s.db, "conversations", "summary_updated_at", "DATETIME"); err != nil {
		return err
	}
	for _, migration := range []struct {
		column string
		def    string
	}{
		{"run_id", "TEXT NOT NULL DEFAULT ''"},
		{"operation_hash", "TEXT NOT NULL DEFAULT ''"},
		{"started_at", "DATETIME"},
		{"completed_at", "DATETIME"},
		{"duration_ms", "INTEGER NOT NULL DEFAULT 0"},
		{"result_json", "TEXT NOT NULL DEFAULT ''"},
		{"changed_paths_json", "TEXT NOT NULL DEFAULT ''"},
		{"version_id", "TEXT NOT NULL DEFAULT ''"},
		{"content_hash", "TEXT NOT NULL DEFAULT ''"},
	} {
		if err := AddColumnIfNotExists(ctx, s.db, "tool_calls", migration.column, migration.def); err != nil {
			return err
		}
	}
	indexes := `
CREATE INDEX IF NOT EXISTS idx_conversations_updated_at ON conversations(updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_conversations_user_id ON conversations(user_id);
CREATE INDEX IF NOT EXISTS idx_messages_conversation_created_at ON messages(conversation_id, created_at ASC);
CREATE INDEX IF NOT EXISTS idx_messages_conversation_segment ON messages(conversation_id, segment_index, created_at ASC);
CREATE INDEX IF NOT EXISTS idx_messages_user_id ON messages(user_id);
CREATE INDEX IF NOT EXISTS idx_tool_calls_conversation_created_at ON tool_calls(conversation_id, created_at ASC);
CREATE INDEX IF NOT EXISTS idx_tool_calls_user_id ON tool_calls(user_id);
CREATE INDEX IF NOT EXISTS idx_tool_calls_run_created_at ON tool_calls(run_id, created_at ASC);
CREATE INDEX IF NOT EXISTS idx_tool_calls_operation_hash ON tool_calls(user_id, run_id, operation_hash);
`
	_, err := s.db.ExecContext(ctx, indexes)
	return err
}

func (s *ConversationStore) ListConversations(ctx context.Context, userID string) ([]Conversation, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, title, title_source, selected_model_provider_id, created_at, updated_at
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
	titleSource := "manual"
	if title == "" {
		title = "新对话"
		titleSource = "default"
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO conversations (id, user_id, title, title_source, selected_model_provider_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, id, input.UserID, title, titleSource, input.SelectedModelProviderID, now, now)
	if err != nil {
		return Conversation{}, err
	}
	return s.GetConversation(ctx, input.UserID, id)
}

func (s *ConversationStore) GetConversation(ctx context.Context, userID, id string) (Conversation, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, user_id, title, title_source, selected_model_provider_id, created_at, updated_at
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
INSERT INTO conversations (id, user_id, title, title_source, selected_model_provider_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, newID, userID, title, "manual", source.SelectedModelProviderID, now, now); err != nil {
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
		if err := saveMessageAttachmentPayloads(ctx, tx, userID, messageID, item.AttachmentPayloadsJSON); err != nil {
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
SET title = ?,
    title_source = 'manual',
    updated_at = ?
WHERE id = ? AND user_id = ?
`, strings.TrimSpace(title), time.Now().UTC(), id, userID)
	return err
}

func (s *ConversationStore) UpdateConversationAutoTitle(ctx context.Context, userID, id, title string) (Conversation, bool, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Conversation{}, false, nil
	}
	result, err := s.db.ExecContext(ctx, `
UPDATE conversations
SET title = ?,
    title_source = 'auto',
    updated_at = ?
WHERE id = ? AND user_id = ? AND title_source IN ('', 'default', 'auto')
`, title, time.Now().UTC(), id, userID)
	if err != nil {
		return Conversation{}, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return Conversation{}, false, err
	}
	if rows == 0 {
		return Conversation{}, false, nil
	}
	item, err := s.GetConversation(ctx, userID, id)
	if err != nil {
		return Conversation{}, false, err
	}
	return item, true, nil
}
