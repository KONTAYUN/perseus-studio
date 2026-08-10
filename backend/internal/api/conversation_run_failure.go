package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"webide-backend/internal/ai"
	"webide-backend/internal/store"
)

func (s *Server) failConversationRun(ctx context.Context, userID, conversationID, messageID, runID string, err error) {
	s.failConversationRunStartedAt(ctx, userID, conversationID, messageID, runID, err, time.Time{})
}

func (s *Server) failConversationRunStartedAt(ctx context.Context, userID, conversationID, messageID, runID string, err error, startedAt time.Time) {
	if current, getErr := s.runs.GetRun(context.Background(), userID, runID); getErr == nil && runIsTerminal(current.Status) {
		return
	}
	message := ai.UserFacingErrorMessage(err)
	if message == "" {
		message = "执行失败"
	}
	status := store.AgentRunFailed
	stage := "failed"
	eventType := "run_failed"
	level := "error"
	failure := ai.FailureMetadataFromError(err)
	if failure.FailureClass == "provider_rate_limited" ||
		failure.FailureClass == "provider_tpm_rate_limited" ||
		failure.FailureClass == "provider_rpm_rate_limited" ||
		failure.FailureClass == "provider_quota_exhausted" {
		stage = "rate_limited"
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		status = store.AgentRunCanceled
		stage = "canceled"
		eventType = "run_canceled"
		level = "warn"
		message = "执行已取消"
	}
	diagnosticID := ""
	if status != store.AgentRunCanceled {
		diagnosticID = s.ensureRunFailureDiagnostic(ctx, userID, conversationID, messageID, runID, err, startedAt)
	} else {
		diagnosticID = ai.DiagnosticIDFromError(err)
	}
	eventCtx := ctx
	if ctx.Err() != nil {
		eventCtx = context.Background()
	}
	if diagnosticID != "" {
		if _, err := s.appendRunEventStrict(eventCtx, store.AgentEventInput{
			RunID:          runID,
			UserID:         userID,
			ConversationID: conversationID,
			MessageID:      messageID,
			Type:           "diagnostic_created",
			Level:          "error",
			Payload: map[string]any{
				"diagnostic_log_id": diagnosticID,
				"message":           "diagnostic log created",
			},
		}); err != nil {
			log.Printf("[agent_run] append diagnostic event failed run=%s diagnostic=%s error=%v", runID, diagnosticID, err)
		}
	}
	failurePayload := map[string]any{
		"failure_class":      failure.FailureClass,
		"retry_after_ms":     failure.RetryAfterMS,
		"provider_id":        failure.ProviderID,
		"model_id":           failure.ModelID,
		"can_resume_now":     failure.CanResumeNow,
		"recommended_action": failure.RecommendedAction,
	}
	if !failure.ResetAt.IsZero() {
		failurePayload["reset_at"] = failure.ResetAt
	}
	failureJSON, _ := json.Marshal(failure)
	failureJSONString := string(failureJSON)
	if _, _, err := s.finalizeRunStrict(eventCtx, userID, runID, status, stage, store.UpdateAgentRunInput{
		ErrorMessage:    &message,
		DiagnosticLogID: diagnosticID,
		FailureJSON:     &failureJSONString,
		SetCompletedAt:  true,
	}, store.AgentEventInput{
		RunID:          runID,
		UserID:         userID,
		ConversationID: conversationID,
		MessageID:      messageID,
		Type:           eventType,
		Level:          level,
		Payload:        mergeFailureEventPayload(failurePayload, message, diagnosticID),
	}); err != nil {
		log.Printf("[agent_run] finalize failure state failed run=%s status=%s stage=%s error=%v", runID, status, stage, err)
	}
}

func mergeFailureEventPayload(metadata map[string]any, message, diagnosticID string) map[string]any {
	payload := map[string]any{
		"error":             message,
		"diagnostic_log_id": diagnosticID,
	}
	for key, value := range metadata {
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			continue
		}
		payload[key] = value
	}
	return payload
}

func (s *Server) completeConversationRunWithBudgetExhaustion(ctx context.Context, userID, conversationID, userMessageID, runID string, assistantMessage store.Message, reply ai.Reply) error {
	message := "达到轮次上限，已总结当前进度，可继续本轮"
	payload := map[string]any{
		"error":             message,
		"message":           message,
		"stage":             "budget_exhausted",
		"incomplete_reason": reply.IncompleteReason,
		"recoverable":       reply.Recoverable,
	}
	if reply.Budget != nil {
		payload["budget_reason"] = reply.Budget.Reason
		payload["round"] = reply.Budget.Round
		payload["max_rounds"] = reply.Budget.MaxRounds
		payload["tool_calls"] = reply.Budget.ToolCalls
		payload["max_tool_calls"] = reply.Budget.MaxToolCalls
	}
	_, _, err := s.finalizeRunStrict(ctx, userID, runID, store.AgentRunFailed, "budget_exhausted", store.UpdateAgentRunInput{
		ErrorMessage:       &message,
		AssistantMessageID: assistantMessage.ID,
		SetCompletedAt:     true,
	}, store.AgentEventInput{
		RunID:          runID,
		UserID:         userID,
		ConversationID: conversationID,
		MessageID:      assistantMessage.ID,
		Type:           "run_failed",
		Level:          "warn",
		Payload:        payload,
	})
	return err
}
