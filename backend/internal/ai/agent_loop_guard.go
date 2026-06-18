package ai

import (
	"fmt"
	"strings"
)

const (
	agentLoopGuardWarnThreshold = 2
	agentLoopGuardStopThreshold = 4
)

func (r *agentRuntime) recordLoopGuardAction(call agentToolCall, isError bool, event ToolEvent, result string) {
	if r == nil {
		return
	}
	key := loopGuardActionKey(call, isError, event)
	if key != "" && key == r.loopGuard.LastActionKey {
		r.loopGuard.RepeatedActionCount++
	} else {
		r.loopGuard.LastActionKey = key
		r.loopGuard.RepeatedActionCount = 1
	}
	if call.Name == "attempt_completion" && isError {
		r.loopGuard.TerminalRejectionCount++
	}
	fingerprint := r.progressFingerprint()
	if fingerprint == r.loopGuard.LastProgressFingerprint {
		r.loopGuard.NoProgressRoundCount++
	} else {
		r.loopGuard.LastProgressFingerprint = fingerprint
		r.loopGuard.NoProgressRoundCount = 0
		r.actionProgress.LastProgressRound = r.currentRound
	}
}

func loopGuardActionKey(call agentToolCall, isError bool, event ToolEvent) string {
	name := strings.TrimSpace(call.Name)
	if name == "" {
		return ""
	}
	target := firstNonEmpty(event.TargetSummary, event.Path, toolTargetSummary(name, executionToolArgumentsJSON(call)))
	status := event.Status
	if status == "" {
		if isError {
			status = "failed"
		} else {
			status = "success"
		}
	}
	return name + "\x00" + status + "\x00" + target
}

func (r *agentRuntime) progressFingerprint() string {
	if r == nil {
		return ""
	}
	return strings.Join(r.verifiedWorkspaceChangedPaths(), "|") + "::" + strings.Join(r.actionProgress.ConditionSatisfiedPaths, "|")
}

func (r *agentRuntime) loopGuardTriggered() (LoopGuardObservation, bool, bool) {
	if r == nil {
		return LoopGuardObservation{}, false, false
	}
	state := r.actionSelectionState(nil)
	reason := ""
	switch {
	case r.loopGuard.TerminalRejectionCount >= agentLoopGuardWarnThreshold && state.RequiresWorkspaceMutation && !state.HasVerifiedMutation:
		reason = "repeated_terminal_rejection_without_verified_mutation"
	case r.loopGuard.RepeatedActionCount >= agentLoopGuardWarnThreshold && state.RequiresWorkspaceMutation && !state.HasVerifiedMutation:
		reason = "repeated_action_without_progress"
	case r.loopGuard.NoProgressRoundCount >= agentLoopGuardWarnThreshold && state.RequiresWorkspaceMutation && !state.HasVerifiedMutation:
		reason = "workspace_action_without_semantic_progress"
	case r.loopGuard.RepeatedActionCount >= agentLoopGuardStopThreshold && !state.HasVerifiedMutation:
		reason = "repeated_tool_action_without_progress"
	}
	if reason == "" {
		return LoopGuardObservation{}, false, false
	}
	stop := r.loopGuard.TerminalRejectionCount >= agentLoopGuardStopThreshold ||
		r.loopGuard.RepeatedActionCount >= agentLoopGuardStopThreshold ||
		r.loopGuard.NoProgressRoundCount >= agentLoopGuardStopThreshold
	observation := LoopGuardObservation{
		Status:             "action_loop_guard",
		ReasonCode:         reason,
		AllowedNextActions: r.narrowedAllowedNextActions(state),
		Message:            "The workflow is repeating without a verified workspace change. Use the required edit action now, or stop with an honest failure if the edit cannot be performed.",
	}
	if state.RequiresWorkspaceMutation && !state.HasVerifiedMutation {
		observation.RequiredNextAction = "apply_workspace_patch"
	}
	return observation, true, stop
}

func (r *agentRuntime) appendLoopGuardObservation(ctxLabel string) bool {
	observation, triggered, stop := r.loopGuardTriggered()
	if !triggered {
		return false
	}
	payload := mustJSON(map[string]any{
		"observation":      observation,
		"action_state":     r.actionSelectionState(nil),
		"loop_guard_state": r.loopGuard,
		"context":          ctxLabel,
	})
	r.transcript.AppendRuntimeObservation(payload, map[string]any{
		"reason":           observation.ReasonCode,
		"observation_type": "loop_guard",
	})
	emitToolLifecycleEvent(r.opts, "agent_loop_guard", "warn", map[string]any{
		"reason_code":              observation.ReasonCode,
		"required_next_action":     observation.RequiredNextAction,
		"allowed_next_actions":     observation.AllowedNextActions,
		"repeated_action_count":    r.loopGuard.RepeatedActionCount,
		"terminal_rejection_count": r.loopGuard.TerminalRejectionCount,
		"no_progress_round_count":  r.loopGuard.NoProgressRoundCount,
		"stop":                     stop,
	})
	return stop
}

func (r *agentRuntime) loopGuardFailureMessage() string {
	observation, triggered, _ := r.loopGuardTriggered()
	if !triggered {
		return ""
	}
	if observation.RequiredNextAction == "" {
		return fmt.Sprintf("agent 连续重复相同工具动作且没有产生新进展，已停止继续绕圈。重复动作次数 %d、无进展轮数 %d。",
			r.loopGuard.RepeatedActionCount, r.loopGuard.NoProgressRoundCount)
	}
	return fmt.Sprintf("agent 连续没有产生已验证的 workspace 变更，已停止继续绕圈。需要执行的下一步是 apply_workspace_patch；当前已拒绝完成 %d 次、无进展轮数 %d。",
		r.loopGuard.TerminalRejectionCount, r.loopGuard.NoProgressRoundCount)
}
