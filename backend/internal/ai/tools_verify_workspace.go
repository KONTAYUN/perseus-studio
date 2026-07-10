package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"webide-backend/internal/workspace"
)

type ToolVerification struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

func (s *Service) verifyToolResult(ctx context.Context, toolName, resultJSON string, opts RunOptions) ToolVerification {
	if !isWorkspaceMutationTool(toolName) && !commandResultHasWorkspaceChanges(toolName, resultJSON) {
		return ToolVerification{Status: "skipped", Message: "not a workspace mutation tool"}
	}
	if strings.TrimSpace(opts.UserID) == "" {
		return ToolVerification{Status: "skipped", Message: "missing user id"}
	}
	files, err := s.factory.Workspace(opts.UserID)
	if err != nil {
		return ToolVerification{Status: "failed", Message: fmt.Sprintf("open workspace: %v", err)}
	}

	var meta struct {
		Path    string `json:"path"`
		OldPath string `json:"old_path"`
		NewPath string `json:"new_path"`
		Hash    string `json:"hash"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &meta); err != nil {
		return ToolVerification{Status: "failed", Message: "tool result was not valid JSON"}
	}

	if isWorkspaceContentMutationTool(toolName) {
		path := firstNonEmpty(meta.Path, meta.NewPath)
		if path == "" {
			return ToolVerification{Status: "failed", Message: "tool result did not include file path"}
		}
		read, err := files.ReadFile(path)
		if err != nil && toolName == "apply_workspace_patch" {
			var patchMeta struct {
				ChangedPaths []string `json:"changed_paths"`
			}
			_ = json.Unmarshal([]byte(resultJSON), &patchMeta)
			for _, candidate := range patchMeta.ChangedPaths {
				if strings.TrimSpace(candidate) == "" || candidate == path {
					continue
				}
				if nextRead, nextErr := files.ReadFile(candidate); nextErr == nil {
					read = nextRead
					err = nil
					path = candidate
					break
				}
			}
		}
		if err != nil {
			if toolName == "apply_workspace_patch" {
				var patchMeta struct {
					ChangedPaths []string `json:"changed_paths"`
				}
				_ = json.Unmarshal([]byte(resultJSON), &patchMeta)
				if len(patchMeta.ChangedPaths) > 0 {
					return ToolVerification{Status: "verified", Message: "patch changed paths recorded; primary path may have been moved to trash", Path: path}
				}
			}
			return ToolVerification{Status: "failed", Message: fmt.Sprintf("file is not readable after write: %v", err), Path: path}
		}
		if meta.Hash != "" && read.Hash != meta.Hash {
			return ToolVerification{Status: "failed", Message: "file hash mismatch after write", Path: path}
		}
		return ToolVerification{Status: "verified", Message: "file exists and is readable", Path: path}
	}

	switch toolName {
	case "manage_workspace_path":
		var data struct {
			Operation string `json:"operation"`
		}
		_ = json.Unmarshal([]byte(resultJSON), &data)
		switch data.Operation {
		case "mkdir":
			return s.verifyManagedMkdir(files, resultJSON)
		case "rename", "move":
			return s.verifyManagedMove(files, resultJSON)
		case "copy":
			return s.verifyManagedCopy(files, resultJSON)
		case "trash":
			return s.verifyManagedTrash(files, resultJSON)
		default:
			return ToolVerification{Status: "failed", Message: "manage result did not include operation"}
		}
	case "organize_workspace_matches":
		var data struct {
			TargetFolder string `json:"target_folder"`
			Moved        []struct {
				OldPath string `json:"old_path"`
				NewPath string `json:"new_path"`
				Status  string `json:"status"`
			} `json:"moved"`
		}
		if err := json.Unmarshal([]byte(resultJSON), &data); err != nil {
			return ToolVerification{Status: "failed", Message: "organize result was not valid JSON"}
		}
		if data.TargetFolder == "" {
			return ToolVerification{Status: "failed", Message: "organize result did not include target folder"}
		}
		if _, _, err := files.List(data.TargetFolder, 1, 1); err != nil {
			return ToolVerification{Status: "failed", Message: fmt.Sprintf("target folder is not listable after organize: %v", err), Path: data.TargetFolder}
		}
		verified := 0
		for _, item := range data.Moved {
			if item.Status != "success" {
				continue
			}
			if item.OldPath == "" || item.NewPath == "" {
				return ToolVerification{Status: "failed", Message: "successful move missing old_path or new_path"}
			}
			if existsWorkspacePath(files, item.OldPath) {
				return ToolVerification{Status: "failed", Message: "old path still exists after organize", Path: item.OldPath}
			}
			if !existsWorkspacePath(files, item.NewPath) {
				return ToolVerification{Status: "failed", Message: "new path does not exist after organize", Path: item.NewPath}
			}
			verified++
		}
		if verified == 0 {
			return ToolVerification{Status: "failed", Message: "no successful organize moves were verified", Path: data.TargetFolder}
		}
		return ToolVerification{Status: "verified", Message: fmt.Sprintf("verified %d organized item(s)", verified), Path: data.TargetFolder}
	case "run_workspace_command":
		var data struct {
			ChangedPaths []string `json:"changed_paths"`
		}
		if err := json.Unmarshal([]byte(resultJSON), &data); err != nil {
			return ToolVerification{Status: "failed", Message: "command result was not valid JSON"}
		}
		if len(data.ChangedPaths) == 0 {
			return ToolVerification{Status: "skipped", Message: "command did not change workspace files"}
		}
		return ToolVerification{Status: "verified", Message: fmt.Sprintf("command changed %d workspace path(s)", len(data.ChangedPaths)), Path: data.ChangedPaths[0]}
	default:
		return ToolVerification{Status: "skipped", Message: "no verifier for tool"}
	}
}

func isWorkspaceContentMutationTool(toolName string) bool {
	entry, ok := toolManifestEntry(toolName)
	if !ok || !entry.Mutation {
		return false
	}
	return entry.StatusKind == "workspace_edit"
}

func commandResultHasWorkspaceChanges(toolName, resultJSON string) bool {
	if toolName != "run_workspace_command" {
		return false
	}
	var data struct {
		ChangedPaths []string `json:"changed_paths"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &data); err != nil {
		return false
	}
	return len(data.ChangedPaths) > 0
}

