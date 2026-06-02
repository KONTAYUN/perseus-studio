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
			"transport":           "assistant_text_patch",
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

func parseWorkspacePatch(input string) (workspacePatch, error) {
	text, err := normalizeWorkspacePatchEnvelope(input)
	if err != nil {
		return workspacePatch{}, err
	}
	lines := splitPatchLines(text)
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "*** Begin Patch" {
		return workspacePatch{}, parsePatchApplyError("patch apply failed: patch must start with *** Begin Patch", "patch must start with *** Begin Patch", 1, patchDiagnosticLineWindow(lines, 0), workspacePatchCandidatePathsLoose(text))
	}
	var patch workspacePatch
	var current *workspacePatchOperation
	var currentHunk *workspacePatchHunk
	finishHunk := func() {
		if current != nil && currentHunk != nil {
			current.Hunks = append(current.Hunks, *currentHunk)
			currentHunk = nil
		}
	}
	finishOp := func() {
		finishHunk()
		if current != nil {
			patch.Operations = append(patch.Operations, *current)
			current = nil
		}
	}
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		header := strings.TrimSpace(strings.TrimSuffix(line, "\n"))
		switch {
		case header == "*** End Patch":
			finishOp()
			if i != len(lines)-1 {
				for _, rest := range lines[i+1:] {
					if strings.TrimSpace(rest) != "" {
						return workspacePatch{}, parsePatchApplyError("patch apply failed: content after *** End Patch", "content after *** End Patch", i+2, patchDiagnosticLineWindow(lines, i+1), workspacePatchCandidatePathsLoose(text))
					}
				}
			}
			return patch, nil
		case strings.HasPrefix(header, "*** Add File: "):
			finishOp()
			current = &workspacePatchOperation{Kind: workspacePatchAdd, Path: strings.TrimSpace(strings.TrimPrefix(header, "*** Add File: "))}
		case strings.HasPrefix(header, "*** Update File: "):
			finishOp()
			current = &workspacePatchOperation{Kind: workspacePatchUpdate, Path: strings.TrimSpace(strings.TrimPrefix(header, "*** Update File: "))}
		case strings.HasPrefix(header, "*** Move to: "):
			if current == nil || current.Kind != workspacePatchUpdate {
				return workspacePatch{}, parsePatchApplyError("patch apply failed: move directive outside update operation", "move directive outside update operation", i+1, patchDiagnosticLineWindow(lines, i), workspacePatchCandidatePathsLoose(text))
			}
			if current.MovePath != "" {
				return workspacePatch{}, parsePatchApplyError(fmt.Sprintf("patch apply failed: duplicate move directive for %q", current.Path), "duplicate move directive", i+1, patchDiagnosticLineWindow(lines, i), workspacePatchCandidatePathsLoose(text))
			}
			current.MovePath = strings.TrimSpace(strings.TrimPrefix(header, "*** Move to: "))
		case strings.HasPrefix(header, "*** Delete File: "):
			finishOp()
			current = &workspacePatchOperation{Kind: workspacePatchDelete, Path: strings.TrimSpace(strings.TrimPrefix(header, "*** Delete File: "))}
		case header == "*** End of File":
			if current == nil || current.Kind != workspacePatchUpdate || currentHunk == nil {
				return workspacePatch{}, parsePatchApplyError("patch apply failed: end-of-file marker outside update hunk", "end-of-file marker outside update hunk", i+1, patchDiagnosticLineWindow(lines, i), workspacePatchCandidatePathsLoose(text))
			}
			currentHunk.EndOfFile = true
		case strings.HasPrefix(header, "*** "):
			return workspacePatch{}, parsePatchApplyError(fmt.Sprintf("patch apply failed: unsupported patch directive %q", header), "unsupported patch directive", i+1, patchDiagnosticLineWindow(lines, i), workspacePatchCandidatePathsLoose(text))
		case strings.HasPrefix(header, "@@"):
			if current == nil || current.Kind != workspacePatchUpdate {
				return workspacePatch{}, parsePatchApplyError("patch apply failed: hunk marker outside update operation", "hunk marker outside update operation", i+1, patchDiagnosticLineWindow(lines, i), workspacePatchCandidatePathsLoose(text))
			}
			context := strings.TrimSpace(strings.TrimPrefix(header, "@@"))
			if currentHunk == nil {
				currentHunk = &workspacePatchHunk{}
			} else if len(currentHunk.OldLines) > 0 || len(currentHunk.NewLines) > 0 {
				finishHunk()
				currentHunk = &workspacePatchHunk{}
			}
			if context != "" {
				currentHunk.Contexts = append(currentHunk.Contexts, context)
			}
		default:
			if current == nil {
				if strings.TrimSpace(line) == "" {
					continue
				}
				return workspacePatch{}, parsePatchApplyError("patch apply failed: content outside operation", "content outside operation", i+1, patchDiagnosticLineWindow(lines, i), workspacePatchCandidatePathsLoose(text))
			}
			if current.Kind == workspacePatchDelete {
				if strings.TrimSpace(line) == "" {
					continue
				}
				return workspacePatch{}, parsePatchApplyError("patch apply failed: delete operation must not contain content", "delete operation must not contain content", i+1, patchDiagnosticLineWindow(lines, i), workspacePatchCandidatePathsLoose(text))
			}
			if line == "" {
				continue
			}
			prefix := line[0]
			content := line[1:]
			switch current.Kind {
			case workspacePatchAdd:
				if prefix != '+' {
					return workspacePatch{}, parsePatchApplyError("patch apply failed: add file lines must start with +", "add file lines must start with +", i+1, patchDiagnosticLineWindow(lines, i), workspacePatchCandidatePathsLoose(text))
				}
				current.Lines = append(current.Lines, content)
			case workspacePatchUpdate:
				if currentHunk == nil {
					currentHunk = &workspacePatchHunk{}
				}
				switch prefix {
				case ' ':
					currentHunk.OldLines = append(currentHunk.OldLines, content)
					currentHunk.NewLines = append(currentHunk.NewLines, content)
				case '-':
					currentHunk.OldLines = append(currentHunk.OldLines, content)
				case '+':
					currentHunk.NewLines = append(currentHunk.NewLines, content)
				default:
					return workspacePatch{}, parsePatchApplyError("patch apply failed: update hunk lines must start with space, -, or +", "update hunk lines must start with space, -, or +", i+1, patchDiagnosticLineWindow(lines, i), workspacePatchCandidatePathsLoose(text))
				}
			}
		}
	}
	return workspacePatch{}, parsePatchApplyError("patch apply failed: missing *** End Patch", "missing *** End Patch", len(lines), patchDiagnosticLineWindow(lines, len(lines)-1), workspacePatchCandidatePathsLoose(text))
}

