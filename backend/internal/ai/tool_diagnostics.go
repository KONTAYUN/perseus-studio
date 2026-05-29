package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"webide-backend/internal/workspace"
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
		"arguments_summary":    summarizeToolArguments(argumentsJSON),
		"target_summary":       strings.TrimSpace(targetSummary),
		"recoverable_by_model": toolFailureRecoverableByModel(failureClass),
	}
	if durationMS > 0 {
		summary["duration_ms"] = durationMS
	}
	if hint := toolFailureRecoveryHint(toolName, errString(err)); hint != "" {
		summary["recovery_hint"] = hint
	}
	raw, _ := json.Marshal(summary)
	return toolDiagnostic{
		Level:          level,
		ErrorMessage:   safeErr,
		RequestSummary: string(raw),
	}
}

func toolFailureClass(toolName string, err error) string {
	if err == nil {
		return ""
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "invalid tool arguments") ||
		strings.Contains(text, "tool arguments are required") ||
		strings.Contains(text, "tool arguments must be"):
		return "tool_invalid_arguments"
	case strings.Contains(text, "all requested files failed to read"):
		return "workspace_batch_read_all_failed"
	case errors.Is(err, fs.ErrNotExist) || strings.Contains(text, "no such file or directory") || strings.Contains(text, "file does not exist"):
		return "workspace_path_not_found"
	case workspace.IsPathError(err) || strings.Contains(text, "path escapes workspace") || strings.Contains(text, "invalid path"):
		return "workspace_path_policy"
	case strings.Contains(text, "未配置可用搜索工具"):
		return "search_provider_not_configured"
	case strings.Contains(text, "tavily extract returned no content") || strings.Contains(text, "当前 provider 不支持 extract"):
		return "web_extract_unavailable"
	case strings.Contains(text, "skill not found") || strings.Contains(text, "skill is disabled"):
		return "skill_unavailable"
	case strings.Contains(text, "zip ") ||
		strings.Contains(text, "skill") && (strings.Contains(text, "安装") || strings.Contains(text, "not a directory") || strings.Contains(text, "mkdir")):
		return "skill_install_invalid_archive"
	case strings.Contains(text, "safety policy"):
		return "command_safety_policy"
	case strings.Contains(text, "exit status") || strings.Contains(text, "timed out"):
		return "command_execution_failed"
	default:
		_ = toolName
		return "tool_execution_failed"
	}
}

func toolDiagnosticLevel(failureClass string) string {
	switch failureClass {
	case "workspace_path_not_found",
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
	switch failureClass {
	case "workspace_path_not_found",
		"workspace_batch_read_all_failed",
		"web_extract_unavailable",
		"skill_unavailable",
		"tool_invalid_arguments":
		return true
	default:
		return false
	}
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
