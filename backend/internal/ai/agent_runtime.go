package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"webide-backend/internal/idgen"
)

func (r *agentRuntime) run(ctx context.Context) (Reply, error) {
	for {
		maxRounds, maxToolCalls := r.loopLimits()
		if r.currentRound >= maxRounds {
			return r.handleBudgetExhausted(ctx, "max_rounds", maxRounds, maxToolCalls)
		}
		if r.toolCalls >= maxToolCalls {
			return r.handleBudgetExhausted(ctx, "max_tool_calls", maxRounds, maxToolCalls)
		}
		round := r.currentRound
		r.currentRound = round + 1
		turn, err := r.callModelWithRecovery(ctx, round)
		if err != nil {
			return Reply{}, err
		}
		r.lastProviderFinishReason = turn.FinishReason
		if len(turn.ToolCalls) == 0 {
			if !r.requiresToolThisTurn() {
				r.commitAssistantContent(turn)
			}
			if reply, ok, err := r.handleNoToolTurn(ctx, turn); ok || err != nil {
				if err == nil && ok {
					r.emitCheckpointRecoveryCompleted()
				}
				return reply, err
			}
			continue
		}
		turn.ToolCalls = normalizeAgentToolCallIDs(turn.ToolCalls)
		if pending, err := r.hasPendingSteers(ctx); err != nil {
			return Reply{}, err
		} else if pending {
			emitToolLifecycleEvent(r.opts, "agent_steer_deferred_stale_turn", "info", map[string]any{
				"round":         round + 1,
				"tool_calls":    len(turn.ToolCalls),
				"message":       "new user guidance arrived before tool execution; stale tool turn skipped",
				"recovery_hint": "apply pending steer before executing model-planned tools",
				"safe_boundary": "before_tool_execution",
			})
			continue
		}
		var skippedToolCalls int
		turn, skippedToolCalls = prepareToolTurnForExecution(turn)
		if skippedToolCalls > 0 {
			emitToolLifecycleEvent(r.opts, "tool_calls_serialized", "info", map[string]any{
				"round":         round + 1,
				"executed":      1,
				"skipped":       skippedToolCalls,
				"message":       "model returned multiple tool calls; executing one before asking for the next step",
				"safe_boundary": "after_each_tool",
			})
		}
		r.commitAssistantContent(turn)
		if r.opts.OnAssistantToolBoundary != nil {
			r.opts.OnAssistantToolBoundary()
		}
		if strings.TrimSpace(turn.Content) != "" {
			r.finalizeActiveAssistantMessage(turn.Content)
		}
		r.invalidTurns = 0
		r.lastProviderEmptyResponse = false
		r.appendAssistantTurn(turn)

		if request, completed, err := r.processToolCalls(ctx, turn.ToolCalls); request != nil || completed || err != nil {
			if err != nil {
				return Reply{}, err
			}
			if request != nil {
				return Reply{BackgroundTask: request, ToolEvents: r.events}, nil
			}
			if strings.TrimSpace(r.completedContent) != "" {
				r.emitCheckpointRecoveryCompleted()
				return Reply{Content: r.completedContent, ToolEvents: r.events}, nil
			}
		}
	}
}

func (r *agentRuntime) loopLimits() (int, int) {
	if r.opts.TaskID != "" {
		return agentTaskMaxRounds, agentTaskMaxToolCalls
	}
	if r.requiresCompletion || r.toolLoopActive || r.opts.ForceTool != "" {
		return agentWorkspaceMaxRounds, agentWorkspaceMaxToolCalls
	}
	return agentChatMaxRounds, agentChatMaxToolCalls
}

func (r *agentRuntime) commitAssistantContent(turn agentModelTurn) {
	content := strings.TrimSpace(turn.Content)
	if content == "" {
		return
	}
	if !turn.PublicTextStreamed {
		emitToolLifecycleEvent(r.opts, "model_delta", "info", map[string]any{"text": turn.Content})
	}
	r.ensureActiveAssistantMessage(turn.Content)
	r.appendActiveAssistantDelta(turn.Content)
}

func (r *agentRuntime) appendAssistantTurn(turn agentModelTurn) {
	r.transcript.AppendAssistantTurn(turn)
}

func (r *agentRuntime) appendToolResults(results []TranscriptToolResult) {
	r.transcript.AppendToolResults(results)
}

func (r *agentRuntime) ensureActiveAssistantMessage(content string) {
	content = strings.TrimSpace(content)
	if content == "" || r.activeAssistantMessageID != "" || r.opts.OnAssistantMessageStarted == nil {
		return
	}
	r.activeAssistantSegment++
	r.activeAssistantMessageID = idgen.New("active_msg")
	r.opts.OnAssistantMessageStarted(r.activeAssistantMessageID, content, r.activeAssistantSegment)
}

func (r *agentRuntime) appendActiveAssistantDelta(delta string) {
	if strings.TrimSpace(delta) == "" || r.activeAssistantMessageID == "" || r.opts.OnAssistantMessageDelta == nil {
		return
	}
	r.opts.OnAssistantMessageDelta(r.activeAssistantMessageID, delta)
}

func (r *agentRuntime) finalizeActiveAssistantMessage(content string) {
	content = strings.TrimSpace(content)
	if r.activeAssistantMessageID == "" || r.opts.OnAssistantMessageCompleted == nil {
		return
	}
	r.opts.OnAssistantMessageCompleted(r.activeAssistantMessageID, content, r.activeAssistantSegment)
	r.activeAssistantMessageID = ""
}

