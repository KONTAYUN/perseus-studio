package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"webide-backend/internal/workspace"
)

func (s *Service) execListWorkspaceFiles(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在查看工作区文件...", "", err
	}
	var args struct {
		Path     string `json:"path"`
		MaxDepth int    `json:"max_depth"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在查看工作区文件...", "", fmt.Errorf("invalid tool arguments")
	}
	nodes, limited, err := files.List(args.Path, args.MaxDepth, 200)
	if err != nil {
		return "", "", "正在查看工作区文件...", args.Path, err
	}
	payload := map[string]any{"path": args.Path, "items": nodes, "limited": limited}
	summary := fmt.Sprintf("listed %d workspace items", countTreeNodes(nodes))
	return mustJSON(payload), summary, "正在查看工作区文件...", args.Path, nil
}

func (s *Service) execReadWorkspaceFile(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在读取工作区文件...", "", err
	}
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在读取工作区文件...", "", fmt.Errorf("invalid tool arguments")
	}
	result, err := files.ReadTextFile(args.Path, 20000)
	label := "正在读取工作区文件..."
	if err != nil {
		return "", "", label, args.Path, err
	}
	summary := fmt.Sprintf("read %s (%d chars, truncated=%t)", result.Path, len([]rune(result.Content)), result.Truncated)
	return mustJSON(result), summary, label, result.Path, nil
}

func (s *Service) execCreateWorkspaceFile(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在创建文件...", "", err
	}
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在创建文件...", "", fmt.Errorf("invalid tool arguments")
	}
	result, err := files.CreateTextFile(ctx, args.Path, args.Content, "ai_create")
	if err != nil {
		return "", "", "正在创建文件...", args.Path, err
	}
	summary := fmt.Sprintf("created file %s", result.Path)
	return mustJSON(result), summary, "正在创建文件...", result.Path, nil
}

func (s *Service) execWriteWorkspaceFile(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在写入文件...", "", err
	}
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在写入文件...", "", fmt.Errorf("invalid tool arguments")
	}
	result, err := files.WriteExistingTextFile(ctx, args.Path, args.Content, "ai_write")
	if err != nil {
		return "", "", "正在写入文件...", args.Path, err
	}
	summary := fmt.Sprintf("wrote file %s", result.Path)
	return mustJSON(result), summary, "正在写入文件...", result.Path, nil
}

func (s *Service) execSaveWorkspaceFile(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在保存文件...", "", err
	}
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Mode    string `json:"mode"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在保存文件...", "", fmt.Errorf("invalid tool arguments")
	}
	switch strings.ToLower(strings.TrimSpace(args.Mode)) {
	case "create":
		result, err := files.CreateTextFile(ctx, args.Path, args.Content, "ai_create")
		if err != nil {
			return "", "", "正在保存文件...", args.Path, err
		}
		payload := map[string]any{
			"operation":     "create",
			"path":          result.Path,
			"hash":          result.Hash,
			"version_id":    result.VersionID,
			"size":          result.Size,
			"changed_paths": result.ChangedPaths,
		}
		return mustJSON(payload), fmt.Sprintf("created file %s", result.Path), "正在保存文件...", result.Path, nil
	case "overwrite":
		result, err := files.WriteExistingTextFile(ctx, args.Path, args.Content, "ai_write")
		if err != nil {
			return "", "", "正在保存文件...", args.Path, err
		}
		payload := map[string]any{
			"operation":     "overwrite",
			"path":          result.Path,
			"hash":          result.Hash,
			"version_id":    result.VersionID,
			"size":          result.Size,
			"changed_paths": result.ChangedPaths,
		}
		return mustJSON(payload), fmt.Sprintf("overwrote file %s", result.Path), "正在保存文件...", result.Path, nil
	default:
		return "", "", "正在保存文件...", args.Path, fmt.Errorf("mode must be create or overwrite")
	}
}

func (s *Service) execMkdirWorkspace(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在创建文件夹...", "", err
	}
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在创建文件夹...", "", fmt.Errorf("invalid tool arguments")
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
		return "", "", "正在重命名...", "", fmt.Errorf("invalid tool arguments")
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
		return "", "", "正在移动...", "", fmt.Errorf("invalid tool arguments")
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
		return "", "", "正在管理路径...", "", fmt.Errorf("invalid tool arguments")
	}
	switch strings.ToLower(strings.TrimSpace(args.Action)) {
	case "mkdir":
		result, err := files.MkdirNew(args.Path)
		if err != nil {
			return "", "", "正在管理路径...", args.Path, err
		}
		payload := map[string]any{
			"operation":     "mkdir",
			"path":          result.Path,
			"changed_paths": result.ChangedPaths,
		}
		return mustJSON(payload), fmt.Sprintf("created folder %s", result.Path), "正在管理路径...", result.Path, nil
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
		return mustJSON(payload), fmt.Sprintf("renamed %s to %s", result.OldPath, result.NewPath), "正在管理路径...", result.NewPath, nil
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
		return mustJSON(payload), fmt.Sprintf("moved %s to %s", result.OldPath, result.NewPath), "正在管理路径...", result.NewPath, nil
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
		return mustJSON(payload), fmt.Sprintf("moved %s to trash %s", result.OldPath, result.NewPath), "正在管理路径...", result.NewPath, nil
	default:
		return "", "", "正在管理路径...", args.Path, fmt.Errorf("action must be mkdir, rename, move, or trash")
	}
}

