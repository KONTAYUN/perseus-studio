package workspace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
)

const maxChangePreviewBytes = 512 << 10

type ChangePreview struct {
	Change      FileChange `json:"change"`
	Before      string     `json:"before"`
	Current     string     `json:"current"`
	CanRevert   bool       `json:"can_revert"`
	Reason      string     `json:"reason,omitempty"`
	RevertState string     `json:"revert_state,omitempty"`
}

func (s *Service) PreviewChange(ctx context.Context, operationID string, change FileChange) (ChangePreview, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	preview := ChangePreview{Change: change}
	var state string
	err := s.db.QueryRowContext(ctx, `SELECT state FROM workspace_change_reverts WHERE user_id = ? AND operation_id = ?`, s.userID, operationID).Scan(&state)
	if err == nil {
		preview.RevertState = state
	} else if !errors.Is(err, sql.ErrNoRows) {
		return preview, err
	}
	before, current, err := s.changeContents(ctx, change)
	preview.Before, preview.Current = string(before), string(current)
	if err != nil {
		preview.Reason = err.Error()
		return preview, nil
	}
	if state != "" {
		preview.Reason = "该变更已撤销或正在等待核验"
		return preview, nil
	}
	if err := s.checkRevertState(change); err != nil {
		preview.Reason = err.Error()
		return preview, nil
	}
	preview.CanRevert = true
	return preview, nil
}

func (s *Service) changeContents(ctx context.Context, change FileChange) ([]byte, []byte, error) {
	var before, current []byte
	if _, _, err := s.sanitizePath(change.Path); err != nil {
		return nil, nil, err
	}
	if isAnyTrashPath(change.Path) {
		return nil, nil, ErrTrashManagedOnly
	}
	switch change.Operation {
	case "update", "move":
		path := change.Path
		if change.Operation == "move" {
			path = change.OldPath
		}
		var content string
		err := s.db.QueryRowContext(ctx, `SELECT content_text FROM file_versions WHERE user_id = ? AND id = ? AND path = ? AND content_hash = ? AND size_bytes <= ?`, s.userID, change.VersionID, path, change.BeforeHash, maxChangePreviewBytes).Scan(&content)
		if err != nil {
			return nil, nil, fmt.Errorf("修改前快照缺失或超过 512 KiB，无法安全撤销: %w", err)
		}
		before = []byte(content)
		if hashBytes(before) != change.BeforeHash {
			return nil, nil, fmt.Errorf("修改前快照校验失败，无法安全撤销")
		}
	case "delete":
		if !isDirectTrashChild(change.TrashPath) {
			return nil, nil, ErrInvalidPath
		}
		var err error
		before, err = s.readChangePreview(change.TrashPath)
		if err != nil {
			return nil, nil, err
		}
		if hashBytes(before) != change.BeforeHash {
			return before, nil, ErrFileConflict
		}
		return before, nil, nil
	case "create":
	default:
		return nil, nil, fmt.Errorf("此变更类型暂不支持撤销")
	}
	var err error
	current, err = s.readChangePreview(change.Path)
	return before, current, err
}

func (s *Service) readChangePreview(path string) ([]byte, error) {
	_, abs, err := s.sanitizePath(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxChangePreviewBytes {
		return nil, fmt.Errorf("文件超过 512 KiB 或不是普通文件")
	}
	file, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	// Recheck the size through a bounded reader even if an external process grows it.
	return readBoundedChange(file)
}

func (s *Service) checkRevertState(change FileChange) error {
	if change.Operation == "delete" {
		if err := s.verifyMissingPath(change.Path); err != nil {
			return ErrFileConflict
		}
		return s.checkFileHash(change.TrashPath, change.BeforeHash)
	}
	if change.Operation == "move" {
		if isAnyTrashPath(change.OldPath) {
			return ErrTrashManagedOnly
		}
		if err := s.verifyMissingPath(change.OldPath); err != nil {
			return ErrFileConflict
		}
	}
	return s.checkFileHash(change.Path, change.Hash)
}
