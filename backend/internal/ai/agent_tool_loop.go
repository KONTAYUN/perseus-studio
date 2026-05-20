package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func (r *agentRuntime) processToolCalls(ctx context.Context, calls []agentToolCall) (*BackgroundTaskRequest, bool, error) {
	calls = normalizeAgentToolCallIDs(calls)
	results := make([]TranscriptToolResult, 0, len(calls))
	for callIndex, call := range calls {
		r.toolCalls++
		_, maxToolCalls := r.loopLimits()
		if r.toolCalls > maxToolCalls {
			r.toolCalls = maxToolCalls
			for _, skipped := range calls[callIndex:] {
				result, event := r.toolBudgetExhaustedResult(skipped, maxToolCalls)
				r.events = append(r.events, event)
				results = append(results, TranscriptToolResult{
					ToolCallID: event.ToolCallID,
					ToolName:   skipped.Name,
					Content:    result,
					IsError:    true,
					Event:      event,
				})
				emitToolLifecycleEvent(r.opts, "tool_result_returned", "warn", map[string]any{
					"tool_call_id": skipped.ID,
					"tool_name":    skipped.Name,
					"round":        r.currentRound,
					"status":       "failed",
					"is_error":     true,
					"reason":       IncompleteReasonBudgetExhausted,
				})
			}
			r.appendToolResults(results)
			if err := r.service.saveRunCheckpoint(ctx, r.opts, r.checkpoint()); err != nil {
				return nil, false, err
			}
			return nil, false, nil
		}
		eventCountBefore := len(r.events)
		resultContent, isError, request, completed, err := r.processOneToolCall(ctx, call)
		if err != nil || request != nil || completed {
			return request, completed, err
		}
		event := ToolEvent{}
		if len(r.events) > eventCountBefore {
			event = r.events[len(r.events)-1]
		}
		results = append(results, TranscriptToolResult{
			ToolCallID: call.ID,
			ToolName:   call.Name,
			Content:    resultContent,
			IsError:    isError,
			Event:      event,
		})
		if event.Status == "success" {
			r.appendToolResults(results)
			results = results[:0]
			if err := r.service.saveRunCheckpoint(ctx, r.opts, r.checkpoint()); err != nil {
				return nil, false, err
			}
		}
	}
	r.appendToolResults(results)
	if err := r.service.saveRunCheckpoint(ctx, r.opts, r.checkpoint()); err != nil {
		return nil, false, err
	}
	return nil, false, nil
}

func (r *agentRuntime) toolBudgetExhaustedResult(call agentToolCall, maxToolCalls int) (string, ToolEvent) {
	if call.ID == "" {
		call.ID = fmt.Sprintf("tool_%d", r.toolCalls)
	}
	message := "Tool call was not executed because the agent reached its tool-call budget. The run will stop and summarize progress."
	payload := map[string]any{
		"status":         IncompleteReasonBudgetExhausted,
		"tool_name":      call.Name,
		"tool_call_id":   call.ID,
		"max_tool_calls": maxToolCalls,
		"message":        message,
	}
	event := ToolEvent{
		ToolCallID: call.ID,
		ToolName:   call.Name,
		Status:     "failed",
		Label:      userFriendlyToolLabel(call.Name),
		Error:      message,
	}
	if event.Label == "" {
		event.Label = "工具未执行"
	}
	return mustJSON(payload), event
}

