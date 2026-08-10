package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"webide-backend/internal/idgen"
	"webide-backend/internal/store"
	"webide-backend/internal/workspace"
)

func (s *Service) executeAndRecordTool(ctx context.Context, conversationID, messageID, name, argumentsJSON, operationHash string, opts RunOptions, profile providerRuntimeProfile) (string, ToolEvent, error) {
	startedAt := time.Now()
	toolCallID := opts.CurrentToolCallID
	if toolCallID == "" {
		toolCallID = idgen.NewAt("tc", startedAt)
	}
	if s.calls != nil && strings.TrimSpace(opts.UserID) != "" && strings.TrimSpace(opts.RunID) != "" {
		if existing, lookupErr := s.calls.GetToolCallForRun(ctx, opts.UserID, opts.RunID, toolCallID); lookupErr == nil {
			if existing.RunID != opts.RunID || existing.ToolName != name || existing.OperationHash != operationHash {
				typed := newToolError(ToolErrorCallIntegrity, "tool call id was replayed with different run, tool, or arguments", nil)
				typed.Details = map[string]any{
					"tool_call_id":       toolCallID,
					"existing_run_id":    existing.RunID,
					"current_run_id":     opts.RunID,
					"existing_tool_name": existing.ToolName,
					"current_tool_name":  name,
				}
				return toolErrorJSONPayload(name, typed), ToolEvent{
					ToolCallID: toolCallID, ToolName: name, StatusKind: manifestStatusKind(name),
					Label: userFriendlyToolLabel(name), Status: "failed", Error: typed.Error(),
					ErrorCode: typed.Code, ErrorCategory: typed.Category,
				}, typed
			}
			if existing.Status == "running" {
				typed := newToolError(ToolErrorCallIntegrity, "tool call replay arrived while the original call is still running", nil)
				typed.Retryable = true
				return toolErrorJSONPayload(name, typed), ToolEvent{
					ToolCallID: toolCallID, ToolName: name, StatusKind: manifestStatusKind(name),
					Label: userFriendlyToolLabel(name), Status: "failed", Error: typed.Error(),
					ErrorCode: typed.Code, ErrorCategory: typed.Category, Retryable: true,
				}, typed
			}
			event := toolEventFromRecordedCall(existing)
			emitToolLifecycleEvent(opts, "tool_call_replayed", "info", map[string]any{
				"tool_call_id":   toolCallID,
				"tool_name":      name,
				"run_id":         opts.RunID,
				"operation_hash": operationHash,
			})
			return existing.ResultJSON, event, nil
		} else if lookupErr != sql.ErrNoRows {
			return "", ToolEvent{}, lookupErr
		}
	}
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
			wrapped := newToolError(ToolErrorLocalPersistence, fmt.Sprintf("local tool ledger start failed for %s", name), recordErr)
			diagID := s.recordLocalPersistenceDiagnostic(ctx, opts, conversationID, messageID, "tool_ledger_start_failed", wrapped)
			emitToolLifecycleEvent(opts, "local_persistence_failed", "error", map[string]any{
				"tool_call_id":      toolCallID,
				"tool_name":         name,
				"stage":             "tool_ledger_start_failed",
				"error":             wrapped.Error(),
				"diagnostic_log_id": diagID,
				"operation_hash":    operationHash,
			})
			return "", ToolEvent{ToolCallID: toolCallID, ToolName: name, StatusKind: manifestStatusKind(name), Label: userFriendlyToolLabel(name), Status: "failed", Error: wrapped.Error(), ErrorCode: wrapped.Code, ErrorCategory: wrapped.Category, Retryable: wrapped.Retryable, RecommendedAction: wrapped.RecommendedAction, OperationHash: operationHash}, WithDiagnosticID(wrapped, diagID)
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
		err = normalizeToolError(err)
		status = "failed"
		errorText = err.Error()
		if !jsonObjectHasField(result, "error") {
			result = toolErrorJSONPayload(name, err)
		}
		summary = errorText
		s.recordToolDiagnostic(ctx, opts, conversationID, messageID, name, argumentsJSON, targetSummary, durationMS, err)
		verification = ToolVerification{Status: "skipped", Message: "tool execution failed"}
	}
	if status == "success" && verification.Status == "failed" {
		status = "failed"
		errorText = verification.Message
		err = newToolError(ToolErrorVerificationFailed, errorText, nil)
		result = toolErrorJSONPayload(name, err)
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
		typed := normalizeToolError(err)
		eventPayload["error"] = errorText
		eventPayload["error_code"] = typed.Code
		eventPayload["error_category"] = typed.Category
		eventPayload["retryable"] = typed.Retryable
		eventPayload["recommended_action"] = typed.RecommendedAction
	}
	event := ToolEvent{ToolCallID: toolCallID, ToolName: name, StatusKind: manifestStatusKind(name), Label: label, Status: status, TargetSummary: targetSummary, OperationHash: operationHash, Path: path, Error: errorText, DurationMS: durationMS}
	if err != nil {
		typed := normalizeToolError(err)
		event.ErrorCode = typed.Code
		event.ErrorCategory = typed.Category
		event.Retryable = typed.Retryable
		event.RecommendedAction = typed.RecommendedAction
	}
	if status == "success" {
		var meta struct {
			Path             string              `json:"path"`
			OldPath          string              `json:"old_path"`
			NewPath          string              `json:"new_path"`
			ChangedPaths     []string            `json:"changed_paths"`
			Moved            []WorkspacePathMove `json:"moved"`
			Operation        string              `json:"operation"`
			Question         string              `json:"question"`
			Options          []string            `json:"options"`
			AllowFreeform    *bool               `json:"allow_freeform"`
			VersionID        string              `json:"version_id"`
			DocumentVersion  string              `json:"document_version"`
			ChangedBlocks    []map[string]any    `json:"changed_blocks"`
			ValidationStatus string              `json:"validation_status"`
			StructureDelta   map[string]any      `json:"structure_delta"`
			Hash             string              `json:"hash"`
			FileHash         string              `json:"file_hash"`
			SkillID          string              `json:"skill_id"`
			Command          string              `json:"command"`
			Cwd              string              `json:"cwd"`
			Stdout           string              `json:"stdout"`
			Stderr           string              `json:"stderr"`
			ExitCode         *int                `json:"exit_code"`
			TimedOut         bool                `json:"timed_out"`
			Provider         string              `json:"provider"`
			URL              string              `json:"url"`
		}
		if err := json.Unmarshal([]byte(result), &meta); err == nil {
			if meta.Path != "" {
				event.Path = meta.Path
			}
			event.OldPath = meta.OldPath
			event.NewPath = meta.NewPath
			event.ChangedPaths = meta.ChangedPaths
			event.Moved = meta.Moved
			event.Operation = meta.Operation
			event.Question = meta.Question
			event.Options = meta.Options
			event.AllowFreeform = meta.AllowFreeform
			event.VersionID = meta.VersionID
			event.DocumentVersion = meta.DocumentVersion
			event.ChangedBlocks = meta.ChangedBlocks
			event.ValidationStatus = meta.ValidationStatus
			event.StructureDelta = meta.StructureDelta
			event.Hash = firstNonEmpty(meta.FileHash, meta.Hash)
			event.SkillID = meta.SkillID
			event.Command = meta.Command
			event.Cwd = meta.Cwd
			event.Stdout = meta.Stdout
			event.Stderr = meta.Stderr
			event.ExitCode = meta.ExitCode
			event.TimedOut = meta.TimedOut
			event.Provider = meta.Provider
			event.URL = meta.URL
		}
	}
	changedPathsJSON := ""
	if len(event.ChangedPaths) > 0 {
		changedRaw, _ := json.Marshal(event.ChangedPaths)
		changedPathsJSON = string(changedRaw)
	}
	if s.calls != nil {
		recorded, recordErr := s.calls.CompleteToolCall(ctx, store.ToolCallInput{
			ID:               toolCallID,
			UserID:           opts.UserID,
			RunID:            opts.RunID,
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
			return result, event, WithDiagnosticID(wrapped, diagID)
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
			mutationID := toolCallID
			if strings.TrimSpace(opts.RunID) != "" {
				mutationID = strings.TrimSpace(opts.RunID) + ":" + toolCallID
			}
			event.Type = "workspace_file_changed"
			event.MutationID = mutationID
			event.Origin = "agent"
			event.Authoritative = true
			if event.VersionID != "" {
				event.HistoryNodesCreated = 1
			}
		}
		emitToolLifecycleEvent(opts, "tool_call_completed", "info", eventPayload)
		if shouldEmitWorkspaceFileChanged(name, event) {
			mutationID := event.MutationID
			emitToolLifecycleEvent(opts, "workspace_file_changed", "info", map[string]any{
				"type":                  "workspace_file_changed",
				"tool_call_id":          toolCallID,
				"tool_name":             name,
				"status_kind":           manifestStatusKind(name),
				"status":                status,
				"path":                  event.Path,
				"old_path":              event.OldPath,
				"new_path":              event.NewPath,
				"changed_paths":         event.ChangedPaths,
				"moved":                 event.Moved,
				"operation":             event.Operation,
				"mutation_id":           mutationID,
				"origin":                "agent",
				"authoritative":         true,
				"hash":                  event.Hash,
				"file_hash":             event.Hash,
				"doc_hash":              event.DocumentVersion,
				"version_id":            event.VersionID,
				"document_version":      event.DocumentVersion,
				"changed_blocks":        event.ChangedBlocks,
				"validation_status":     event.ValidationStatus,
				"structure_delta":       event.StructureDelta,
				"history_nodes_created": event.HistoryNodesCreated,
				"operation_hash":        event.OperationHash,
			})
		}
	} else {
		log.Printf("[ai] tool conversation=%s name=%s status=%s path=%s error=%s", conversationID, name, status, path, errorText)
		logTask(opts, "error", fmt.Sprintf("tool call failed: %s: %s", name, errorText))
		emitToolLifecycleEvent(opts, "tool_call_failed", "error", eventPayload)
	}
	return result, event, nil
}

func toolEventFromRecordedCall(item store.ToolCall) ToolEvent {
	event := ToolEvent{
		ToolCallID:    item.ID,
		ToolName:      item.ToolName,
		StatusKind:    manifestStatusKind(item.ToolName),
		Label:         userFriendlyToolLabel(item.ToolName),
		Status:        item.Status,
		Error:         item.Error,
		OperationHash: item.OperationHash,
		VersionID:     item.VersionID,
		Hash:          item.ContentHash,
		DurationMS:    item.DurationMS,
	}
	_ = json.Unmarshal([]byte(item.ChangedPathsJSON), &event.ChangedPaths)
	var resultMeta struct {
		DocumentVersion string `json:"document_version"`
	}
	if json.Unmarshal([]byte(item.ResultJSON), &resultMeta) == nil {
		event.DocumentVersion = resultMeta.DocumentVersion
	}
	if len(event.ChangedPaths) == 1 {
		event.Path = event.ChangedPaths[0]
	}
	return event
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
	case "edit_workspace_note_blocks":
		return "edit_note_blocks"
	case "create_workspace_note":
		return "create_note"
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
		case event.OldPath != "" && event.NewPath != "" && event.Operation == "copy":
			return "copy"
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
