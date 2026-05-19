package ai

import (
	"context"
	"encoding/json"
	"fmt"
)

func (s *Service) loadRunCheckpoint(ctx context.Context, opts RunOptions) (AgentRuntimeCheckpoint, bool) {
	if s == nil || s.runs == nil || opts.UserID == "" || opts.RunID == "" {
		return AgentRuntimeCheckpoint{}, false
	}
	item, err := s.runs.LatestCheckpoint(ctx, opts.UserID, opts.RunID)
	if err != nil {
		return AgentRuntimeCheckpoint{}, false
	}
	var checkpoint AgentRuntimeCheckpoint
	if err := json.Unmarshal([]byte(item.StateJSON), &checkpoint); err != nil {
		return AgentRuntimeCheckpoint{}, false
	}
	checkpoint.Seq = item.Seq
	return checkpoint, true
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
