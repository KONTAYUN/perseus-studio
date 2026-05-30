package ai

import (
	"encoding/json"
	"strconv"
	"strings"
)

const recommendedWorkspaceChunkChars = 4000

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

func annotateStreamToolDiagnostics(summary *ProviderRequestSummary, turn agentModelTurn) {
	if summary == nil {
		return
	}
	summary.StreamFinishReason = strings.TrimSpace(turn.FinishReason)
	summary.StreamUsage = turn.Usage
	summary.StreamToolCalls = streamToolCallLogs(turn.ToolCalls)
}

func streamToolCallLogs(calls []agentToolCall) []StreamToolCallLog {
	if len(calls) == 0 {
		return nil
	}
	items := make([]StreamToolCallLog, 0, len(calls))
	for index, call := range calls {
		args := strings.TrimSpace(call.ArgumentsJSON)
		item := StreamToolCallLog{
			Index:          index,
			ID:             strings.TrimSpace(call.ID),
			Name:           strings.TrimSpace(call.Name),
			ArgumentsChars: len([]rune(args)),
			ArgumentsBytes: len([]byte(args)),
			ArgumentsHead:  truncateRunes(args, 240),
			ArgumentsTail:  truncateRunes(tailRunes(args, 240), 240),
			TargetSummary:  toolTargetSummary(call.Name, args),
		}
		var decoded map[string]any
		if args == "" {
			item.ArgumentsJSONError = "empty arguments"
		} else if err := json.Unmarshal([]byte(args), &decoded); err != nil {
			item.ArgumentsJSONError = err.Error()
			item.TargetSummary = pathSummary(partialJSONStringField(args, "path"))
			item.ContentChars = len([]rune(partialJSONStringField(args, "content")))
			item.OldTextChars = len([]rune(partialJSONStringField(args, "old_text")))
			item.NewTextChars = len([]rune(partialJSONStringField(args, "new_text")))
		} else {
			item.ArgumentsJSONValid = true
			if content, ok := decoded["content"].(string); ok {
				item.ContentChars = len([]rune(content))
			}
			if oldText, ok := decoded["old_text"].(string); ok {
				item.OldTextChars = len([]rune(oldText))
			}
			if newText, ok := decoded["new_text"].(string); ok {
				item.NewTextChars = len([]rune(newText))
			}
			if item.Name == "edit_workspace_lines" && item.NewTextChars == 0 {
				item.NewTextChars = len([]rune(partialJSONStringField(args, "new_text")))
			}
		}
		if item.Name == "save_workspace_file" && (!item.ArgumentsJSONValid || item.ContentChars > recommendedWorkspaceChunkChars) {
			item.LooksLikeLongWrite = true
			item.RecommendedTool = "write_workspace_text"
			item.RecommendedChunkSize = recommendedWorkspaceChunkChars
		}
		if item.Name == "write_workspace_text" && (!item.ArgumentsJSONValid || item.ContentChars > recommendedWorkspaceChunkChars) {
			item.LooksLikeLongWrite = true
			item.RecommendedTool = "write_workspace_text"
			item.RecommendedChunkSize = recommendedWorkspaceChunkChars
		}
		if item.Name == "edit_workspace_text" && (!item.ArgumentsJSONValid || item.OldTextChars+item.NewTextChars > recommendedWorkspaceChunkChars) {
			item.LooksLikeLargeEdit = true
			item.RecommendedTool = "edit_workspace_lines"
		}
		if item.Name == "edit_workspace_lines" && (!item.ArgumentsJSONValid || item.NewTextChars > recommendedWorkspaceChunkChars) {
			item.LooksLikeLargeEdit = true
			item.RecommendedTool = "edit_workspace_lines"
			item.RecommendedChunkSize = recommendedWorkspaceChunkChars
		}
		items = append(items, item)
	}
	return items
}

func partialJSONStringField(value, field string) string {
	if strings.TrimSpace(value) == "" || strings.TrimSpace(field) == "" {
		return ""
	}
	index := strings.Index(value, strconv.Quote(field))
	if index < 0 {
		return ""
	}
	rest := value[index+len(strconv.Quote(field)):]
	colon := strings.Index(rest, ":")
	if colon < 0 {
		return ""
	}
	rest = strings.TrimSpace(rest[colon+1:])
	if !strings.HasPrefix(rest, `"`) {
		return ""
	}
	rest = rest[1:]
	escaped := false
	for index, ch := range rest {
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			return unquotePartialJSONString(rest[:index])
		}
	}
	return unquotePartialJSONString(rest)
}

func unquotePartialJSONString(value string) string {
	if value == "" {
		return ""
	}
	decoded, err := strconv.Unquote(`"` + value + `"`)
	if err == nil {
		return decoded
	}
	return strings.ReplaceAll(value, `\"`, `"`)
}

func hasCompleteToolCallArguments(call agentToolCall) bool {
	if strings.TrimSpace(call.Name) == "" || strings.TrimSpace(call.ArgumentsJSON) == "" {
		return false
	}
	var decoded any
	return json.Unmarshal([]byte(call.ArgumentsJSON), &decoded) == nil
}

func tailRunes(value string, maxLen int) string {
	if maxLen <= 0 || value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxLen {
		return value
	}
	return string(runes[len(runes)-maxLen:])
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
