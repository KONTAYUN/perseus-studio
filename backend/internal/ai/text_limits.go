package ai

import (
	"fmt"
	"strings"
)

// These helpers are for bounded auxiliary prompts and tool payloads. Conversation
// history is budgeted by ContextAssembler and must not use character truncation.
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
