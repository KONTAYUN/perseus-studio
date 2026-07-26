package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"webide-backend/internal/workspace"
)

type workspacePatchOperationKind string

const (
	workspacePatchAdd    workspacePatchOperationKind = "add"
	workspacePatchUpdate workspacePatchOperationKind = "update"
	workspacePatchDelete workspacePatchOperationKind = "delete"
)

type workspacePatch struct {
	Operations []workspacePatchOperation
}

type workspacePatchOperation struct {
	Kind     workspacePatchOperationKind
	Path     string
	MovePath string
	Hunks    []workspacePatchHunk
	Lines    []string
}

type workspacePatchHunk struct {
	Contexts  []string
	OldLines  []string
	NewLines  []string
	EndOfFile bool
}

type workspacePatchApplyResult struct {
	Path         string
	ChangedPaths []string
	Hash         string
	VersionID    string
	Size         int64
	LineCount    int
	CharCount    int
	Added        int
	Updated      int
	Deleted      int
	DiffSummary  string
}

type workspacePatchApplyError struct {
	Message         string
	Path            string
	PatchBlockIndex int
	OperationIndex  int
	HunkIndex       int
	Reason          string
	FailureLine     int
	FailureContext  []string
	CandidateLines  []string
	CandidatePaths  []string
}

func (e workspacePatchApplyError) Error() string {
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	if strings.TrimSpace(e.Reason) != "" {
		return "patch apply failed: " + e.Reason
	}
	return "patch apply failed"
}

func patchApplyError(message, path string, operationIndex, hunkIndex int, reason string, candidates []string) error {
	return workspacePatchApplyError{
		Message:        message,
		Path:           strings.TrimSpace(path),
		OperationIndex: operationIndex,
		HunkIndex:      hunkIndex,
		Reason:         strings.TrimSpace(reason),
		CandidateLines: candidates,
	}
}

func parsePatchApplyError(message, reason string, line int, context []string, paths []string) error {
	return workspacePatchApplyError{
		Message:        message,
		Reason:         strings.TrimSpace(reason),
		FailureLine:    line,
		FailureContext: trimPatchDiagnosticStrings(context),
		CandidatePaths: normalizeCandidatePaths(paths...),
	}
}

type plannedWorkspacePatchWrite struct {
	path       string
	content    string
	oldContent string
	create     bool
}

type plannedWorkspacePatchDelete struct {
	path    string
	trashed string
}

type plannedWorkspacePatchMove struct {
	oldPath string
	newPath string
}

