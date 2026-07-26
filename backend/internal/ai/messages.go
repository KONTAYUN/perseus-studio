package ai

import (
	"fmt"
	"strings"
)

func normalizeOpenAIContent(content any) string {
	switch value := content.(type) {
	case nil:
		return ""
	case string:
		return value
	case []any:
		var parts []string
		for _, item := range value {
			if object, ok := item.(map[string]any); ok {
				if text, ok := object["text"].(string); ok && text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return fmt.Sprint(value)
	}
}

func openAIToolNames(tools []map[string]any) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		function, _ := tool["function"].(map[string]any)
		name, _ := function["name"].(string)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func anthropicToolNames(tools []map[string]any) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		name, _ := tool["name"].(string)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func extractOpenAIToolCalls(message openAIMessage) []openAIToolCall {
	if len(message.ToolCalls) > 0 {
		return message.ToolCalls
	}
	if message.FunctionCall != nil && message.FunctionCall.Name != "" {
		return []openAIToolCall{
			{
				ID:       "legacy_function_call",
				Type:     "function",
				Function: *message.FunctionCall,
			},
		}
	}
	return nil
}
