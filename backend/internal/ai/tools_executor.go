package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"webide-backend/internal/failure"
	"webide-backend/internal/idgen"
	"webide-backend/internal/store"
	"webide-backend/internal/workspace"
)

func (s *Service) executeAndRecordTool(ctx context.Context, conversationID, messageID, name, argumentsJSON, operationHash string, opts RunOptions, profile providerRuntimeProfile) (string, ToolEvent, error) {
	startedAt := time.Now()
	toolCallID := idgen.NewAt("tc", startedAt)
	targetSummary := toolTargetSummary(name, argumentsJSON)
	logTask(opts, "info", fmt.Sprintf("tool call started: %s", name))
	emitToolLifecycleEvent(opts, "tool_call_started", "info", map[string]any{
		"tool_call_id":      toolCallID,
		"tool_name":         name,
		"status_kind":       manifestStatusKind(name),
		"status":            "running",
		"operation_hash":    operationHash,
		"arguments_summary": summarizeToolArguments(argumentsJSON),
		"target_summary":    targetSummary,
	})
	if s.calls != nil {
		recorded, recordErr := s.calls.StartToolCall(ctx, store.ToolCallInput{
			ID:             toolCallID,
			UserID:         opts.UserID,
			RunID:          opts.RunID,
			ConversationID: conversationID,
			MessageID:      messageID,
			TaskID:         opts.TaskID,
			ToolName:       name,
			OperationHash:  operationHash,
			ArgumentsJSON:  ledgerToolArgumentsJSON(argumentsJSON),
			StartedAt:      startedAt,
		})
		if recordErr != nil {
			wrapped := fmt.Errorf("local tool ledger start failed for %s: %w", name, recordErr)
			diagID := s.recordLocalPersistenceDiagnostic(ctx, opts, conversationID, messageID, "tool_ledger_start_failed", wrapped)
			emitToolLifecycleEvent(opts, "local_persistence_failed", "error", map[string]any{
				"tool_call_id":      toolCallID,
				"tool_name":         name,
				"stage":             "tool_ledger_start_failed",
				"error":             wrapped.Error(),
				"diagnostic_log_id": diagID,
				"operation_hash":    operationHash,
			})
			return "", ToolEvent{ToolCallID: toolCallID, ToolName: name, StatusKind: manifestStatusKind(name), Label: "工具未执行", Status: "failed", Error: wrapped.Error(), OperationHash: operationHash}, WithDiagnosticID(wrapped, diagID)
		}
		if recorded.ID != "" {
			toolCallID = recorded.ID
		}
	}
	result, summary, label, path, err := s.executeTool(ctx, name, argumentsJSON, opts)
	durationMS := time.Since(startedAt).Milliseconds()
	verification := s.verifyToolResult(ctx, name, result, opts)
	status := "success"
	errorText := ""
	if err != nil {
		status = "failed"
		errorText = err.Error()
		if !jsonObjectHasField(result, "error") {
			result = mustJSON(map[string]any{
				"error":         errorText,
				"failure_class": toolFailureClass(name, err),
				"recovery_hint": recoveryHintForFailure(toolFailureClass(name, err)),
			})
		}
		var failurePayload map[string]any
		if json.Unmarshal([]byte(result), &failurePayload) == nil {
			failurePayload["failure_class"] = toolFailureClass(name, err)
			result = mustJSON(failurePayload)
		}
		summary = errorText
		s.recordToolDiagnostic(ctx, opts, conversationID, messageID, name, argumentsJSON, targetSummary, durationMS, err)
		verification = ToolVerification{Status: "skipped", Message: "tool execution failed"}
	}
	if status == "success" && verification.Status == "failed" {
		status = "failed"
		err = failure.New("workspace_verification_failed", verification.Message, nil)
		errorText = verification.Message
		result = mustJSON(map[string]any{
			"error":         errorText,
			"recovery_hint": "The tool ran but verification failed. Inspect the workspace state before claiming success.",
			"verification":  verification,
		})
		summary = "workspace verification failed: " + verification.Message
		s.recordToolDiagnostic(ctx, opts, conversationID, messageID, name, argumentsJSON, targetSummary, durationMS, fmt.Errorf("workspace verification failed: %s", verification.Message))
	}
	if status == "success" {
		label = userFriendlyToolLabel(name)
	}
	eventPayload := map[string]any{
		"tool_call_id":      toolCallID,
		"tool_name":         name,
		"status_kind":       manifestStatusKind(name),
		"status":            status,
		"label":             label,
		"path":              path,
		"operation_hash":    operationHash,
		"arguments_summary": summarizeToolArguments(argumentsJSON),
		"result_summary":    truncate(summary, 500),
		"target_summary":    targetSummary,
		"duration_ms":       durationMS,
		"verification":      verification,
	}
	if errorText != "" {
		eventPayload["error"] = errorText
		eventPayload["recovery_hint"] = recoveryHintForFailure(toolFailureClass(name, err))
	}
	event := ToolEvent{FailureClass: toolFailureClass(name, err), ToolCallID: toolCallID, ToolName: name, StatusKind: manifestStatusKind(name), Label: label, Status: status, TargetSummary: targetSummary, OperationHash: operationHash, Path: path, Error: errorText, DurationMS: durationMS}
	if status == "success" {
		var meta struct {
			ExecutionOutcome string                 `json:"execution_outcome"`
			Path             string                 `json:"path"`
			OldPath          string                 `json:"old_path"`
			NewPath          string                 `json:"new_path"`
			ChangedPaths     []string               `json:"changed_paths"`
			Moved            []WorkspacePathMove    `json:"moved"`
			Changes          []workspace.FileChange `json:"changes"`
			Operation        string                 `json:"operation"`
			Question         string                 `json:"question"`
			Options          []string               `json:"options"`
			AllowFreeform    *bool                  `json:"allow_freeform"`
			VersionID        string                 `json:"version_id"`
			Hash             string                 `json:"hash"`
			SkillID          string                 `json:"skill_id"`
			Command          string                 `json:"command"`
			Cwd              string                 `json:"cwd"`
			Stdout           string                 `json:"stdout"`
			Stderr           string                 `json:"stderr"`
			ExitCode         *int                   `json:"exit_code"`
			TimedOut         bool                   `json:"timed_out"`
			Provider         string                 `json:"provider"`
			URL              string                 `json:"url"`
		}
		if err := json.Unmarshal([]byte(result), &meta); err == nil {
			if meta.Path != "" {
				event.Path = meta.Path
			}
			event.OldPath = meta.OldPath
			event.NewPath = meta.NewPath
			event.ChangedPaths = meta.ChangedPaths
			event.Moved = meta.Moved
			event.Changes = meta.Changes
			event.Operation = meta.Operation
			event.Question = meta.Question
			event.Options = meta.Options
			event.AllowFreeform = meta.AllowFreeform
			event.VersionID = meta.VersionID
			event.Hash = meta.Hash
			event.SkillID = meta.SkillID
			event.Command = meta.Command
			event.Cwd = meta.Cwd
			event.Stdout = meta.Stdout
			event.Stderr = meta.Stderr
			event.ExitCode = meta.ExitCode
			event.TimedOut = meta.TimedOut
			event.ExecutionOutcome = meta.ExecutionOutcome
			eventPayload["execution_outcome"] = meta.ExecutionOutcome
			event.Provider = meta.Provider
			event.URL = meta.URL
		}
	}
	event.Label = completedToolLabel(event)
	eventPayload["label"] = event.Label
	eventPayload["failure_class"] = event.FailureClass
	changedPathsJSON := ""
	if len(event.ChangedPaths) > 0 {
		changedRaw, _ := json.Marshal(event.ChangedPaths)
		changedPathsJSON = string(changedRaw)
	}
	if s.calls != nil {
		recorded, recordErr := s.calls.CompleteToolCall(ctx, store.ToolCallInput{
			ID:               toolCallID,
			UserID:           opts.UserID,
			ResultSummary:    truncate(summary, 500),
			Status:           status,
			Error:            truncate(errorText, 500),
			CompletedAt:      time.Now().UTC(),
			DurationMS:       durationMS,
			ResultJSON:       ledgerToolResultJSON(name, result),
			ChangedPathsJSON: changedPathsJSON,
			VersionID:        event.VersionID,
			ContentHash:      event.Hash,
		})
		if recordErr != nil {
			wrapped := fmt.Errorf("local tool ledger completion failed for %s: %w", name, recordErr)
			diagID := s.recordLocalPersistenceDiagnostic(ctx, opts, conversationID, messageID, "tool_ledger_complete_failed", wrapped)
			emitToolLifecycleEvent(opts, "local_persistence_failed", "error", map[string]any{
				"tool_call_id":      toolCallID,
				"tool_name":         name,
				"stage":             "tool_ledger_complete_failed",
				"error":             wrapped.Error(),
				"diagnostic_log_id": diagID,
				"operation_hash":    operationHash,
				"changed_paths":     event.ChangedPaths,
				"path":              event.Path,
				"version_id":        event.VersionID,
				"hash":              event.Hash,
			})
			return budgetToolResultForModel(name, result, profile), event, WithDiagnosticID(wrapped, diagID)
		}
		if recorded.ID != "" {
			toolCallID = recorded.ID
			event.ToolCallID = recorded.ID
			eventPayload["tool_call_id"] = recorded.ID
		}
	}
	if status == "success" {
		event.TargetSummary = completedToolTargetSummary(name, event, argumentsJSON)
		eventPayload["target_summary"] = event.TargetSummary
		if event.Path != "" {
			eventPayload["path"] = event.Path
		}
		if event.OldPath != "" {
			eventPayload["old_path"] = event.OldPath
		}
		if event.NewPath != "" {
			eventPayload["new_path"] = event.NewPath
		}
		if len(event.ChangedPaths) > 0 {
			eventPayload["changed_paths"] = event.ChangedPaths
		}
		if len(event.Moved) > 0 {
			eventPayload["moved"] = event.Moved
		}
		if len(event.Changes) > 0 {
			eventPayload["changes"] = event.Changes
		}
		event.Operation = workspaceFileChangedOperation(name, event)
		if event.Operation != "" {
			eventPayload["operation"] = event.Operation
		}
		if event.Hash != "" {
			eventPayload["hash"] = event.Hash
		}
		if event.VersionID != "" {
			eventPayload["version_id"] = event.VersionID
		}
	}
	if status == "success" {
		log.Printf("[ai] tool conversation=%s name=%s status=%s path=%s", conversationID, name, status, path)
		logTask(opts, "info", fmt.Sprintf("tool call done: %s", name))
		if shouldEmitWorkspaceFileChanged(name, event) {
			event.Type = "workspace_file_changed"
		}
		emitToolLifecycleEvent(opts, "tool_call_completed", "info", eventPayload)
		if shouldEmitWorkspaceFileChanged(name, event) {
			emitToolLifecycleEvent(opts, "workspace_file_changed", "info", map[string]any{
				"type":           "workspace_file_changed",
				"tool_call_id":   toolCallID,
				"tool_name":      name,
				"status_kind":    manifestStatusKind(name),
				"status":         status,
				"path":           event.Path,
				"old_path":       event.OldPath,
				"new_path":       event.NewPath,
				"changed_paths":  event.ChangedPaths,
				"moved":          event.Moved,
				"changes":        event.Changes,
				"operation":      event.Operation,
				"hash":           event.Hash,
				"version_id":     event.VersionID,
				"operation_hash": event.OperationHash,
			})
		}
	} else {
		log.Printf("[ai] tool conversation=%s name=%s status=%s path=%s error=%s", conversationID, name, status, path, errorText)
		logTask(opts, "error", fmt.Sprintf("tool call failed: %s: %s", name, errorText))
		emitToolLifecycleEvent(opts, "tool_call_failed", "error", eventPayload)
	}
	return budgetToolResultForModel(name, result, profile), event, nil
}

