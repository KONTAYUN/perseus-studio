package ai

import (
	"fmt"
	"strings"

	"webide-backend/internal/store"
)

func buildOpenAIMessages(messages []store.Message, attachments []AttachmentPayload) []map[string]any {
	items := make([]map[string]any, 0, len(messages))
	lastUserIndex := lastUserMessageIndex(messages)
	for index, item := range messages {
		content := any(item.Content)
		attachments := attachmentsForMessage(item, attachments, index == lastUserIndex)
		if len(attachments) > 0 {
			parts := []map[string]any{
				{"type": "text", "text": buildUserTextWithAttachments(item.Content, attachments)},
			}
			for _, attachment := range attachments {
				if attachment.Kind != "image" || attachment.DataBase64 == "" {
					continue
				}
				parts = append(parts, map[string]any{
					"type": "image_url",
					"image_url": map[string]any{
						"url": fmt.Sprintf("data:%s;base64,%s", attachment.MIMEType, attachment.DataBase64),
					},
				})
			}
			content = parts
		}
		items = append(items, map[string]any{
			"role":    item.Role,
			"content": content,
		})
	}
	return items
}

func buildAnthropicMessages(messages []store.Message, attachments []AttachmentPayload) (string, []map[string]any) {
	var systemParts []string
	items := make([]map[string]any, 0, len(messages))
	lastUserIndex := lastUserMessageIndex(messages)
	for index, item := range messages {
		if item.Role == "system" {
			if item.Content != "" {
				systemParts = append(systemParts, item.Content)
			}
			continue
		}
		role := item.Role
		if role != "user" && role != "assistant" {
			continue
		}
		content := any(item.Content)
		attachments := attachmentsForMessage(item, attachments, index == lastUserIndex)
		if len(attachments) > 0 {
			blocks := []map[string]any{
				{"type": "text", "text": buildUserTextWithAttachments(item.Content, attachments)},
			}
			for _, attachment := range attachments {
				if attachment.Kind != "image" || attachment.DataBase64 == "" {
					continue
				}
				blocks = append(blocks, map[string]any{
					"type": "image",
					"source": map[string]any{
						"type":       "base64",
						"media_type": attachment.MIMEType,
						"data":       attachment.DataBase64,
					},
				})
			}
			content = blocks
		}
		items = append(items, map[string]any{
			"role":    role,
			"content": content,
		})
	}
	return strings.Join(systemParts, "\n\n"), items
}

func lastUserMessageIndex(messages []store.Message) int {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == "user" {
			return index
		}
	}
	return -1
}

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
