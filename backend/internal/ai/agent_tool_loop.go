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
		r.appendKernelToolCallItem(call)
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
		r.appendKernelToolResultItem(call, event, isError, resultContent)
		r.recordLoopGuardAction(call, isError, event, resultContent)
		if isError && call.Name == "attempt_completion" && isActionBoundaryObservation(resultContent) {
			r.appendToolResults(results)
			results = results[:0]
			r.transcript.AppendRuntimeObservation(resultContent, map[string]any{
				"reason":           "terminal_action_rejected",
				"observation_type": "action_rejected",
				"rejected_action":  "attempt_completion",
			})
			if err := r.service.saveRunCheckpoint(ctx, r.opts, r.checkpoint()); err != nil {
				return nil, false, err
			}
			continue
		}
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
			return nil, false, newAgentRuntimeError(AgentRuntimeToolErrorLoop, r.repeatedToolErrorMessage(), nil)
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
	previewStarted := false
	previewResolved := false
	defer func() {
		if previewStarted && !previewResolved && r.editPreviews != nil {
			r.editPreviews.revert(call.ID, "tool_not_executed")
		}
	}()
	if errText := r.normalizeAndValidateToolCall(&call); errText != "" {
		if r.editPreviews != nil {
			r.editPreviews.revert(call.ID, "invalid_tool_arguments")
		}
		r.recordToolError(call.Name, executionToolArgumentsJSON(call), errText)
		if call.Name == "apply_workspace_patch" {
			return r.actionRejectedObservation(errText, "apply_workspace_patch_json_tool_call", nil), true, nil, false, nil
		}
		event := r.invalidToolCallEvent(ctx, call, errText)
		r.events = append(r.events, event)
		return r.toolErrorJSON(call.Name, errText), true, nil, false, nil
	}
	if r.allowTask && call.Name == "start_background_task" {
		request, err := parseBackgroundTaskRequest(call.ArgumentsJSON, r.currentUserMessageContent())
		if err != nil {
			r.recordToolError(call.Name, executionToolArgumentsJSON(call), err.Error())
			return r.toolErrorJSON(call.Name, err.Error()), true, nil, false, nil
		}
		r.terminalState = AgentTerminationDelegated
		return "", false, &request, false, nil
	}
	if call.Name == "attempt_completion" {
		content, isError, completed := r.handleCompletionAttempt(call.ArgumentsJSON)
		return content, isError, nil, completed, nil
	}
	if isUserRequestTool(call.Name) {
		content, isError, completed := r.handleUserRequest(call.Name, call.ID, call.ArgumentsJSON)
		return content, isError, nil, completed, nil
	}
	if call.Name == "update_task_plan" {
		content, isError := r.handleTaskPlanUpdate(call.ID, call.ArgumentsJSON)
		return content, isError, nil, false, nil
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
	if preconditionErr := r.validateHTMLNoteMutationPrecondition(ctx, call); preconditionErr != nil {
		typed := normalizeToolError(preconditionErr)
		errText := typed.Error()
		r.recordTypedToolError(call.Name, executionToolArgumentsJSON(call), typed)
		emitToolLifecycleEvent(r.opts, "html_note_write_guard_rejected", "warn", map[string]any{
			"tool_call_id": call.ID,
			"tool_name":    call.Name,
			"status_kind":  manifestStatusKind(call.Name),
			"reason":       errText,
		})
		event := r.invalidToolCallEvent(ctx, call, errText)
		r.events = append(r.events, event)
		return toolErrorJSONPayload(call.Name, typed), true, nil, false, nil
	}
	if preflightErr := r.validateNoteBlockEditExecutionPreflight(ctx, call); preflightErr != nil {
		typed := normalizeToolError(preflightErr)
		typed.Phase = "preflight"
		errText := typed.Error()
		r.recordTypedToolError(call.Name, executionToolArgumentsJSON(call), typed)
		emitToolLifecycleEvent(r.opts, "html_note_edit_preflight_rejected", "warn", map[string]any{
			"tool_call_id": call.ID,
			"tool_name":    call.Name,
			"phase":        typed.Phase,
			"error_code":   typed.Code,
		})
		event := r.invalidToolCallEvent(ctx, call, errText)
		event.ErrorCode = typed.Code
		event.ErrorCategory = typed.Category
		event.ErrorPhase = typed.Phase
		event.Retryable = typed.Retryable
		event.RecommendedAction = typed.RecommendedAction
		r.events = append(r.events, event)
		return toolErrorJSONPayload(call.Name, typed), true, nil, false, nil
	}
	if strategyErr := r.rejectRepeatedFailedStrategy(call); strategyErr != nil {
		r.recordTypedToolError(call.Name, executionToolArgumentsJSON(call), strategyErr)
		event := r.invalidToolCallEvent(ctx, call, strategyErr.Error())
		typed := normalizeToolError(strategyErr)
		event.ErrorCode = typed.Code
		event.ErrorCategory = typed.Category
		event.Retryable = typed.Retryable
		event.RecommendedAction = typed.RecommendedAction
		r.events = append(r.events, event)
		return toolErrorJSONPayload(call.Name, typed), true, nil, false, nil
	}
	if r.editPreviews != nil {
		r.editPreviews.handle(ctx, ToolCallArgumentsDelta{
			ToolCallID: call.ID, ToolName: call.Name, ArgumentsJSON: executionToolArgumentsJSON(call),
		}, true)
		previewStarted = true
	}
	// The operation hash is diagnostic only. Exact transport replay is keyed by
	// run_id + tool_call_id in executeAndRecordTool.
	operationHash := mutationOperationHash(call.Name, executionToolArgumentsJSON(call))
	execOpts := r.opts
	execOpts.CurrentToolCallID = call.ID
	result, event, execErr := r.service.executeAndRecordTool(ctx, r.conversationID, r.messageID, call.Name, executionToolArgumentsJSON(call), operationHash, execOpts, r.profile)
	previewResolved = true
	if r.editPreviews != nil {
		if event.Status == "success" {
			r.editPreviews.complete(call.ID, event)
		} else {
			r.editPreviews.revert(call.ID, firstNonEmpty(string(event.ErrorCode), "tool_failed"))
		}
	}
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
		r.recordChunkWriteSuccess(call, &event)
		result = r.annotateChunkWriteResult(call, result)
	}
	if event.Status == "success" && call.Name == "run_workspace_command" && len(event.ChangedPaths) > 0 {
		r.performedMutation = true
	}
	if event.Status == "success" {
		r.recordStrategySuccess(call, event)
		r.clearToolErrors()
	} else {
		r.recordTypedToolError(call.Name, executionToolArgumentsJSON(call), toolErrorFromEvent(event))
		r.recordConditionSatisfiedFromToolResult(call, event, result)
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
		"recovery_hint":     toolFailureRecoveryHint(err),
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
		var decoded map[string]any
		if err := json.Unmarshal([]byte(argumentsJSON), &decoded); err != nil {
			return "", "tool arguments must be valid JSON: " + err.Error()
		}
		if strings.TrimSpace(stringFromAny(decoded["patch"])) == "" {
			return "", "patch is required"
		}
		decoded["transport"] = "structured_tool"
		raw, _ := json.Marshal(decoded)
		return string(raw), ""
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
