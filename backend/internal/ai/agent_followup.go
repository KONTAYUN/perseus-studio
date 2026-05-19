package ai

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"webide-backend/internal/store"
)

func (r *agentRuntime) handleFollowupQuestion(toolCallID, argumentsJSON string) (string, bool, bool) {
	startedAt := time.Now()
	var attempt followupQuestionAttempt
	if err := json.Unmarshal([]byte(argumentsJSON), &attempt); err != nil {
		r.recordToolError()
		return r.toolErrorJSON("ask_followup_question", "invalid ask_followup_question arguments: "+err.Error()), true, false
	}
	attempt.Question = strings.TrimSpace(attempt.Question)
	if attempt.Question == "" {
		r.recordToolError()
		return r.toolErrorJSON("ask_followup_question", "question is required"), true, false
	}
	attempt.Reason = strings.TrimSpace(attempt.Reason)
	attempt.Options = normalizeFollowupOptions(attempt.Options)
	allowFreeform := true
	if attempt.AllowFreeform != nil {
		allowFreeform = *attempt.AllowFreeform
	}
	emitToolLifecycleEvent(r.opts, "tool_call_started", "info", map[string]any{
		"tool_call_id":      toolCallID,
		"tool_name":         "ask_followup_question",
		"status":            "running",
		"arguments_summary": summarizeToolArguments(argumentsJSON),
	})
	if r.service != nil && r.service.calls != nil {
		recorded, recordErr := r.service.calls.StartToolCall(context.Background(), store.ToolCallInput{
			ID:             toolCallID,
			UserID:         r.opts.UserID,
			RunID:          r.opts.RunID,
			ConversationID: r.conversationID,
			MessageID:      r.messageID,
			TaskID:         r.opts.TaskID,
			ToolName:       "ask_followup_question",
			ArgumentsJSON:  ledgerToolArgumentsJSON(argumentsJSON),
			StartedAt:      startedAt,
		})
		if recordErr != nil {
			r.recordToolError()
			diagID := r.service.recordLocalPersistenceDiagnostic(context.Background(), r.opts, r.conversationID, r.messageID, "tool_ledger_start_failed", recordErr)
			emitToolLifecycleEvent(r.opts, "local_persistence_failed", "error", map[string]any{
				"tool_call_id":      toolCallID,
				"tool_name":         "ask_followup_question",
				"stage":             "tool_ledger_start_failed",
				"error":             recordErr.Error(),
				"diagnostic_log_id": diagID,
			})
			return r.toolErrorJSON("ask_followup_question", "local tool ledger start failed: "+recordErr.Error()), true, false
		}
		if recorded.ID != "" {
			toolCallID = recorded.ID
		}
	}
	content := attempt.Question
	r.completedContent = content
	resultSummary := "asked user a follow-up question"
	durationMS := time.Since(startedAt).Milliseconds()
	event := ToolEvent{
		ToolCallID:    toolCallID,
		ToolName:      "ask_followup_question",
		Label:         "已询问用户",
		Status:        "success",
		Question:      attempt.Question,
		Options:       attempt.Options,
		AllowFreeform: &allowFreeform,
		DurationMS:    durationMS,
	}
	resultPayload := mustJSON(map[string]any{
		"status":         "waiting_for_user_answer",
		"tool_name":      "ask_followup_question",
		"question":       attempt.Question,
		"options":        attempt.Options,
		"allow_freeform": allowFreeform,
	})
	if r.service != nil && r.service.calls != nil {
		recorded, recordErr := r.service.calls.CompleteToolCall(context.Background(), store.ToolCallInput{
			ID:            toolCallID,
			UserID:        r.opts.UserID,
			ResultSummary: resultSummary,
			Status:        "success",
			CompletedAt:   time.Now().UTC(),
			DurationMS:    durationMS,
			ResultJSON:    resultPayload,
		})
		if recordErr != nil {
			r.recordToolError()
			diagID := r.service.recordLocalPersistenceDiagnostic(context.Background(), r.opts, r.conversationID, r.messageID, "tool_ledger_complete_failed", recordErr)
			emitToolLifecycleEvent(r.opts, "local_persistence_failed", "error", map[string]any{
				"tool_call_id":      toolCallID,
				"tool_name":         "ask_followup_question",
				"stage":             "tool_ledger_complete_failed",
				"error":             recordErr.Error(),
				"diagnostic_log_id": diagID,
			})
			return r.toolErrorJSON("ask_followup_question", "local tool ledger completion failed: "+recordErr.Error()), true, false
		}
		if recorded.ID != "" {
			event.ToolCallID = recorded.ID
			toolCallID = recorded.ID
		}
	}
	r.events = append(r.events, event)
	emitToolLifecycleEvent(r.opts, "tool_call_completed", "info", map[string]any{
		"tool_call_id":      toolCallID,
		"tool_name":         "ask_followup_question",
		"status":            "success",
		"label":             event.Label,
		"question":          attempt.Question,
		"options":           attempt.Options,
		"allow_freeform":    allowFreeform,
		"arguments_summary": summarizeToolArguments(argumentsJSON),
		"result_summary":    resultSummary,
		"duration_ms":       durationMS,
		"verification":      ToolVerification{Status: "skipped", Message: "question tool does not mutate workspace"},
	})
	r.finalizeActiveAssistantMessage(content)
	emitToolLifecycleEvent(r.opts, "user_question_requested", "info", map[string]any{
		"tool_call_id":   toolCallID,
		"tool_name":      "ask_followup_question",
		"status":         "success",
		"question":       attempt.Question,
		"options":        attempt.Options,
		"allow_freeform": allowFreeform,
		"reason":         attempt.Reason,
		"result_summary": resultSummary,
	})
	emitToolLifecycleEvent(r.opts, "model_message_completed", "info", map[string]any{"content": content})
	return resultPayload, false, true
}

func normalizeFollowupOptions(options []string) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(options))
	for _, option := range options {
		option = strings.TrimSpace(option)
		if option == "" || seen[option] {
			continue
		}
		seen[option] = true
		normalized = append(normalized, truncatePlainRunes(option, 80))
		if len(normalized) >= 4 {
			break
		}
	}
	if len(normalized) == 1 {
		return nil
	}
	return normalized
}
