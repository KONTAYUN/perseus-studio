package ai

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"

	"webide-backend/internal/workspace"
)

type ToolErrorCode string

const (
	ToolErrorInvalidArguments      ToolErrorCode = "tool_invalid_arguments"
	ToolErrorUnsupportedFileType   ToolErrorCode = "workspace_unsupported_file_type"
	ToolErrorTextMatchMissing      ToolErrorCode = "workspace_text_match_not_found"
	ToolErrorTextMatchAmbiguous    ToolErrorCode = "workspace_text_match_ambiguous"
	ToolErrorPatchApplyFailed      ToolErrorCode = "workspace_patch_apply_failed"
	ToolErrorLineRangeInvalid      ToolErrorCode = "workspace_line_range_invalid"
	ToolErrorPathExists            ToolErrorCode = "workspace_path_already_exists"
	ToolErrorBatchReadFailed       ToolErrorCode = "workspace_batch_read_all_failed"
	ToolErrorPathNotFound          ToolErrorCode = "workspace_path_not_found"
	ToolErrorPathPolicy            ToolErrorCode = "workspace_path_policy"
	ToolErrorSearchNotConfigured   ToolErrorCode = "search_provider_not_configured"
	ToolErrorWebExtractUnavailable ToolErrorCode = "web_extract_unavailable"
	ToolErrorSkillUnavailable      ToolErrorCode = "skill_unavailable"
	ToolErrorSkillArchiveInvalid   ToolErrorCode = "skill_install_invalid_archive"
	ToolErrorCommandRejected       ToolErrorCode = "command_safety_policy"
	ToolErrorCommandTimedOut       ToolErrorCode = "command_timed_out"
	ToolErrorCommandFailed         ToolErrorCode = "command_execution_failed"
	ToolErrorVerificationFailed    ToolErrorCode = "tool_verification_failed"
	ToolErrorLocalPersistence      ToolErrorCode = "tool_local_persistence_failed"
	ToolErrorExecutionFailed       ToolErrorCode = "tool_execution_failed"
)

type ToolError struct {
	Code              ToolErrorCode
	Category          string
	Retryable         bool
	Target            string
	Details           map[string]any
	RecommendedAction string
	Cause             error
	Message           string
}

func (e *ToolError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return string(e.Code)
}

