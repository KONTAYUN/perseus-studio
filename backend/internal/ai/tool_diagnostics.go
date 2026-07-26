package ai

import (
	"encoding/json"
	"fmt"
	"strings"
)

type toolDiagnostic struct {
	Level          string
	ErrorMessage   string
	RequestSummary string
}

func buildToolDiagnostic(toolName, argumentsJSON, targetSummary string, durationMS int64, err error) toolDiagnostic {
	failureClass := toolFailureClass(toolName, err)
	level := toolDiagnosticLevel(failureClass)
	safeErr := sanitizeToolDiagnosticError(err)
	summary := map[string]any{
		"failure_class":        failureClass,
		"tool_name":            toolName,
		"tool_manifest_digest": toolManifestDigest(),
		"arguments_summary":    summarizeToolArguments(argumentsJSON),
		"target_summary":       strings.TrimSpace(targetSummary),
		"recoverable_by_model": toolFailureRecoverableByModel(failureClass),
	}
	if metadata := manifestToolDiagnosticMetadata(toolName); len(metadata) > 0 {
		summary["tool_manifest_entry"] = metadata
	}
	if protocol := manifestRecommendedProtocol(toolName); protocol != "" {
		summary["recommended_protocol"] = protocol
	}
	if primary := manifestPrimaryEditProtocol(); primary != "" {
		summary["internal_edit_protocol"] = primary
	}
	if durationMS > 0 {
		summary["duration_ms"] = durationMS
	}
	if hint := toolFailureRecoveryHint(err); hint != "" {
		summary["recovery_hint"] = hint
	}
	if structured := toolErrorSummary(err); len(structured) > 0 {
		summary["structured_error"] = structured
	}
	if details := toolArgumentDiagnosticDetails(toolName, argumentsJSON); len(details) > 0 {
		summary["argument_details"] = details
	}
	raw, _ := json.Marshal(summary)
	return toolDiagnostic{
		Level:          level,
		ErrorMessage:   safeErr,
		RequestSummary: string(raw),
	}
}

func toolArgumentDiagnosticDetails(toolName, argumentsJSON string) map[string]any {
	var args map[string]any
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return nil
	}
	switch toolName {
	case "apply_workspace_patch":
		patch := stringField(args, "patch")
		details := map[string]any{
			"path":              pathSummary(firstPatchPathFromText(patch)),
			"patch_chars":       len([]rune(patch)),
			"patch_sha":         hashWorkspaceText(patch),
			"patch_head":        patchDiagnosticHead(patch),
			"patch_tail":        patchDiagnosticTail(patch),
			"patch_block_count": countWorkspacePatchBlocks(patch),
			"detected_paths":    workspacePatchCandidatePathsLoose(patch),
			"transport":         "structured_tool",
		}
		return details
	case "edit_workspace_text":
		return map[string]any{
			"path":           pathSummary(stringField(args, "path")),
			"old_text_chars": len([]rune(stringField(args, "old_text"))),
			"new_text_chars": len([]rune(stringField(args, "new_text"))),
			"replace_all":    boolField(args, "replace_all"),
		}
	case "replace_workspace_lines":
		return map[string]any{
			"path":           pathSummary(stringField(args, "path")),
			"start_line":     intField(args, "start_line"),
			"end_line":       intField(args, "end_line"),
			"new_text_chars": len([]rune(stringField(args, "new_text"))),
		}
	case "save_workspace_file", "write_workspace_chunk":
		return map[string]any{
			"path":          pathSummary(stringField(args, "path")),
			"content_chars": len([]rune(stringField(args, "content"))),
			"mode":          stringField(args, "mode"),
		}
	default:
		return nil
	}
}

func boolField(values map[string]any, key string) bool {
	value, _ := values[key].(bool)
	return value
}

func intField(values map[string]any, key string) int {
	switch value := values[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}

func toolFailureClass(toolName string, err error) string {
	_ = toolName
	if err == nil {
		return ""
	}
	return string(normalizeToolError(err).Code)
}

func toolDiagnosticLevel(failureClass string) string {
	switch failureClass {
	case "workspace_path_not_found",
		"workspace_path_already_exists",
		"workspace_unsupported_file_type",
		"workspace_text_match_not_found",
		"workspace_text_match_ambiguous",
		"workspace_line_range_invalid",
		"workspace_patch_apply_failed",
		"workspace_batch_read_all_failed",
		"search_provider_not_configured",
		"web_extract_unavailable",
		"skill_unavailable",
		"tool_invalid_arguments":
		return "warn"
	default:
		return "error"
	}
}

func toolFailureRecoverableByModel(failureClass string) bool {
	return toolErrorMetadataForCode(ToolErrorCode(failureClass)).Retryable
}

func sanitizeToolDiagnosticError(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	text = sanitizeWorkspaceAbsolutePaths(text)
	return text
}

func sanitizeWorkspaceAbsolutePaths(text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	normalized := strings.ReplaceAll(text, "\\", "/")
	marker := "/workspace/"
	for {
		index := strings.Index(normalized, marker)
		if index < 0 {
			break
		}
		start := index
		for start > 0 && !isDiagnosticPathBoundary(rune(normalized[start-1])) {
			start--
		}
		relativeStart := index + len(marker)
		end := relativeStart
		for end < len(normalized) && !isDiagnosticPathBoundary(rune(normalized[end])) {
			end++
		}
		replacement := "workspace:" + normalized[relativeStart:end]
		normalized = normalized[:start] + replacement + normalized[end:]
	}
	return normalized
}

func isDiagnosticPathBoundary(ch rune) bool {
	switch ch {
	case ' ', '\t', '\n', '\r', '"', '\'', ':':
		return true
	default:
		return false
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprint(err)
}