func applyWorkspacePatchUpdate(content string, hunks []workspacePatchHunk) (string, error) {
	if len(hunks) == 0 {
		return "", patchApplyError("update operation contains no hunks", "", 0, 0, "update operation contains no hunks", nil)
	}
	eol := dominantEOL(content)
	lines := splitWorkspacePatchContentLines(normalizePatchNewlines(content))
	type replacement struct {
		start int
		oldN  int
		new   []string
	}
	var replacements []replacement
	searchStart := 0
	for index, hunk := range hunks {
		oldLines := trimPatchLineSuffixes(hunk.OldLines)
		newLines := trimPatchLineSuffixes(hunk.NewLines)
		if len(oldLines) == 0 {
			return "", patchApplyError(fmt.Sprintf("hunk %d has no removable/context lines", index+1), "", 0, index+1, "hunk has no removable/context lines", nil)
		}
		for _, context := range hunk.Contexts {
			context = strings.TrimSpace(context)
			if context == "" {
				continue
			}
			contextIndex, matches := seekWorkspacePatchSequence(lines, []string{context}, searchStart)
			if matches == 0 {
				candidates := similarWorkspacePatchCandidates(lines, []string{context}, 3)
				return "", patchApplyError(fmt.Sprintf("update hunk %d context %q did not match", index+1, context), "", 0, index+1, "context did not match", candidates)
			}
			if matches > 1 {
				return "", patchApplyError(fmt.Sprintf("update hunk %d context %q matched %d times; add more context", index+1, context, matches), "", 0, index+1, "context matched multiple times", nil)
			}
			searchStart = contextIndex + 1
		}
		found, matches := seekWorkspacePatchSequence(lines, oldLines, searchStart, hunk.EndOfFile)
		if matches == 0 {
			return "", workspacePatchHunkError(lines, oldLines, index+1, "did not match")
		}
		if matches > 1 {
			return "", patchApplyError(fmt.Sprintf("update hunk %d matched %d times; add more context", index+1, matches), "", 0, index+1, "hunk matched multiple times", nil)
		}
		replacements = append(replacements, replacement{start: found, oldN: len(oldLines), new: newLines})
		searchStart = found + len(oldLines)
	}
	for index := len(replacements) - 1; index >= 0; index-- {
		item := replacements[index]
		next := make([]string, 0, len(lines)-item.oldN+len(item.new))
		next = append(next, lines[:item.start]...)
		next = append(next, item.new...)
		next = append(next, lines[item.start+item.oldN:]...)
		lines = next
	}
	if len(lines) == 0 || lines[len(lines)-1] != "" {
		lines = append(lines, "")
	}
	return restoreEOL(strings.Join(lines, "\n"), eol), nil
}