func (s *Service) execOrganizeWorkspaceMatches(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在整理文件...", "", err
	}
	var args struct {
		SourcePath     string `json:"source_path"`
		MatchText      string `json:"match_text"`
		TargetFolder   string `json:"target_folder"`
		IncludeFolders bool   `json:"include_folders"`
		MaxItems       int    `json:"max_items"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在整理文件...", "", fmt.Errorf("invalid tool arguments")
	}
	args.MatchText = strings.TrimSpace(args.MatchText)
	args.TargetFolder = strings.Trim(strings.TrimSpace(strings.ReplaceAll(args.TargetFolder, "\\", "/")), "/")
	if args.MatchText == "" {
		return "", "", "正在整理文件...", "", fmt.Errorf("match_text is required")
	}
	if args.TargetFolder == "" {
		return "", "", "正在整理文件...", "", fmt.Errorf("target_folder is required")
	}
	if args.MaxItems <= 0 {
		args.MaxItems = 200
	}
	if args.MaxItems > 500 {
		args.MaxItems = 500
	}

	if _, _, err := files.List(args.TargetFolder, 1, 1); err != nil {
		if _, mkdirErr := files.MkdirNew(args.TargetFolder); mkdirErr != nil {
			return "", "", "正在整理文件...", args.TargetFolder, fmt.Errorf("create target folder: %w", mkdirErr)
		}
	}

	nodes, limited, err := files.List(args.SourcePath, 20, 2000)
	if err != nil {
		return "", "", "正在整理文件...", args.SourcePath, err
	}
	matches := collectOrganizeMatches(nodes, args.MatchText, args.TargetFolder, args.IncludeFolders, args.MaxItems)
	if len(matches) == 0 {
		return "", "", "正在整理文件...", args.TargetFolder, fmt.Errorf("no workspace items matched %q", args.MatchText)
	}

	type moveResult struct {
		OldPath string `json:"old_path"`
		NewPath string `json:"new_path"`
		Status  string `json:"status"`
		Error   string `json:"error,omitempty"`
	}
	var moved []moveResult
	var changed []string
	for _, match := range matches {
		result, err := files.MovePath(match.Path, args.TargetFolder)
		if err != nil {
			moved = append(moved, moveResult{OldPath: match.Path, Status: "failed", Error: err.Error()})
			continue
		}
		moved = append(moved, moveResult{OldPath: result.OldPath, NewPath: result.NewPath, Status: "success"})
		changed = append(changed, result.OldPath, result.NewPath)
	}
	successCount := 0
	failedCount := 0
	for _, item := range moved {
		if item.Status == "success" {
			successCount++
		} else {
			failedCount++
		}
	}
	if successCount == 0 {
		return "", "", "正在整理文件...", args.TargetFolder, fmt.Errorf("failed to move all matched items")
	}
	payload := map[string]any{
		"path":            args.TargetFolder,
		"target_folder":   args.TargetFolder,
		"match_text":      args.MatchText,
		"source_path":     args.SourcePath,
		"moved":           moved,
		"moved_count":     successCount,
		"failed_count":    failedCount,
		"limited":         limited,
		"changed_paths":   changed,
		"include_folders": args.IncludeFolders,
	}
	summary := fmt.Sprintf("organized %d matched item(s) into %s", successCount, args.TargetFolder)
	if failedCount > 0 {
		summary = fmt.Sprintf("%s; %d item(s) failed", summary, failedCount)
	}
	return mustJSON(payload), summary, "正在整理文件...", args.TargetFolder, nil
}

func collectOrganizeMatches(nodes []workspace.TreeNode, matchText, targetFolder string, includeFolders bool, maxItems int) []workspace.TreeNode {
	var matches []workspace.TreeNode
	needle := strings.ToLower(matchText)
	target := strings.Trim(targetFolder, "/")
	var walk func([]workspace.TreeNode)
	walk = func(items []workspace.TreeNode) {
		for _, item := range items {
			if len(matches) >= maxItems {
				return
			}
			path := strings.Trim(item.Path, "/")
			if workspace.IsTrashPath(path) || workspace.IsLegacyTrashPath(path) {
				continue
			}
			if path == "" || path == target || strings.HasPrefix(path, target+"/") {
				if item.Type == "folder" && path != target && !strings.HasPrefix(path, target+"/") {
					walk(item.Children)
				}
				continue
			}
			if item.Type == "folder" {
				if includeFolders && organizeNodeMatches(item, needle) {
					matches = append(matches, item)
					continue
				}
				walk(item.Children)
				continue
			}
			if organizeNodeMatches(item, needle) {
				matches = append(matches, item)
			}
		}
	}
	walk(nodes)
	return matches
}

func organizeNodeMatches(node workspace.TreeNode, needle string) bool {
	name := strings.ToLower(node.Name)
	path := strings.ToLower(node.Path)
	base := strings.ToLower(filepath.Base(node.Path))
	return strings.Contains(name, needle) || strings.Contains(path, needle) || strings.Contains(base, needle)
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
		return "", "", "正在删除到回收站...", "", fmt.Errorf("invalid tool arguments")
	}
	result, err := files.MoveToTrash(args.Path)
	if err != nil {
		return "", "", "正在删除到回收站...", args.Path, err
	}
	summary := fmt.Sprintf("moved %s to trash %s", result.OldPath, result.NewPath)
	return mustJSON(result), summary, "正在删除到回收站...", result.NewPath, nil
}

func countTreeNodes(nodes []workspace.TreeNode) int {
	count := 0
	for _, node := range nodes {
		count++
		count += countTreeNodes(node.Children)
	}
	return count
}
