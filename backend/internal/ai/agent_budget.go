package ai

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
)

const IncompleteReasonBudgetExhausted = "budget_exhausted"

func (r *agentRuntime) handleBudgetExhausted(ctx context.Context, reason string, maxRounds, maxToolCalls int) (Reply, error) {
	info := &BudgetExhaustionInfo{
		Reason:       reason,
		Round:        r.currentRound,
		MaxRounds:    maxRounds,
		ToolCalls:    r.toolCalls,
		MaxToolCalls: maxToolCalls,
	}
	payload := budgetEventPayload(info)
	emitToolLifecycleEvent(r.opts, "agent_budget_exhausted", "warn", payload)
	emitToolLifecycleEvent(r.opts, "agent_budget_summary_started", "info", payload)
	logTask(r.opts, "warn", fmt.Sprintf("agent budget exhausted: %s", reason))

	originalTranscript := r.transcript.Clone()
	r.transcript.AppendUserText(r.budgetSummaryPrompt(info), map[string]any{
		"reason":         IncompleteReasonBudgetExhausted,
		"budget_reason":  reason,
		"max_rounds":     maxRounds,
		"max_tool_calls": maxToolCalls,
	})
	r.lastContextSnapshot = r.contextManager.BuildWithOptions(r.protocol, r.messages, r.attachments, r.transcript, r.profile, r.currentRound+1, r.providerMessageBuildOptions())
	r.annotateRequestSummary()
	r.annotateBudgetSummaryRequest(info)
	r.lastContextSnapshot.Summary.ActionBoundaryActive = false
	emitProviderRequestSummary(r.opts, r.lastContextSnapshot.Summary)

	turn, err := r.service.providerV2(r.protocol).Turn(ctx, providerTurnRequest{
		ConversationID:       r.conversationID,
		MessageID:            r.messageID,
		Provider:             r.provider,
		Protocol:             r.protocol,
		Tools:                nil,
		ToolChoice:           providerToolChoice{Mode: providerToolChoiceNone},
		DisableTools:         true,
		Snapshot:             r.lastContextSnapshot,
		PerformedMutation:    r.performedMutation,
		HasToolEvents:        len(r.events) > 0,
		ActionBoundaryActive: false,
		Opts:                 r.opts,
	})
	r.transcript = originalTranscript

	content := strings.TrimSpace(turn.Content)
	if err != nil || content == "" || len(turn.ToolCalls) > 0 {
		if err != nil {
			log.Printf("[ai] budget summary model call failed conversation=%s error=%v", r.conversationID, err)
			failedPayload := budgetEventPayload(info)
			failedPayload["error"] = sanitizeUserFacingAIError(err.Error())
			emitToolLifecycleEvent(r.opts, "agent_budget_summary_failed", "warn", failedPayload)
		} else {
			emitToolLifecycleEvent(r.opts, "agent_budget_summary_failed", "warn", budgetEventPayload(info))
		}
		content = r.fallbackBudgetSummary(info)
	} else {
		completedPayload := budgetEventPayload(info)
		completedPayload["summary_chars"] = len([]rune(content))
		emitToolLifecycleEvent(r.opts, "agent_budget_summary_completed", "info", completedPayload)
	}

	emitToolLifecycleEvent(r.opts, "model_message_completed", "info", map[string]any{"content": content})
	if err := r.service.saveRunCheckpoint(ctx, r.opts, r.budgetResumeCheckpoint(info)); err != nil {
		return Reply{}, err
	}
	return Reply{
		Content:          content,
		ToolEvents:       r.events,
		IncompleteReason: IncompleteReasonBudgetExhausted,
		Recoverable:      true,
		Budget:           info,
	}, nil
}

func (r *agentRuntime) annotateBudgetSummaryRequest(info *BudgetExhaustionInfo) {
	if info == nil {
		return
	}
	r.lastContextSnapshot.Summary.BudgetReason = info.Reason
	r.lastContextSnapshot.Summary.MaxRounds = info.MaxRounds
	r.lastContextSnapshot.Summary.CurrentRound = info.Round
	r.lastContextSnapshot.Summary.MaxToolCalls = info.MaxToolCalls
	r.lastContextSnapshot.Summary.CurrentToolCalls = info.ToolCalls
}

