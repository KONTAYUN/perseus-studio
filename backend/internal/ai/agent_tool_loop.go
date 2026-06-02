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
					"status_kind":  manifestStatusKind(skipped.Name),
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
		if isError && r.shouldStopAfterToolError() {
			results := append([]TranscriptToolResult(nil), results...)
			r.appendToolResults(results)
			if err := r.service.saveRunCheckpoint(ctx, r.opts, r.checkpoint()); err != nil {
				return nil, false, err
			}
			emitToolLifecycleEvent(r.opts, "tool_error_loop_stopped", "error", map[string]any{
				"reason":                    "too many consecutive tool errors",
				"consecutive_tool_errors":   r.consecutiveToolErrors,
				"repeated_tool_error_count": r.repeatedToolErrorCount,
				"tool_name":                 call.Name,
				"status_kind":               manifestStatusKind(call.Name),
				"tool_call_id":              call.ID,
				"arguments_summary":         summarizeToolArguments(executionToolArgumentsJSON(call)),
			})
			return nil, false, fmt.Errorf("%s", r.repeatedToolErrorMessage())
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
		StatusKind: manifestStatusKind(call.Name),
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
	if errText := r.normalizeAndValidateToolCall(&call); errText != "" {
		r.recordToolError(call.Name, executionToolArgumentsJSON(call), errText)
		event := r.invalidToolCallEvent(ctx, call, errText)
		r.events = append(r.events, event)
		return r.toolErrorJSON(call.Name, errText), true, nil, false, nil
	}
	if r.allowTask && call.Name == "start_background_task" {
		r.requiresCompletion = true
		request, err := parseBackgroundTaskRequest(call.ArgumentsJSON, lastUserMessageContent(r.messages))
		if err != nil {
			r.recordToolError(call.Name, executionToolArgumentsJSON(call), err.Error())
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
		r.recordToolError(call.Name, executionToolArgumentsJSON(call), errText)
		emitToolLifecycleEvent(r.opts, "tool_scope_rejected", "warn", map[string]any{
			"tool_call_id": call.ID,
			"tool_name":    call.Name,
			"status_kind":  manifestStatusKind(call.Name),
			"reason":       errText,
		})
		return r.toolErrorJSON(call.Name, errText), true, nil, false, nil
	}
	if errText := r.validateChunkWriteToolCall(call); errText != "" {
		r.recordToolError(call.Name, executionToolArgumentsJSON(call), errText)
		event := r.invalidToolCallEvent(ctx, call, errText)
		r.events = append(r.events, event)
		return r.toolErrorJSON(call.Name, errText), true, nil, false, nil
	}
	r.toolLoopActive = true
	if toolRequiresCompletionAfterUse(call.Name) {
		r.requiresCompletion = true
	}
	operationHash := ""
	if isWorkspaceMutationTool(call.Name) {
		operationHash = mutationOperationHash(call.Name, executionToolArgumentsJSON(call))
		if previous, ok := r.appliedMutations[operationHash]; ok {
			result, event := r.alreadyAppliedToolResult(call, previous)
			r.events = append(r.events, event)
			emitToolLifecycleEvent(r.opts, "tool_call_completed", "info", map[string]any{
				"tool_call_id":    call.ID,
				"tool_name":       call.Name,
				"status_kind":     manifestStatusKind(call.Name),
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
				"status_kind":     manifestStatusKind(call.Name),
				"round":           r.currentRound,
				"status":          "success",
				"is_error":        false,
				"operation_hash":  operationHash,
				"already_applied": true,
			})
			return result, false, nil, false, nil
		}
	}
	result, event, execErr := r.service.executeAndRecordTool(ctx, r.conversationID, r.messageID, call.Name, executionToolArgumentsJSON(call), operationHash, r.opts, r.profile)
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
		r.recordChunkWriteSuccess(call, &event)
		result = r.annotateChunkWriteResult(call, result)
	}
	if event.Status == "success" && call.Name == "run_workspace_command" && len(event.ChangedPaths) > 0 {
		r.performedMutation = true
		r.requiresCompletion = true
	}
	if event.Status == "success" {
		r.clearToolErrors()
	} else {
		r.recordToolError(call.Name, executionToolArgumentsJSON(call), event.Error)
	}
	emitToolLifecycleEvent(r.opts, "tool_result_returned", eventLevel(event.Status), map[string]any{
		"tool_call_id": call.ID,
		"tool_name":    call.Name,
		"status_kind":  manifestStatusKind(call.Name),
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
	if previous.StatusKind == "" {
		previous.StatusKind = manifestStatusKind(call.Name)
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

func (r *agentRuntime) invalidToolCallEvent(ctx context.Context, call agentToolCall, message string) ToolEvent {
	argumentsJSON := executionToolArgumentsJSON(call)
	targetSummary := toolTargetSummary(call.Name, argumentsJSON)
	err := invalidToolArgumentsError(message)
	if r.service != nil {
		r.service.recordToolDiagnostic(ctx, r.opts, r.conversationID, r.messageID, call.Name, argumentsJSON, targetSummary, 0, err)
	}
	event := ToolEvent{
		ToolCallID:    call.ID,
		ToolName:      call.Name,
		StatusKind:    manifestStatusKind(call.Name),
		Label:         userFriendlyToolLabel(call.Name),
		Status:        "failed",
		TargetSummary: targetSummary,
		Error:         err.Error(),
	}
	if event.Label == "" {
		event.Label = "工具参数无效"
	}
	emitToolLifecycleEvent(r.opts, "tool_call_failed", "warn", map[string]any{
		"tool_call_id":      call.ID,
		"tool_name":         call.Name,
		"status_kind":       manifestStatusKind(call.Name),
		"status":            "failed",
		"label":             event.Label,
		"error":             err.Error(),
		"arguments_summary": summarizeToolArguments(argumentsJSON),
		"target_summary":    targetSummary,
		"recovery_hint":     toolFailureRecoveryHint(call.Name, err.Error()),
		"verification":      ToolVerification{Status: "skipped", Message: "tool arguments failed validation before execution"},
	})
	return event
}

func (r *agentRuntime) validateToolCall(call agentToolCall) string {
	_, errText := r.normalizedToolArguments(call)
	return errText
}

func (r *agentRuntime) normalizeAndValidateToolCall(call *agentToolCall) string {
	if call == nil {
		return "tool call is required"
	}
	argumentsJSON, errText := r.normalizedToolArguments(*call)
	if errText != "" {
		return errText
	}
	call.ArgumentsJSON = argumentsJSON
	return ""
}

func (r *agentRuntime) normalizedToolArguments(call agentToolCall) (string, string) {
	if strings.TrimSpace(call.Name) == "" {
		return "", "tool name is required"
	}
	argumentsJSON := strings.TrimSpace(executionToolArgumentsJSON(call))
	if !r.isKnownTool(call.Name) {
		return "", fmt.Sprintf("unknown or unavailable tool %q", call.Name)
	}
	if call.Name == "apply_workspace_patch" {
		if strings.TrimSpace(call.PatchText) == "" {
			return "", "apply_workspace_patch is an internal assistant text patch channel; output a complete *** Begin Patch / *** End Patch patch block instead of calling it as a JSON tool"
		}
		argumentsJSON := strings.TrimSpace(executionToolArgumentsJSON(call))
		var decoded any
		if err := json.Unmarshal([]byte(argumentsJSON), &decoded); err != nil {
			return "", "tool arguments must be valid JSON: " + err.Error()
		}
		return argumentsJSON, ""
	}
	schema := toolInputSchema(call.Name, r.tools())
	if len(schema) == 0 && isLegacyWorkspaceMutationTool(call.Name) {
		schema = legacyWorkspaceMutationToolInputSchema(call.Name)
	}
	if argumentsJSON == "" {
		if toolSchemaAllowsEmptyArguments(schema) {
			argumentsJSON = "{}"
		} else {
			return "", "tool arguments are required and must match the tool schema"
		}
	}
	var decoded any
	if err := json.Unmarshal([]byte(argumentsJSON), &decoded); err != nil {
		return "", "tool arguments must be valid JSON: " + err.Error()
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		return "", "tool arguments must be a JSON object"
	}
	if errText := validateToolArgumentsAgainstSchemaMap(object, schema); errText != "" {
		return "", errText
	}
	return argumentsJSON, ""
}

func (r *agentRuntime) isKnownTool(name string) bool {
	if name == "apply_workspace_patch" {
		return true
	}
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
	return isLegacyWorkspaceMutationTool(name)
}

func isLegacyWorkspaceMutationTool(name string) bool {
	return manifestToolIsLegacyCompat(name) && manifestToolIsMutation(name)
}

func legacyWorkspaceMutationToolInputSchema(name string) map[string]any {
	return workspaceToolInputSchemaByName(name)
}
