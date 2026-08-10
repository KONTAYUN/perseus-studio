package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"webide-backend/internal/idgen"
)

func (r *agentRuntime) run(ctx context.Context) (Reply, error) {
	return r.runTurnEngine(ctx)
}

func (r *agentRuntime) beginPresentationSegment() {
	r.presentationEventStart = len(r.events)
}

func (r *agentRuntime) presentationEvents() []ToolEvent {
	start := r.presentationEventStart
	if start < 0 || start > len(r.events) {
		start = len(r.events)
	}
	return append([]ToolEvent(nil), r.events[start:]...)
}

func (r *agentRuntime) replyEvents() ([]ToolEvent, []ToolEvent) {
	return append([]ToolEvent(nil), r.events...), r.presentationEvents()
}

func (r *agentRuntime) loopLimits() (int, int) {
	if r.opts.TaskID != "" {
		return agentTaskMaxRounds, agentTaskMaxToolCalls
	}
	if r.opts.ForceTool != "" || attachmentsIncludeWorkspaceReference(r.attachments) {
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
		emitToolLifecycleEvent(r.opts, "model_delta", "info", map[string]any{
			"text":         turn.Content,
			"message_kind": "assistant_commentary",
		})
		r.ensureActiveAssistantMessage(turn.Content)
		r.appendActiveAssistantDelta(turn.Content)
		return
	}
	if strings.TrimSpace(turn.PublicStreamedContent) == "" {
		return
	}
	r.ensureActiveAssistantMessage(turn.PublicStreamedContent)
}

func (r *agentRuntime) commitAssistantActionPreamble(turn agentModelTurn, kind ModelResponseKind) {
	if strings.TrimSpace(turn.Content) == "" {
		return
	}
	if kind == ModelResponseTerminalAction {
		if r.opts.OnAssistantCurrentMessageDiscarded != nil {
			r.opts.OnAssistantCurrentMessageDiscarded("terminal_action_owns_final_content")
		}
		return
	}
	r.commitAssistantContent(turn)
	if kind == ModelResponseJSONToolCall && r.opts.OnAssistantToolBoundary != nil {
		r.opts.OnAssistantToolBoundary()
	}
}

func (r *agentRuntime) shouldFinalizeActionPreamble(turn agentModelTurn, kind ModelResponseKind) bool {
	if strings.TrimSpace(turn.Content) == "" {
		return false
	}
	return kind != ModelResponseTerminalAction && kind != ModelResponseJSONToolCall
}

func (r *agentRuntime) appendAssistantTurn(turn agentModelTurn) {
	r.transcript.AppendAssistantTurn(turn)
	if len(turn.ToolCalls) > 0 {
		r.appendKernelAssistantItem("commentary", turn.Content)
	}
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

func (r *agentRuntime) discardActiveAssistantMessage(reason string) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "assistant_action_boundary_rejected"
	}
	if r.activeAssistantMessageID == "" {
		return
	}
	if r.opts.OnAssistantMessageDiscarded != nil {
		r.opts.OnAssistantMessageDiscarded(r.activeAssistantMessageID, reason, r.activeAssistantSegment)
	}
	r.activeAssistantMessageID = ""
}

func (r *agentRuntime) fail(code AgentRuntimeErrorCode, message string, cause error) (Reply, error) {
	err := newAgentRuntimeError(code, message, cause)
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
	r.recordTypedToolError(toolName, argumentsJSON, newToolError(ToolErrorExecutionFailed, message, nil))
}

func (r *agentRuntime) recordTypedToolError(toolName, argumentsJSON string, err error) {
	typed := normalizeToolError(err)
	r.lastToolFailed = true
	r.lastToolFailure = runtimeToolFailure{
		ToolName:          strings.TrimSpace(toolName),
		Message:           strings.TrimSpace(typed.Error()),
		FailureClass:      string(typed.Code),
		Category:          typed.Category,
		Target:            toolTargetSummary(toolName, argumentsJSON),
		Mutation:          isWorkspaceMutationTool(toolName) || (toolName == "run_workspace_command" && strings.TrimSpace(argumentsJSON) != ""),
		Recoverable:       typed.Retryable,
		RecommendedAction: typed.RecommendedAction,
	}
	r.recordConditionSatisfiedFromToolError(toolName, argumentsJSON, err)
	countsTowardExecutionStreak := typed.Phase == "execution" || typed.Phase == "verification"
	if countsTowardExecutionStreak {
		r.consecutiveToolErrors++
		r.recordStrategyFailure(toolName, argumentsJSON, typed)
		key := repeatedToolErrorKey(toolName, argumentsJSON, string(typed.Code))
		if key != "" && key == r.repeatedToolErrorKey {
			r.repeatedToolErrorCount++
		} else {
			r.repeatedToolErrorKey = key
			r.repeatedToolErrorCount = 1
		}
	} else {
		// A read/validation prerequisite is a recoverable phase boundary. It
		// must not consume the mutation execution failure budget.
		r.repeatedToolErrorKey = ""
		r.repeatedToolErrorCount = 0
	}
	if r.consecutiveToolErrors >= agentMaxConsecutiveToolErrs {
		emitToolLifecycleEvent(r.opts, "completion_rejected", "warn", map[string]any{"reason": "too many consecutive tool errors"})
	}
}

func (r *agentRuntime) clearToolErrors() {
	r.lastToolFailed = false
	r.lastToolFailure = runtimeToolFailure{}
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
	return toolErrorJSONPayload(toolName, newToolError(ToolErrorInvalidArguments, message, nil))
}

func mutationOperationHash(toolName, argumentsJSON string) string {
	var decoded any
	if err := json.Unmarshal([]byte(argumentsJSON), &decoded); err != nil {
		decoded = strings.TrimSpace(argumentsJSON)
	}
	if object, ok := decoded.(map[string]any); ok && toolName == "apply_workspace_patch" {
		if patch, ok := object["patch"].(string); ok && strings.TrimSpace(patch) != "" {
			object["patch_sha256"] = hashWorkspaceText(patch)
			object["patch_chars"] = len([]rune(patch))
			object["patch"] = "[patch omitted from hash canonical payload]"
		}
	}
	canonical, err := json.Marshal(decoded)
	if err != nil {
		canonical = []byte(strings.TrimSpace(argumentsJSON))
	}
	sum := sha256.Sum256([]byte(toolName + "\x00" + string(canonical)))
	return hex.EncodeToString(sum[:])
}

func userFriendlyToolLabel(toolName string) string {
	if entry, ok := toolManifestEntry(toolName); ok && strings.TrimSpace(entry.Label) != "" {
		return entry.Label
	}
	switch toolName {
	case "find_workspace_files", "search_workspace_text", "inspect_workspace":
		return "已查看工作区"
	case "read_workspace_note_view", "read_workspace_text", "read_workspace_files":
		return "已读取文件"
	case "apply_workspace_patch", "edit_workspace_text", "replace_workspace_lines", "save_workspace_file", "write_workspace_chunk", "finalize_workspace_write":
		return "已编辑文件"
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
	case requestUserInputToolName:
		return "等待用户输入"
	case requestUserChoiceToolName:
		return "等待用户选择"
	case legacyFollowupQuestionToolName:
		return "已询问用户"
	case "update_task_plan":
		return "已更新任务计划"
	default:
		return "已处理"
	}
}
