package ai

import (
	"context"
	"fmt"
	"sort"
	"time"
	"webide-backend/internal/failure"
	"webide-backend/internal/workspace"
)

func commitWorkspacePatch(ctx context.Context, files *workspace.Service, writes []plannedWorkspacePatchWrite, moves []plannedWorkspacePatchMove, deletes []plannedWorkspacePatchDelete, added, updated, deleted int) (out workspacePatchApplyResult, commitErr error) {
	entries, err := workspacePatchJournalEntries(files, writes, moves, deletes)
	if err != nil {
		return out, err
	}
	journalID, err := files.BeginPatchJournal(ctx, entries)
	if err != nil {
		return out, err
	}
	defer func() {
		finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if commitErr != nil {
			commitErr = failure.New("workspace_patch_commit_failed", fmt.Sprintf("补丁提交未完成，恢复日志 %s 已保留，请核验文件后恢复: %v", journalID, commitErr), commitErr)
		}
		if commitErr == nil {
			if err := files.FinishPatchJournal(finishCtx, journalID); err != nil {
				commitErr = failure.New("local_persistence_failure", "补丁已修改文件，但恢复日志确认失败；需核验后恢复", err)
			}
		}
	}()
	var changedPaths []string
	var pathMoves []WorkspacePathMove
	var changes []workspace.FileChange
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
			expectedHash := hashWorkspaceText(item.oldContent)
			result, err = files.WriteFileIfUnchanged(ctx, item.path, item.content, "ai_apply_patch", &expectedHash)
		}
		if err != nil {
			return workspacePatchApplyResult{}, patchRollbackError(fmt.Errorf("patch apply failed: write %q: %w", item.path, err), rollbackWorkspacePatchWrites(ctx, files, appliedWrites))
		}
		appliedWrites = append(appliedWrites, item)
		operation, beforeHash := "update", hashWorkspaceText(item.oldContent)
		if item.create {
			operation, beforeHash = "create", ""
		}
		changes = append(changes, workspace.FileChange{Operation: operation, Path: result.Path, BeforeHash: beforeHash, Hash: result.Hash, VersionID: result.VersionID})
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
		result, err := files.RenameIfUnchanged(ctx, item.oldPath, item.newPath, item.hash)
		if err != nil {
			return workspacePatchApplyResult{}, patchRollbackError(fmt.Errorf("patch apply failed: move %q to %q: %w", item.oldPath, item.newPath, err), rollbackWorkspacePatchMoves(files, appliedMoves), rollbackWorkspacePatchWrites(ctx, files, appliedWrites))
		}
		appliedMoves = append(appliedMoves, item)
		for index := range changes {
			if changes[index].Path == item.oldPath {
				changes[index].Operation, changes[index].OldPath, changes[index].Path = "move", item.oldPath, item.newPath
			}
		}
		if primaryPath == item.oldPath {
			primaryPath = result["new_path"]
		}
		changedPaths = append(changedPaths, result["old_path"], result["new_path"])
		pathMoves = append(pathMoves, WorkspacePathMove{OldPath: result["old_path"], NewPath: result["new_path"], Status: "success"})
	}
	for _, item := range deletes {
		result, err := files.MoveToTrashIfUnchanged(ctx, item.path, item.hash)
		if err != nil {
			return workspacePatchApplyResult{}, patchRollbackError(fmt.Errorf("patch apply failed: delete %q: %w", item.path, err), rollbackWorkspacePatchDeletes(files, deletes), rollbackWorkspacePatchMoves(files, appliedMoves), rollbackWorkspacePatchWrites(ctx, files, appliedWrites))
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
		pathMoves = append(pathMoves, WorkspacePathMove{OldPath: result.OldPath, NewPath: result.NewPath, Status: "success"})
		changes = append(changes, workspace.FileChange{Operation: "delete", Path: item.path, TrashPath: result.NewPath, BeforeHash: item.hash})
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
		Moved:        pathMoves,
		Changes:      changes,
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
