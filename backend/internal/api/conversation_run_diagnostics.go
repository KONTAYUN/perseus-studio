package api

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"webide-backend/internal/ai"
)

func isExpectedRunCancellation(err error) bool {
	return ai.IsContextCanceledError(err)
}

func (s *Server) runPersistenceDiagnosticSummary(userID, runID, stage string, startedAt time.Time) string {
	summary := map[string]any{
		"failure_class": "local_run_persistence",
		"run_id":        runID,
		"stage":         stage,
	}
	if !startedAt.IsZero() {
		summary["duration_ms"] = time.Since(startedAt).Milliseconds()
	}
	if entry := s.runCancelSnapshot(runID); entry != nil {
		summary["cancel_source"] = entry.source
		summary["cancel_reason"] = entry.reason
		if !entry.requestedAt.IsZero() {
			summary["cancel_requested_at"] = entry.requestedAt.Format(time.RFC3339Nano)
		}
	}
	if s.runs != nil && strings.TrimSpace(userID) != "" && strings.TrimSpace(runID) != "" {
		if run, err := s.runs.GetRun(context.Background(), userID, runID); err == nil {
			summary["run_status"] = run.Status
			if run.Stage.Valid {
				summary["run_stage"] = run.Stage.String
			}
			if run.StartedAt.Valid {
				summary["run_started_at"] = run.StartedAt.Time.Format(time.RFC3339Nano)
			}
			if run.CompletedAt.Valid {
				summary["run_completed_at"] = run.CompletedAt.Time.Format(time.RFC3339Nano)
			}
		}
	}
	raw, _ := json.Marshal(summary)
	return string(raw)
}

func (s *Server) runCancelSnapshot(runID string) *runCancelEntry {
	if s == nil {
		return nil
	}
	s.runCancelMu.Lock()
	defer s.runCancelMu.Unlock()
	entry := s.runCancels[runID]
	if entry == nil || (entry.source == "" && entry.reason == "" && entry.requestedAt.IsZero()) {
		return nil
	}
	return &runCancelEntry{
		source:      entry.source,
		reason:      entry.reason,
		requestedAt: entry.requestedAt,
	}
}
