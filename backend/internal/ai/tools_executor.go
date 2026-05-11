package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"webide-backend/internal/idgen"
	"webide-backend/internal/store"
)

func (s *Service) executeAndRecordTool(ctx context.Context, conversationID, messageID, name, argumentsJSON string, opts RunOptions, profile providerRuntimeProfile) (string, ToolEvent) {
	startedAt := time.Now()
	toolCallID := idgen.NewAt("tc", startedAt)
	logTask(opts, "info", fmt.Sprintf("tool call started: %s", name))
	emitToolLifecycleEvent(opts, "tool_call_started", "info", map[string]any{
		"tool_call_id":      toolCallID,
		"tool_name":         name,
		"status":            "running",
		"arguments_summary": summarizeToolArguments(argumentsJSON),
	})
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
	if s.calls != nil {
		recorded, recordErr := s.calls.RecordToolCall(ctx, store.ToolCallInput{
			ID:             toolCallID,
			UserID:         opts.UserID,
			ConversationID: conversationID,
			MessageID:      messageID,
			TaskID:         opts.TaskID,
			ToolName:       name,
			ArgumentsJSON:  truncate(argumentsJSON, 2000),
			ResultSummary:  truncate(summary, 500),
			Status:         status,
			Error:          truncate(errorText, 500),
		})
		if recordErr == nil && recorded.ID != "" {
			toolCallID = recorded.ID
		}
	}
	eventPayload := map[string]any{
		"tool_call_id":      toolCallID,
		"tool_name":         name,
		"status":            status,
		"label":             label,
		"path":              path,
		"arguments_summary": summarizeToolArguments(argumentsJSON),
		"result_summary":    truncate(summary, 500),
		"duration_ms":       durationMS,
		"verification":      verification,
	}
	if errorText != "" {
		eventPayload["error"] = errorText
		eventPayload["recovery_hint"] = toolFailureRecoveryHint(name, errorText)
	}
	event := ToolEvent{ToolCallID: toolCallID, ToolName: name, Label: label, Status: status, Path: path, Error: errorText, DurationMS: durationMS}
	if status == "success" {
		var meta struct {
			Path         string   `json:"path"`
			OldPath      string   `json:"old_path"`
			NewPath      string   `json:"new_path"`
			ChangedPaths []string `json:"changed_paths"`
			VersionID    string   `json:"version_id"`
			Hash         string   `json:"hash"`
			SkillID      string   `json:"skill_id"`
			Command      string   `json:"command"`
			Cwd          string   `json:"cwd"`
			Stdout       string   `json:"stdout"`
			Stderr       string   `json:"stderr"`
			ExitCode     *int     `json:"exit_code"`
			TimedOut     bool     `json:"timed_out"`
			Provider     string   `json:"provider"`
			URL          string   `json:"url"`
		}
		if err := json.Unmarshal([]byte(result), &meta); err == nil {
			if meta.Path != "" {
				event.Path = meta.Path
			}
			event.OldPath = meta.OldPath
			event.NewPath = meta.NewPath
			event.ChangedPaths = meta.ChangedPaths
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
		if len(event.ChangedPaths) > 0 || event.Path != "" || event.OldPath != "" || event.NewPath != "" {
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
	return budgetToolResultForModel(name, result, profile), event
}

func emitToolLifecycleEvent(opts RunOptions, eventType, level string, payload map[string]any) {
	if opts.Event != nil {
		opts.Event(eventType, level, payload)
	}
}

func summarizeToolArguments(argumentsJSON string) string {
	var decoded any
	if err := json.Unmarshal([]byte(argumentsJSON), &decoded); err != nil {
		return truncate(argumentsJSON, 500)
	}
	return summarizeToolArgumentValue(decoded)
}

func summarizeToolArgumentValue(value any) string {
	switch item := value.(type) {
	case map[string]any:
		summary := make(map[string]any, len(item))
		for key, raw := range item {
			if key == "content" {
				if text, ok := raw.(string); ok {
					summary[key] = fmt.Sprintf("[content %d chars]", len([]rune(text)))
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

func toolFailureRecoveryHint(toolName, errorText string) string {
	lowerError := strings.ToLower(errorText)
	switch toolName {
	case "read_workspace_text":
		if strings.Contains(lowerError, "not exist") || strings.Contains(lowerError, "no such") || strings.Contains(lowerError, "不存在") {
			return "The file path may be wrong or stale. Inspect the workspace, locate the correct path, then retry reading. Do not claim you read the file until read_workspace_text succeeds."
		}
		return "Check the path and retry only after locating the file inside the current user's workspace."
	case "save_workspace_file":
		if strings.Contains(lowerError, "not exist") || strings.Contains(lowerError, "no such") || strings.Contains(lowerError, "不存在") {
			return "The target file does not exist. If the user wanted a new file, retry save_workspace_file with mode=create; otherwise inspect/read the correct existing file before overwriting."
		}
		return "Read the target file again if needed, adjust the save arguments, then retry. Do not report success until save_workspace_file succeeds."
	case "manage_workspace_path":
		return "Verify the target path with inspect_workspace before retrying. If the request is ambiguous or destructive, ask the user to confirm the exact target."
	case "organize_workspace_matches":
		return "Narrow the source_path or match_text and retry only if the user's intent is clear. Do not claim files were organized until verification succeeds."
	case "run_workspace_command":
		if strings.Contains(lowerError, "safety policy") || strings.Contains(lowerError, "rejected") {
			return "The command was blocked by the workspace safety policy. Do not try to bypass it; explain the limitation or choose a safer workspace-scoped command."
		}
		return "Use the error output to adjust the command, keeping cwd inside the workspace. Do not report command success until run_workspace_command succeeds."
	case "perseus_search_web", "web_search":
		return "Search failed. Try a shorter or more neutral query once, or continue without web results and clearly say that live search failed."
	case "web_extract":
		return "Extraction failed. Verify the URL from search results, try another result URL, or continue with the available search summary."
	case "read_skill":
		return "Verify the skill id with list_skills, then retry only for selected or relevant skills."
	case "load_perseus_toolset":
		return "Load only a valid toolset name from the available toolset list."
	default:
		return "Inspect the error, correct the tool arguments, and retry only if the next action is safe and clearly requested."
	}
}

func logTask(opts RunOptions, level, message string) {
	if opts.Log != nil {
		opts.Log(level, message)
	}
}

func parseBackgroundTaskRequest(argumentsJSON, fallbackGoal string) (BackgroundTaskRequest, error) {
	var request BackgroundTaskRequest
	if err := json.Unmarshal([]byte(argumentsJSON), &request); err != nil {
		return BackgroundTaskRequest{}, fmt.Errorf("invalid start_background_task arguments: %w", err)
	}
	request.Title = strings.TrimSpace(request.Title)
	request.Goal = strings.TrimSpace(request.Goal)
	if request.Goal == "" {
		request.Goal = strings.TrimSpace(fallbackGoal)
	}
	if request.Title == "" {
		request.Title = request.Goal
	}
	request.Title = truncatePlainRunes(request.Title, 60)
	for index := range request.ExpectedOutputs {
		request.ExpectedOutputs[index].Type = strings.TrimSpace(request.ExpectedOutputs[index].Type)
		request.ExpectedOutputs[index].Path = strings.Trim(strings.ReplaceAll(request.ExpectedOutputs[index].Path, "\\", "/"), "/")
		request.ExpectedOutputs[index].Description = strings.TrimSpace(request.ExpectedOutputs[index].Description)
	}
	if len(request.ExpectedOutputs) == 0 {
		request.ExpectedOutputs = []ExpectedOutput{{Type: "message", Description: "任务完成说明"}}
	}
	return request, nil
}

func truncatePlainRunes(value string, maxLen int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > maxLen {
		return string(runes[:maxLen])
	}
	return string(runes)
}

func (s *Service) executeTool(ctx context.Context, name, argumentsJSON string, opts RunOptions) (string, string, string, string, error) {
	switch name {
	case "attempt_completion":
		return execAttemptCompletion(argumentsJSON)
	case "load_perseus_toolset":
		return execLoadPerseusToolset(argumentsJSON)
	case "inspect_workspace":
		return s.execListWorkspaceFiles(ctx, opts.UserID, argumentsJSON)
	case "read_workspace_text":
		return s.execReadWorkspaceFile(ctx, opts.UserID, argumentsJSON)
	case "save_workspace_file":
		return s.execSaveWorkspaceFile(ctx, opts.UserID, argumentsJSON)
	case "manage_workspace_path":
		return s.execManageWorkspacePath(ctx, opts.UserID, argumentsJSON)
	case "organize_workspace_matches":
		return s.execOrganizeWorkspaceMatches(ctx, opts.UserID, argumentsJSON)
	case "list_workspace_files", "read_workspace_file", "create_workspace_file", "write_workspace_file", "mkdir_workspace", "rename_workspace_path", "move_workspace_path", "delete_workspace_path":
		return "", "", "正在使用工作区工具...", "", fmt.Errorf("legacy workspace tool %s has been replaced; use inspect_workspace, read_workspace_text, save_workspace_file, manage_workspace_path, or organize_workspace_matches", name)
	case "run_workspace_command":
		return s.execRunWorkspaceCommand(ctx, opts.UserID, argumentsJSON)
	case "perseus_search_web", "web_search":
		return s.execWebSearch(ctx, opts.UserID, argumentsJSON)
	case "web_extract":
		return s.execWebExtract(ctx, opts.UserID, argumentsJSON)
	case "list_skills":
		return s.execListSkills(ctx, opts.UserID, argumentsJSON)
	case "read_skill":
		return s.execReadSkill(ctx, opts.UserID, argumentsJSON)
	case "install_skill_from_attachment":
		return s.execInstallSkillFromAttachment(ctx, opts.UserID, argumentsJSON, opts)
	case "delete_skill":
		return s.execDeleteSkill(ctx, opts.UserID, argumentsJSON)
	default:
		return "", "", "正在使用工作区工具...", "", fmt.Errorf("unknown tool: %s", name)
	}
}

func execAttemptCompletion(argumentsJSON string) (string, string, string, string, error) {
	var args struct {
		Summary      string   `json:"summary"`
		ChangedPaths []string `json:"changed_paths"`
		FailedItems  []string `json:"failed_items"`
		Notes        string   `json:"notes"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在完成任务...", "", fmt.Errorf("invalid attempt_completion arguments")
	}
	args.Summary = strings.TrimSpace(args.Summary)
	args.Notes = strings.TrimSpace(args.Notes)
	if args.Summary == "" {
		return "", "", "正在完成任务...", "", fmt.Errorf("summary is required")
	}
	payload := map[string]any{
		"summary":       args.Summary,
		"changed_paths": args.ChangedPaths,
		"failed_items":  args.FailedItems,
		"notes":         args.Notes,
	}
	return mustJSON(payload), "attempted completion", "正在完成任务...", "", nil
}

func execLoadPerseusToolset(argumentsJSON string) (string, string, string, string, error) {
	var args struct {
		Toolset string `json:"toolset"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在加载工具...", "", fmt.Errorf("invalid load_perseus_toolset arguments: %w", err)
	}
	args.Toolset = strings.TrimSpace(args.Toolset)
	availableTools, ok := toolsForToolset(args.Toolset)
	if !ok {
		return "", "", "正在加载工具...", "", fmt.Errorf("unknown toolset: %s", args.Toolset)
	}
	result := mustJSON(map[string]any{
		"loaded_toolset":  args.Toolset,
		"available_tools": availableTools,
		"instruction":     "Continue with the newly loaded tools when they are needed for the user's request.",
	})
	return result, fmt.Sprintf("loaded toolset: %s", args.Toolset), "正在加载工具...", "", nil
}
