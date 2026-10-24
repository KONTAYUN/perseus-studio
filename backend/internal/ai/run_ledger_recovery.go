package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"webide-backend/internal/store"
)

// A completed ledger entry survives the window between a filesystem write and
// the next runtime checkpoint. An interrupted side effect cannot be retried
// safely merely because its result is missing from the transcript.
func (r *agentRuntime) restoreDurableToolLedger(ctx context.Context) error {
	if r.opts.RunID == "" || r.service.calls == nil {
		return nil
	}
	calls, err := r.service.calls.ListRunToolCalls(ctx, r.opts.UserID, r.opts.RunID)
	if err != nil {
		return fmt.Errorf("读取任务工具账本失败，已停止恢复以避免重复修改：%w", err)
	}
	recorded := map[string]bool{}
	for _, event := range r.events {
		recorded[event.ToolCallID] = true
	}
	for _, call := range calls {
		mutates := isWorkspaceMutationTool(call.ToolName)
		externalEffect := call.ToolName == "run_workspace_command" || call.ToolName == "start_background_task" || call.ToolName == "delete_skill" || call.ToolName == "install_skill_from_attachment"
		if (call.Status == "running" && (mutates || externalEffect)) || (!recorded[call.ID] && (externalEffect || (mutates && call.Status != "success"))) {
			return fmt.Errorf("工具 %s 在上次中断前已开始，但结果尚未确认（%s）。请先核对工作区或命令结果，再开启新一轮；本轮不会盲目重试", call.ToolName, call.ID)
		}
		if call.Status != "success" || !mutates || call.OperationHash == "" {
			continue
		}
		if _, exists := r.appliedMutations[call.OperationHash]; exists {
			continue
		}
		// The user may have edited files since the crash. A replayed successful
		// result must not assert that stale contents are still present.
		if verification := r.service.verifyToolResult(ctx, call.ToolName, call.ResultJSON, r.opts); verification.Status == "failed" {
			return fmt.Errorf("恢复前文件核验失败（%s）：%s。请核对工作区后开启新一轮", call.ID, verification.Message)
		}
		event := toolEventFromLedger(call)
		if event.Operation == "document_repair_staged" || event.Operation == "document_chunk_staged" {
			if !recorded[call.ID] {
				r.events = append(r.events, event)
				if event.Operation == "document_chunk_staged" {
					r.appliedMutations[call.OperationHash] = event
					r.recordChunkWriteSuccess(agentToolCall{Name: call.ToolName, ArgumentsJSON: call.ArgumentsJSON}, &event)
				}
			}
			r.requiresCompletion = true
			continue
		}
		r.appliedMutations[call.OperationHash] = event
		r.events = append(r.events, event)
		r.performedMutation = true
		r.requiresCompletion = true
	}
	return nil
}

func toolEventFromLedger(call store.ToolCall) ToolEvent {
	var event ToolEvent
	_ = json.Unmarshal([]byte(call.ResultJSON), &event)
	event.ToolCallID = call.ID
	event.ToolName = call.ToolName
	event.Status = call.Status
	event.StatusKind = manifestStatusKind(call.ToolName)
	event.OperationHash = call.OperationHash
	if len(event.ChangedPaths) == 0 {
		_ = json.Unmarshal([]byte(call.ChangedPathsJSON), &event.ChangedPaths)
	}
	return event
}
