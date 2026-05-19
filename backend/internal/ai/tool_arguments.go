package ai

import (
	"encoding/json"
	"fmt"
	"strings"
)

func summarizeToolArguments(argumentsJSON string) string {
	var decoded any
	if err := json.Unmarshal([]byte(argumentsJSON), &decoded); err != nil {
		return truncate(argumentsJSON, 500)
	}
	return summarizeToolArgumentValue(decoded)
}

func ledgerToolArgumentsJSON(argumentsJSON string) string {
	var decoded any
	if err := json.Unmarshal([]byte(argumentsJSON), &decoded); err != nil {
		return mustJSON(map[string]any{
			"raw":                            truncate(argumentsJSON, 1500),
			"parse_error":                    err.Error(),
			"arguments_truncated_for_ledger": true,
		})
	}
	sanitized, truncated := ledgerToolArgumentValue(decoded)
	encoded := mustJSON(sanitized)
	if len([]rune(encoded)) <= 2000 {
		return encoded
	}
	return mustJSON(map[string]any{
		"arguments_summary":              summarizeToolArguments(argumentsJSON),
		"arguments_truncated_for_ledger": true,
		"original_chars":                 len([]rune(argumentsJSON)),
		"sanitized_chars":                len([]rune(encoded)),
		"had_truncated_fields":           truncated,
	})
}

func ledgerToolArgumentValue(value any) (any, bool) {
	const maxStringRunes = 500
	switch item := value.(type) {
	case map[string]any:
		next := make(map[string]any, len(item)+1)
		truncated := false
		for key, raw := range item {
			cleaned, wasTruncated := ledgerToolArgumentValue(raw)
			next[key] = cleaned
			if wasTruncated {
				truncated = true
			}
		}
		if truncated {
			next["arguments_truncated_for_ledger"] = true
		}
		return next, truncated
	case []any:
		next := make([]any, len(item))
		truncated := false
		for index, raw := range item {
			cleaned, wasTruncated := ledgerToolArgumentValue(raw)
			next[index] = cleaned
			if wasTruncated {
				truncated = true
			}
		}
		return next, truncated
	case string:
		runes := []rune(item)
		if len(runes) <= maxStringRunes {
			return item, false
		}
		return fmt.Sprintf("%s\n...[truncated for ledger: kept %d of %d chars]", string(runes[:maxStringRunes]), maxStringRunes, len(runes)), true
	default:
		return item, false
	}
}

func summarizeToolArgumentValue(value any) string {
	switch item := value.(type) {
	case map[string]any:
		summary := make(map[string]any, len(item))
		for key, raw := range item {
			if key == "content" {
				if text, ok := raw.(string); ok {
					summary[key] = fmt.Sprintf("[content %d chars]", len([]rune(text)))
					continue
				}
			}
			summary[key] = summarizeToolArgumentScalar(raw)
		}
		return truncate(mustJSON(summary), 500)
	default:
		return truncate(mustJSON(item), 500)
	}
}

func summarizeToolArgumentScalar(value any) any {
	switch item := value.(type) {
	case string:
		return truncateRunes(item, 160)
	case []any:
		return fmt.Sprintf("[array %d items]", len(item))
	case map[string]any:
		return fmt.Sprintf("[object %d keys]", len(item))
	default:
		return item
	}
}

func truncatePlainRunes(value string, maxLen int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > maxLen {
		return string(runes[:maxLen])
	}
	return string(runes)
}
