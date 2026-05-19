package ai

import (
	"encoding/json"
	"strings"
)

func openAITranscriptMessages(entries []TranscriptEntry) []map[string]any {
	items := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		switch entry.Kind {
		case transcriptEntryAssistant:
			message := openAIMessage{Role: "assistant"}
			if strings.TrimSpace(entry.Text) != "" {
				message.Content = entry.Text
			}
			if len(entry.ToolCalls) > 0 {
				for _, call := range entry.ToolCalls {
					message.ToolCalls = append(message.ToolCalls, openAIToolCall{
						ID:   call.ID,
						Type: "function",
						Function: openAIFunctionCall{
							Name:      call.Name,
							Arguments: json.RawMessage(strconvQuoteJSON(call.ArgumentsJSON)),
						},
					})
				}
			}
			items = append(items, message.toRuntimeMessage())
		case transcriptEntryTool:
			for _, result := range entry.ToolResults {
				items = append(items, map[string]any{
					"role":         "tool",
					"tool_call_id": result.ToolCallID,
					"content":      result.Content,
				})
			}
		case transcriptEntryUser:
			items = append(items, map[string]any{
				"role":    "user",
				"content": entry.Text,
			})
		}
	}
	return items
}

func anthropicTranscriptMessages(entries []TranscriptEntry) []map[string]any {
	items := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		switch entry.Kind {
		case transcriptEntryAssistant:
			content := make([]map[string]any, 0, 1+len(entry.ToolCalls))
			if strings.TrimSpace(entry.Text) != "" {
				content = append(content, map[string]any{"type": "text", "text": entry.Text})
			}
			for _, call := range entry.ToolCalls {
				input := cloneMapAny(call.AnthropicInput)
				if input == nil {
					var decoded map[string]any
					if err := json.Unmarshal([]byte(call.ArgumentsJSON), &decoded); err == nil {
						input = decoded
					}
				}
				if input == nil {
					input = map[string]any{}
				}
				content = append(content, map[string]any{"type": "tool_use", "id": call.ID, "name": call.Name, "input": input})
			}
			items = append(items, map[string]any{"role": "assistant", "content": content})
		case transcriptEntryTool:
			content := make([]map[string]any, 0, len(entry.ToolResults))
			for _, result := range entry.ToolResults {
				item := map[string]any{
					"type":        "tool_result",
					"tool_use_id": result.ToolCallID,
					"content":     result.Content,
				}
				if result.IsError {
					item["is_error"] = true
				}
				content = append(content, item)
			}
			items = append(items, map[string]any{"role": "user", "content": content})
		case transcriptEntryUser:
			items = append(items, map[string]any{"role": "user", "content": entry.Text})
		}
	}
	return items
}
