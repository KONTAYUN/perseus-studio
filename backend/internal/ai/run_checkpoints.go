package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

func (s *Service) loadRunCheckpoint(ctx context.Context, opts RunOptions) (AgentRuntimeCheckpoint, bool) {
	checkpoint, ok, _ := s.loadRunCheckpointStrict(ctx, opts)
	return checkpoint, ok
}

// Missing is safe to start; unreadable or corrupt persisted state is not.
func (s *Service) loadRunCheckpointStrict(ctx context.Context, opts RunOptions) (AgentRuntimeCheckpoint, bool, error) {
	if s == nil || s.runs == nil || opts.UserID == "" || opts.RunID == "" {
		return AgentRuntimeCheckpoint{}, false, nil
	}
	item, err := s.runs.LatestCheckpoint(ctx, opts.UserID, opts.RunID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AgentRuntimeCheckpoint{}, false, nil
		}
		return AgentRuntimeCheckpoint{}, false, fmt.Errorf("read run checkpoint: %w", err)
	}
	var checkpoint AgentRuntimeCheckpoint
	if err := json.Unmarshal([]byte(item.StateJSON), &checkpoint); err != nil {
		return AgentRuntimeCheckpoint{}, false, fmt.Errorf("decode run checkpoint: %w", err)
	}
	checkpoint.Seq = item.Seq
	return checkpoint, true, nil
}

func (s *Service) saveRunCheckpoint(ctx context.Context, opts RunOptions, checkpoint AgentRuntimeCheckpoint) error {
	if s == nil || s.runs == nil || opts.UserID == "" || opts.RunID == "" {
		return nil
	}
	raw, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("marshal run checkpoint: %w", err)
	}
	if _, err := s.runs.SaveCheckpoint(ctx, opts.UserID, opts.RunID, string(raw)); err != nil {
		return fmt.Errorf("save run checkpoint: %w", err)
	}
	return nil
}

func (s *Service) clearRunCheckpoints(ctx context.Context, opts RunOptions) error {
	if s == nil || s.runs == nil || opts.UserID == "" || opts.RunID == "" {
		return nil
	}
	if err := s.runs.DeleteCheckpoints(ctx, opts.UserID, opts.RunID); err != nil {
		return fmt.Errorf("delete run checkpoints: %w", err)
	}
	return nil
}

func (s *Service) ClearRunCheckpoints(ctx context.Context, opts RunOptions) error {
	return s.clearRunCheckpoints(ctx, opts)
}
