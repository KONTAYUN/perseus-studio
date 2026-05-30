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
	if !isWorkspaceMutationTool(toolName) {
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

	switch toolName {
	case "edit_workspace_lines", "write_workspace_text", "edit_workspace_text", "replace_workspace_lines", "save_workspace_file", "write_workspace_chunk":
		path := firstNonEmpty(meta.Path, meta.NewPath)
		if path == "" {
			return ToolVerification{Status: "failed", Message: "tool result did not include file path"}
		}
		read, err := files.ReadFile(path)
		if err != nil {
			return ToolVerification{Status: "failed", Message: fmt.Sprintf("file is not readable after write: %v", err), Path: path}
		}
		if meta.Hash != "" && read.Hash != meta.Hash {
			return ToolVerification{Status: "failed", Message: "file hash mismatch after write", Path: path}
		}
		return ToolVerification{Status: "verified", Message: "file exists and is readable", Path: path}
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
	default:
		return ToolVerification{Status: "skipped", Message: "no verifier for tool"}
	}
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
