package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"webide-backend/internal/failure"

	"webide-backend/internal/workspace"
)

func (s *Service) execSaveWorkspaceFile(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在编辑文件...", "", err
	}
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Mode    string `json:"mode"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在编辑文件...", "", failure.New("tool_invalid_arguments", "invalid tool arguments", nil)
	}
	if len([]rune(args.Content)) > recommendedWorkspaceChunkChars {
		return "", "", "正在编辑文件...", args.Path, fmt.Errorf("invalid tool arguments: compatibility save content exceeds %d characters; submit the file change as a *** Begin Patch text patch", recommendedWorkspaceChunkChars)
	}
	switch strings.ToLower(strings.TrimSpace(args.Mode)) {
	case "create":
		result, err := files.CreateTextFile(ctx, args.Path, args.Content, "ai_create")
		if err != nil {
			return "", "", "正在编辑文件...", args.Path, err
		}
		payload := map[string]any{
			"operation":     "create",
			"path":          result.Path,
			"hash":          result.Hash,
			"version_id":    result.VersionID,
			"size":          result.Size,
			"changed_paths": result.ChangedPaths,
		}
		return mustJSON(workspaceToolPayload("save_workspace_file", result.Path, payload)), fmt.Sprintf("created file %s", result.Path), "正在编辑文件...", result.Path, nil
	case "overwrite":
		result, err := files.WriteExistingTextFile(ctx, args.Path, args.Content, "ai_write")
		if err != nil {
			return "", "", "正在编辑文件...", args.Path, err
		}
		payload := map[string]any{
			"operation":     "overwrite",
			"path":          result.Path,
			"hash":          result.Hash,
			"version_id":    result.VersionID,
			"size":          result.Size,
			"changed_paths": result.ChangedPaths,
		}
		return mustJSON(workspaceToolPayload("save_workspace_file", result.Path, payload)), fmt.Sprintf("overwrote file %s", result.Path), "正在编辑文件...", result.Path, nil
	default:
		return "", "", "正在编辑文件...", args.Path, fmt.Errorf("invalid tool arguments: mode must be create or overwrite")
	}
}

func (s *Service) execWriteWorkspaceChunk(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在编辑文件...", "", err
	}
	var args struct {
		Path       string `json:"path"`
		Content    string `json:"content"`
		Mode       string `json:"mode"`
		ChunkIndex int    `json:"chunk_index"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在编辑文件...", "", failure.New("tool_invalid_arguments", "invalid tool arguments", nil)
	}
	if len([]rune(args.Content)) > recommendedWorkspaceChunkChars {
		return "", "", "正在编辑文件...", args.Path, fmt.Errorf("invalid tool arguments: compatibility write content exceeds %d characters", recommendedWorkspaceChunkChars)
	}
	if args.ChunkIndex <= 0 {
		return "", "", "正在编辑文件...", args.Path, fmt.Errorf("invalid tool arguments: chunk_index must be a positive integer")
	}
	mode := strings.ToLower(strings.TrimSpace(args.Mode))
	var result workspaceWriteResult
	switch mode {
	case "create":
		if args.ChunkIndex != 1 {
			return "", "", "正在编辑文件...", args.Path, fmt.Errorf("invalid tool arguments: create chunk_index must be 1")
		}
		writeResult, err := files.CreateTextFile(ctx, args.Path, args.Content, "ai_chunk_create")
		if err != nil {
			return "", "", "正在编辑文件...", args.Path, err
		}
		result = workspaceWriteResult{operation: "create", write: writeResult}
	case "overwrite":
		if args.ChunkIndex != 1 {
			return "", "", "正在编辑文件...", args.Path, fmt.Errorf("invalid tool arguments: overwrite chunk_index must be 1")
		}
		writeResult, err := files.WriteExistingTextFile(ctx, args.Path, args.Content, "ai_chunk_overwrite")
		if err != nil {
			return "", "", "正在编辑文件...", args.Path, err
		}
		result = workspaceWriteResult{operation: "overwrite", write: writeResult}
	case "append":
		writeResult, err := files.AppendTextFile(ctx, args.Path, args.Content, "ai_chunk_append")
		if err != nil {
			return "", "", "正在编辑文件...", args.Path, err
		}
		result = workspaceWriteResult{operation: "append", write: writeResult}
	default:
		return "", "", "正在编辑文件...", args.Path, fmt.Errorf("invalid tool arguments: mode must be create, overwrite, or append")
	}
	payload := map[string]any{
		"operation":      result.operation,
		"path":           result.write.Path,
		"hash":           result.write.Hash,
		"version_id":     result.write.VersionID,
		"size":           result.write.Size,
		"chunk_chars":    len([]rune(args.Content)),
		"chunk_index":    args.ChunkIndex,
		"needs_finalize": true,
		"changed_paths":  result.write.ChangedPaths,
		"next_hint":      "The compatibility write was applied. Continue only if this existing write sequence requires more content.",
	}
	return mustJSON(workspaceToolPayload("write_workspace_chunk", result.write.Path, payload)), fmt.Sprintf("%s compatibility write segment %d in %s (%d chars)", result.operation, args.ChunkIndex, result.write.Path, len([]rune(args.Content))), "正在编辑文件...", result.write.Path, nil
}

