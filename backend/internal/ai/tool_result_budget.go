package ai

import (
	"encoding/json"
	"fmt"
)

func budgetToolResultForModel(toolName, result string, profile providerRuntimeProfile) string {
	if result == "" {
		return result
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		return truncateWithContextNote(result, profile.MaxToolResultRunes, "tool result")
	}
	switch toolName {
	case "run_workspace_command":
		trimMapString(payload, "stdout", profile.MaxCommandOutputRunes)
		trimMapString(payload, "stderr", profile.MaxCommandOutputRunes/2)
		payload["model_view"] = "stdout/stderr may be truncated for stability; the UI keeps the full command event separately."
	case "read_workspace_text":
		trimMapString(payload, "content", profile.MaxReadFileContentRunes)
		if fitPayloadStringFieldToBudget(payload, "content", profile.MaxToolResultRunes) {
			raw, _ := json.Marshal(payload)
			return string(raw)
		}
	case "read_workspace_files":
		budgetBatchReadPayload(payload, profile)
		if fitBatchReadPayloadToBudget(payload, profile.MaxToolResultRunes) {
			raw, _ := json.Marshal(payload)
			return string(raw)
		}
	case "perseus_search_web", "web_search":
		budgetWebSearchPayload(payload, profile)
	case "web_extract":
		trimMapString(payload, "content", profile.MaxWebExtractRunes)
	default:
		// Keep structural metadata, but avoid returning a very large raw object to the model.
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return compactToolResultJSON(toolName, result, profile.MaxToolResultRunes)
	}
	if len([]rune(string(raw))) > profile.MaxToolResultRunes {
		return compactToolResultJSON(toolName, string(raw), profile.MaxToolResultRunes)
	}
	return string(raw)
}

func budgetBatchReadPayload(payload map[string]any, profile providerRuntimeProfile) {
	rawItems, ok := payload["items"].([]any)
	if !ok {
		return
	}
	for _, item := range rawItems {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		trimMapString(object, "content", profile.MaxReadFileContentRunes)
	}
	payload["items"] = rawItems
}

func fitPayloadStringFieldToBudget(payload map[string]any, key string, maxRunes int) bool {
	if maxRunes <= 0 {
		return false
	}
	for attempts := 0; attempts < 8; attempts++ {
		raw, err := json.Marshal(payload)
		if err != nil {
			return false
		}
		over := len([]rune(string(raw))) - maxRunes
		if over <= 0 {
			return true
		}
		value, ok := payload[key].(string)
		if !ok {
			return false
		}
		runes := []rune(value)
		if len(runes) == 0 {
			return false
		}
		nextLen := len(runes) - over - 200
		if nextLen < 0 {
			nextLen = 0
		}
		payload[key] = string(runes[:nextLen])
		payload[key+"_truncated_for_model"] = true
		payload[key+"_truncation_note"] = fmt.Sprintf("kept first %d chars for model stability", nextLen)
	}
	return false
}

func fitBatchReadPayloadToBudget(payload map[string]any, maxRunes int) bool {
	if maxRunes <= 0 {
		return false
	}
	rawItems, ok := payload["items"].([]any)
	if !ok {
		return false
	}
	for attempts := 0; attempts < 80; attempts++ {
		raw, err := json.Marshal(payload)
		if err != nil {
			return false
		}
		over := len([]rune(string(raw))) - maxRunes
		if over <= 0 {
			return true
		}
		item, content := largestBatchReadContent(rawItems)
		if item == nil || content == "" {
			if len(rawItems) <= 1 {
				return false
			}
			rawItems = rawItems[:len(rawItems)-1]
			payload["items"] = rawItems
			payload["items_truncated_for_model"] = true
			payload["model_view"] = "Some batch read items were omitted from model context for stability. Re-read specific paths if needed."
			continue
		}
		runes := []rune(content)
		nextLen := len(runes) - over - 200
		if nextLen < 0 {
			nextLen = 0
		}
		item["content"] = string(runes[:nextLen])
		item["content_truncated_for_model"] = true
		item["content_truncation_note"] = fmt.Sprintf("kept first %d chars for model stability", nextLen)
		payload["model_view"] = "Batch read contents may be truncated for model stability; path, range, and failure metadata are preserved."
	}
	return false
}

func largestBatchReadContent(items []any) (map[string]any, string) {
	var selected map[string]any
	selectedContent := ""
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		content, _ := item["content"].(string)
		if len([]rune(content)) > len([]rune(selectedContent)) {
			selected = item
			selectedContent = content
		}
	}
	return selected, selectedContent
}

func compactToolResultJSON(toolName, raw string, maxRunes int) string {
	if maxRunes <= 0 {
		maxRunes = 2000
	}
	prefixBudget := maxRunes - 300
	if prefixBudget < 200 {
		prefixBudget = 200
	}
	prefix, truncated := truncatePlainForModel(raw, prefixBudget)
	payload := map[string]any{
		"tool_name":      toolName,
		"truncated":      truncated,
		"raw_prefix":     prefix,
		"model_view":     "The tool result was too large and was compacted to valid JSON for model stability. Retry with a narrower request if more detail is needed.",
		"original_chars": len([]rune(raw)),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return `{"truncated":true,"model_view":"tool result was too large"}`
	}
	return string(encoded)
}

func trimMapString(payload map[string]any, key string, maxRunes int) {
	value, ok := payload[key].(string)
	if !ok {
		return
	}
	trimmed, truncated := truncatePlainForModel(value, maxRunes)
	if !truncated {
		return
	}
	payload[key] = trimmed
	payload[key+"_truncated_for_model"] = true
	payload[key+"_truncation_note"] = fmt.Sprintf("kept first %d chars for model stability", maxRunes)
}

func budgetWebSearchPayload(payload map[string]any, profile providerRuntimeProfile) {
	rawResults, ok := payload["results"].([]any)
	if !ok {
		return
	}
	if len(rawResults) > 6 {
		rawResults = rawResults[:6]
		payload["results_truncated_for_model"] = true
	}
	for _, item := range rawResults {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		trimMapString(object, "snippet", profile.MaxWebSearchSnippetRunes)
		trimMapString(object, "content", profile.MaxWebSearchSnippetRunes)
	}
	payload["results"] = rawResults
	payload["model_view"] = "Search results are intentionally concise. Use web_extract on a specific result URL when full page content is needed."
}
