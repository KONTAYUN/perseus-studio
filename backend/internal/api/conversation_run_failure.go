package api

import (
	"context"
	"strings"

	"webide-backend/internal/ai"
	"webide-backend/internal/store"
)

func (s *Server) failConversationRun(ctx context.Context, userID, conversationID, messageID, runID string, err error) {
	message := ai.UserFacingErrorMessage(err)
	if message == "" {
		message = "执行失败"
	}
	diagnosticID := ai.DiagnosticIDFromError(err)
	status := store.AgentRunFailed
	stage := "failed"
	eventType := "run_failed"
	level := "error"
	if ctx.Err() != nil || strings.Contains(strings.ToLower(message), "context canceled") {
		status = store.AgentRunCanceled
		stage = "canceled"
		eventType = "run_canceled"
		level = "warn"
		message = "执行已取消"
	}
	eventCtx := ctx
	if ctx.Err() != nil {
		eventCtx = context.Background()
	}
	if diagnosticID != "" {
		_, _ = s.appendRunEventStrict(eventCtx, store.AgentEventInput{
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
		})
	}
	_, _ = s.appendRunEventStrict(eventCtx, store.AgentEventInput{
		RunID:          runID,
		UserID:         userID,
		ConversationID: conversationID,
		MessageID:      messageID,
		Type:           eventType,
		Level:          level,
		Payload: map[string]any{
			"error":             message,
			"diagnostic_log_id": diagnosticID,
		},
	})
	_, _ = s.publishRunStrict(context.Background(), userID, runID, status, stage, store.UpdateAgentRunInput{
		ErrorMessage:    &message,
		DiagnosticLogID: diagnosticID,
		SetCompletedAt:  true,
	})
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
	if _, err := s.appendRunEventStrict(ctx, store.AgentEventInput{
		RunID:          runID,
		UserID:         userID,
		ConversationID: conversationID,
		MessageID:      assistantMessage.ID,
		Type:           "run_failed",
		Level:          "warn",
		Payload:        payload,
	}); err != nil {
		return err
	}
	_, err := s.publishRunStrict(ctx, userID, runID, store.AgentRunFailed, "budget_exhausted", store.UpdateAgentRunInput{
		ErrorMessage:       &message,
		AssistantMessageID: assistantMessage.ID,
		SetCompletedAt:     true,
	})
	return err
}
