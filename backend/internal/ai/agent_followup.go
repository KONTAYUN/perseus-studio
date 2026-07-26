package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"webide-backend/internal/store"
)

func (r *agentRuntime) handleUserRequest(toolName, toolCallID, argumentsJSON string) (string, bool, bool) {
	startedAt := time.Now()
	var attempt userRequestAttempt
	if err := json.Unmarshal([]byte(argumentsJSON), &attempt); err != nil {
		message := fmt.Sprintf("invalid %s arguments: %v", toolName, err)
		r.recordToolError(toolName, argumentsJSON, message)
		return r.toolErrorJSON(toolName, message), true, false
	}
	attempt.Question = strings.TrimSpace(attempt.Question)
	if attempt.Question == "" {
		r.recordToolError(toolName, argumentsJSON, "question is required")
		return r.toolErrorJSON(toolName, "question is required"), true, false
	}

	switch toolName {
	case requestUserInputToolName:
		if len(normalizeUserRequestOptions(attempt.Options, 3)) > 0 {
			message := "request_user_input does not accept options; ask for open input directly"
			r.recordToolError(toolName, argumentsJSON, message)
			return r.toolErrorJSON(toolName, message), true, false
		}
		attempt.Options = nil
	case requestUserChoiceToolName:
		attempt.Options = normalizeUserRequestOptions(attempt.Options, 3)
		if len(attempt.Options) < 2 || len(attempt.Options) > 3 {
			message := "options must contain exactly 2 or 3 non-empty, mutually exclusive choices"
			r.recordToolError(toolName, argumentsJSON, message)
			return r.toolErrorJSON(toolName, message), true, false
		}
	case legacyFollowupQuestionToolName:
		attempt.Options = normalizeUserRequestOptions(attempt.Options, 4)
		if len(attempt.Options) == 0 {
			message := "options must contain between 1 and 4 non-empty suggestions"
			r.recordToolError(toolName, argumentsJSON, message)
			return r.toolErrorJSON(toolName, message), true, false
		}
	default:
		message := "unsupported user request tool"
		r.recordToolError(toolName, argumentsJSON, message)
		return r.toolErrorJSON(toolName, message), true, false
	}

	statusKind := manifestStatusKind(toolName)
	emitToolLifecycleEvent(r.opts, "tool_call_started", "info", map[string]any{
		"tool_call_id": toolCallID, "tool_name": toolName, "status_kind": statusKind,
		"status": "running", "arguments_summary": summarizeToolArguments(argumentsJSON),
	})
	if r.service != nil && r.service.calls != nil {
		recorded, err := r.service.calls.StartToolCall(context.Background(), store.ToolCallInput{
			ID: toolCallID, UserID: r.opts.UserID, RunID: r.opts.RunID,
			ConversationID: r.conversationID, MessageID: r.messageID, TaskID: r.opts.TaskID,
			ToolName: toolName, ArgumentsJSON: ledgerToolArgumentsJSON(argumentsJSON), StartedAt: startedAt,
		})
		if err != nil {
			r.recordToolError(toolName, argumentsJSON, "local tool ledger start failed: "+err.Error())
			diagID := r.service.recordLocalPersistenceDiagnostic(context.Background(), r.opts, r.conversationID, r.messageID, "tool_ledger_start_failed", err)
			emitToolLifecycleEvent(r.opts, "local_persistence_failed", "error", map[string]any{
				"tool_call_id": toolCallID, "tool_name": toolName,
				"stage": "tool_ledger_start_failed", "error": err.Error(), "diagnostic_log_id": diagID,
			})
			return r.toolErrorJSON(toolName, "local tool ledger start failed: "+err.Error()), true, false
		}
		if recorded.ID != "" {
			toolCallID = recorded.ID
		}
	}

	r.completedContent = attempt.Question
	r.terminalState = AgentTerminationWaitingForUser
	event := ToolEvent{
		Type: userRequestEventName(toolName), ToolCallID: toolCallID, ToolName: toolName,
		StatusKind: statusKind, Label: userFriendlyToolLabel(toolName), Status: "waiting_for_user",
		Question: attempt.Question, Options: attempt.Options, DurationMS: time.Since(startedAt).Milliseconds(),
	}
	r.events = append(r.events, event)
	r.finalizeActiveAssistantMessage(attempt.Question)
	emitToolLifecycleEvent(r.opts, userRequestEventName(toolName), "info", map[string]any{
		"tool_call_id": toolCallID, "tool_name": toolName, "status_kind": statusKind,
		"status": "waiting_for_user", "question": attempt.Question, "options": attempt.Options,
		"message_kind": userRequestMessageKind(toolName), "result_summary": "waiting for the user's response",
	})
	emitToolLifecycleEvent(r.opts, "model_message_completed", "info", map[string]any{"content": attempt.Question})
	return mustJSON(map[string]any{
		"status": "waiting_for_user", "tool_name": toolName, "message_kind": userRequestMessageKind(toolName),
		"question": attempt.Question, "options": attempt.Options,
	}), false, true
}

func (r *agentRuntime) handleFollowupQuestion(toolCallID, argumentsJSON string) (string, bool, bool) {
	return r.handleUserRequest(legacyFollowupQuestionToolName, toolCallID, argumentsJSON)
}

func userRequestEventName(toolName string) string {
	switch toolName {
	case requestUserInputToolName:
		return "user_input_requested"
	case requestUserChoiceToolName:
		return "user_choice_requested"
	default:
		return "user_question_requested"
	}
}

func normalizeUserRequestOptions(options []string, limit int) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(options))
	for _, option := range options {
		option = strings.TrimSpace(option)
		if option == "" || seen[option] {
			continue
		}
		seen[option] = true
		normalized = append(normalized, truncatePlainRunes(option, 120))
		if limit > 0 && len(normalized) >= limit {
			break
		}
	}
	return normalized
}

func normalizeFollowupOptions(options []string) []string {
	return normalizeUserRequestOptions(options, 4)
}
