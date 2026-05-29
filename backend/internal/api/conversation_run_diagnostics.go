package api

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"webide-backend/internal/ai"
	"webide-backend/internal/store"
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

func (s *Server) ensureRunFailureDiagnostic(ctx context.Context, userID, conversationID, messageID, runID string, err error, startedAt time.Time) string {
	if s == nil || s.diagnostics == nil || err == nil {
		return ai.DiagnosticIDFromError(err)
	}
	if id := ai.DiagnosticIDFromError(err); id != "" {
		return id
	}
	if ctx != nil && ctx.Err() != nil {
		return ""
	}
	if isExpectedRunCancellation(err) {
		return ""
	}
	input := store.DiagnosticLogInput{
		UserID:         userID,
		ConversationID: conversationID,
		MessageID:      messageID,
		Stage:          "run_failed",
		Level:          "error",
		ErrorMessage:   ai.UserFacingErrorMessage(err),
		RequestSummary: s.runFailureDiagnosticSummary(userID, runID, err, startedAt),
	}
	if run, runErr := s.runs.GetRun(context.Background(), userID, runID); runErr == nil {
		input.ModelProviderID = run.ModelProviderID
		if run.Stage.Valid && strings.TrimSpace(run.Stage.String) != "" {
			input.Stage = "run_failed:" + run.Stage.String
		}
		s.populateRunFailureDiagnosticProviderFields(context.Background(), &input, run.ModelProviderID)
	}
	item, appendErr := s.diagnostics.Append(context.Background(), input)
	if appendErr != nil {
		return ""
	}
	return item.ID
}

func (s *Server) populateRunFailureDiagnosticProviderFields(ctx context.Context, input *store.DiagnosticLogInput, modelProviderID string) {
	if s == nil || s.configs == nil || input == nil || strings.TrimSpace(modelProviderID) == "" {
		return
	}
	provider, err := s.configs.GetModelProviderAny(ctx, modelProviderID)
	if err != nil {
		return
	}
	input.ModelProviderID = provider.ID
	input.ModelName = provider.Name
	input.ModelID = provider.ModelID
	input.ProviderName = provider.Name
	input.Protocol = provider.Protocol
	if parsed, parseErr := url.Parse(strings.TrimSpace(provider.BaseURL)); parseErr == nil {
		input.BaseURLHost = parsed.Host
	}
}

func (s *Server) runFailureDiagnosticSummary(userID, runID string, err error, startedAt time.Time) string {
	summary := map[string]any{
		"failure_class": "local_run_failure",
		"run_id":        runID,
	}
	if !startedAt.IsZero() {
		summary["duration_ms"] = time.Since(startedAt).Milliseconds()
	}
	if s != nil && s.runs != nil && strings.TrimSpace(userID) != "" && strings.TrimSpace(runID) != "" {
		if run, runErr := s.runs.GetRun(context.Background(), userID, runID); runErr == nil {
			summary["run_status"] = run.Status
			summary["model_provider_id"] = run.ModelProviderID
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
	if failureClass := localRunFailureClass(err); failureClass != "" {
		summary["failure_class"] = failureClass
	}
	raw, _ := json.Marshal(summary)
	return string(raw)
}

func localRunFailureClass(err error) string {
	if err == nil {
		return ""
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "工具调用连续失败") || strings.Contains(text, "同一个工具调用连续失败"):
		return "agent_runtime_tool_error_loop"
	case strings.Contains(text, "模型没有返回可执行工具调用") || strings.Contains(text, "workspace 任务未执行完成"):
		return "agent_runtime_no_tool_completion"
	case strings.Contains(text, "模型没有调用所需的 workspace 变更工具"):
		return "agent_runtime_missing_workspace_mutation"
	case strings.Contains(text, "unsupported protocol"):
		return "model_provider_unsupported_protocol"
	case strings.Contains(text, "selected model is disabled"):
		return "model_provider_disabled"
	case strings.Contains(text, "sql: no rows in result set"):
		return "local_missing_record"
	case strings.Contains(text, "run checkpoint") || strings.Contains(text, "tool_ledger") || strings.Contains(text, "local") && strings.Contains(text, "persistence"):
		return "local_run_persistence"
	default:
		return "local_run_failure"
	}
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
