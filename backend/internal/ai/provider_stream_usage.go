package ai

import (
	"encoding/json"
	"strings"
)

func updateProviderUsageFromAny(usage *ProviderUsage, value any) {
	if usage == nil || value == nil {
		return
	}
	raw, ok := value.(map[string]any)
	if !ok {
		return
	}
	if next := int64FromAny(raw["input_tokens"]); next > 0 {
		usage.InputTokens = next
	}
	if next := int64FromAny(raw["output_tokens"]); next > 0 {
		usage.OutputTokens = next
	}
	if next := int64FromAny(raw["cache_creation_input_tokens"]); next > 0 {
		usage.CacheCreationTokens = next
	}
	if next := int64FromAny(raw["cache_read_input_tokens"]); next > 0 {
		usage.CacheReadTokens = next
	}
}

func int64FromAny(value any) int64 {
	switch item := value.(type) {
	case float64:
		return int64(item)
	case int:
		return int64(item)
	case int64:
		return item
	default:
		return 0
	}
}

func hasCompleteToolCalls(turn agentModelTurn) bool {
	for _, call := range turn.ToolCalls {
		if hasCompleteToolCallArguments(call) {
			return true
		}
	}
	return false
}

func hasIncompleteToolCallArguments(turn agentModelTurn) bool {
	for _, call := range turn.ToolCalls {
		if strings.TrimSpace(call.Name) == "" {
			continue
		}
		args := strings.TrimSpace(call.ArgumentsJSON)
		if args == "" {
			return true
		}
		var decoded any
		if err := json.Unmarshal([]byte(args), &decoded); err != nil {
			return true
		}
	}
	return false
}

func hasCompleteToolCallArguments(call agentToolCall) bool {
	if strings.TrimSpace(call.Name) == "" || strings.TrimSpace(call.ArgumentsJSON) == "" {
		return false
	}
	var decoded any
	return json.Unmarshal([]byte(call.ArgumentsJSON), &decoded) == nil
}

func intFromAny(value any) int {
	switch item := value.(type) {
	case float64:
		return int(item)
	case int:
		return item
	default:
		return 0
	}
}
