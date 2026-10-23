package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

func readBoundedChange(reader io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maxChangePreviewBytes+1))
	if len(raw) > maxChangePreviewBytes {
		return nil, fmt.Errorf("文件超过 512 KiB")
	}
	return raw, err
}

// A pending journal entry is intentionally not retried: disk may already have
// changed before the final SQLite write failed or the process stopped.
func (s *Service) RevertChange(ctx context.Context, operationID string, change FileChange) (FileChange, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return FileChange{}, err
	}
	var state, result string
	err := s.db.QueryRowContext(ctx, `SELECT state, result_json FROM workspace_change_reverts WHERE user_id = ? AND operation_id = ?`, s.userID, operationID).Scan(&state, &result)
	if err == nil {
		if state != "completed" {
			return FileChange{}, fmt.Errorf("上次撤销结果尚未确认，请核验工作区；不会重复执行")
		}
		var reverted FileChange
		if err := json.Unmarshal([]byte(result), &reverted); err != nil {
			return FileChange{}, err
		}
		return reverted, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return FileChange{}, err
	}
	before, _, err := s.changeContents(ctx, change)
	if err != nil {
		return FileChange{}, err
	}
	if err := s.checkRevertState(change); err != nil {
		return FileChange{}, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO workspace_change_reverts (user_id, operation_id, state, created_at) VALUES (?,?,'pending',?)`, s.userID, operationID, time.Now().UTC())
	if err != nil {
		return FileChange{}, err
	}
	reverted, err := s.revertChange(ctx, change, before)
	if err != nil {
		return FileChange{}, fmt.Errorf("撤销未完成，请核验工作区后处理: %w", err)
	}
	raw, err := json.Marshal(reverted)
	if err != nil {
		return FileChange{}, err
	}
	// Finish recording even when the browser disconnects after the filesystem commit.
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, err = s.db.ExecContext(finishCtx, `UPDATE workspace_change_reverts SET state = 'completed', result_json = ? WHERE user_id = ? AND operation_id = ?`, string(raw), s.userID, operationID)
	if err != nil {
		return FileChange{}, fmt.Errorf("文件已撤销，但记录保存失败，请核验工作区: %w", err)
	}
	return reverted, nil
}

func (s *Service) revertChange(ctx context.Context, change FileChange, before []byte) (FileChange, error) {
	switch change.Operation {
	case "create":
		result, err := s.moveToTrash(change.Path)
		return FileChange{Operation: "delete", Path: change.Path, TrashPath: result.NewPath, BeforeHash: change.Hash}, err
	case "delete":
		// Validate that trash metadata still points to the original recorded path.
		item, found, err := s.lookupTrashItem(change.TrashPath)
		if err != nil {
			return FileChange{}, err
		}
		if !found || item.OriginalPath != change.Path {
			return FileChange{}, fmt.Errorf("回收站恢复路径与原变更不一致")
		}
		_, err = s.restoreTrashItem(change.TrashPath, false)
		return FileChange{Operation: "create", Path: change.Path, Hash: change.BeforeHash}, err
	case "update", "move":
		write, err := s.writeBytes(ctx, change.Path, before, "manual_revert")
		if err != nil {
			return FileChange{}, err
		}
		inverse := FileChange{Operation: "update", Path: change.Path, BeforeHash: change.Hash, Hash: write.Hash, VersionID: write.VersionID}
		if change.Operation == "move" {
			if _, err := s.renameValidated(change.Path, change.OldPath); err != nil {
				return FileChange{}, err
			}
			inverse.Operation, inverse.OldPath, inverse.Path = "move", change.Path, change.OldPath
		}
		return inverse, nil
	}
	return FileChange{}, fmt.Errorf("unsupported change")
}
