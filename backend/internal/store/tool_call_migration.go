package store

import (
	"context"
	"database/sql"
	"fmt"
)

func (s *ConversationStore) migrateToolCallRunScopedPrimaryKey(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(tool_calls)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	primaryKeys := map[string]int{}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, dataType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if primaryKey > 0 {
			primaryKeys[name] = primaryKey
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if primaryKeys["run_id"] == 1 && primaryKeys["id"] == 2 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `ALTER TABLE tool_calls RENAME TO tool_calls_legacy_call_scope`); err != nil {
		return fmt.Errorf("rename legacy tool_calls: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
CREATE TABLE tool_calls (
  id TEXT NOT NULL,
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
  created_at DATETIME NOT NULL,
  PRIMARY KEY (run_id, id)
)`); err != nil {
		return fmt.Errorf("create run-scoped tool_calls: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO tool_calls (
  id, user_id, run_id, conversation_id, message_id, task_id, tool_name, operation_hash,
  arguments_json, result_summary, status, error, started_at, completed_at, duration_ms,
  result_json, changed_paths_json, version_id, content_hash, created_at
)
SELECT
  id, user_id, run_id, conversation_id, message_id, task_id, tool_name, operation_hash,
  arguments_json, result_summary, status, error, started_at, completed_at, duration_ms,
  result_json, changed_paths_json, version_id, content_hash, created_at
FROM tool_calls_legacy_call_scope`); err != nil {
		return fmt.Errorf("copy run-scoped tool_calls: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE tool_calls_legacy_call_scope`); err != nil {
		return fmt.Errorf("drop legacy tool_calls: %w", err)
	}
	return tx.Commit()
}
