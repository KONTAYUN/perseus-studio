package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"webide-backend/internal/failure"
)

func (s *Service) execMkdirWorkspace(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在创建文件夹...", "", err
	}
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在创建文件夹...", "", failure.New("tool_invalid_arguments", "invalid tool arguments", nil)
	}
	result, err := files.MkdirNew(args.Path)
	if err != nil {
		return "", "", "正在创建文件夹...", args.Path, err
	}
	summary := fmt.Sprintf("created folder %s", result.Path)
	return mustJSON(result), summary, "正在创建文件夹...", result.Path, nil
}

func (s *Service) execRenameWorkspacePath(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在重命名...", "", err
	}
	var args struct {
		OldPath string `json:"old_path"`
		NewName string `json:"new_name"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在重命名...", "", failure.New("tool_invalid_arguments", "invalid tool arguments", nil)
	}
	result, err := files.RenameWithNewName(args.OldPath, args.NewName)
	if err != nil {
		return "", "", "正在重命名...", args.OldPath, err
	}
	summary := fmt.Sprintf("renamed %s to %s", result.OldPath, result.NewPath)
	return mustJSON(result), summary, "正在重命名...", result.NewPath, nil
}

func (s *Service) execMoveWorkspacePath(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在移动...", "", err
	}
	var args struct {
		OldPath       string `json:"old_path"`
		NewParentPath string `json:"new_parent_path"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在移动...", "", failure.New("tool_invalid_arguments", "invalid tool arguments", nil)
	}
	result, err := files.MovePath(args.OldPath, args.NewParentPath)
	if err != nil {
		return "", "", "正在移动...", args.OldPath, err
	}
	summary := fmt.Sprintf("moved %s to %s", result.OldPath, result.NewPath)
	return mustJSON(result), summary, "正在移动...", result.NewPath, nil
}

func (s *Service) execManageWorkspacePath(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在管理路径...", "", err
	}
	var args struct {
		Action        string `json:"action"`
		Path          string `json:"path"`
		NewName       string `json:"new_name"`
		NewParentPath string `json:"new_parent_path"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在管理路径...", "", failure.New("tool_invalid_arguments", "invalid tool arguments", nil)
	}
	switch strings.ToLower(strings.TrimSpace(args.Action)) {
	case "mkdir":
		result, err := files.MkdirNew(args.Path)
		if err != nil {
			if payload, path, ok := s.workspacePathExistsObservation(userID, "manage_workspace_path", "mkdir", args.Path); ok {
				return payload, "path already exists", "正在管理路径...", path, fmt.Errorf("path already exists")
			}
			return "", "", "正在管理路径...", args.Path, err
		}
		payload := map[string]any{
			"operation":     "mkdir",
			"path":          result.Path,
			"changed_paths": result.ChangedPaths,
		}
		return mustJSON(workspaceToolPayload("manage_workspace_path", result.Path, payload)), fmt.Sprintf("created folder %s", result.Path), "正在管理路径...", result.Path, nil
	case "rename":
		result, err := files.RenameWithNewName(args.Path, args.NewName)
		if err != nil {
			return "", "", "正在管理路径...", args.Path, err
		}
		payload := map[string]any{
			"operation":     "rename",
			"path":          result.Path,
			"old_path":      result.OldPath,
			"new_path":      result.NewPath,
			"changed_paths": result.ChangedPaths,
		}
		return mustJSON(workspaceToolPayload("manage_workspace_path", result.NewPath, payload)), fmt.Sprintf("renamed %s to %s", result.OldPath, result.NewPath), "正在管理路径...", result.NewPath, nil
	case "move":
		result, err := files.MovePath(args.Path, args.NewParentPath)
		if err != nil {
			return "", "", "正在管理路径...", args.Path, err
		}
		payload := map[string]any{
			"operation":     "move",
			"path":          result.Path,
			"old_path":      result.OldPath,
			"new_path":      result.NewPath,
			"changed_paths": result.ChangedPaths,
		}
		return mustJSON(workspaceToolPayload("manage_workspace_path", result.NewPath, payload)), fmt.Sprintf("moved %s to %s", result.OldPath, result.NewPath), "正在管理路径...", result.NewPath, nil
	case "trash":
		result, err := files.MoveToTrash(args.Path)
		if err != nil {
			return "", "", "正在管理路径...", args.Path, err
		}
		payload := map[string]any{
			"operation":     "trash",
			"path":          result.Path,
			"old_path":      result.OldPath,
			"new_path":      result.NewPath,
			"changed_paths": result.ChangedPaths,
		}
		return mustJSON(workspaceToolPayload("manage_workspace_path", result.NewPath, payload)), fmt.Sprintf("moved %s to trash %s", result.OldPath, result.NewPath), "正在管理路径...", result.NewPath, nil
	default:
		return "", "", "正在管理路径...", args.Path, fmt.Errorf("action must be mkdir, rename, move, or trash")
	}
}

func (s *Service) workspacePathExistsObservation(userID, toolName, operation, path string) (string, string, bool) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", false
	}
	info, err := files.StatPath(path)
	if err != nil {
		return "", "", false
	}
	pathType := "file"
	if info.Type == "folder" {
		pathType = "folder"
	}
	payload := workspaceToolErrorPayload(toolName, info.Path, map[string]any{
		"operation":     operation,
		"path":          info.Path,
		"error":         "path already exists",
		"reason":        "already_exists",
		"path_type":     pathType,
		"recoverable":   true,
		"recovery_hint": alreadyExistsRecoveryHint(pathType),
	})
	return mustJSON(payload), info.Path, true
}

func alreadyExistsRecoveryHint(pathType string) string {
	if pathType == "folder" {
		return "The folder already exists. Continue using this directory or inspect it before writing; do not repeat mkdir."
	}
	return "A file already exists at this path. Choose a different folder path or ask before replacing it."
}

func (s *Service) execDeleteWorkspacePath(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在删除到回收站...", "", err
	}
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在删除到回收站...", "", failure.New("tool_invalid_arguments", "invalid tool arguments", nil)
	}
	result, err := files.MoveToTrash(args.Path)
	if err != nil {
		return "", "", "正在删除到回收站...", args.Path, err
	}
	summary := fmt.Sprintf("moved %s to trash %s", result.OldPath, result.NewPath)
	return mustJSON(workspaceToolPayload("manage_workspace_path", result.NewPath, map[string]any{
		"operation":     "trash",
		"path":          result.Path,
		"old_path":      result.OldPath,
		"new_path":      result.NewPath,
		"changed_paths": result.ChangedPaths,
	})), summary, "正在删除到回收站...", result.NewPath, nil
}