func splitWorkspacePatchContentLines(content string) []string {
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func trimPatchLineSuffixes(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, strings.TrimSuffix(line, "\n"))
	}
	return out
}

func normalizeWorkspacePatchEnvelope(input string) (string, error) {
	text := normalizePatchNewlines(input)
	blocks, err := extractWorkspacePatchEnvelopes(text)
	if err != nil {
		return "", err
	}
	if len(blocks) == 0 {
		return "", parsePatchApplyError("patch apply failed: missing *** Begin Patch", "missing *** Begin Patch", 0, nil, workspacePatchCandidatePathsLoose(text))
	}
	if len(blocks) == 1 {
		return blocks[0], nil
	}
	var builder strings.Builder
	builder.WriteString("*** Begin Patch\n")
	for _, block := range blocks {
		lines := splitPatchLines(block)
		for index, line := range lines {
			header := strings.TrimSpace(strings.TrimSuffix(line, "\n"))
			if index == 0 && header == "*** Begin Patch" {
				continue
			}
			if header == "*** End Patch" {
				continue
			}
			builder.WriteString(line)
		}
		if !strings.HasSuffix(builder.String(), "\n") {
			builder.WriteString("\n")
		}
	}
	builder.WriteString("*** End Patch")
	return builder.String(), nil
}

func extractWorkspacePatchEnvelopes(text string) ([]string, error) {
	lines := splitPatchLines(text)
	var blocks []string
	var current strings.Builder
	inPatch := false
	startLine := 0
	for index, line := range lines {
		header := strings.TrimSpace(strings.TrimSuffix(line, "\n"))
		switch header {
		case "*** Begin Patch":
			if inPatch {
				return nil, parsePatchApplyError("patch apply failed: nested *** Begin Patch", "nested *** Begin Patch", index+1, patchDiagnosticLineWindow(lines, index), workspacePatchCandidatePathsLoose(text))
			}
			inPatch = true
			startLine = index + 1
			current.Reset()
			current.WriteString("*** Begin Patch\n")
		case "*** End Patch":
			if !inPatch {
				return nil, parsePatchApplyError("patch apply failed: *** End Patch before *** Begin Patch", "*** End Patch before *** Begin Patch", index+1, patchDiagnosticLineWindow(lines, index), workspacePatchCandidatePathsLoose(text))
			}
			current.WriteString("*** End Patch")
			blocks = append(blocks, current.String())
			inPatch = false
			current.Reset()
		default:
			if inPatch {
				current.WriteString(line)
			}
		}
	}
	if inPatch {
		return nil, parsePatchApplyError("patch apply failed: missing *** End Patch", "missing *** End Patch", startLine, patchDiagnosticLineWindow(lines, len(lines)-1), workspacePatchCandidatePathsLoose(text))
	}
	return blocks, nil
}

