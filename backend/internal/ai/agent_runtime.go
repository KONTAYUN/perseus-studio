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
		prePatchPublicStreamed := turn.PublicTextStreamed
		prePatchPublicContent := turn.PublicStreamedContent
		classification := ModelResponseDispatcher{}.Classify(turn)
		turn = classification.Turn
		emitToolLifecycleEvent(r.opts, "model_response_classified", "info", map[string]any{
			"round":         round + 1,
			"kind":          string(classification.Kind),
			"tool_names":    classification.ToolNames,
			"mode":          string(r.executionMode()),
			"finish_reason": turn.FinishReason,
		})
		if classification.Kind == ModelResponseTextPatchAction {
			if prePatchPublicStreamed && strings.TrimSpace(prePatchPublicContent) != strings.TrimSpace(turn.PublicStreamedContent) {
				r.discardActiveAssistantMessage("assistant_text_patch_reconciled")
			}
			patchText := ""
			if len(turn.ToolCalls) > 0 {
				patchText = turn.ToolCalls[0].PatchText
			}
			emitToolLifecycleEvent(r.opts, "assistant_text_patch_captured", "info", map[string]any{
				"round":                round + 1,
				"tool_name":            "apply_workspace_patch",
				"status_kind":          manifestStatusKind("apply_workspace_patch"),
				"transport":            "assistant_text_patch",
				"patch_chars":          len([]rune(patchText)),
				"patch_sha":            hashWorkspaceText(patchText),
				"patch_block_count":    countWorkspacePatchBlocks(patchText),
				"detected_paths":       workspacePatchCandidatePathsLoose(patchText),
				"discarded_tool_calls": classification.Patch.DiscardedToolCalls,
				"discarded_tool_names": classification.Patch.DiscardedToolNames,
				"action_boundary":      "assistant_text_patch_supersedes_json_tool_calls",
			})
		}
		r.lastProviderFinishReason = turn.FinishReason
		if len(turn.ToolCalls) == 0 {
			contentPolicy := r.contentOnlyCompletionPolicy(turn)
			r.lastContextSnapshot.Summary.ContentOnlyPolicy = contentPolicy.Reason
			r.lastContextSnapshot.Summary.AcceptedContentOnlyAfterTools = contentPolicy.Accept && r.actionBoundaryActive()
			emitToolLifecycleEvent(r.opts, "content_only_completion_policy", "info", map[string]any{
				"round":  round + 1,
				"accept": contentPolicy.Accept,
				"reason": contentPolicy.Reason,
				"mode":   string(r.executionMode()),
			})
			if contentPolicy.Accept {
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
		r.ensureActiveAssistantMessage(turn.Content)
		r.appendActiveAssistantDelta(turn.Content)
		return
	}
	if strings.TrimSpace(turn.PublicStreamedContent) == "" {
		return
	}
	r.ensureActiveAssistantMessage(turn.PublicStreamedContent)
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

func (r *agentRuntime) correctiveMessage(content string) string {
	rejectedAction := "assistant_text"
	if strings.TrimSpace(content) == "" {
		rejectedAction = "empty_response"
		return r.actionRejectedObservation("empty model response while an action is required", rejectedAction, nil)
	}
	if r.toolLoopActive {
		return r.actionRejectedObservation("plain assistant text did not execute an action in the active tool workflow", rejectedAction, nil)
	}
	return r.actionRejectedObservation("plain assistant text cannot complete an action-required task", rejectedAction, nil)
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
	failureClass := toolFailureClass(toolName, fmt.Errorf("%s", message))
	r.lastToolFailure = runtimeToolFailure{
		ToolName:     strings.TrimSpace(toolName),
		Message:      strings.TrimSpace(message),
		FailureClass: failureClass,
		Target:       toolTargetSummary(toolName, argumentsJSON),
		Mutation:     isWorkspaceMutationTool(toolName) || (toolName == "run_workspace_command" && strings.TrimSpace(argumentsJSON) != ""),
		Recoverable:  toolFailureRecoverableByModel(failureClass),
	}
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
	case "read_workspace_text", "read_workspace_files":
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
