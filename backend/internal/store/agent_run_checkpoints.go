package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type AgentRunCheckpoint struct {
	ID        string    `json:"id"`
	RunID     string    `json:"run_id"`
	UserID    string    `json:"user_id"`
	Seq       int64     `json:"seq"`
	StateJSON string    `json:"state_json"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *AgentRunStore) SaveCheckpoint(ctx context.Context, userID, runID, stateJSON string) (AgentRunCheckpoint, error) {
	now := time.Now().UTC()
	var raw [4]byte
	id := nextStoreID("cp", now)
	if _, err := rand.Read(raw[:]); err == nil {
		id = fmt.Sprintf("%s_%s", id, hex.EncodeToString(raw[:]))
	}
	row := s.db.QueryRowContext(ctx, `
INSERT INTO agent_run_checkpoints (id, run_id, user_id, seq, state_json, created_at)
VALUES (
  ?, ?, ?, COALESCE((SELECT MAX(seq) + 1 FROM agent_run_checkpoints WHERE run_id = ?), 1), ?, ?
)
RETURNING id, run_id, user_id, seq, state_json, created_at
`, id, strings.TrimSpace(runID), strings.TrimSpace(userID), strings.TrimSpace(runID), stateJSON, now)
	return scanAgentRunCheckpoint(row)
}

func (s *AgentRunStore) LatestCheckpoint(ctx context.Context, userID, runID string) (AgentRunCheckpoint, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, run_id, user_id, seq, state_json, created_at
FROM agent_run_checkpoints
WHERE run_id = ? AND user_id = ?
ORDER BY seq DESC
LIMIT 1
`, runID, userID)
	return scanAgentRunCheckpoint(row)
}

func (s *AgentRunStore) DeleteCheckpoints(ctx context.Context, userID, runID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM agent_run_checkpoints WHERE run_id = ? AND user_id = ?`, runID, userID)
	return err
}

func scanAgentRunCheckpoint(scn scanner) (AgentRunCheckpoint, error) {
	var item AgentRunCheckpoint
	err := scn.Scan(&item.ID, &item.RunID, &item.UserID, &item.Seq, &item.StateJSON, &item.CreatedAt)
	return item, err
}

var _ scanner = (*sql.Row)(nil)
