package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"webide-backend/internal/workspace"
)

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
	if len([]rune(args.Content)) > recommendedWorkspaceChunkChars {
		return "", "", "正在保存文件...", args.Path, fmt.Errorf("invalid tool arguments: content must be at most %d characters; use write_workspace_chunk for longer text", recommendedWorkspaceChunkChars)
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
		return mustJSON(workspaceToolPayload("save_workspace_file", result.Path, payload)), fmt.Sprintf("created file %s", result.Path), "正在保存文件...", result.Path, nil
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
		return mustJSON(workspaceToolPayload("save_workspace_file", result.Path, payload)), fmt.Sprintf("overwrote file %s", result.Path), "正在保存文件...", result.Path, nil
	default:
		return "", "", "正在保存文件...", args.Path, fmt.Errorf("invalid tool arguments: mode must be create or overwrite")
	}
}

func (s *Service) execWriteWorkspaceChunk(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在分块写入文件...", "", err
	}
	var args struct {
		Path       string `json:"path"`
		Content    string `json:"content"`
		Mode       string `json:"mode"`
		ChunkIndex int    `json:"chunk_index"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在分块写入文件...", "", fmt.Errorf("invalid tool arguments")
	}
	if len([]rune(args.Content)) > recommendedWorkspaceChunkChars {
		return "", "", "正在分块写入文件...", args.Path, fmt.Errorf("invalid tool arguments: content must be at most %d characters", recommendedWorkspaceChunkChars)
	}
	if args.ChunkIndex <= 0 {
		return "", "", "正在分块写入文件...", args.Path, fmt.Errorf("invalid tool arguments: chunk_index must be a positive integer")
	}
	mode := strings.ToLower(strings.TrimSpace(args.Mode))
	var result workspaceWriteResult
	switch mode {
	case "create":
		if args.ChunkIndex != 1 {
			return "", "", "正在分块写入文件...", args.Path, fmt.Errorf("invalid tool arguments: create chunk_index must be 1")
		}
		writeResult, err := files.CreateTextFile(ctx, args.Path, args.Content, "ai_chunk_create")
		if err != nil {
			return "", "", "正在分块写入文件...", args.Path, err
		}
		result = workspaceWriteResult{operation: "create", write: writeResult}
	case "overwrite":
		if args.ChunkIndex != 1 {
			return "", "", "正在分块写入文件...", args.Path, fmt.Errorf("invalid tool arguments: overwrite chunk_index must be 1")
		}
		writeResult, err := files.WriteExistingTextFile(ctx, args.Path, args.Content, "ai_chunk_overwrite")
		if err != nil {
			return "", "", "正在分块写入文件...", args.Path, err
		}
		result = workspaceWriteResult{operation: "overwrite", write: writeResult}
	case "append":
		writeResult, err := files.AppendTextFile(ctx, args.Path, args.Content, "ai_chunk_append")
		if err != nil {
			return "", "", "正在分块写入文件...", args.Path, err
		}
		result = workspaceWriteResult{operation: "append", write: writeResult}
	default:
		return "", "", "正在分块写入文件...", args.Path, fmt.Errorf("invalid tool arguments: mode must be create, overwrite, or append")
	}
	payload := map[string]any{
		"operation":     result.operation,
		"path":          result.write.Path,
		"hash":          result.write.Hash,
		"version_id":    result.write.VersionID,
		"size":          result.write.Size,
		"chunk_chars":   len([]rune(args.Content)),
		"chunk_index":   args.ChunkIndex,
		"changed_paths": result.write.ChangedPaths,
		"next_hint":     "Continue with write_workspace_chunk mode=append for the next chunk, or call attempt_completion when the file is complete.",
	}
	return mustJSON(workspaceToolPayload("write_workspace_chunk", result.write.Path, payload)), fmt.Sprintf("%s chunk %d in %s (%d chars)", result.operation, args.ChunkIndex, result.write.Path, len([]rune(args.Content))), "正在分块写入文件...", result.write.Path, nil
}

type workspaceWriteResult struct {
	operation string
	write     workspace.FileWriteResult
}

func (s *Service) execEditWorkspaceText(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在编辑文件...", "", err
	}
	var args struct {
		Path       string `json:"path"`
		OldText    string `json:"old_text"`
		NewText    string `json:"new_text"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在编辑文件...", "", fmt.Errorf("invalid tool arguments")
	}
	result, replacements, err := files.EditTextFile(ctx, args.Path, args.OldText, args.NewText, "ai_edit", args.ReplaceAll)
	if err != nil {
		return "", "", "正在编辑文件...", args.Path, err
	}
	payload := workspaceToolPayload("edit_workspace_text", result.Path, map[string]any{
		"operation":     "edit",
		"path":          result.Path,
		"hash":          result.Hash,
		"version_id":    result.VersionID,
		"size":          result.Size,
		"replacements":  replacements,
		"changed_paths": result.ChangedPaths,
		"next_hint":     "The edit was applied and verified. Continue with another precise edit if needed, otherwise call attempt_completion.",
	})
	return mustJSON(payload), fmt.Sprintf("edited %s (%d replacement(s))", result.Path, replacements), "正在编辑文件...", result.Path, nil
}
