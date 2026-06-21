package ai

import (
	"encoding/json"
	"strconv"
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
		case transcriptEntryRuntimeObservation:
			items = append(items, map[string]any{
				"role":    "system",
				"content": entry.Text,
				"name":    "perseus_runtime_observation",
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
	idRewriter := newAnthropicToolUseIDRewriter()
	for index := 0; index < len(entries); index++ {
		entry := entries[index]
		switch entry.Kind {
		case transcriptEntryAssistant:
			if len(entry.AnthropicContent) > 0 {
				items = append(items, map[string]any{"role": "assistant", "content": idRewriter.rewriteAssistantContent(cloneMapAnySlice(entry.AnthropicContent))})
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
				content = append(content, map[string]any{"type": "tool_use", "id": idRewriter.rewriteToolUseID(call.ID), "name": call.Name, "input": input})
			}
			items = append(items, map[string]any{"role": "assistant", "content": content})
		case transcriptEntryTool:
			content := anthropicToolResultContent(entry.ToolResults, idRewriter)
			for index+1 < len(entries) && entries[index+1].Kind == transcriptEntryTool {
				index++
				content = append(content, anthropicToolResultContent(entries[index].ToolResults, idRewriter)...)
			}
			items = append(items, map[string]any{"role": "user", "content": content})
		case transcriptEntryUser:
			items = append(items, map[string]any{"role": "user", "content": entry.Text})
		case transcriptEntryRuntimeObservation:
			items = append(items, map[string]any{"role": "user", "content": []map[string]any{{
				"type": "text",
				"text": "Perseus runtime observation. This is environment feedback, not user input.\n" + entry.Text,
			}}})
		case transcriptEntryCompactionSummary:
			items = append(items, map[string]any{"role": "user", "content": []map[string]any{{
				"type": "text",
				"text": entry.Text,
			}}})
		}
	}
	return items
}

type anthropicToolUseIDRewriter struct {
	seen    map[string]int
	pending map[string]string
}

func newAnthropicToolUseIDRewriter() *anthropicToolUseIDRewriter {
	return &anthropicToolUseIDRewriter{
		seen:    map[string]int{},
		pending: map[string]string{},
	}
}

func (r *anthropicToolUseIDRewriter) rewriteAssistantContent(content []map[string]any) []map[string]any {
	if r == nil {
		return content
	}
	for index := range content {
		if content[index]["type"] != "tool_use" {
			continue
		}
		content[index]["id"] = r.rewriteToolUseID(strings.TrimSpace(stringFromAny(content[index]["id"])))
	}
	return content
}

func (r *anthropicToolUseIDRewriter) rewriteToolUseID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" || r == nil {
		return id
	}
	count := r.seen[id]
	r.seen[id] = count + 1
	if count == 0 {
		r.pending[id] = id
		return id
	}
	next := id + "_" + strconv.Itoa(count+1)
	r.pending[id] = next
	return next
}

func (r *anthropicToolUseIDRewriter) rewriteToolResultID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" || r == nil {
		return id
	}
	if next := r.pending[id]; next != "" {
		delete(r.pending, id)
		return next
	}
	return id
}

func anthropicToolResultContent(results []TranscriptToolResult, idRewriter *anthropicToolUseIDRewriter) []map[string]any {
	if idRewriter == nil {
		idRewriter = newAnthropicToolUseIDRewriter()
	}
	content := make([]map[string]any, 0, len(results))
	for _, result := range results {
		item := map[string]any{
			"type":        "tool_result",
			"tool_use_id": idRewriter.rewriteToolResultID(result.ToolCallID),
			"content":     providerToolObservationContent(result),
		}
		if result.IsError {
			item["is_error"] = true
		}
		content = append(content, item)
	}
	return content
}
