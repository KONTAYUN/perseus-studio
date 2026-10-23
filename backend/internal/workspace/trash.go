package workspace

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"webide-backend/internal/idgen"
)

func (s *Service) MoveToTrash(path string) (PathChangeResult, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.moveToTrash(path)
}

func (s *Service) moveToTrash(path string) (PathChangeResult, error) {
	if err := s.migrateLegacyTrashDirLocked(); err != nil {
		return PathChangeResult{}, err
	}
	cleanPath, absPath, err := s.sanitizePath(path)
	if err != nil {
		return PathChangeResult{}, err
	}
	if isAnyTrashPath(cleanPath) {
		return PathChangeResult{}, fmt.Errorf("cannot delete %s itself", TrashDirName)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return PathChangeResult{}, err
	}

	trashAbs := filepath.Join(s.rootAbs, TrashDirName)
	if err := os.MkdirAll(trashAbs, 0o755); err != nil {
		return PathChangeResult{}, err
	}

	targetName, err := uniqueTrashName(trashAbs, filepath.Base(cleanPath))
	if err != nil {
		return PathChangeResult{}, err
	}
	trashPath := TrashDirName + "/" + targetName
	_, absTrashPath, err := s.sanitizePath(trashPath)
	if err != nil {
		return PathChangeResult{}, err
	}

	if err := movePathWithFallback(absPath, absTrashPath); err != nil {
		return PathChangeResult{}, err
	}
	if err := s.recordTrashItem(cleanPath, trashPath, info); err != nil {
		return PathChangeResult{}, errors.Join(err, os.Rename(absTrashPath, absPath))
	}
	if err := s.removeOrderMetadataForPath(cleanPath); err != nil {
		rollbackErr := os.Rename(absTrashPath, absPath)
		if rollbackErr == nil {
			rollbackErr = s.deleteTrashItemRecord(trashPath)
		}
		return PathChangeResult{}, errors.Join(err, rollbackErr)
	}

	return PathChangeResult{
		Path:         trashPath,
		OldPath:      cleanPath,
		NewPath:      trashPath,
		ChangedPaths: []string{cleanPath, trashPath},
	}, nil
}

func (s *Service) recordTrashItem(originalPath, trashPath string, info os.FileInfo) error {
	itemType := "file"
	size := info.Size()
	if info.IsDir() {
		itemType = "folder"
		size = 0
	}
	_, err := s.db.Exec(`
INSERT INTO trash_items (id, user_id, original_path, trash_path, type, size_bytes, deleted_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(user_id, trash_path) DO UPDATE SET original_path = excluded.original_path,
  type = excluded.type, size_bytes = excluded.size_bytes, deleted_at = excluded.deleted_at
`, idgen.New("trash"), s.userID, originalPath, trashPath, itemType, size, time.Now().UTC())
	return err
}

func (s *Service) TrashItems() ([]TrashItem, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.migrateLegacyTrashDirLocked(); err != nil {
		return nil, err
	}
	trashAbs := filepath.Join(s.rootAbs, TrashDirName)
	entries, err := os.ReadDir(trashAbs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []TrashItem{}, nil
		}
		return nil, err
	}

	existing := make(map[string]bool, len(entries))
	items := make([]TrashItem, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		trashPath := TrashDirName + "/" + entry.Name()
		existing[trashPath] = true
		item, found, err := s.lookupTrashItem(trashPath)
		if err != nil {
			return nil, err
		}
		if !found {
			item = TrashItem{
				ID:           trashPath,
				Name:         entry.Name(),
				Path:         trashPath,
				OriginalPath: entry.Name(),
				DeletedAt:    info.ModTime().UTC(),
			}
			if err := s.recordTrashItem(item.OriginalPath, trashPath, info); err != nil {
				return nil, err
			}
		}
		item.Name = entry.Name()
		item.Path = trashPath
		item.ModifiedAt = info.ModTime().UTC()
		item.Size = info.Size()
		if entry.IsDir() {
			item.Type = "folder"
			item.Size = 0
		} else if item.Type == "" {
			item.Type = "file"
		}
		items = append(items, item)
	}
	if err := s.deleteStaleTrashItems(existing); err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].DeletedAt.After(items[j].DeletedAt)
	})
	return items, nil
}

