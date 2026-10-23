package ai

import (
	"bytes"
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
	raw, err := marshalModelJSON(value)
	if err != nil {
		return `{"error":"failed to encode tool result"}`
	}
	return string(raw)
}

// Tool JSON is a text message inside a provider request, never an HTML script.
// Keep source &, < and > readable; the outer request encoder still escapes its
// envelope. Do not unescape strings afterward: literal file escapes must survive.
func marshalModelJSON(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte{'\n'}), nil
}

func truncate(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}
