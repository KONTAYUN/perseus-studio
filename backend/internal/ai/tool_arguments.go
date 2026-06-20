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

func executionToolArgumentsJSON(call agentToolCall) string {
	if call.Name == "apply_workspace_patch" && strings.TrimSpace(call.PatchText) != "" {
		return mustJSON(map[string]any{
			"patch":     call.PatchText,
			"transport": "assistant_text_patch",
		})
	}
	return call.ArgumentsJSON
}

func ledgerToolCallArgumentsJSON(call agentToolCall) string {
	return ledgerToolArgumentsJSON(executionToolArgumentsJSON(call))
}

func toolCallTranscriptArgumentsJSON(call agentToolCall) string {
	if call.Name == "apply_workspace_patch" && strings.TrimSpace(call.PatchText) != "" {
		return mustJSON(map[string]any{
			"patch":       "[assistant_text_patch captured by backend]",
			"transport":   "assistant_text_patch",
			"patch_chars": len([]rune(call.PatchText)),
		})
	}
	return call.ArgumentsJSON
}

func summarizeToolArgumentValue(value any) string {
	switch item := value.(type) {
	case map[string]any:
		summary := make(map[string]any, len(item))
		for key, raw := range item {
			if key == "content" || key == "patch" {
				if text, ok := raw.(string); ok {
					summary[key] = fmt.Sprintf("[%s %d chars]", key, len([]rune(text)))
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

func toolTargetSummary(name, argumentsJSON string) string {
	var decoded map[string]any
	if err := json.Unmarshal([]byte(argumentsJSON), &decoded); err != nil {
		return ""
	}
	switch name {
	case "find_workspace_files":
		return quotedSummary(stringField(decoded, "query", "pattern"))
	case "search_workspace_text", "perseus_search_web", "web_search":
		return quotedSummary(stringField(decoded, "query"))
	case "inspect_workspace", "read_workspace_note_view", "read_workspace_text", "apply_workspace_patch", "edit_workspace_text", "replace_workspace_lines", "save_workspace_file", "write_workspace_chunk", "finalize_workspace_write":
		if name == "apply_workspace_patch" {
			if path := firstPatchPathFromText(stringField(decoded, "patch")); path != "" {
				return pathSummary(path)
			}
		}
		return pathSummary(stringField(decoded, "path"))
	case "read_workspace_files":
		return pathsCountSummary(stringSliceField(decoded, "paths"))
	case "manage_workspace_path":
		return pathSummary(stringField(decoded, "path"))
	case "organize_workspace_matches":
		target := pathSummary(stringField(decoded, "target_folder"))
		match := quotedSummary(stringField(decoded, "match_text"))
		if target != "" && match != "" {
			return target + " / " + match
		}
		if target != "" {
			return target
		}
		return match
	case "web_extract":
		return truncatePlainRunes(stringField(decoded, "url"), 96)
	case "run_workspace_command":
		if cwd := pathSummary(stringField(decoded, "cwd")); cwd != "" {
			return cwd
		}
		return "workspace"
	case "read_skill", "delete_skill":
		return truncatePlainRunes(stringField(decoded, "skill_id"), 80)
	case "install_skill_from_attachment":
		return truncatePlainRunes(stringField(decoded, "attachment_id"), 80)
	default:
		return ""
	}
}

func completedToolTargetSummary(name string, event ToolEvent, argumentsJSON string) string {
	if len(event.ChangedPaths) > 1 {
		return fmt.Sprintf("%d 个路径", len(event.ChangedPaths))
	}
	for _, candidate := range []string{event.Path, event.NewPath, event.OldPath, event.URL, event.SkillID, event.Cwd} {
		if text := pathSummary(candidate); text != "" {
			return text
		}
	}
	if len(event.ChangedPaths) == 1 {
		return pathSummary(event.ChangedPaths[0])
	}
	return toolTargetSummary(name, argumentsJSON)
}

func stringField(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func stringSliceField(values map[string]any, key string) []string {
	raw, ok := values[key].([]any)
	if !ok {
		return nil
	}
	items := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
			items = append(items, text)
		}
	}
	return items
}

func quotedSummary(value string) string {
	value = truncatePlainRunes(value, 80)
	if value == "" {
		return ""
	}
	return "“" + value + "”"
}

func pathSummary(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	value = strings.Trim(value, "/")
	if value == "" || value == "." {
		return ""
	}
	return truncatePlainRunes(value, 96)
}

func pathsCountSummary(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	if len(paths) == 1 {
		return pathSummary(paths[0])
	}
	return fmt.Sprintf("%d 个文件", len(paths))
}