func (s *Service) RestoreTrashItem(trashPath string, overwrite bool) (PathChangeResult, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.restoreTrashItem(trashPath, overwrite)
}

func (s *Service) restoreTrashItem(trashPath string, overwrite bool) (PathChangeResult, error) {
	if err := s.migrateLegacyTrashDirLocked(); err != nil {
		return PathChangeResult{}, err
	}
	cleanTrashPath, absTrashPath, err := s.sanitizePath(trashPath)
	if err != nil {
		return PathChangeResult{}, err
	}
	if !isDirectTrashChild(cleanTrashPath) {
		return PathChangeResult{}, ErrInvalidPath
	}
	if _, err := os.Stat(absTrashPath); err != nil {
		return PathChangeResult{}, err
	}

	item, found, err := s.lookupTrashItem(cleanTrashPath)
	if err != nil {
		return PathChangeResult{}, err
	}
	originalPath := filepath.Base(cleanTrashPath)
	if found && strings.TrimSpace(item.OriginalPath) != "" {
		originalPath = item.OriginalPath
	}
	cleanOriginalPath, absOriginalPath, err := s.sanitizePath(originalPath)
	if err != nil {
		return PathChangeResult{}, err
	}
	if isAnyTrashPath(cleanOriginalPath) {
		return PathChangeResult{}, ErrInvalidPath
	}

	var backup PathChangeResult
	if _, err := os.Stat(absOriginalPath); err == nil {
		if !overwrite {
			return PathChangeResult{}, ErrPathExists
		}
		backup, err = s.moveToTrash(cleanOriginalPath)
		if err != nil {
			return PathChangeResult{}, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return PathChangeResult{}, err
	}
	rollbackBackup := func() error {
		if backup.NewPath == "" {
			return nil
		}
		_, absBackup, err := s.sanitizePath(backup.NewPath)
		if err == nil {
			err = os.Rename(absBackup, absOriginalPath)
		}
		if err == nil {
			err = s.deleteTrashItemRecord(backup.NewPath)
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absOriginalPath), 0o755); err != nil {
		return PathChangeResult{}, errors.Join(err, rollbackBackup())
	}
	if err := movePathWithFallback(absTrashPath, absOriginalPath); err != nil {
		return PathChangeResult{}, errors.Join(err, rollbackBackup())
	}
	if err := s.deleteTrashItemRecord(cleanTrashPath); err != nil {
		rollbackErr := os.Rename(absOriginalPath, absTrashPath)
		if rollbackErr == nil {
			rollbackErr = rollbackBackup()
		}
		return PathChangeResult{}, errors.Join(err, rollbackErr)
	}
	return PathChangeResult{
		Path:         cleanOriginalPath,
		OldPath:      cleanTrashPath,
		NewPath:      cleanOriginalPath,
		ChangedPaths: []string{cleanTrashPath, cleanOriginalPath},
	}, nil
}

func (s *Service) DeleteTrashItem(trashPath string) (map[string]any, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.migrateLegacyTrashDirLocked(); err != nil {
		return nil, err
	}
	cleanTrashPath, absTrashPath, err := s.sanitizePath(trashPath)
	if err != nil {
		return nil, err
	}
	if !isDirectTrashChild(cleanTrashPath) {
		return nil, ErrInvalidPath
	}
	if err := os.RemoveAll(absTrashPath); err != nil {
		return nil, err
	}
	if err := s.deleteTrashItemRecord(cleanTrashPath); err != nil {
		return nil, err
	}
	return map[string]any{"path": cleanTrashPath, "deleted": true}, nil
}

func (s *Service) lookupTrashItem(trashPath string) (TrashItem, bool, error) {
	var item TrashItem
	err := s.db.QueryRow(`
SELECT id, original_path, trash_path, type, size_bytes, deleted_at
FROM trash_items
WHERE user_id = ? AND trash_path = ?
`, s.userID, trashPath).Scan(&item.ID, &item.OriginalPath, &item.Path, &item.Type, &item.Size, &item.DeletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return TrashItem{}, false, nil
	}
	if err != nil {
		return TrashItem{}, false, err
	}
	item.Name = filepath.Base(item.Path)
	return item, true, nil
}

func (s *Service) deleteTrashItemRecord(trashPath string) error {
	_, err := s.db.Exec(`DELETE FROM trash_items WHERE user_id = ? AND trash_path = ?`, s.userID, trashPath)
	return err
}

func (s *Service) deleteStaleTrashItems(existing map[string]bool) error {
	rows, err := s.db.Query(`SELECT trash_path FROM trash_items WHERE user_id = ?`, s.userID)
	if err != nil {
		return err
	}
	var stale []string
	for rows.Next() {
		var trashPath string
		if err := rows.Scan(&trashPath); err != nil {
			_ = rows.Close()
			return err
		}
		if !existing[trashPath] {
			stale = append(stale, trashPath)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, trashPath := range stale {
		if err := s.deleteTrashItemRecord(trashPath); err != nil {
			return err
		}
	}
	return nil
}

func isAnyTrashPath(path string) bool {
	return IsTrashPath(path) || IsLegacyTrashPath(path)
}

func isDirectTrashChild(path string) bool {
	return IsTrashPath(path) && path != TrashDirName && getParentPath(path) == TrashDirName
}

func trashManagedOnlyError() error {
	return ErrTrashManagedOnly
}

func IsTrashPath(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")))
	return clean == TrashDirName || strings.HasPrefix(clean, TrashDirName+"/")
}

func IsLegacyTrashPath(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")))
	return clean == LegacyTrashDirName || strings.HasPrefix(clean, LegacyTrashDirName+"/")
}

func (s *Service) EmptyTrash() (map[string]any, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.migrateLegacyTrashDirLocked(); err != nil {
		return nil, err
	}
	cleanPath, trashAbs, err := s.sanitizePath(TrashDirName)
	if err != nil {
		return nil, err
	}
	if cleanPath != TrashDirName {
		return nil, fmt.Errorf("invalid trash path")
	}

	entries, err := os.ReadDir(trashAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{"path": TrashDirName, "deleted_count": 0, "message": "回收站不存在或为空"}, nil
		}
		return nil, err
	}
	if len(entries) == 0 {
		return map[string]any{"path": TrashDirName, "deleted_count": 0, "message": "回收站为空"}, nil
	}

	deletedCount := 0
	for _, entry := range entries {
		target := filepath.Join(trashAbs, entry.Name())
		if !strings.HasPrefix(target, trashAbs+string(os.PathSeparator)) {
			return nil, fmt.Errorf("trash path escapes workspace")
		}
		if err := os.RemoveAll(target); err != nil {
			return nil, err
		}
		if err := s.deleteTrashItemRecord(TrashDirName + "/" + entry.Name()); err != nil {
			return nil, err
		}
		deletedCount++
	}
	return map[string]any{"path": TrashDirName, "deleted_count": deletedCount}, nil
}

func uniqueTrashName(trashAbs, baseName string) (string, error) {
	tryName := baseName
	for attempt := 0; attempt < 20; attempt++ {
		target := filepath.Join(trashAbs, tryName)
		if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
			return tryName, nil
		} else if err != nil {
			return "", err
		}

		timestamp := fmt.Sprintf("%s-%d", time.Now().UTC().Format("20060102-150405.000000000"), attempt)
		ext := filepath.Ext(baseName)
		nameOnly := strings.TrimSuffix(baseName, ext)
		if ext == "" {
			tryName = fmt.Sprintf("%s-%s", nameOnly, timestamp)
		} else {
			tryName = fmt.Sprintf("%s-%s%s", nameOnly, timestamp, ext)
		}
	}
	return "", fmt.Errorf("failed to allocate trash path")
}
