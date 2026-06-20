package ai

import (
	"encoding/json"
	"fmt"
	"strings"
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
		preferNumberedReadContentForModel(payload)
		trimReadContentFields(payload, profile.MaxReadFileContentRunes)
		if fitPayloadStringFieldsToBudget(payload, []string{"numbered_content", "content"}, profile.MaxToolResultRunes) {
			raw, _ := json.Marshal(payload)
			return string(raw)
		}
	case "read_workspace_note_view":
		trimMapString(payload, "content", profile.MaxReadFileContentRunes)
		if fitPayloadStringFieldsToBudget(payload, []string{"content"}, profile.MaxToolResultRunes) {
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

func preferNumberedReadContentForModel(payload map[string]any) {
	numbered, _ := payload["numbered_content"].(string)
	if numbered == "" {
		if content, _ := payload["content"].(string); content != "" {
			payload["read_content_state"] = string(ReadContentStateAvailable)
		}
		return
	}
	if _, ok := payload["content"].(string); ok {
		delete(payload, "content")
		payload["content_omitted_for_model"] = true
	}
	payload["read_content_state"] = string(ReadContentStateAvailable)
	payload["model_view"] = "numbered_content is the reusable current file view for this range; re-read only if exact text outside this range is needed, this view is truncated, or the file changed later."
}

func trimReadContentFields(payload map[string]any, maxRunes int) {
	trimMapString(payload, "numbered_content", maxRunes)
	trimMapString(payload, "content", maxRunes)
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
		preferNumberedReadContentForModel(object)
		trimReadContentFields(object, profile.MaxReadFileContentRunes)
	}
	payload["items"] = rawItems
	payload["model_view"] = "Batch read items use one reusable file view per item; re-read a specific path/range only when its exact text is missing, truncated, or stale after a later file change."
}

func fitPayloadStringFieldsToBudget(payload map[string]any, keys []string, maxRunes int) bool {
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
		key, value := largestPayloadStringField(payload, keys)
		if key == "" || value == "" {
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
		payload["model_view_truncated"] = true
		payload["read_content_state"] = string(ReadContentStateModelViewTruncated)
		payload[key+"_truncation_note"] = fmt.Sprintf("kept first %d chars for model stability", nextLen)
		attachReadContinuationHint(payload)
	}
	return false
}

func largestPayloadStringField(payload map[string]any, keys []string) (string, string) {
	selectedKey := ""
	selectedValue := ""
	for _, key := range keys {
		value, _ := payload[key].(string)
		if len([]rune(value)) > len([]rune(selectedValue)) {
			selectedKey = key
			selectedValue = value
		}
	}
	return selectedKey, selectedValue
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
		item, key, content := largestBatchReadContent(rawItems)
		if item == nil || key == "" || content == "" {
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
		item[key] = string(runes[:nextLen])
		item[key+"_truncated_for_model"] = true
		item["model_view_truncated"] = true
		item["read_content_state"] = string(ReadContentStateModelViewTruncated)
		item[key+"_truncation_note"] = fmt.Sprintf("kept first %d chars for model stability", nextLen)
		attachReadContinuationHint(item)
		payload["model_view"] = "Batch read contents may be truncated for model stability; path, range, and failure metadata are preserved."
	}
	return false
}

func largestBatchReadContent(items []any) (map[string]any, string, string) {
	var selected map[string]any
	selectedKey := ""
	selectedContent := ""
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		for _, key := range []string{"numbered_content", "content"} {
			content, _ := item[key].(string)
			if len([]rune(content)) > len([]rune(selectedContent)) {
				selected = item
				selectedKey = key
				selectedContent = content
			}
		}
	}
	return selected, selectedKey, selectedContent
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
		"model_view":     "The tool result is too large for this model view. Retry with a narrower request if more detail is needed.",
		"original_chars": len([]rune(raw)),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return `{"truncated":true,"model_view":"tool result was too large"}`
	}
	return string(encoded)
}

type ReadContentState string

const (
	ReadContentStateAvailable          ReadContentState = "available"
	ReadContentStateModelViewTruncated ReadContentState = "model_view_truncated"
	ReadContentStateSourceTruncated    ReadContentState = "source_truncated"
	ReadContentStateStaleAfterMutation ReadContentState = "stale_after_mutation"
	ReadContentStateMetadataOnly       ReadContentState = "metadata_only"
)

func attachReadContinuationHint(payload map[string]any) {
	if payload == nil {
		return
	}
	nextStart := intFromAny(payload["end_line"])
	total := intFromAny(payload["total_lines"])
	if nextStart > 0 && (total == 0 || nextStart < total) {
		payload["next_start_line"] = nextStart + 1
	}
	var parts []string
	if path := strings.TrimSpace(stringFromAny(payload["path"])); path != "" {
		parts = append(parts, "path="+path)
	}
	if start := intFromAny(payload["start_line"]); start > 0 {
		parts = append(parts, fmt.Sprintf("start_line=%d", start))
	}
	if end := intFromAny(payload["end_line"]); end > 0 {
		parts = append(parts, fmt.Sprintf("end_line=%d", end))
	}
	if len(parts) > 0 {
		payload["next_action"] = "Use read_workspace_text or read_workspace_files with a narrower line range when exact text is needed (" + strings.Join(parts, ", ") + ")."
	}
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
