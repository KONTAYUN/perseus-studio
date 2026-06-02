package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

func messageMetaJSON(kind string, segmentIndex int, runID ...string) string {
	payload := map[string]any{
		"kind":          kind,
		"segment_index": segmentIndex,
	}
	if len(runID) > 0 {
		if trimmed := strings.TrimSpace(runID[0]); trimmed != "" {
			payload["run_id"] = trimmed
		}
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

func eventPayloadString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

type assistantSegmentBuffer struct {
	current strings.Builder
	Items   []string
}

func (b *assistantSegmentBuffer) AppendDelta(delta string) {
	if delta == "" {
		return
	}
	b.current.WriteString(delta)
}

func (b *assistantSegmentBuffer) Flush(finalContent string) {
	content := finalContent
	if strings.TrimSpace(content) == "" {
		content = b.current.String()
	}
	content = strings.TrimSpace(content)
	if content != "" && !isPlaceholderAssistantSegment(content) {
		b.Items = append(b.Items, content)
	}
	b.current.Reset()
}

func isPlaceholderAssistantSegment(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" || len([]rune(content)) > 8 {
		return false
	}
	for _, item := range content {
		switch item {
		case '.', '…', '。':
			continue
		default:
			return false
		}
	}
	return true
}

func splitAssistantDelta(text string, maxRunes int) []string {
	if text == "" {
		return nil
	}
	if maxRunes <= 0 {
		maxRunes = 512
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return []string{text}
	}
	chunks := make([]string, 0, (len(runes)/maxRunes)+1)
	for start := 0; start < len(runes); start += maxRunes {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks
}

func missingAssistantFinalDelta(currentContent string, finalContent string) string {
	finalContent = strings.TrimSpace(finalContent)
	if finalContent == "" {
		return ""
	}
	if strings.TrimSpace(currentContent) == finalContent {
		return ""
	}
	if currentContent == "" {
		return finalContent
	}
	if strings.HasPrefix(finalContent, currentContent) {
		return finalContent[len(currentContent):]
	}
	trimmedCurrent := strings.TrimSpace(currentContent)
	if trimmedCurrent != "" && strings.HasPrefix(finalContent, trimmedCurrent) {
		return finalContent[len(trimmedCurrent):]
	}
	return ""
}

type assistantStreamState struct {
	ActiveMessageID      string
	CompletionPreviewID  string
	LastFinalizedContent string
}

func shouldAppendModelDelta(state assistantStreamState) bool {
	return state.CompletionPreviewID == "" || state.ActiveMessageID != state.CompletionPreviewID
}

func shouldFinalizeModelMessageCompleted(finalContent string, lastFinalizedContent string) bool {
	finalContent = strings.TrimSpace(finalContent)
	if finalContent == "" {
		return false
	}
	return finalContent != strings.TrimSpace(lastFinalizedContent)
}