func (s *Service) verifyManagedMkdir(files *workspace.Service, resultJSON string) ToolVerification {
	var meta struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &meta); err != nil || meta.Path == "" {
		return ToolVerification{Status: "failed", Message: "mkdir result did not include folder path"}
	}
	if _, _, err := files.List(meta.Path, 1, 1); err != nil {
		return ToolVerification{Status: "failed", Message: fmt.Sprintf("folder is not listable after mkdir: %v", err), Path: meta.Path}
	}
	return ToolVerification{Status: "verified", Message: "folder exists and is listable", Path: meta.Path}
}

func (s *Service) verifyManagedMove(files *workspace.Service, resultJSON string) ToolVerification {
	var meta struct {
		OldPath string `json:"old_path"`
		NewPath string `json:"new_path"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &meta); err != nil || meta.OldPath == "" || meta.NewPath == "" {
		return ToolVerification{Status: "failed", Message: "move/rename result did not include old_path and new_path"}
	}
	if existsWorkspacePath(files, meta.OldPath) {
		return ToolVerification{Status: "failed", Message: "old path still exists after move/rename", Path: meta.OldPath}
	}
	if !existsWorkspacePath(files, meta.NewPath) {
		return ToolVerification{Status: "failed", Message: "new path does not exist after move/rename", Path: meta.NewPath}
	}
	return ToolVerification{Status: "verified", Message: "old path removed and new path exists", Path: meta.NewPath}
}

func (s *Service) verifyManagedCopy(files *workspace.Service, resultJSON string) ToolVerification {
	var meta struct {
		OldPath string `json:"old_path"`
		NewPath string `json:"new_path"`
		Hash    string `json:"hash"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &meta); err != nil || meta.OldPath == "" || meta.NewPath == "" {
		return ToolVerification{Status: "failed", Message: "copy result did not include old_path and new_path"}
	}
	if !existsWorkspacePath(files, meta.OldPath) {
		return ToolVerification{Status: "failed", Message: "source path does not exist after copy", Path: meta.OldPath}
	}
	read, err := files.ReadFile(meta.NewPath)
	if err != nil {
		return ToolVerification{Status: "failed", Message: fmt.Sprintf("copied file is not readable: %v", err), Path: meta.NewPath}
	}
	if meta.Hash != "" && read.Hash != meta.Hash {
		return ToolVerification{Status: "failed", Message: "copied file hash mismatch", Path: meta.NewPath}
	}
	return ToolVerification{Status: "verified", Message: "source and copied file exist", Path: meta.NewPath}
}

func (s *Service) verifyManagedTrash(files *workspace.Service, resultJSON string) ToolVerification {
	var meta struct {
		OldPath string `json:"old_path"`
		NewPath string `json:"new_path"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &meta); err != nil || meta.OldPath == "" || meta.NewPath == "" {
		return ToolVerification{Status: "failed", Message: "trash result did not include old_path and trash path"}
	}
	if existsWorkspacePath(files, meta.OldPath) {
		return ToolVerification{Status: "failed", Message: "old path still exists after trash", Path: meta.OldPath}
	}
	if !existsTrashItem(files, meta.NewPath) {
		return ToolVerification{Status: "failed", Message: "trash path does not exist after trash", Path: meta.NewPath}
	}
	return ToolVerification{Status: "verified", Message: "old path removed and trash path exists", Path: meta.NewPath}
}

func existsTrashItem(files *workspace.Service, path string) bool {
	items, err := files.TrashItems()
	if err != nil {
		return false
	}
	for _, item := range items {
		if item.Path == path {
			return true
		}
	}
	return false
}

func existsWorkspacePath(files *workspace.Service, path string) bool {
	if _, err := files.ReadFile(path); err == nil {
		return true
	}
	if _, _, err := files.List(path, 1, 1); err == nil {
		return true
	}
	return false
}
