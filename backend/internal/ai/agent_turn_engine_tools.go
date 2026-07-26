package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func (r *agentRuntime) executeTurnEngineToolCalls(ctx context.Context, calls []agentToolCall, repeats *agentTurnRepeatDetector) (*BackgroundTaskRequest, bool, error) {
	calls = normalizeAgentToolCallIDs(calls)
	_, maxToolCalls := r.turnEngineLimits()
	for callIndex, call := range calls {
		if r.toolCalls >= maxToolCalls {
			for _, skipped := range calls[callIndex:] {
				result, event := r.toolBudgetExhaustedResult(skipped, maxToolCalls)
				r.events = append(r.events, event)
				r.appendTurnEngineToolResult(skipped, event, true, result)
			}
			if err := r.service.saveRunCheckpoint(ctx, r.opts, r.checkpoint()); err != nil {
				return nil, false, err
			}
			return nil, false, nil
		}
		if repeats != nil && !repeats.allow(call) {
			result, event := r.turnEngineUnsupportedToolResult(call, "the same tool call repeated too many times; provide a different action or ask the user for guidance")
			r.appendTurnEngineToolResult(call, event, true, result)
			r.appendTurnEngineAbortedToolCalls(calls[callIndex+1:], "run stopped after repeated identical tool call")
			if err := r.service.saveRunCheckpoint(ctx, r.opts, r.checkpoint()); err != nil {
				return nil, false, err
			}
			return nil, false, r.failTurnEngineRepeat(call)
		}

		r.toolCalls++
		eventCountBefore := len(r.events)
		r.appendKernelToolCallItem(call)
		result, isError, request, completed, err := r.processOneToolCall(ctx, call)
		waitingForUser := completed && r.terminalState == AgentTerminationWaitingForUser

		event := ToolEvent{}
		if len(r.events) > eventCountBefore {
			event = r.events[len(r.events)-1]
		}
		if err != nil {
			if event.ToolName == "" {
				event = r.turnEngineExecutionErrorEvent(call, err)
			}
			result = firstNonEmpty(result, r.toolErrorJSON(call.Name, err.Error()))
			isError = true
		}
		if !waitingForUser {
			r.appendTurnEngineToolResult(call, event, isError, result)
		}
		if err != nil {
			r.appendTurnEngineAbortedToolCalls(calls[callIndex+1:], "run stopped after tool execution error")
		}
		if isError && r.shouldStopAfterToolError() {
			r.appendTurnEngineAbortedToolCalls(calls[callIndex+1:], "run stopped after too many consecutive tool errors")
			emitToolLifecycleEvent(r.opts, "tool_error_loop_stopped", "error", map[string]any{
				"reason":                    "too many consecutive tool errors",
				"consecutive_tool_errors":   r.consecutiveToolErrors,
				"repeated_tool_error_count": r.repeatedToolErrorCount,
				"tool_name":                 call.Name,
				"tool_call_id":              call.ID,
			})
			if checkpointErr := r.service.saveRunCheckpoint(ctx, r.opts, r.checkpoint()); checkpointErr != nil {
				return nil, false, checkpointErr
			}
			return nil, false, newAgentRuntimeError(AgentRuntimeToolErrorLoop, r.repeatedToolErrorMessage(), nil)
		}
		if request != nil || completed {
			r.appendTurnEngineAbortedToolCalls(calls[callIndex+1:], "run stopped after terminal tool result")
		} else if callIndex+1 < len(calls) {
			r.appendTurnEngineAbortedToolCalls(calls[callIndex+1:], "serial tool execution requires the model to reevaluate after the first tool result")
		}
		if checkpointErr := r.service.saveRunCheckpoint(ctx, r.opts, r.checkpoint()); checkpointErr != nil {
			return nil, false, checkpointErr
		}
		if err != nil {
			return nil, false, err
		}
		if request != nil || completed {
			return request, completed, nil
		}
		if callIndex+1 < len(calls) {
			return nil, false, nil
		}
	}
	return nil, false, nil
}

