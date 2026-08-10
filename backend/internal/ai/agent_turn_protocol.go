package ai

import (
	"context"
	"fmt"
	"strings"
)

const rooStyleNoToolFeedback = `[AUTOMATED AGENT PROTOCOL FEEDBACK]
Your previous response did not use an action and cannot end this turn.
- If the user's task is complete, call attempt_completion with the final user-visible answer.
- If progress requires open-ended information or external results that only the user can provide, call request_user_input.
- If the user must choose between two or three bounded, mutually exclusive options, call request_user_choice.
- Otherwise call the next available tool required to make progress.
Do not answer this automated feedback conversationally.`

func (r *agentRuntime) handleTurnEngineActionSetMistake(ctx context.Context, turn agentModelTurn, detail string) (Reply, bool, error) {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		detail = "invalid structured action set"
	}
	r.invalidTurns++
	r.transcript.AppendAssistantTurn(turn)
	r.appendKernelAssistantItem("protocol", turn.Content)
	r.appendTurnEngineAbortedToolCalls(turn.ToolCalls, detail)

	feedback := `[AUTOMATED AGENT PROTOCOL FEEDBACK]
The previous response selected an invalid combination of actions.
- Existing HTML notes must use edit_workspace_note_blocks after reading the current index and target blocks.
- New HTML notes must use create_workspace_note. Other text-file edits use apply_workspace_patch.
- Terminal, user-wait, and background-delegation tools must be the only tool call in their response.
- Otherwise continue with one or more non-terminal structured tools.
Do not repeat the invalid action set.`
	if r.invalidTurns >= agentMaxProtocolMistakes {
		message := fmt.Sprintf("model returned an invalid structured action set %d consecutive times: %s", r.invalidTurns, detail)
		reply, err := r.fail(AgentRuntimeProtocolViolation, message, nil)
		return reply, true, err
	}
	r.appendKernelItem(AgentTurnItem{Kind: "protocol_feedback", Phase: "protocol", Status: "completed", Content: feedback})
	r.transcript.AppendRuntimeObservation(feedback, map[string]any{
		"reason": "invalid_action_set", "detail": detail, "attempt": r.invalidTurns, "limit": agentMaxProtocolMistakes,
	})
	if r.contextPersistenceErr != nil {
		return Reply{}, true, r.contextPersistenceErr
	}
	if err := r.service.saveRunCheckpoint(ctx, r.opts, r.checkpoint()); err != nil {
		return Reply{}, true, err
	}
	emitToolLifecycleEvent(r.opts, "agent_protocol_retry", "warn", map[string]any{
		"reason": "invalid_action_set", "detail": detail, "attempt": r.invalidTurns, "limit": agentMaxProtocolMistakes,
	})
	return Reply{}, false, nil
}

func (r *agentRuntime) handleTurnEngineProtocolMistake(ctx context.Context, turn agentModelTurn) (Reply, bool, error) {
	content := strings.TrimSpace(turn.Content)
	reason := "missing_tool_call"
	if content == "" {
		reason = "empty_model_response"
	}
	r.lastProviderEmptyResponse = content == ""
	r.invalidTurns++
	r.discardActiveAssistantMessage("agent_protocol_action_required")
	r.transcript.AppendAssistantTurn(turn)
	if content != "" {
		r.appendKernelItem(AgentTurnItem{
			Kind:    "assistant_message",
			Phase:   "protocol",
			Status:  "rejected",
			Content: content,
		})
	}

	if r.invalidTurns >= agentMaxProtocolMistakes {
		message := fmt.Sprintf("model failed to select an agent action %d consecutive times", r.invalidTurns)
		baseErr := fmt.Errorf("%s", message)
		diagID := r.service.recordProviderDiagnosticWithSummary(
			ctx, r.opts, r.provider, r.conversationID, r.messageID,
			"agent_protocol_violation", "error", 0, baseErr, r.lastContextSnapshot.Summary,
		)
		emitToolLifecycleEvent(r.opts, "agent_protocol_failed", "error", map[string]any{
			"reason": reason, "attempt": r.invalidTurns, "limit": agentMaxProtocolMistakes,
			"diagnostic_log_id": diagID,
		})
		reply, failErr := r.fail(AgentRuntimeProtocolViolation, message, nil)
		return reply, true, WithDiagnosticID(failErr, diagID)
	}

	r.appendKernelItem(AgentTurnItem{
		Kind:    "protocol_feedback",
		Phase:   "protocol",
		Status:  "completed",
		Content: rooStyleNoToolFeedback,
	})
	r.transcript.AppendRuntimeObservation(rooStyleNoToolFeedback, map[string]any{
		"reason": reason, "observation_type": "protocol_feedback",
		"attempt": r.invalidTurns, "limit": agentMaxProtocolMistakes,
	})
	if r.contextPersistenceErr != nil {
		return Reply{}, true, r.contextPersistenceErr
	}
	if err := r.service.saveRunCheckpoint(ctx, r.opts, r.checkpoint()); err != nil {
		return Reply{}, true, err
	}
	emitToolLifecycleEvent(r.opts, "agent_protocol_retry", "warn", map[string]any{
		"reason": reason, "attempt": r.invalidTurns, "limit": agentMaxProtocolMistakes,
	})
	return Reply{}, false, nil
}