func jsonObjectHasField(raw string, field string) bool {
	if raw == "" || field == "" {
		return false
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return false
	}
	_, ok := decoded[field]
	return ok
}

func shouldEmitWorkspaceFileChanged(name string, event ToolEvent) bool {
	if len(event.ChangedPaths) == 0 && event.Path == "" && event.OldPath == "" && event.NewPath == "" {
		return false
	}
	if isWorkspaceMutationTool(name) {
		return true
	}
	return name == "run_workspace_command" && len(event.ChangedPaths) > 0
}

func workspaceFileChangedOperation(name string, event ToolEvent) string {
	switch name {
	case "apply_workspace_patch":
		return "patch"
	case "save_workspace_file", "write_workspace_chunk", "finalize_workspace_write", "edit_workspace_text", "replace_workspace_lines":
		return "update"
	case "organize_workspace_matches":
		return "organize"
	case "run_workspace_command":
		if len(event.ChangedPaths) > 0 {
			return "command_unknown"
		}
	case "manage_workspace_path":
		switch {
		case event.OldPath != "" && event.NewPath != "" && (workspace.IsTrashPath(event.NewPath) || workspace.IsLegacyTrashPath(event.NewPath)):
			return "trash"
		case event.OldPath != "" && event.NewPath != "":
			return "move"
		case event.Path != "":
			return "mkdir"
		}
	}
	if event.Operation != "" {
		return event.Operation
	}
	return ""
}