func (r *agentRuntime) budgetSummaryPrompt(info *BudgetExhaustionInfo) string {
	changedPaths := strings.Join(r.changedPathsForBudgetSummary(), "\n- ")
	if changedPaths == "" {
		changedPaths = "无已记录的变更路径。"
	} else {
		changedPaths = "- " + changedPaths
	}
	return fmt.Sprintf(`本轮 agent 已达到执行预算上限，必须停止调用工具并给用户一段交接总结。不要调用任何工具，也不要声称任务完整完成。

停止原因：%s
当前轮次：%d/%d
当前工具调用：%d/%d

请用用户的语言简洁总结：
1. 已经完成或确认的内容。
2. 已成功变更或涉及的路径。
3. 仍未完成、失败或需要用户继续确认的事项。
4. 用户点击“继续本轮”后，下一步应从哪里接着做。

已记录的变更路径：
%s`, info.Reason, info.Round, info.MaxRounds, info.ToolCalls, info.MaxToolCalls, changedPaths)
}

func (r *agentRuntime) fallbackBudgetSummary(info *BudgetExhaustionInfo) string {
	paths := r.changedPathsForBudgetSummary()
	var b strings.Builder
	b.WriteString("本轮已达到执行预算上限，因此先在这里暂停并保留断点。\n\n")
	b.WriteString(fmt.Sprintf("- 停止原因：%s。\n", budgetReasonLabel(info.Reason)))
	b.WriteString(fmt.Sprintf("- 当前进度：模型轮次 %d/%d，工具调用 %d/%d。\n", info.Round, info.MaxRounds, info.ToolCalls, info.MaxToolCalls))
	if len(paths) > 0 {
		b.WriteString("- 已记录的变更路径：")
		b.WriteString(strings.Join(paths, "、"))
		b.WriteString("。\n")
	} else {
		b.WriteString("- 当前没有可汇总的已记录变更路径。\n")
	}
	b.WriteString("- 后续可以点击“继续本轮”，我会从断点继续，不重复已经成功执行的 workspace 变更。")
	return b.String()
}

func (r *agentRuntime) budgetResumeCheckpoint(info *BudgetExhaustionInfo) AgentRuntimeCheckpoint {
	checkpoint := r.checkpoint()
	checkpoint.Transcript = newAgentTranscript()
	checkpoint.Transcript.AppendUserText("上一轮已达到执行预算上限，并已向用户总结当前进度。请根据上一条 assistant 总结、当前 workspace 状态和已验证工具事件继续；不要重复已经成功执行的 workspace 变更。需要更多信息时继续调用工具，完成后调用 attempt_completion。", map[string]any{
		"reason":         IncompleteReasonBudgetExhausted,
		"budget_reason":  info.Reason,
		"max_rounds":     info.MaxRounds,
		"max_tool_calls": info.MaxToolCalls,
	})
	checkpoint.CurrentRound = 0
	checkpoint.ToolCalls = 0
	checkpoint.InvalidTurns = 0
	checkpoint.ConsecutiveToolErrors = 0
	checkpoint.CompletedContent = ""
	checkpoint.RequiresCompletion = checkpoint.RequiresCompletion || checkpoint.ToolLoopActive || checkpoint.PerformedMutation
	checkpoint.ToolLoopActive = true
	return checkpoint
}

func (r *agentRuntime) changedPathsForBudgetSummary() []string {
	seen := map[string]bool{}
	var paths []string
	for _, event := range r.events {
		if event.Status != "success" {
			continue
		}
		for _, path := range append([]string{event.Path, event.NewPath}, event.ChangedPaths...) {
			path = strings.TrimSpace(path)
			if path == "" || seen[path] {
				continue
			}
			seen[path] = true
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

func budgetEventPayload(info *BudgetExhaustionInfo) map[string]any {
	return map[string]any{
		"reason":             IncompleteReasonBudgetExhausted,
		"budget_reason":      info.Reason,
		"round":              info.Round,
		"max_rounds":         info.MaxRounds,
		"tool_calls":         info.ToolCalls,
		"max_tool_calls":     info.MaxToolCalls,
		"message":            "达到轮次上限，正在总结当前进度",
		"recoverable":        true,
		"incomplete_reason":  IncompleteReasonBudgetExhausted,
		"user_facing_status": "达到轮次上限，已总结当前进度，可继续本轮",
	}
}

func budgetReasonLabel(reason string) string {
	switch reason {
	case "max_tool_calls":
		return "工具调用次数达到上限"
	default:
		return "模型轮次达到上限"
	}
}