func (r *agentRuntime) processOneToolCall(ctx context.Context, call agentToolCall) (string, bool, *BackgroundTaskRequest, bool, error) {
	if call.ID == "" {
		call.ID = fmt.Sprintf("tool_%d", r.toolCalls)
	}
	if errText := r.validateToolCall(call); errText != "" {
		r.recordToolError()
		return r.toolErrorJSON(call.Name, errText), true, nil, false, nil
	}
	if r.allowTask && call.Name == "start_background_task" {
		r.requiresCompletion = true
		request, err := parseBackgroundTaskRequest(call.ArgumentsJSON, lastUserMessageContent(r.messages))
		if err != nil {
			r.recordToolError()
			return r.toolErrorJSON(call.Name, err.Error()), true, nil, false, nil
		}
		return "", false, &request, false, nil
	}
	if call.Name == "attempt_completion" {
		content, isError, completed := r.handleCompletionAttempt(call.ArgumentsJSON)
		return content, isError, nil, completed, nil
	}
	if call.Name == "ask_followup_question" {
		content, isError, completed := r.handleFollowupQuestion(call.ID, call.ArgumentsJSON)
		return content, isError, nil, completed, nil
	}
	if errText := r.validateSelectedReferenceScope(call); errText != "" {
		r.recordToolError()
		emitToolLifecycleEvent(r.opts, "tool_scope_rejected", "warn", map[string]any{
			"tool_call_id": call.ID,
			"tool_name":    call.Name,
			"reason":       errText,
		})
		return r.toolErrorJSON(call.Name, errText), true, nil, false, nil
	}
	r.toolLoopActive = true
	if toolRequiresCompletionAfterUse(call.Name) {
		r.requiresCompletion = true
	}
	operationHash := ""
	if isWorkspaceMutationTool(call.Name) {
		operationHash = mutationOperationHash(call.Name, call.ArgumentsJSON)
		if previous, ok := r.appliedMutations[operationHash]; ok {
			result, event := r.alreadyAppliedToolResult(call, previous)
			r.events = append(r.events, event)
			emitToolLifecycleEvent(r.opts, "tool_call_completed", "info", map[string]any{
				"tool_call_id":    call.ID,
				"tool_name":       call.Name,
				"round":           r.currentRound,
				"status":          "success",
				"label":           event.Label,
				"path":            event.Path,
				"old_path":        event.OldPath,
				"new_path":        event.NewPath,
				"changed_paths":   event.ChangedPaths,
				"operation_hash":  operationHash,
				"already_applied": true,
				"result_summary":  "workspace mutation already applied in this run",
				"verification":    ToolVerification{Status: "verified", Message: "already applied earlier in this run", Path: event.Path},
			})
			emitToolLifecycleEvent(r.opts, "tool_result_returned", "info", map[string]any{
				"tool_call_id":    call.ID,
				"tool_name":       call.Name,
				"round":           r.currentRound,
				"status":          "success",
				"is_error":        false,
				"operation_hash":  operationHash,
				"already_applied": true,
			})
			return result, false, nil, false, nil
		}
	}
	result, event, execErr := r.service.executeAndRecordTool(ctx, r.conversationID, r.messageID, call.Name, call.ArgumentsJSON, operationHash, r.opts, r.profile)
	event.OperationHash = operationHash
	r.events = append(r.events, event)
	if execErr != nil {
		return result, true, nil, false, execErr
	}
	if event.Status == "success" && call.Name == "load_perseus_toolset" {
		markLoadedToolset(r.loadedToolsets, call.ArgumentsJSON, r.allowTask)
	}
	if event.Status == "success" && isWorkspaceMutationTool(call.Name) {
		r.performedMutation = true
		if operationHash != "" {
			r.appliedMutations[operationHash] = event
		}
	}
	if event.Status == "success" {
		r.lastToolFailed = false
		r.consecutiveToolErrors = 0
	} else {
		r.recordToolError()
	}
	emitToolLifecycleEvent(r.opts, "tool_result_returned", eventLevel(event.Status), map[string]any{
		"tool_call_id": call.ID,
		"tool_name":    call.Name,
		"round":        r.currentRound,
		"status":       event.Status,
		"is_error":     event.Status != "success",
	})
	return result, event.Status != "success", nil, false, nil
}

func (r *agentRuntime) alreadyAppliedToolResult(call agentToolCall, previous ToolEvent) (string, ToolEvent) {
	previous.ToolCallID = call.ID
	previous.Status = "success"
	previous.Label = userFriendlyToolLabel(call.Name)
	if previous.Label == "" {
		previous.Label = "已处理"
	}
	if previous.ToolName == "" {
		previous.ToolName = call.Name
	}
	payload := map[string]any{
		"status":          "already_applied",
		"tool_name":       call.Name,
		"operation_hash":  previous.OperationHash,
		"path":            previous.Path,
		"old_path":        previous.OldPath,
		"new_path":        previous.NewPath,
		"changed_paths":   previous.ChangedPaths,
		"version_id":      previous.VersionID,
		"hash":            previous.Hash,
		"message":         "This workspace mutation was already applied in this run. Continue from the verified existing result instead of repeating it.",
		"recovery_hint":   "Do not repeat the same mutation. Use the existing changed_paths and continue, then call attempt_completion when the task is done.",
		"already_applied": true,
	}
	return mustJSON(payload), previous
}

func (r *agentRuntime) validateToolCall(call agentToolCall) string {
	if strings.TrimSpace(call.Name) == "" {
		return "tool name is required"
	}
	if !r.isKnownTool(call.Name) {
		return fmt.Sprintf("unknown or unavailable tool %q", call.Name)
	}
	if strings.TrimSpace(call.ArgumentsJSON) == "" {
		call.ArgumentsJSON = "{}"
	}
	var decoded any
	if err := json.Unmarshal([]byte(call.ArgumentsJSON), &decoded); err != nil {
		return "tool arguments must be valid JSON: " + err.Error()
	}
	if _, ok := decoded.(map[string]any); !ok {
		return "tool arguments must be a JSON object"
	}
	return ""
}

func (r *agentRuntime) isKnownTool(name string) bool {
	for _, tool := range r.tools() {
		switch r.protocol {
		case agentProtocolAnthropic:
			if tool["name"] == name {
				return true
			}
		default:
			fn, _ := tool["function"].(map[string]any)
			if fn["name"] == name {
				return true
			}
		}
	}
	return false
}
