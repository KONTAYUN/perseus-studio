package api

import (
	"context"
	"errors"
	"log"
	"time"
	"webide-backend/internal/failure"

	"webide-backend/internal/ai"
	"webide-backend/internal/store"
)

func (s *Server) failConversationRun(ctx context.Context, userID, conversationID, messageID, runID string, err error) {
	s.failConversationRunStartedAt(ctx, userID, conversationID, messageID, runID, err, time.Time{})
}

func (s *Server) failConversationRunStartedAt(ctx context.Context, userID, conversationID, messageID, runID string, err error, startedAt time.Time) {
	message := ai.UserFacingErrorMessage(err)
	if message == "" {
		message = "执行失败"
	}
	status := store.AgentRunFailed
	stage := "failed"
	if failure.Code(err) == "" && (errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled)) {
		status = store.AgentRunCanceled
		stage = "canceled"
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
	if err := s.failRunStrict(context.Background(), userID, runID, status, stage, store.UpdateAgentRunInput{
		ErrorMessage: &message, DiagnosticLogID: diagnosticID, SetCompletedAt: true,
	}, map[string]any{"error": message, "failure_class": localRunFailureClass(err), "diagnostic_log_id": diagnosticID}); err != nil {
		log.Printf("[agent_run] commit failure failed run=%s status=%s error=%v", runID, status, err)
	}
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
	return s.failRunStrict(ctx, userID, runID, store.AgentRunFailed, "budget_exhausted", store.UpdateAgentRunInput{
		ErrorMessage: &message, AssistantMessageID: assistantMessage.ID, SetCompletedAt: true,
	}, payload)
}