func seekWorkspacePatchSequence(lines, pattern []string, start int, eof ...bool) (int, int) {
	if len(pattern) == 0 || len(pattern) > len(lines) {
		return -1, 0
	}
	if start < 0 {
		start = 0
	}
	best := -1
	matches := 0
	for _, matcher := range []func(string, string) bool{
		func(a, b string) bool { return a == b },
		func(a, b string) bool { return strings.TrimRight(a, " \t") == strings.TrimRight(b, " \t") },
		func(a, b string) bool { return strings.TrimSpace(a) == strings.TrimSpace(b) },
		func(a, b string) bool { return normalizeWorkspacePatchUnicode(a) == normalizeWorkspacePatchUnicode(b) },
	} {
		best = -1
		matches = 0
		for index := start; index+len(pattern) <= len(lines); index++ {
			if len(eof) > 0 && eof[0] && index+len(pattern) != len(lines) {
				continue
			}
			if workspacePatchLinesMatch(lines[index:index+len(pattern)], pattern, matcher) {
				best = index
				matches++
			}
		}
		if matches > 0 {
			return best, matches
		}
	}
	return -1, 0
}

func workspacePatchHunkError(lines, pattern []string, hunkIndex int, reason string) error {
	candidates := similarWorkspacePatchCandidates(lines, pattern, 3)
	if len(candidates) == 0 {
		return patchApplyError(fmt.Sprintf("update hunk %d %s; read the target range again and submit a corrected patch", hunkIndex, reason), "", 0, hunkIndex, "hunk "+reason, nil)
	}
	return patchApplyError(fmt.Sprintf("update hunk %d %s; nearby candidates: %s", hunkIndex, reason, strings.Join(candidates, " | ")), "", 0, hunkIndex, "hunk "+reason, candidates)
}

func similarWorkspacePatchCandidates(lines, pattern []string, limit int) []string {
	if limit <= 0 || len(lines) == 0 || len(pattern) == 0 {
		return nil
	}
	size := len(pattern)
	if size > len(lines) {
		size = len(lines)
	}
	needle := normalizeWorkspacePatchCandidate(strings.Join(pattern, "\n"))
	type candidate struct {
		line  int
		text  string
		score int
	}
	var candidates []candidate
	for index := 0; index+size <= len(lines); index++ {
		text := strings.Join(lines[index:index+size], "\n")
		score := workspacePatchCandidateScore(needle, normalizeWorkspacePatchCandidate(text))
		if score <= 0 {
			continue
		}
		candidates = append(candidates, candidate{line: index + 1, text: truncateRunesForPatch(text, 180), score: score})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].line < candidates[j].line
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	out := make([]string, 0, len(candidates))
	for _, item := range candidates {
		out = append(out, fmt.Sprintf("line %d: %q", item.line, item.text))
	}
	return out
}

func normalizeWorkspacePatchCandidate(value string) string {
	return strings.Join(strings.Fields(normalizeWorkspacePatchUnicode(value)), " ")
}

func workspacePatchCandidateScore(needle, value string) int {
	if needle == "" || value == "" {
		return 0
	}
	if needle == value {
		return len(needle) + 1000
	}
	score := 0
	for _, token := range strings.Fields(needle) {
		if len([]rune(token)) < 2 {
			continue
		}
		if strings.Contains(value, token) {
			score += len([]rune(token))
		}
	}
	return score
}

func truncateRunesForPatch(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
}

func workspacePatchLinesMatch(lines, pattern []string, matcher func(string, string) bool) bool {
	for index := range pattern {
		if !matcher(lines[index], pattern[index]) {
			return false
		}
	}
	return true
}

func normalizeWorkspacePatchUnicode(value string) string {
	replacer := strings.NewReplacer(
		"\u2010", "-", "\u2011", "-", "\u2012", "-", "\u2013", "-", "\u2014", "-", "\u2015", "-", "\u2212", "-",
		"\u2018", "'", "\u2019", "'", "\u201A", "'", "\u201B", "'",
		"\u201C", "\"", "\u201D", "\"", "\u201E", "\"", "\u201F", "\"",
		"\u00A0", " ", "\u2002", " ", "\u2003", " ", "\u2004", " ", "\u2005", " ", "\u2006", " ", "\u2007", " ",
		"\u2008", " ", "\u2009", " ", "\u200A", " ", "\u202F", " ", "\u205F", " ", "\u3000", " ",
	)
	return strings.TrimSpace(replacer.Replace(value))
}

func splitPatchLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.SplitAfter(text, "\n")
}

func countWorkspacePatchBlocks(text string) int {
	blocks, err := extractWorkspacePatchEnvelopes(normalizePatchNewlines(text))
	if err != nil {
		return strings.Count(text, "*** Begin Patch")
	}
	return len(blocks)
}

func patchDiagnosticHead(text string) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > 600 {
		runes = runes[:600]
	}
	return string(runes)
}

func patchDiagnosticTail(text string) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > 600 {
		runes = runes[len(runes)-600:]
	}
	return string(runes)
}

func patchDiagnosticLineWindow(lines []string, index int) []string {
	if len(lines) == 0 || index < 0 {
		return nil
	}
	start := index - 2
	if start < 0 {
		start = 0
	}
	end := index + 3
	if end > len(lines) {
		end = len(lines)
	}
	out := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		out = append(out, fmt.Sprintf("%d: %s", i+1, strings.TrimSuffix(lines[i], "\n")))
	}
	return trimPatchDiagnosticStrings(out)
}

func trimPatchDiagnosticStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, truncateRunes(value, 300))
	}
	return out
}

func normalizePatchNewlines(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

func dominantEOL(text string) string {
	crlf := strings.Count(text, "\r\n")
	lfOnly := strings.Count(text, "\n") - crlf
	if crlf > lfOnly {
		return "\r\n"
	}
	return "\n"
}

func restoreEOL(text, eol string) string {
	if eol == "\r\n" {
		return strings.ReplaceAll(text, "\n", "\r\n")
	}
	return text
}

func workspacePatchDiffSummary(added, updated, deleted int, changedPaths []string) string {
	parts := make([]string, 0, 4)
	if added > 0 {
		parts = append(parts, fmt.Sprintf("added %d file(s)", added))
	}
	if updated > 0 {
		parts = append(parts, fmt.Sprintf("updated %d file(s)", updated))
	}
	if deleted > 0 {
		parts = append(parts, fmt.Sprintf("deleted %d file(s)", deleted))
	}
	if len(parts) == 0 {
		parts = append(parts, "applied patch")
	}
	if len(changedPaths) > 0 {
		parts = append(parts, fmt.Sprintf("changed %d path(s)", len(changedPaths)))
	}
	return strings.Join(parts, "; ")
}

func firstPatchPathFromText(patchText string) string {
	patch, err := parseWorkspacePatch(patchText)
	if err != nil || len(patch.Operations) == 0 {
		return ""
	}
	return patch.Operations[0].Path
}

func workspacePatchCandidatePaths(patchText string) []string {
	patch, err := parseWorkspacePatch(patchText)
	if err != nil {
		return workspacePatchCandidatePathsLoose(patchText)
	}
	var paths []string
	for _, op := range patch.Operations {
		paths = append(paths, op.Path)
		if strings.TrimSpace(op.MovePath) != "" {
			paths = append(paths, op.MovePath)
		}
	}
	return normalizeCandidatePaths(paths...)
}

func workspacePatchCandidatePathsLoose(patchText string) []string {
	lines := splitPatchLines(normalizePatchNewlines(patchText))
	var paths []string
	for _, line := range lines {
		header := strings.TrimSpace(strings.TrimSuffix(line, "\n"))
		for _, prefix := range []string{"*** Add File: ", "*** Update File: ", "*** Delete File: ", "*** Move to: "} {
			if strings.HasPrefix(header, prefix) {
				paths = append(paths, strings.TrimSpace(strings.TrimPrefix(header, prefix)))
				break
			}
		}
	}
	return normalizeCandidatePaths(paths...)
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := map[string]bool{}
	next := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		next = append(next, value)
	}
	return next
}
