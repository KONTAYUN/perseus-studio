package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"webide-backend/internal/workspace"
)

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

	var moved []WorkspacePathMove
	var changed []string
	for _, match := range matches {
		result, err := files.MovePath(match.Path, args.TargetFolder)
		if err != nil {
			moved = append(moved, WorkspacePathMove{OldPath: match.Path, Status: "failed", Error: err.Error()})
			continue
		}
		moved = append(moved, WorkspacePathMove{OldPath: result.OldPath, NewPath: result.NewPath, Status: "success"})
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
		"operation":       "organize",
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
	return mustJSON(workspaceToolPayload("organize_workspace_matches", args.TargetFolder, payload)), summary, "正在整理文件...", args.TargetFolder, nil
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
