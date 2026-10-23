package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func (s *Service) migrateLegacyTrashDir() error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.migrateLegacyTrashDirLocked()
}

func (s *Service) migrateLegacyTrashDirLocked() error {
	_, legacyAbs, err := s.sanitizePath(LegacyTrashDirName)
	if err != nil {
		return err
	}
	_, currentAbs, err := s.sanitizePath(TrashDirName)
	if err != nil {
		return err
	}

	legacyInfo, err := os.Stat(legacyAbs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if !legacyInfo.IsDir() {
		return nil
	}

	currentInfo, err := os.Stat(currentAbs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return movePathWithFallback(legacyAbs, currentAbs)
		}
		return err
	}
	if !currentInfo.IsDir() {
		return fmt.Errorf("%s exists but is not a folder", TrashDirName)
	}

	entries, err := os.ReadDir(legacyAbs)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		targetName, err := uniqueTrashName(currentAbs, entry.Name())
		if err != nil {
			return err
		}
		if err := movePathWithFallback(filepath.Join(legacyAbs, entry.Name()), filepath.Join(currentAbs, targetName)); err != nil {
			return err
		}
	}
	return os.Remove(legacyAbs)
}