func (s *Service) execApplyWorkspacePatch(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在编辑文件...", "", err
	}
	var args struct {
		Patch     string `json:"patch"`
		Transport string `json:"transport"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在编辑文件...", "", fmt.Errorf("invalid tool arguments")
	}
	patchText := strings.TrimSpace(args.Patch)
	if patchText == "" {
		return "", "", "正在编辑文件...", "", fmt.Errorf("invalid tool arguments: patch is required")
	}
	result, err := applyWorkspacePatch(ctx, files, patchText)
	if err != nil {
		payload := map[string]any{
			"error":               err.Error(),
			"applied":             false,
			"files_changed":       false,
			"transport":           "structured_tool",
			"failure_stage":       "patch_apply_verification",
			"recovery_hint":       "The patch was captured, but verification failed before writing. Read the target file/range again and submit a corrected *** Begin Patch text patch before claiming completion.",
			"unapplied_patch_sha": hashWorkspaceText(patchText),
		}
		if paths := workspacePatchCandidatePaths(patchText); len(paths) > 0 {
			payload["patch_paths"] = paths
		}
		var patchErr workspacePatchApplyError
		if ok := errorAsWorkspacePatchApplyError(err, &patchErr); ok {
			if patchErr.Path != "" {
				payload["path"] = patchErr.Path
			}
			if patchErr.PatchBlockIndex > 0 {
				payload["patch_block_index"] = patchErr.PatchBlockIndex
			}
			if patchErr.OperationIndex > 0 {
				payload["operation_index"] = patchErr.OperationIndex
			}
			if patchErr.HunkIndex > 0 {
				payload["hunk_index"] = patchErr.HunkIndex
			}
			if patchErr.Reason != "" {
				payload["reason"] = patchErr.Reason
			}
			if patchErr.FailureLine > 0 {
				payload["failure_line"] = patchErr.FailureLine
			}
			if len(patchErr.FailureContext) > 0 {
				payload["failure_context"] = patchErr.FailureContext
			}
			if len(patchErr.CandidateLines) > 0 {
				payload["candidate_lines"] = patchErr.CandidateLines
			}
			if len(patchErr.CandidatePaths) > 0 {
				payload["candidate_paths"] = patchErr.CandidatePaths
			}
		}
		payload["patch_chars"] = len([]rune(patchText))
		payload["patch_sha"] = hashWorkspaceText(patchText)
		payload["patch_head"] = patchDiagnosticHead(patchText)
		payload["patch_tail"] = patchDiagnosticTail(patchText)
		if count := countWorkspacePatchBlocks(patchText); count > 0 {
			payload["patch_block_count"] = count
		}
		if path := firstPatchPathFromText(patchText); path != "" {
			if _, exists := payload["path"]; !exists {
				payload["path"] = path
			}
		}
		return mustJSON(payload), err.Error(), "正在编辑文件...", firstPatchPathFromText(patchText), err
	}
	payload := workspaceToolPayload("apply_workspace_patch", result.Path, map[string]any{
		"operation":     "apply_patch",
		"path":          result.Path,
		"changed_paths": result.ChangedPaths,
		"hash":          result.Hash,
		"version_id":    result.VersionID,
		"size":          result.Size,
		"line_count":    result.LineCount,
		"char_count":    result.CharCount,
		"diff_summary":  result.DiffSummary,
	})
	return mustJSON(payload), result.DiffSummary, "正在编辑文件...", result.Path, nil
}

func errorAsWorkspacePatchApplyError(err error, target *workspacePatchApplyError) bool {
	if err == nil || target == nil {
		return false
	}
	if item, ok := err.(workspacePatchApplyError); ok {
		*target = item
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return errorAsWorkspacePatchApplyError(wrapped.Unwrap(), target)
	}
	return false
}

func applyWorkspacePatch(ctx context.Context, files *workspace.Service, patchText string) (workspacePatchApplyResult, error) {
	patch, err := parseWorkspacePatch(patchText)
	if err != nil {
		return workspacePatchApplyResult{}, err
	}
	if len(patch.Operations) == 0 {
		return workspacePatchApplyResult{}, fmt.Errorf("patch apply failed: patch contains no operations")
	}

	writes := make([]plannedWorkspacePatchWrite, 0)
	moves := make([]plannedWorkspacePatchMove, 0)
	deletes := make([]plannedWorkspacePatchDelete, 0)
	seen := map[string]bool{}
	seenChangePaths := map[string]bool{}
	added, updated, deleted := 0, 0, 0

	for opIndex, op := range patch.Operations {
		operationIndex := opIndex + 1
		path := strings.TrimSpace(op.Path)
		movePath := strings.TrimSpace(op.MovePath)
		if path == "" {
			return workspacePatchApplyResult{}, patchApplyError(fmt.Sprintf("patch apply failed: operation %d missing path", operationIndex), "", operationIndex, 0, "operation missing path", nil)
		}
		if seen[path] {
			return workspacePatchApplyResult{}, patchApplyError(fmt.Sprintf("patch apply failed: path %q appears in multiple operations", path), path, operationIndex, 0, "path appears in multiple operations", nil)
		}
		seen[path] = true
		if seenChangePaths[path] {
			return workspacePatchApplyResult{}, patchApplyError(fmt.Sprintf("patch apply failed: path %q is both a source and target", path), path, operationIndex, 0, "path is both a source and target", nil)
		}
		seenChangePaths[path] = true
		if !workspace.IsSupportedTextPath(path) {
			return workspacePatchApplyResult{}, patchApplyError(fmt.Sprintf("patch apply failed: unsupported file type for %q", path), path, operationIndex, 0, "unsupported file type", nil)
		}
		if movePath != "" {
			if op.Kind != workspacePatchUpdate {
				return workspacePatchApplyResult{}, patchApplyError("patch apply failed: move is only supported on update file operations", path, operationIndex, 0, "move directive outside update operation", nil)
			}
			if seenChangePaths[movePath] {
				return workspacePatchApplyResult{}, patchApplyError(fmt.Sprintf("patch apply failed: target path %q conflicts with another operation", movePath), movePath, operationIndex, 0, "move target conflicts with another operation", nil)
			}
			seenChangePaths[movePath] = true
			if !workspace.IsSupportedTextPath(movePath) {
				return workspacePatchApplyResult{}, patchApplyError(fmt.Sprintf("patch apply failed: unsupported move target file type for %q", movePath), movePath, operationIndex, 0, "unsupported move target file type", nil)
			}
			if _, err := files.ReadFile(movePath); err == nil {
				return workspacePatchApplyResult{}, patchApplyError(fmt.Sprintf("patch apply failed: move target already exists: %s", movePath), movePath, operationIndex, 0, "move target already exists", nil)
			}
		}
		switch op.Kind {
		case workspacePatchAdd:
			if _, err := files.ReadFile(path); err == nil {
				return workspacePatchApplyResult{}, patchApplyError(fmt.Sprintf("patch apply failed: add file already exists: %s", path), path, operationIndex, 0, "add file already exists", nil)
			}
			writes = append(writes, plannedWorkspacePatchWrite{path: path, content: strings.Join(op.Lines, ""), create: true})
			added++
		case workspacePatchUpdate:
			read, err := files.ReadFile(path)
			if err != nil {
				return workspacePatchApplyResult{}, patchApplyError(fmt.Sprintf("patch apply failed: update file not readable %q: %v", path, err), path, operationIndex, 0, "update file not readable", nil)
			}
			next, err := applyWorkspacePatchUpdate(read.Content, op.Hunks)
			if err != nil {
				if varErr, ok := err.(workspacePatchApplyError); ok {
					varErr.Path = path
					varErr.OperationIndex = operationIndex
					varErr.Message = fmt.Sprintf("patch apply failed: %s: %s", path, varErr.Error())
					return workspacePatchApplyResult{}, varErr
				}
				return workspacePatchApplyResult{}, patchApplyError(fmt.Sprintf("patch apply failed: %s: %v", path, err), path, operationIndex, 0, err.Error(), nil)
			}
			writes = append(writes, plannedWorkspacePatchWrite{path: path, content: next, oldContent: read.Content})
			if movePath != "" && movePath != path {
				moves = append(moves, plannedWorkspacePatchMove{oldPath: path, newPath: movePath})
			}
			updated++
		case workspacePatchDelete:
			if movePath != "" {
				return workspacePatchApplyResult{}, patchApplyError("patch apply failed: delete operation must not include move target", path, operationIndex, 0, "delete operation must not include move target", nil)
			}
			if _, err := files.ReadFile(path); err != nil {
				return workspacePatchApplyResult{}, patchApplyError(fmt.Sprintf("patch apply failed: delete file not readable %q: %v", path, err), path, operationIndex, 0, "delete file not readable", nil)
			}
			deletes = append(deletes, plannedWorkspacePatchDelete{path: path})
			deleted++
		default:
			return workspacePatchApplyResult{}, patchApplyError(fmt.Sprintf("patch apply failed: unsupported operation for %q", path), path, operationIndex, 0, "unsupported operation", nil)
		}
	}

	var changedPaths []string
	var primaryPath, primaryHash, primaryVersion string
	var primarySize int64
	totalLines, totalChars := 0, 0
	appliedWrites := make([]plannedWorkspacePatchWrite, 0, len(writes))
	appliedMoves := make([]plannedWorkspacePatchMove, 0, len(moves))
	for _, item := range writes {
		var result workspace.FileWriteResult
		var err error
		if item.create {
			result, err = files.CreateTextFile(ctx, item.path, item.content, "ai_apply_patch")
		} else {
			result, err = files.WriteExistingTextFile(ctx, item.path, item.content, "ai_apply_patch")
		}
		if err != nil {
			rollbackWorkspacePatchWrites(ctx, files, appliedWrites)
			return workspacePatchApplyResult{}, fmt.Errorf("patch apply failed: write %q: %w", item.path, err)
		}
		appliedWrites = append(appliedWrites, item)
		if primaryPath == "" {
			primaryPath = result.Path
			primaryHash = result.Hash
			primaryVersion = result.VersionID
			primarySize = result.Size
		}
		changedPaths = append(changedPaths, result.ChangedPaths...)
		totalLines += countWorkspaceTextLines(item.content)
		totalChars += len([]rune(item.content))
	}
	for _, item := range moves {
		result, err := files.Rename(item.oldPath, item.newPath)
		if err != nil {
			rollbackWorkspacePatchMoves(files, appliedMoves)
			rollbackWorkspacePatchWrites(ctx, files, appliedWrites)
			return workspacePatchApplyResult{}, fmt.Errorf("patch apply failed: move %q to %q: %w", item.oldPath, item.newPath, err)
		}
		appliedMoves = append(appliedMoves, item)
		if primaryPath == item.oldPath {
			primaryPath = result["new_path"]
		}
		changedPaths = append(changedPaths, result["old_path"], result["new_path"])
	}
	for _, item := range deletes {
		result, err := files.MoveToTrash(item.path)
		if err != nil {
			rollbackWorkspacePatchDeletes(files, deletes)
			rollbackWorkspacePatchMoves(files, appliedMoves)
			rollbackWorkspacePatchWrites(ctx, files, appliedWrites)
			return workspacePatchApplyResult{}, fmt.Errorf("patch apply failed: delete %q: %w", item.path, err)
		}
		for index := range deletes {
			if deletes[index].path == item.path {
				deletes[index].trashed = result.NewPath
				break
			}
		}
		if primaryPath == "" {
			primaryPath = result.OldPath
		}
		changedPaths = append(changedPaths, result.ChangedPaths...)
	}
	changedPaths = uniqueNonEmptyStrings(changedPaths)
	sort.Strings(changedPaths)
	if primaryPath == "" && len(changedPaths) > 0 {
		primaryPath = changedPaths[0]
	}
	summary := workspacePatchDiffSummary(added, updated, deleted, changedPaths)
	return workspacePatchApplyResult{
		Path:         primaryPath,
		ChangedPaths: changedPaths,
		Hash:         primaryHash,
		VersionID:    primaryVersion,
		Size:         primarySize,
		LineCount:    totalLines,
		CharCount:    totalChars,
		Added:        added,
		Updated:      updated,
		Deleted:      deleted,
		DiffSummary:  summary,
	}, nil
}

func rollbackWorkspacePatchMoves(files *workspace.Service, moves []plannedWorkspacePatchMove) {
	for index := len(moves) - 1; index >= 0; index-- {
		item := moves[index]
		_, _ = files.Rename(item.newPath, item.oldPath)
	}
}

func rollbackWorkspacePatchWrites(ctx context.Context, files *workspace.Service, writes []plannedWorkspacePatchWrite) {
	for index := len(writes) - 1; index >= 0; index-- {
		item := writes[index]
		if item.create {
			_, _ = files.DeleteTrashItem(item.path)
			_, _ = files.Delete(item.path)
			continue
		}
		_, _ = files.WriteExistingTextFile(ctx, item.path, item.oldContent, "ai_apply_patch_rollback")
	}
}

func rollbackWorkspacePatchDeletes(files *workspace.Service, deletes []plannedWorkspacePatchDelete) {
	for index := len(deletes) - 1; index >= 0; index-- {
		item := deletes[index]
		if strings.TrimSpace(item.trashed) == "" {
			continue
		}
		_, _ = files.RestoreTrashItem(item.trashed, false)
	}
}