func (e *ToolError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func newToolError(code ToolErrorCode, message string, cause error) *ToolError {
	meta := toolErrorMetadataForCode(code)
	return &ToolError{
		Code:              code,
		Category:          meta.Category,
		Retryable:         meta.Retryable,
		RecommendedAction: meta.RecommendedAction,
		Cause:             cause,
		Message:           message,
	}
}

func toolErrorFromCode(code ToolErrorCode, message string) error {
	return newToolError(code, message, nil)
}

func normalizeToolError(err error) *ToolError {
	if err == nil {
		return nil
	}
	var typed *ToolError
	if errors.As(err, &typed) {
		return typed
	}
	switch {
	case errors.Is(err, workspace.ErrUnsupportedFile):
		return newToolError(ToolErrorUnsupportedFileType, err.Error(), err)
	case errors.Is(err, workspace.ErrTextMatchMissing):
		return newToolError(ToolErrorTextMatchMissing, err.Error(), err)
	case errors.Is(err, workspace.ErrTextMatchMany):
		return newToolError(ToolErrorTextMatchAmbiguous, err.Error(), err)
	case errors.Is(err, workspace.ErrInvalidLineRange):
		return newToolError(ToolErrorLineRangeInvalid, err.Error(), err)
	case errors.Is(err, workspace.ErrPathExists):
		return newToolError(ToolErrorPathExists, err.Error(), err)
	case errors.Is(err, workspace.ErrPathNotFound), errors.Is(err, fs.ErrNotExist):
		return newToolError(ToolErrorPathNotFound, err.Error(), err)
	case workspace.IsPathError(err):
		return newToolError(ToolErrorPathPolicy, err.Error(), err)
	case errors.Is(err, workspace.ErrCommandRejected):
		return newToolError(ToolErrorCommandRejected, err.Error(), err)
	case errors.Is(err, workspace.ErrCommandTimedOut), errors.Is(err, context.DeadlineExceeded):
		return newToolError(ToolErrorCommandTimedOut, err.Error(), err)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return newToolError(ToolErrorCommandFailed, err.Error(), err)
	}
	return newToolError(ToolErrorExecutionFailed, err.Error(), err)
}

type toolErrorMetadata struct {
	Category          string
	Retryable         bool
	RecommendedAction string
}

func toolErrorMetadataForCode(code ToolErrorCode) toolErrorMetadata {
	switch code {
	case ToolErrorInvalidArguments:
		return toolErrorMetadata{"protocol", true, "Correct the tool arguments and retry with the same tool only when the target is still valid."}
	case ToolErrorUnsupportedFileType:
		return toolErrorMetadata{"workspace_read", false, "Use a supported text or note view, or ask the user for a supported export."}
	case ToolErrorTextMatchMissing, ToolErrorTextMatchAmbiguous, ToolErrorPatchApplyFailed, ToolErrorLineRangeInvalid:
		return toolErrorMetadata{"workspace_edit", true, "Read the current target again, then submit a corrected patch against the current content."}
	case ToolErrorPathExists:
		return toolErrorMetadata{"workspace_path", true, "Inspect the existing path and choose a non-conflicting action."}
	case ToolErrorBatchReadFailed, ToolErrorPathNotFound:
		return toolErrorMetadata{"workspace_read", true, "Locate the current workspace path before retrying."}
	case ToolErrorPathPolicy, ToolErrorCommandRejected:
		return toolErrorMetadata{"policy", false, "Do not bypass the policy; choose a workspace-scoped safe action or explain the limitation."}
	case ToolErrorSearchNotConfigured:
		return toolErrorMetadata{"configuration", false, "Continue without live search or ask the user to configure a search provider."}
	case ToolErrorWebExtractUnavailable:
		return toolErrorMetadata{"web", true, "Try another verified result URL or continue from the available search summary."}
	case ToolErrorSkillUnavailable:
		return toolErrorMetadata{"skill", true, "List available skills and retry with an enabled skill id."}
	case ToolErrorSkillArchiveInvalid:
		return toolErrorMetadata{"skill", false, "Use a valid skill archive and retry installation."}
	case ToolErrorCommandTimedOut, ToolErrorCommandFailed:
		return toolErrorMetadata{"command", true, "Inspect the structured exit status and output, then adjust the command if it remains safe."}
	case ToolErrorVerificationFailed:
		return toolErrorMetadata{"verification", true, "Inspect the workspace state and retry the mutation only after resolving the verification mismatch."}
	case ToolErrorLocalPersistence:
		return toolErrorMetadata{"persistence", false, "Pause the run and repair local persistence before continuing."}
	default:
		return toolErrorMetadata{"execution", false, "Inspect the structured error details before choosing another action."}
	}
}

func toolErrorSummary(err error) map[string]any {
	typed := normalizeToolError(err)
	if typed == nil {
		return nil
	}
	out := map[string]any{
		"code":               typed.Code,
		"category":           typed.Category,
		"retryable":          typed.Retryable,
		"recommended_action": typed.RecommendedAction,
	}
	if typed.Target != "" {
		out["target"] = typed.Target
	}
	if len(typed.Details) > 0 {
		out["details"] = typed.Details
	}
	return out
}

func toolErrorJSONPayload(toolName string, err error) string {
	typed := normalizeToolError(err)
	if typed == nil {
		typed = newToolError(ToolErrorExecutionFailed, "tool execution failed", nil)
	}
	payload := map[string]any{
		"error":              typed.Error(),
		"error_code":         typed.Code,
		"error_category":     typed.Category,
		"retryable":          typed.Retryable,
		"tool_name":          toolName,
		"recommended_action": typed.RecommendedAction,
	}
	if typed.Target != "" {
		payload["target"] = typed.Target
	}
	if len(typed.Details) > 0 {
		payload["details"] = typed.Details
	}
	return mustJSON(payload)
}

func toolErrorFromEvent(event ToolEvent) error {
	if event.ErrorCode == "" {
		return newToolError(ToolErrorExecutionFailed, event.Error, nil)
	}
	return &ToolError{
		Code:              event.ErrorCode,
		Category:          event.ErrorCategory,
		Retryable:         event.Retryable,
		RecommendedAction: event.RecommendedAction,
		Message:           event.Error,
		Target:            event.TargetSummary,
	}
}

func wrapToolError(code ToolErrorCode, format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	return newToolError(code, message, nil)
}
