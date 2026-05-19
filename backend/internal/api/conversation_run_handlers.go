package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

func messageMetaJSON(kind string, segmentIndex int) string {
	raw, _ := json.Marshal(map[string]any{
		"kind":          kind,
		"segment_index": segmentIndex,
	})
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
	if content != "" {
		b.Items = append(b.Items, content)
	}
	b.current.Reset()
}
