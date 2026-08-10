package ai

import (
	"context"
	"strings"
)

const agentTurnRepeatLimit = 3

const turnEngineProtocolInstructions = `
Turn protocol override for this run:
- Every response must select one available action. Plain assistant text never ends an agent turn.
- If the task is complete, call attempt_completion with the final user-visible answer.
- If progress requires open-ended information or external results that only the user can provide, call request_user_input.
- If the user must make a bounded decision with two or three mutually exclusive options, call request_user_choice.
- Otherwise call the next tool needed to make progress.
- The start_background_task tool may end the foreground turn after its request has been validated and recorded.
- Use edit_workspace_note_blocks for existing HTML notes and create_workspace_note for new HTML notes.
- Use apply_workspace_patch for non-HTML file-content changes. Assistant text never executes file changes.
- A terminal tool must be the only tool call in its response.
- Tool results are recorded before the next model step. Do not claim external work without a successful tool result.
`

type agentTurnRepeatDetector struct {
	previous string
	count    int
}

func (r *agentRuntime) turnEngineLimits() (int, int) {
	if r != nil && strings.TrimSpace(r.opts.TaskID) != "" {
		return agentTaskMaxRounds, agentTaskMaxToolCalls
	}
	return agentWorkspaceMaxRounds, agentWorkspaceMaxToolCalls
}

// runTurnEngine uses an explicit Roo-style action boundary: tools continue the
// turn, either user-request tool waits for the user, and attempt_completion ends
// the task. Plain assistant text is protocol feedback, never a terminal item.
func (r *agentRuntime) runTurnEngine(ctx context.Context) (Reply, error) {
	repeats := agentTurnRepeatDetector{}
	r.emitKernelTurnStarted()
	emitToolLifecycleEvent(r.opts, "turn_engine_started", "info", map[string]any{
		"kernel_version": r.kernel.Version,
		"kernel_mode":    r.kernel.Mode,
		"protocol":       "structured_tools",
		"provider_id":    r.provider.ID,
		"model_id":       r.provider.ModelID,
	})

	for {
		maxRounds, maxToolCalls := r.turnEngineLimits()
		if r.currentRound >= maxRounds {
			return r.handleBudgetExhausted(ctx, "max_rounds", maxRounds, maxToolCalls)
		}
		if r.toolCalls >= maxToolCalls {
			return r.handleBudgetExhausted(ctx, "max_tool_calls", maxRounds, maxToolCalls)
		}

		round := r.currentRound
		r.currentRound = round + 1
		turn, err := r.callModelForTurnEngine(ctx, round)
		if err != nil {
			return Reply{}, err
		}
		r.lastProviderFinishReason = turn.FinishReason

		turn.ToolCalls = normalizeAgentToolCallIDs(turn.ToolCalls)

		if len(turn.ToolCalls) == 0 {
			if reply, stop, err := r.handleTurnEngineProtocolMistake(ctx, turn); stop || err != nil {
				return reply, err
			}
			continue
		}
		if reason := validateTurnEngineActionSet(turn.ToolCalls); reason != "" {
			r.discardActiveAssistantMessage("invalid_structured_action_set")
			if reply, stop, err := r.handleTurnEngineActionSetMistake(ctx, turn, reason); stop || err != nil {
				return reply, err
			}
			continue
		}

		if pending, pendingErr := r.hasPendingSteers(ctx); pendingErr != nil {
			return Reply{}, pendingErr
		} else if pending {
			r.discardActiveAssistantMessage("user_guidance_superseded_tool_turn")
			r.appendAssistantTurn(turn)
			r.appendTurnEngineAbortedToolCalls(turn.ToolCalls, "new user guidance arrived before tool execution")
			emitToolLifecycleEvent(r.opts, "agent_steer_deferred_stale_turn", "info", map[string]any{
				"round": round + 1, "tool_calls": len(turn.ToolCalls),
				"safe_boundary": "before_tool_execution",
			})
			if err := r.service.saveRunCheckpoint(ctx, r.opts, r.checkpoint()); err != nil {
				return Reply{}, err
			}
			continue
		}

		r.invalidTurns = 0
		responseKind := ModelResponseDispatcher{}.Classify(turn).Kind
		if turnEngineCallsCanPublishCommentary(turn.ToolCalls) {
			r.commitAssistantActionPreamble(turn, responseKind)
		} else if strings.TrimSpace(turn.Content) != "" && r.opts.OnAssistantCurrentMessageDiscarded != nil {
			r.opts.OnAssistantCurrentMessageDiscarded("terminal_or_wait_action_owns_presentation")
		}
		r.appendAssistantTurn(turn)
		if request, completed, err := r.executeTurnEngineToolCalls(ctx, turn.ToolCalls, &repeats); err != nil {
			return Reply{}, err
		} else if request != nil {
			r.terminalState = AgentTerminationDelegated
			r.emitCheckpointRecoveryCompleted()
			runEvents, presentationEvents := r.replyEvents()
			return Reply{BackgroundTask: request, RunEvents: runEvents, PresentationEvents: presentationEvents, TerminalState: r.terminalState}, nil
		} else if completed {
			content := strings.TrimSpace(r.completedContent)
			if content == "" || r.terminalState == "" {
				return Reply{}, r.failProtocolTerminalState()
			}
			r.emitCheckpointRecoveryCompleted()
			runEvents, presentationEvents := r.replyEvents()
			return Reply{Content: content, RunEvents: runEvents, PresentationEvents: presentationEvents, TerminalState: r.terminalState}, nil
		}
	}
}

func turnEngineCallsCanPublishCommentary(calls []agentToolCall) bool {
	if len(calls) == 0 {
		return false
	}
	for _, call := range calls {
		name := strings.TrimSpace(call.Name)
		if name == "start_background_task" || isUserRequestTool(name) {
			return false
		}
		if entry, ok := toolManifestEntry(name); ok && entry.Terminal {
			return false
		}
	}
	return true
}

func (r *agentRuntime) failProtocolTerminalState() error {
	message := "terminal agent tool completed without a valid terminal state"
	emitToolLifecycleEvent(r.opts, "agent_protocol_failed", "error", map[string]any{
		"reason": "missing_terminal_state",
	})
	_, err := r.fail(AgentRuntimeProtocolViolation, message, nil)
	return err
}

func validateTurnEngineActionSet(calls []agentToolCall) string {
	if len(calls) <= 1 {
		return ""
	}
	for _, call := range calls {
		if entry, ok := toolManifestEntry(strings.TrimSpace(call.Name)); ok && entry.Terminal {
			return "terminal tools must be the only tool call in a model response"
		}
		if isUserRequestTool(call.Name) || call.Name == "start_background_task" {
			return "user-wait and delegation tools must be the only tool call in a model response"
		}
	}
	return ""
}
