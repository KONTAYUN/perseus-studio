package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"webide-backend/internal/idgen"
	"webide-backend/internal/store"
)

func (s *Service) executeAndRecordTool(ctx context.Context, conversationID, messageID, name, argumentsJSON, operationHash string, opts RunOptions, profile providerRuntimeProfile) (string, ToolEvent, error) {
	startedAt := time.Now()
	toolCallID := idgen.NewAt("tc", startedAt)
	logTask(opts, "info", fmt.Sprintf("tool call started: %s", name))
	emitToolLifecycleEvent(opts, "tool_call_started", "info", map[string]any{
		"tool_call_id":      toolCallID,
		"tool_name":         name,
		"status":            "running",
		"operation_hash":    operationHash,
		"arguments_summary": summarizeToolArguments(argumentsJSON),
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
			return "", ToolEvent{ToolCallID: toolCallID, ToolName: name, Label: userFriendlyToolLabel(name), Status: "failed", Error: wrapped.Error(), OperationHash: operationHash}, WithDiagnosticID(wrapped, diagID)
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
		result = mustJSON(map[string]any{
			"error":         errorText,
			"recovery_hint": toolFailureRecoveryHint(name, errorText),
		})
		summary = errorText
		s.recordToolDiagnostic(ctx, opts, conversationID, messageID, name, err)
		verification = ToolVerification{Status: "skipped", Message: "tool execution failed"}
	}
	if status == "success" && verification.Status == "failed" {
		status = "failed"
		errorText = verification.Message
		result = mustJSON(map[string]any{
			"error":         errorText,
			"recovery_hint": "The tool ran but verification failed. Inspect the workspace state before claiming success.",
			"verification":  verification,
		})
		summary = "workspace verification failed: " + verification.Message
		s.recordToolDiagnostic(ctx, opts, conversationID, messageID, name, fmt.Errorf("workspace verification failed: %s", verification.Message))
	}
	if status == "success" {
		label = userFriendlyToolLabel(name)
	}
	eventPayload := map[string]any{
		"tool_call_id":      toolCallID,
		"tool_name":         name,
		"status":            status,
		"label":             label,
		"path":              path,
		"operation_hash":    operationHash,
		"arguments_summary": summarizeToolArguments(argumentsJSON),
		"result_summary":    truncate(summary, 500),
		"duration_ms":       durationMS,
		"verification":      verification,
	}
	if errorText != "" {
		eventPayload["error"] = errorText
		eventPayload["recovery_hint"] = toolFailureRecoveryHint(name, errorText)
	}
	event := ToolEvent{ToolCallID: toolCallID, ToolName: name, Label: label, Status: status, OperationHash: operationHash, Path: path, Error: errorText, DurationMS: durationMS}
	if status == "success" {
		var meta struct {
			Path          string   `json:"path"`
			OldPath       string   `json:"old_path"`
			NewPath       string   `json:"new_path"`
			ChangedPaths  []string `json:"changed_paths"`
			Question      string   `json:"question"`
			Options       []string `json:"options"`
			AllowFreeform *bool    `json:"allow_freeform"`
			VersionID     string   `json:"version_id"`
			Hash          string   `json:"hash"`
			SkillID       string   `json:"skill_id"`
			Command       string   `json:"command"`
			Cwd           string   `json:"cwd"`
			Stdout        string   `json:"stdout"`
			Stderr        string   `json:"stderr"`
			ExitCode      *int     `json:"exit_code"`
			TimedOut      bool     `json:"timed_out"`
			Provider      string   `json:"provider"`
			URL           string   `json:"url"`
		}
		if err := json.Unmarshal([]byte(result), &meta); err == nil {
			if meta.Path != "" {
				event.Path = meta.Path
			}
			event.OldPath = meta.OldPath
			event.NewPath = meta.NewPath
			event.ChangedPaths = meta.ChangedPaths
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
			ResultSummary:    truncate(summary, 500),
			Status:           status,
			Error:            truncate(errorText, 500),
			CompletedAt:      time.Now().UTC(),
			DurationMS:       durationMS,
			ResultJSON:       truncate(result, 20000),
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
		emitToolLifecycleEvent(opts, "tool_call_completed", "info", eventPayload)
		if isWorkspaceMutationTool(name) && (len(event.ChangedPaths) > 0 || event.Path != "" || event.OldPath != "" || event.NewPath != "") {
			emitToolLifecycleEvent(opts, "workspace_file_changed", "info", map[string]any{
				"tool_call_id":   toolCallID,
				"tool_name":      name,
				"status":         status,
				"path":           event.Path,
				"old_path":       event.OldPath,
				"new_path":       event.NewPath,
				"changed_paths":  event.ChangedPaths,
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
