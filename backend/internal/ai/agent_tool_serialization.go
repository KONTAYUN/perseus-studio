package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Only one tool call runs per model turn. Calls beyond the first are never
// dropped silently: the model is told which calls did not run. When the call
// that would run deletes a path, none of them run: such a batch is usually one
// plan (delete, then recreate with new content) and running only the delete
// would leave the user's file in the trash.

const serializedToolArgumentsSummaryRunes = 160

func (r *agentRuntime) processSerializedToolCalls(ctx context.Context, calls, skipped []agentToolCall) (*BackgroundTaskRequest, bool, error) {
	if shouldHoldToolBatch(calls, skipped) {
		return nil, false, r.holdMutatingToolBatch(ctx, calls, skipped)
	}
	request, completed, err := r.processToolCalls(ctx, calls)
	if err != nil || request != nil || completed || len(skipped) == 0 {
		return request, completed, err
	}
	r.transcript.AppendRuntimeObservation(skippedToolCallsObservation(calls, skipped), map[string]any{
		"reason":           "tool_calls_serialized",
		"observation_type": "tool_calls_not_executed",
		"skipped_tools":    toolCallNames(skipped),
	})
	return nil, false, r.service.saveRunCheckpoint(ctx, r.opts, r.checkpoint())
}

func (r *agentRuntime) holdMutatingToolBatch(ctx context.Context, calls, skipped []agentToolCall) error {
	calls = normalizeAgentToolCallIDs(calls)
	all := append(append([]agentToolCall(nil), calls...), skipped...)
	message := heldToolBatchMessage(all)
	results := make([]TranscriptToolResult, 0, len(calls))
	for _, call := range calls {
		event := ToolEvent{
			ToolCallID: call.ID,
			ToolName:   call.Name,
			StatusKind: manifestStatusKind(call.Name),
			Status:     "failed",
			Label:      "工具未执行",
			Error:      message,
		}
		r.events = append(r.events, event)
		results = append(results, TranscriptToolResult{
			ToolCallID: call.ID,
			ToolName:   call.Name,
			Content: mustJSON(map[string]any{
				"status":           "not_executed",
				"reason":           "multiple_tool_calls_with_file_change",
				"files_changed":    false,
				"message":          message,
				"unexecuted_calls": describeToolCalls(all),
			}),
			IsError: true,
			Event:   event,
		})
		emitToolLifecycleEvent(r.opts, "tool_result_returned", "warn", map[string]any{
			"tool_call_id": call.ID,
			"tool_name":    call.Name,
			"status_kind":  manifestStatusKind(call.Name),
			"round":        r.currentRound,
			"status":       "failed",
			"is_error":     true,
			"reason":       "multiple_tool_calls_with_file_change",
		})
	}
	r.appendToolResults(results)
	return r.service.saveRunCheckpoint(ctx, r.opts, r.checkpoint())
}

func shouldHoldToolBatch(calls, skipped []agentToolCall) bool {
	if len(skipped) == 0 {
		return false
	}
	for _, call := range calls {
		if toolCallDeletesWorkspacePath(call) {
			return true
		}
	}
	return false
}

func toolCallDeletesWorkspacePath(call agentToolCall) bool {
	var args struct {
		Patch  string `json:"patch"`
		Action string `json:"action"`
	}
	if err := json.Unmarshal([]byte(executionToolArgumentsJSON(call)), &args); err != nil {
		return false
	}
	switch call.Name {
	case "apply_workspace_patch":
		patch, err := parseWorkspacePatch(args.Patch)
		if err != nil {
			return strings.Contains(args.Patch, "*** Delete File: ")
		}
		for _, op := range patch.Operations {
			if op.Kind == workspacePatchDelete {
				return true
			}
		}
	case "manage_workspace_path":
		return strings.EqualFold(strings.TrimSpace(args.Action), "trash")
	}
	return false
}

func describeToolCalls(calls []agentToolCall) []string {
	items := make([]string, 0, len(calls))
	for index, call := range calls {
		summary := truncate(summarizeToolArguments(executionToolArgumentsJSON(call)), serializedToolArgumentsSummaryRunes)
		items = append(items, fmt.Sprintf("%d. %s %s", index+1, call.Name, summary))
	}
	return items
}

func heldToolBatchMessage(calls []agentToolCall) string {
	return fmt.Sprintf("You returned %d tool calls in one turn, but only one tool call runs per turn. Because the call that would run deletes a path, none of the calls were executed and no files changed. Send the calls again one per turn, in order. To rewrite or reformat a whole file, use a single apply_workspace_patch with *** Replace File: path; never delete a file in order to rewrite it.", len(calls))
}

func skippedToolCallsObservation(executed, skipped []agentToolCall) string {
	return fmt.Sprintf("Only the first tool call of your last turn (%s) was executed; one tool call runs per turn. These %d calls were NOT executed:\n%s\nCall them again one per turn if they are still needed.",
		strings.Join(toolCallNames(executed), ", "), len(skipped), strings.Join(describeToolCalls(skipped), "\n"))
}
