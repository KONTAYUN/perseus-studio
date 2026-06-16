package ai

import "strings"

type ModelResponseDispatcher struct{}

func (d ModelResponseDispatcher) Classify(turn agentModelTurn) ModelResponseClassification {
	patch := normalizeTextPatchTurn(turn)
	if patch.Extracted {
		return ModelResponseClassification{
			Kind:      ModelResponseTextPatchAction,
			Turn:      patch.Turn,
			Patch:     patch,
			ToolNames: toolCallNames(patch.Turn.ToolCalls),
		}
	}
	if len(turn.ToolCalls) == 0 {
		if strings.TrimSpace(turn.Content) == "" {
			return ModelResponseClassification{Kind: ModelResponseEmptyOrReasoningOnly, Turn: turn}
		}
		return ModelResponseClassification{Kind: ModelResponseContentOnly, Turn: turn}
	}
	if len(turn.ToolCalls) == 1 && turn.ToolCalls[0].Name == "attempt_completion" {
		return ModelResponseClassification{Kind: ModelResponseTerminalAction, Turn: turn, ToolNames: toolCallNames(turn.ToolCalls)}
	}
	return ModelResponseClassification{Kind: ModelResponseJSONToolCall, Turn: turn, ToolNames: toolCallNames(turn.ToolCalls)}
}
