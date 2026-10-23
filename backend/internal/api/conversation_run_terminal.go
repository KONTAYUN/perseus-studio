package api

import (
	"context"

	"webide-backend/internal/store"
	"webide-backend/internal/tasks"
)

func (s *Server) completeRunStrict(ctx context.Context, userID, runID string, input store.CompleteAgentRunInput) error {
	run, event, err := s.runs.CompleteRun(ctx, userID, runID, input)
	if err != nil {
		return err
	}
	s.runBroker.Publish(runID, tasks.Event{Type: "event", Log: event})
	s.runBroker.Publish(runID, tasks.Event{Type: "run", Task: run})
	return nil
}

func (s *Server) failRunStrict(ctx context.Context, userID, runID string, status store.AgentRunStatus, stage string, input store.UpdateAgentRunInput, payload any) error {
	run, event, err := s.runs.FailRun(ctx, userID, runID, status, stage, input, payload)
	if err != nil {
		return err
	}
	if event.ID != "" {
		s.runBroker.Publish(runID, tasks.Event{Type: "event", Log: event})
	}
	s.runBroker.Publish(runID, tasks.Event{Type: "run", Task: run})
	return nil
}