func (r *agentRuntime) appendTurnEngineAbortedToolCalls(calls []agentToolCall, reason string) {
	for index, call := range calls {
		if strings.TrimSpace(call.ID) == "" {
			call.ID = fmt.Sprintf("tool_%d", r.toolCalls+index+1)
		}
		message := strings.TrimSpace(reason)
		if message == "" {
			message = "run stopped before this tool call could execute"
		}
		event := ToolEvent{
			Type:       "tool_call_aborted",
			ToolCallID: call.ID,
			ToolName:   call.Name,
			StatusKind: "warn",
			Label:      firstNonEmpty(userFriendlyToolLabel(call.Name), call.Name),
			Status:     "aborted",
			Error:      message,
		}
		result := mustJSON(map[string]any{
			"status":       "aborted",
			"tool_name":    call.Name,
			"tool_call_id": call.ID,
			"message":      message,
		})
		r.events = append(r.events, event)
		r.appendKernelToolCallItem(call)
		r.appendTurnEngineToolResult(call, event, true, result)
	}
}

func (r *agentRuntime) appendTurnEngineToolResult(call agentToolCall, event ToolEvent, isError bool, result string) {
	if event.ToolCallID == "" {
		event.ToolCallID = call.ID
	}
	if event.ToolName == "" {
		event.ToolName = call.Name
	}
	if event.Status == "" {
		if isError {
			event.Status = "failed"
		} else {
			event.Status = "success"
		}
	}
	r.appendToolResults([]TranscriptToolResult{{
		ToolCallID: call.ID,
		ToolName:   call.Name,
		Content:    result,
		IsError:    isError,
		Event:      event,
	}})
	r.appendKernelToolResultItem(call, event, isError, result)
	if call.Name == "update_task_plan" && !isError {
		r.persistPendingTaskPlan(call.ID)
	}
	r.recordKernelToolProgress(call, isError, event, result)
	emitToolLifecycleEvent(r.opts, "tool_result_returned", eventLevel(event.Status), map[string]any{
		"tool_call_id": call.ID,
		"tool_name":    call.Name,
		"round":        r.currentRound,
		"status":       event.Status,
		"is_error":     isError,
	})
}

func (r *agentRuntime) turnEngineUnsupportedToolResult(call agentToolCall, reason string) (string, ToolEvent) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "tool is not available in this turn protocol"
	}
	r.recordToolError(call.Name, executionToolArgumentsJSON(call), reason)
	event := r.invalidToolCallEvent(context.Background(), call, reason)
	r.events = append(r.events, event)
	return r.toolErrorJSON(call.Name, reason), event
}

func (r *agentRuntime) turnEngineExecutionErrorEvent(call agentToolCall, err error) ToolEvent {
	typed := normalizeToolError(err)
	r.recordTypedToolError(call.Name, executionToolArgumentsJSON(call), typed)
	event := r.invalidToolCallEvent(context.Background(), call, typed.Error())
	event.ErrorCode = typed.Code
	event.ErrorCategory = typed.Category
	event.Retryable = typed.Retryable
	event.RecommendedAction = typed.RecommendedAction
	return event
}

func (r *agentRuntime) failTurnEngineRepeat(call agentToolCall) error {
	message := fmt.Sprintf("agent repeated the same tool call too many times: %s", strings.TrimSpace(call.Name))
	emitToolLifecycleEvent(r.opts, "turn_engine_repeat_blocked", "warn", map[string]any{
		"tool_name": call.Name,
		"round":     r.currentRound,
		"limit":     agentTurnRepeatLimit,
	})
	return fmt.Errorf("%s", message)
}

func (d *agentTurnRepeatDetector) allow(call agentToolCall) bool {
	if d == nil {
		return true
	}
	key := canonicalTurnEngineToolCall(call)
	if key == "" {
		return true
	}
	if key == d.previous {
		d.count++
	} else {
		d.previous = key
		d.count = 0
	}
	if d.count >= agentTurnRepeatLimit {
		d.previous = ""
		d.count = 0
		return false
	}
	return true
}

func canonicalTurnEngineToolCall(call agentToolCall) string {
	arguments := strings.TrimSpace(executionToolArgumentsJSON(call))
	var decoded any
	if arguments != "" && json.Unmarshal([]byte(arguments), &decoded) != nil {
		decoded = arguments
	}
	encoded, err := json.Marshal(map[string]any{
		"name":   strings.TrimSpace(call.Name),
		"params": decoded,
	})
	if err != nil {
		return strings.TrimSpace(call.Name) + "\x00" + arguments
	}
	return string(encoded)
}
