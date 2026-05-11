package ai

import (
	"encoding/json"
	"strings"
)

func valueOrUnknown(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return value
}

func valueOrRoot(value string) string {
	if strings.TrimSpace(value) == "" {
		return "."
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func truncateRunes(value string, limit int) string {
	result, _ := truncateRunesWithFlag(value, limit)
	return result
}

func truncateRunesWithFlag(value string, limit int) (string, bool) {
	runes := []rune(value)
	if len(runes) <= limit {
		return value, false
	}
	return string(runes[:limit]) + "\n...[truncated]", true
}

func mustJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return `{"error":"failed to encode tool result"}`
	}
	return string(raw)
}

func truncate(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}
