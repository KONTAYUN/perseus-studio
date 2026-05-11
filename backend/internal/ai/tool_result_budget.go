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