func (s *Service) execFinalizeWorkspaceWrite(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在校验文件...", "", err
	}
	var args struct {
		Path           string `json:"path"`
		ExpectedChunks int    `json:"expected_chunks"`
		ExpectedLines  int    `json:"expected_lines"`
		ExpectedChars  int    `json:"expected_chars"`
		FinalHash      string `json:"final_hash"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在校验文件...", "", failure.New("tool_invalid_arguments", "invalid tool arguments", nil)
	}
	if args.ExpectedChunks <= 0 {
		return "", "", "正在校验文件...", args.Path, fmt.Errorf("invalid tool arguments: expected_chunks must be a positive integer")
	}
	if args.ExpectedLines <= 0 {
		return "", "", "正在校验文件...", args.Path, fmt.Errorf("invalid tool arguments: expected_lines must be a positive integer")
	}
	if args.ExpectedChars < 0 {
		return "", "", "正在校验文件...", args.Path, fmt.Errorf("invalid tool arguments: expected_chars must be a non-negative integer")
	}
	read, err := files.ReadFile(args.Path)
	if err != nil {
		return "", "", "正在校验文件...", args.Path, err
	}
	actualChars := len([]rune(read.Content))
	actualLines := countWorkspaceTextLines(read.Content)
	if actualLines != args.ExpectedLines {
		return finalizeWorkspaceWriteMismatch("expected_lines", read.Path, map[string]any{
			"actual_lines":   actualLines,
			"expected_lines": args.ExpectedLines,
		})
	}
	if actualChars != args.ExpectedChars {
		return finalizeWorkspaceWriteMismatch("expected_chars", read.Path, map[string]any{
			"actual_chars":   actualChars,
			"expected_chars": args.ExpectedChars,
		})
	}
	hash := read.Hash
	if hash == "" {
		hash = hashWorkspaceText(read.Content)
	}
	if strings.TrimSpace(args.FinalHash) != "" && !strings.EqualFold(strings.TrimSpace(args.FinalHash), hash) {
		return finalizeWorkspaceWriteMismatch("final_hash", read.Path, map[string]any{
			"actual_hash":   hash,
			"expected_hash": strings.TrimSpace(args.FinalHash),
		})
	}
	payload := workspaceToolPayload("finalize_workspace_write", read.Path, map[string]any{
		"operation":       "finalize_write",
		"path":            read.Path,
		"chunks_verified": args.ExpectedChunks,
		"lines":           actualLines,
		"chars":           actualChars,
		"hash":            hash,
		"size":            read.Size,
		"changed_paths":   []string{read.Path},
		"next_hint":       "The compatibility file write was finalized and verified. It is now safe to call attempt_completion if the task is otherwise complete.",
	})
	return mustJSON(payload), fmt.Sprintf("finalized %s (%d segments, %d lines, %d chars)", read.Path, args.ExpectedChunks, actualLines, actualChars), "正在校验文件...", read.Path, nil
}

func finalizeWorkspaceWriteMismatch(kind, path string, values map[string]any) (string, string, string, string, error) {
	message := "write finalization failed: " + kind + " mismatch"
	payload := map[string]any{
		"error":         message,
		"path":          path,
		"recovery_hint": "Read the file or continue the existing write sequence before claiming completion.",
	}
	for key, value := range values {
		payload[key] = value
	}
	return mustJSON(payload), message, "正在校验文件...", path, fmt.Errorf("%s", message)
}

func countWorkspaceTextLines(text string) int {
	if text == "" {
		return 1
	}
	return strings.Count(text, "\n") + 1
}

func hashWorkspaceText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
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
		return "", "", "正在编辑文件...", "", failure.New("tool_invalid_arguments", "invalid tool arguments", nil)
	}
	result, err := files.EditTextFileSmart(ctx, args.Path, args.OldText, args.NewText, "ai_edit", args.ReplaceAll)
	if err != nil {
		return "", "", "正在编辑文件...", args.Path, err
	}
	write := result.WriteResult
	payload := workspaceToolPayload("edit_workspace_text", write.Path, map[string]any{
		"operation":     "edit",
		"path":          write.Path,
		"hash":          write.Hash,
		"version_id":    write.VersionID,
		"size":          write.Size,
		"replacements":  result.Replacements,
		"match_kind":    result.MatchKind,
		"changed_paths": write.ChangedPaths,
		"next_hint":     "The edit was applied and verified. Continue with another patch if needed, otherwise call attempt_completion.",
	})
	return mustJSON(payload), fmt.Sprintf("edited %s (%d replacement(s), %s match)", write.Path, result.Replacements, result.MatchKind), "正在编辑文件...", write.Path, nil
}

func (s *Service) execReplaceWorkspaceLines(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在编辑文件...", "", err
	}
	var args struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
		NewText   string `json:"new_text"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在编辑文件...", "", failure.New("tool_invalid_arguments", "invalid tool arguments", nil)
	}
	result, err := files.ReplaceTextFileLines(ctx, args.Path, args.StartLine, args.EndLine, args.NewText, "ai_line_replace")
	if err != nil {
		return "", "", "正在编辑文件...", args.Path, err
	}
	write := result.WriteResult
	payload := workspaceToolPayload("replace_workspace_lines", write.Path, map[string]any{
		"operation":          "replace_lines",
		"path":               write.Path,
		"hash":               write.Hash,
		"version_id":         write.VersionID,
		"size":               write.Size,
		"start_line":         result.StartLine,
		"end_line":           result.EndLine,
		"inserted_lines":     result.InsertedLines,
		"total_lines_before": result.TotalLinesBefore,
		"total_lines_after":  result.TotalLinesAfter,
		"changed_paths":      write.ChangedPaths,
		"next_hint":          "The file change was applied and verified. Continue with another patch if needed, otherwise call attempt_completion.",
	})
	return mustJSON(payload), fmt.Sprintf("replaced lines %d-%d in %s", result.StartLine, result.EndLine, write.Path), "正在编辑文件...", write.Path, nil
}
