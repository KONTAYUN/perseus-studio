package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"webide-backend/internal/workspace"
)

func rollbackWorkspacePatchMoves(files *workspace.Service, moves []plannedWorkspacePatchMove) error {
	var failures []error
	for index := len(moves) - 1; index >= 0; index-- {
		item := moves[index]
		if _, err := files.RenameIfUnchanged(context.Background(), item.newPath, item.oldPath, item.hash); err != nil {
			failures = append(failures, fmt.Errorf("restore move %s: %w", item.oldPath, err))
		}
	}
	return errors.Join(failures...)
}

func rollbackWorkspacePatchWrites(ctx context.Context, files *workspace.Service, writes []plannedWorkspacePatchWrite) error {
	ctx = context.WithoutCancel(ctx)
	var failures []error
	for index := len(writes) - 1; index >= 0; index-- {
		item := writes[index]
		if item.create {
			if err := files.RemoveCreatedFileIfUnchanged(ctx, item.path, hashWorkspaceText(item.content)); err != nil {
				failures = append(failures, fmt.Errorf("remove created file %s: %w", item.path, err))
			}
			continue
		}
		expectedHash := hashWorkspaceText(item.content)
		if _, err := files.WriteFileIfUnchanged(ctx, item.path, item.oldContent, "ai_apply_patch_rollback", &expectedHash); err != nil {
			failures = append(failures, fmt.Errorf("restore file %s: %w", item.path, err))
		}
	}
	return errors.Join(failures...)
}

func rollbackWorkspacePatchDeletes(files *workspace.Service, deletes []plannedWorkspacePatchDelete) error {
	var failures []error
	for index := len(deletes) - 1; index >= 0; index-- {
		item := deletes[index]
		if strings.TrimSpace(item.trashed) == "" {
			continue
		}
		if _, err := files.RestoreTrashItem(item.trashed, false); err != nil {
			failures = append(failures, fmt.Errorf("restore deleted file %s: %w", item.path, err))
		}
	}
	return errors.Join(failures...)
}

func patchRollbackError(cause error, failures ...error) error {
	if failure := errors.Join(failures...); failure != nil {
		return fmt.Errorf("%w; rollback incomplete, inspect workspace before retrying: %v", cause, failure)
	}
	return cause
}
