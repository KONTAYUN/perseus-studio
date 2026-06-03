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
							Arguments: json.RawMessage(strconvQuoteJSON(toolCallTranscriptArgumentsJSON(call))),
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
					"content":      providerToolObservationContent(result),
				})
			}
		case transcriptEntryUser:
			items = append(items, map[string]any{
				"role":    "user",
				"content": entry.Text,
			})
		case transcriptEntryCompactionSummary:
			items = append(items, map[string]any{
				"role":    "system",
				"content": entry.Text,
				"name":    "perseus_context_summary",
			})
		}
	}
	return items
}

func anthropicTranscriptMessages(entries []TranscriptEntry) []map[string]any {
	items := make([]map[string]any, 0, len(entries))
	for index := 0; index < len(entries); index++ {
		entry := entries[index]
		switch entry.Kind {
		case transcriptEntryAssistant:
			if len(entry.AnthropicContent) > 0 {
				items = append(items, map[string]any{"role": "assistant", "content": cloneMapAnySlice(entry.AnthropicContent)})
				continue
			}
			content := make([]map[string]any, 0, 1+len(entry.ToolCalls))
			if strings.TrimSpace(entry.Text) != "" {
				content = append(content, map[string]any{"type": "text", "text": entry.Text})
			}
			for _, call := range entry.ToolCalls {
				input := cloneMapAny(call.AnthropicInput)
				if input == nil {
					var decoded map[string]any
					if err := json.Unmarshal([]byte(toolCallTranscriptArgumentsJSON(call)), &decoded); err == nil {
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
			content := anthropicToolResultContent(entry.ToolResults)
			for index+1 < len(entries) && entries[index+1].Kind == transcriptEntryTool {
				index++
				content = append(content, anthropicToolResultContent(entries[index].ToolResults)...)
			}
			items = append(items, map[string]any{"role": "user", "content": content})
		case transcriptEntryUser:
			items = append(items, map[string]any{"role": "user", "content": entry.Text})
		case transcriptEntryCompactionSummary:
			items = append(items, map[string]any{"role": "user", "content": []map[string]any{{
				"type": "text",
				"text": entry.Text,
			}}})
		}
	}
	return items
}

func anthropicToolResultContent(results []TranscriptToolResult) []map[string]any {
	content := make([]map[string]any, 0, len(results))
	for _, result := range results {
		item := map[string]any{
			"type":        "tool_result",
			"tool_use_id": result.ToolCallID,
			"content":     providerToolObservationContent(result),
		}
		if result.IsError {
			item["is_error"] = true
		}
		content = append(content, item)
	}
	return content
}
