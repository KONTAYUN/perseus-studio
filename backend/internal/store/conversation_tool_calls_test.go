package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestToolCallIDsAreScopedToRun(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()

	conversations := NewConversationStore(db)
	if err := conversations.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	for _, runID := range []string{"run-a", "run-b"} {
		if _, err := conversations.RecordToolCall(ctx, ToolCallInput{
			ID:             "provider-call-1",
			UserID:         "user-a",
			RunID:          runID,
			ConversationID: "conversation-a",
			ToolName:       "read_workspace_text",
			ArgumentsJSON:  `{"path":"note.txt"}`,
			ResultSummary:  runID,
			Status:         "success",
		}); err != nil {
			t.Fatalf("RecordToolCall for %s returned error: %v", runID, err)
		}
	}

	first, err := conversations.GetToolCallForRun(ctx, "user-a", "run-a", "provider-call-1")
	if err != nil {
		t.Fatalf("GetToolCallForRun run-a returned error: %v", err)
	}
	second, err := conversations.GetToolCallForRun(ctx, "user-a", "run-b", "provider-call-1")
	if err != nil {
		t.Fatalf("GetToolCallForRun run-b returned error: %v", err)
	}
	if first.ResultSummary != "run-a" || second.ResultSummary != "run-b" {
		t.Fatalf("run-scoped tool calls were mixed: first=%#v second=%#v", first, second)
	}
}

func TestMigrateToolCallsFromGlobalToRunScopedID(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatalf("sql.Open returned error: %v", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, `
CREATE TABLE tool_calls (
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
INSERT INTO tool_calls (
  id, user_id, run_id, tool_name, arguments_json, result_summary, status, error, created_at
) VALUES (
  'provider-call-1', 'user-a', 'run-old', 'read_workspace_text',
  '{"path":"note.txt"}', 'legacy', 'success', '', CURRENT_TIMESTAMP
);`); err != nil {
		t.Fatalf("create legacy tool_calls returned error: %v", err)
	}

	conversations := NewConversationStore(db)
	if err := conversations.Migrate(ctx); err != nil {
		t.Fatalf("Migrate legacy tool_calls returned error: %v", err)
	}
	if _, err := conversations.RecordToolCall(ctx, ToolCallInput{
		ID:             "provider-call-1",
		UserID:         "user-a",
		RunID:          "run-new",
		ConversationID: "conversation-a",
		ToolName:       "read_workspace_text",
		ArgumentsJSON:  `{"path":"other.txt"}`,
		ResultSummary:  "new",
		Status:         "success",
	}); err != nil {
		t.Fatalf("RecordToolCall after migration returned error: %v", err)
	}

	for _, runID := range []string{"run-old", "run-new"} {
		if _, err := conversations.GetToolCallForRun(ctx, "user-a", runID, "provider-call-1"); err != nil {
			t.Fatalf("GetToolCallForRun %s after migration returned error: %v", runID, err)
		}
	}
}
