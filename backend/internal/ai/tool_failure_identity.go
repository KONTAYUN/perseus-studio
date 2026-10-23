package ai

import (
	"context"
	"errors"
	"io/fs"

	"webide-backend/internal/failure"
	"webide-backend/internal/workspace"
)

func toolFailureClass(toolName string, err error) string {
	if err == nil {
		return ""
	}
	if code := failure.Code(err); code != "" {
		return code
	}
	var patch workspacePatchApplyError
	switch {
	case errors.Is(err, context.Canceled):
		return "execution_canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "execution_timeout"
	case workspace.IsPathError(err):
		return "workspace_path_policy"
	case errors.Is(err, workspace.ErrFileConflict):
		return "workspace_file_conflict"
	case errors.Is(err, workspace.ErrPathExists):
		return "workspace_path_already_exists"
	case errors.Is(err, fs.ErrNotExist):
		return "workspace_path_not_found"
	case errors.As(err, &patch):
		return "workspace_patch_apply_failed"
	default:
		return "tool_execution_failed"
	}
}

func recoveryHintForFailure(code string) string {
	switch code {
	case "workspace_patch_apply_failed", "workspace_exact_edit_unstable", "workspace_file_conflict":
		return "Read the current target again. Preserve exact indentation and characters, then submit a corrected structured apply_workspace_patch call (*** Begin Patch ... *** End Patch). Do not repeat an unchanged rejected patch."
	case "workspace_path_policy", "command_disabled", "command_safety_policy":
		return "Respect the execution policy. Do not bypass it with another tool; report blocked or partial if the task requires unavailable access."
	case "workspace_unsupported_file_type":
		return "This tool only reads supported plain-text files; explain the unsupported format or choose an appropriate available tool."
	case "command_execution_failed", "execution_timeout":
		return "Inspect exit_code, timed_out and stdout/stderr. A completed invocation does not mean the command succeeded; verify existing side effects before retrying."
	case "tool_invalid_arguments":
		return "Correct the arguments to match the tool schema, then continue."
	default:
		return "Inspect the structured failure and current workspace state before retrying; report limitations honestly."
	}
}
