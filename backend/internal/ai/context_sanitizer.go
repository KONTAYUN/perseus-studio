package ai

import (
	"fmt"
	"strings"

	"webide-backend/internal/store"
)

func sanitizeConversationMessages(messages []store.Message, profile providerRuntimeProfile) []store.Message {
	if len(messages) == 0 {
		return messages
	}
	messages = dropAssistantPlaceholderMessages(messages)
	start := 0
	if profile.MaxHistoryMessages > 0 && len(messages) > profile.MaxHistoryMessages {
		start = len(messages) - profile.MaxHistoryMessages
		for start > 0 && start < len(messages) && messages[start].Role == "assistant" {
			start++
		}
	}
	items := append([]store.Message(nil), messages[start:]...)
	recentFrom := len(items) - 12
	if recentFrom < 0 {
		recentFrom = 0
	}
	for index := range items {
		limit := profile.MaxOlderMessageRunes
		if index >= recentFrom {
			limit = profile.MaxRecentMessageRunes
		}
		items[index].Content = truncateWithContextNote(items[index].Content, limit, "message content")
	}
	if start > 0 && len(items) > 0 {
		prefix := fmt.Sprintf("[已自动省略 %d 条较早历史消息，以降低上下文噪声；如需旧内容，请让用户重新引用或说明。]\n\n", start)
		items[0].Content = prefix + items[0].Content
	}
	return items
}

func dropAssistantPlaceholderMessages(messages []store.Message) []store.Message {
	if len(messages) == 0 {
		return messages
	}
	next := make([]store.Message, 0, len(messages))
	for _, message := range messages {
		if message.Role == "assistant" && isToolPreamblePlaceholder(message.Content) {
			continue
		}
		next = append(next, message)
	}
	return next
}

func sanitizeAttachmentPayloads(attachments []AttachmentPayload, profile providerRuntimeProfile) []AttachmentPayload {
	if len(attachments) == 0 {
		return attachments
	}
	items := append([]AttachmentPayload(nil), attachments...)
	for index := range items {
		limit := profile.MaxAttachmentTextRunes
		if items[index].Kind == "selected_skill" {
			limit = profile.MaxSkillTextRunes
		}
		items[index].Text = truncateWithContextNote(items[index].Text, limit, "attachment text")
		if items[index].Text != attachments[index].Text {
			items[index].Truncated = true
		}
		for panelIndex := range items[index].EditorPanels {
			items[index].EditorPanels[panelIndex].OpenTabs = limitStringSlice(items[index].EditorPanels[panelIndex].OpenTabs, 12)
		}
	}
	return items
}

func truncateWithContextNote(value string, maxRunes int, label string) string {
	if maxRunes <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + fmt.Sprintf("\n\n[%s truncated: kept first %d of %d chars]", label, maxRunes, len(runes))
}

func truncatePlainForModel(value string, maxRunes int) (string, bool) {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 {
		return value, false
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value, false
	}
	return string(runes[:maxRunes]), true
}

func limitStringSlice(items []string, maxItems int) []string {
	if maxItems <= 0 || len(items) <= maxItems {
		return items
	}
	return append([]string(nil), items[:maxItems]...)
}
