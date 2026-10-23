package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// These checks share the content-write lock. They protect against other Service
// operations; external programs still require process or OS isolation.
func (s *Service) RenameIfUnchanged(ctx context.Context, oldPath, newPath, expectedHash string) (map[string]string, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.checkFileHash(oldPath, expectedHash); err != nil {
		return nil, err
	}
	result, err := s.renameValidated(oldPath, newPath)
	if err != nil {
		return nil, err
	}
	return map[string]string{"old_path": result.OldPath, "new_path": result.NewPath}, nil
}

func (s *Service) MoveToTrashIfUnchanged(ctx context.Context, path, expectedHash string) (PathChangeResult, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return PathChangeResult{}, err
	}
	if err := s.checkFileHash(path, expectedHash); err != nil {
		return PathChangeResult{}, err
	}
	return s.moveToTrash(path)
}

func (s *Service) checkFileHash(path, expectedHash string) error {
	_, abs, err := s.sanitizePath(path)
	if err != nil {
		return err
	}
	file, err := os.Open(abs)
	if os.IsNotExist(err) {
		return ErrFileConflict
	}
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return ErrInvalidPath
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	if expectedHash == "" || hex.EncodeToString(hash.Sum(nil)) != expectedHash {
		return ErrFileConflict
	}
	return nil
}
