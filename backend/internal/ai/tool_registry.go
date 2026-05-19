package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

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

func (s *Service) executeTool(ctx context.Context, name, argumentsJSON string, opts RunOptions) (string, string, string, string, error) {
	switch name {
	case "attempt_completion":
		return execAttemptCompletion(argumentsJSON)
	case "load_perseus_toolset":
		return execLoadPerseusToolset(argumentsJSON)
	case "ask_followup_question":
		return execAskFollowupQuestion(argumentsJSON)
	case "inspect_workspace":
		return s.execListWorkspaceFiles(ctx, opts.UserID, argumentsJSON)
	case "find_workspace_files":
		return s.execFindWorkspaceFiles(ctx, opts.UserID, argumentsJSON)
	case "search_workspace_text":
		return s.execSearchWorkspaceText(ctx, opts.UserID, argumentsJSON)
	case "read_workspace_text":
		return s.execReadWorkspaceFile(ctx, opts.UserID, argumentsJSON)
	case "read_workspace_files":
		return s.execReadWorkspaceFiles(ctx, opts.UserID, argumentsJSON)
	case "edit_workspace_text":
		return s.execEditWorkspaceText(ctx, opts.UserID, argumentsJSON)
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

func execAskFollowupQuestion(argumentsJSON string) (string, string, string, string, error) {
	var args followupQuestionAttempt
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在询问用户...", "", fmt.Errorf("invalid ask_followup_question arguments")
	}
	args.Question = strings.TrimSpace(args.Question)
	if args.Question == "" {
		return "", "", "正在询问用户...", "", fmt.Errorf("question is required")
	}
	args.Options = normalizeFollowupOptions(args.Options)
	allowFreeform := true
	if args.AllowFreeform != nil {
		allowFreeform = *args.AllowFreeform
	}
	payload := map[string]any{
		"status":         "waiting_for_user_answer",
		"tool_name":      "ask_followup_question",
		"question":       args.Question,
		"options":        args.Options,
		"allow_freeform": allowFreeform,
	}
	return mustJSON(payload), "asked user a follow-up question", "已询问用户", "", nil
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