func (r *agentRuntime) correctiveMessage(content string) string {
	if strings.TrimSpace(content) == "" {
		return "你的上一轮响应没有文本也没有工具调用。当前任务需要通过工具执行并验证；请调用合适的 workspace/search/command/skill 工具，完成后调用 attempt_completion。"
	}
	if r.toolLoopActive {
		return "你的上一轮响应只是普通文本，没有继续调用工具，也没有调用 attempt_completion。当前 run 已经在工具工作流中：如果还需要继续处理，请直接调用下一个合适的工具；如果工具工作已经完成，请调用 attempt_completion 交付结果。"
	}
	return "你的上一轮响应没有调用工具，因此不能完成当前任务。请根据用户请求调用必要工具；工具失败会以 tool_result 返回给你，修正后继续。所有需要改变 workspace 的任务完成前必须调用 attempt_completion。"
}

func (r *agentRuntime) fail(message string, cause error) (Reply, error) {
	err := errors.New(message)
	if cause != nil {
		err = fmt.Errorf("%s: %w", message, cause)
	}
	log.Printf("[ai] agent runtime failed conversation=%s provider=%s model=%s finish_reason=%s empty=%v error=%v", r.conversationID, r.provider.Name, r.provider.ModelID, r.lastProviderFinishReason, r.lastProviderEmptyResponse, err)
	emitToolLifecycleEvent(r.opts, "run_failed", "error", map[string]any{
		"error":         err.Error(),
		"provider":      r.provider.Name,
		"model":         r.provider.ModelID,
		"finish_reason": r.lastProviderFinishReason,
		"stage":         "agent_loop",
	})
	return Reply{}, err
}

func (r *agentRuntime) recordToolError(toolName, argumentsJSON, message string) {
	r.lastToolFailed = true
	r.consecutiveToolErrors++
	key := repeatedToolErrorKey(toolName, argumentsJSON, message)
	if key != "" && key == r.repeatedToolErrorKey {
		r.repeatedToolErrorCount++
	} else {
		r.repeatedToolErrorKey = key
		r.repeatedToolErrorCount = 1
	}
	if r.consecutiveToolErrors >= agentMaxConsecutiveToolErrs {
		emitToolLifecycleEvent(r.opts, "completion_rejected", "warn", map[string]any{"reason": "too many consecutive tool errors"})
	}
}

func (r *agentRuntime) clearToolErrors() {
	r.lastToolFailed = false
	r.consecutiveToolErrors = 0
	r.repeatedToolErrorKey = ""
	r.repeatedToolErrorCount = 0
}

func (r *agentRuntime) shouldStopAfterToolError() bool {
	return r.consecutiveToolErrors >= agentMaxConsecutiveToolErrs ||
		r.repeatedToolErrorCount >= agentMaxConsecutiveToolErrs
}

func repeatedToolErrorKey(toolName, argumentsJSON, message string) string {
	toolName = strings.TrimSpace(toolName)
	message = strings.TrimSpace(message)
	if toolName == "" || message == "" {
		return ""
	}
	return toolName + "\x00" + mutationOperationHash(toolName, argumentsJSON) + "\x00" + message
}

func (r *agentRuntime) repeatedToolErrorMessage() string {
	if r.repeatedToolErrorCount >= agentMaxConsecutiveToolErrs {
		return fmt.Sprintf("同一个工具调用连续失败 %d 次，已停止继续重试，避免重复写入错误日志。请检查模型供应商的工具调用参数输出是否完整。", r.repeatedToolErrorCount)
	}
	return fmt.Sprintf("工具调用连续失败 %d 次，已停止继续重试，避免重复写入错误日志。请检查模型供应商的工具调用参数输出是否完整。", r.consecutiveToolErrors)
}

func (r *agentRuntime) toolErrorJSON(toolName, message string) string {
	return mustJSON(map[string]any{
		"error":         message,
		"tool_name":     toolName,
		"recovery_hint": "Fix the tool arguments or choose a valid available tool, then continue. Do not claim success until a tool succeeds and completion is accepted.",
	})
}

func mutationOperationHash(toolName, argumentsJSON string) string {
	var decoded any
	if err := json.Unmarshal([]byte(argumentsJSON), &decoded); err != nil {
		decoded = strings.TrimSpace(argumentsJSON)
	}
	canonical, err := json.Marshal(decoded)
	if err != nil {
		canonical = []byte(strings.TrimSpace(argumentsJSON))
	}
	sum := sha256.Sum256([]byte(toolName + "\x00" + string(canonical)))
	return hex.EncodeToString(sum[:])
}

func userFriendlyToolLabel(toolName string) string {
	switch toolName {
	case "find_workspace_files", "search_workspace_text", "inspect_workspace":
		return "已查看工作区"
	case "read_workspace_text", "read_workspace_files":
		return "已读取文件"
	case "edit_workspace_lines", "edit_workspace_text", "replace_workspace_lines":
		return "已编辑文件"
	case "write_workspace_text", "save_workspace_file", "write_workspace_chunk":
		return "已保存文件"
	case "manage_workspace_path":
		return "已管理路径"
	case "organize_workspace_matches":
		return "已整理文件"
	case "run_workspace_command":
		return "已运行命令"
	case "perseus_search_web", "web_search":
		return "已搜索资料"
	case "web_extract":
		return "已读取网页"
	case "list_skills", "read_skill", "install_skill_from_attachment", "delete_skill":
		return "已处理技能"
	case "ask_followup_question":
		return "已询问用户"
	default:
		return "已处理"
	}
}

func toolRequiresCompletionAfterUse(name string) bool {
	switch name {
	case "install_skill_from_attachment", "delete_skill", "run_workspace_command", "start_background_task":
		return true
	default:
		return isWorkspaceMutationTool(name)
	}
}
