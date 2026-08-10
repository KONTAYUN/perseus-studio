package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

const legacyPreviewEventCleanupMigration = "remove_persisted_edit_preview_snapshots_v1"

type AgentEditPreview struct {
	PreviewID    string    `json:"preview_id"`
	RunID        string    `json:"run_id"`
	UserID       string    `json:"user_id"`
	ToolCallID   string    `json:"tool_call_id"`
	Path         string    `json:"path"`
	BaseHash     string    `json:"base_hash,omitempty"`
	Revision     int       `json:"revision"`
	SnapshotJSON string    `json:"snapshot_json"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type AgentEditPreviewInput struct {
	PreviewID    string
	RunID        string
	UserID       string
	ToolCallID   string
	Path         string
	BaseHash     string
	Revision     int
	SnapshotJSON string
}

func (s *AgentRunStore) UpsertEditPreview(ctx context.Context, input AgentEditPreviewInput) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO agent_edit_previews (
  preview_id, run_id, user_id, tool_call_id, path, base_hash, revision, snapshot_json, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(preview_id) DO UPDATE SET
  run_id = excluded.run_id,
  user_id = excluded.user_id,
  tool_call_id = excluded.tool_call_id,
  path = excluded.path,
  base_hash = excluded.base_hash,
  revision = excluded.revision,
  snapshot_json = excluded.snapshot_json,
  updated_at = excluded.updated_at
WHERE excluded.revision > agent_edit_previews.revision
`, input.PreviewID, input.RunID, input.UserID, input.ToolCallID, input.Path,
		input.BaseHash, input.Revision, input.SnapshotJSON, time.Now().UTC())
	return err
}

func (s *AgentRunStore) DeleteEditPreview(ctx context.Context, userID, previewID string) error {
	_, err := s.db.ExecContext(ctx, `
DELETE FROM agent_edit_previews
WHERE preview_id = ? AND user_id = ?
`, previewID, userID)
	return err
}

func (s *AgentRunStore) DeleteRunEditPreviews(ctx context.Context, userID, runID string) error {
	_, err := s.db.ExecContext(ctx, `
DELETE FROM agent_edit_previews
WHERE run_id = ? AND user_id = ?
`, runID, userID)
	return err
}

func (s *AgentRunStore) ListEditPreviews(ctx context.Context, userID, runID string) ([]AgentEditPreview, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT preview_id, run_id, user_id, tool_call_id, path, base_hash, revision, snapshot_json, updated_at
FROM agent_edit_previews
WHERE run_id = ? AND user_id = ?
ORDER BY updated_at, preview_id
`, runID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []AgentEditPreview
	for rows.Next() {
		var item AgentEditPreview
		if err := rows.Scan(
			&item.PreviewID, &item.RunID, &item.UserID, &item.ToolCallID, &item.Path,
			&item.BaseHash, &item.Revision, &item.SnapshotJSON, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *AgentRunStore) cleanupLegacyPreviewEvents(ctx context.Context) error {
	var completed string
	err := s.db.QueryRowContext(ctx, `
SELECT name FROM agent_data_migrations WHERE name = ?
`, legacyPreviewEventCleanupMigration).Scan(&completed)
	if err == nil {
		return nil
	}
	if err != sql.ErrNoRows {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
DELETE FROM agent_events
WHERE type IN ('workspace_edit_preview_started', 'workspace_edit_preview_updated')
`)
	if err != nil {
		return err
	}
	deleted, _ := result.RowsAffected()
	details, _ := json.Marshal(map[string]any{"deleted_events": deleted})
	if _, err := tx.ExecContext(ctx, `
INSERT INTO agent_data_migrations (name, details_json, completed_at)
VALUES (?, ?, ?)
`, legacyPreviewEventCleanupMigration, string(details), time.Now().UTC()); err != nil {
		return err
	}
	return tx.Commit()
}
