package ai

import (
	"context"
	"encoding/json"
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
	return checkpoint, true
}

func (s *Service) saveRunCheckpoint(ctx context.Context, opts RunOptions, checkpoint AgentRuntimeCheckpoint) {
	if s == nil || s.runs == nil || opts.UserID == "" || opts.RunID == "" {
		return
	}
	raw, err := json.Marshal(checkpoint)
	if err != nil {
		return
	}
	_, _ = s.runs.SaveCheckpoint(ctx, opts.UserID, opts.RunID, string(raw))
}

func (s *Service) clearRunCheckpoints(ctx context.Context, opts RunOptions) {
	if s == nil || s.runs == nil || opts.UserID == "" || opts.RunID == "" {
		return
	}
	_ = s.runs.DeleteCheckpoints(ctx, opts.UserID, opts.RunID)
}

func (s *Service) ClearRunCheckpoints(ctx context.Context, opts RunOptions) {
	s.clearRunCheckpoints(ctx, opts)
}
