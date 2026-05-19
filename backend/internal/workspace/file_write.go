package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (s *Service) WriteFile(ctx context.Context, path, content, source string) (FileWriteResult, error) {
	return s.WriteBytes(ctx, path, []byte(content), source)
}

func (s *Service) WriteBytes(ctx context.Context, path string, content []byte, source string) (FileWriteResult, error) {
	cleanPath, absPath, err := s.sanitizePath(path)
	if err != nil {
		return FileWriteResult{}, err
	}
	if isAnyTrashPath(cleanPath) {
		return FileWriteResult{}, trashManagedOnlyError()
	}

	if source == "" {
		source = "manual_write"
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return FileWriteResult{}, err
	}

	var versionID string
	if info, statErr := os.Stat(absPath); statErr == nil && !info.IsDir() {
		oldContent, err := os.ReadFile(absPath)
		if err != nil {
			return FileWriteResult{}, err
		}
		versionID, err = s.createFileVersion(ctx, cleanPath, oldContent, source)
		if err != nil {
			return FileWriteResult{}, err
		}
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return FileWriteResult{}, statErr
	}

	if err := writeAtomic(absPath, content); err != nil {
		return FileWriteResult{}, err
	}

	return FileWriteResult{
		Path:         cleanPath,
		Hash:         hashBytes(content),
		VersionID:    versionID,
		Size:         int64(len(content)),
		ChangedPaths: []string{cleanPath},
	}, nil
}

func (s *Service) CreateFile(ctx context.Context, path, content string) (FileWriteResult, error) {
	return s.CreateBytes(ctx, path, []byte(content))
}

func (s *Service) CreateTextFile(ctx context.Context, path, content, source string) (FileWriteResult, error) {
	if source == "" {
		source = "ai_create"
	}
	return s.CreateFile(ctx, path, content)
}

func (s *Service) CreateBytes(ctx context.Context, path string, content []byte) (FileWriteResult, error) {
	cleanPath, absPath, err := s.sanitizePath(path)
	if err != nil {
		return FileWriteResult{}, err
	}
	if isAnyTrashPath(cleanPath) {
		return FileWriteResult{}, trashManagedOnlyError()
	}
	if _, err := os.Stat(absPath); err == nil {
		return FileWriteResult{}, fmt.Errorf("path already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return FileWriteResult{}, err
	}
	return s.WriteBytes(ctx, cleanPath, content, "create")
}

func (s *Service) WriteExistingTextFile(ctx context.Context, path, content, source string) (FileWriteResult, error) {
	cleanPath, absPath, err := s.sanitizePath(path)
	if err != nil {
		return FileWriteResult{}, err
	}
	if isAnyTrashPath(cleanPath) {
		return FileWriteResult{}, trashManagedOnlyError()
	}
	info, err := os.Stat(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return FileWriteResult{}, fmt.Errorf("file does not exist")
		}
		return FileWriteResult{}, err
	}
	if info.IsDir() {
		return FileWriteResult{}, fmt.Errorf("path is a directory")
	}
	if !isTextPath(cleanPath) {
		return FileWriteResult{}, fmt.Errorf("only text files can be written")
	}
	if source == "" {
		source = "ai_write"
	}
	return s.WriteBytes(ctx, cleanPath, []byte(content), source)
}

func (s *Service) EditTextFile(ctx context.Context, path, oldText, newText, source string, replaceAll bool) (FileWriteResult, int, error) {
	cleanPath, absPath, err := s.sanitizePath(path)
	if err != nil {
		return FileWriteResult{}, 0, err
	}
	if isAnyTrashPath(cleanPath) {
		return FileWriteResult{}, 0, trashManagedOnlyError()
	}
	info, err := os.Stat(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return FileWriteResult{}, 0, fmt.Errorf("file does not exist")
		}
		return FileWriteResult{}, 0, err
	}
	if info.IsDir() {
		return FileWriteResult{}, 0, fmt.Errorf("path is a directory")
	}
	if !isTextPath(cleanPath) {
		return FileWriteResult{}, 0, fmt.Errorf("only text files can be edited")
	}
	if oldText == "" {
		return FileWriteResult{}, 0, fmt.Errorf("old_text is required")
	}
	raw, err := os.ReadFile(absPath)
	if err != nil {
		return FileWriteResult{}, 0, err
	}
	content := string(raw)
	count := strings.Count(content, oldText)
	if count == 0 {
		return FileWriteResult{}, 0, fmt.Errorf("old_text was not found")
	}
	if !replaceAll && count > 1 {
		return FileWriteResult{}, 0, fmt.Errorf("old_text matched %d times; set replace_all=true or provide a more specific old_text", count)
	}
	limit := 1
	if replaceAll {
		limit = -1
	}
	nextContent := strings.Replace(content, oldText, newText, limit)
	if source == "" {
		source = "ai_edit"
	}
	result, err := s.WriteBytes(ctx, cleanPath, []byte(nextContent), source)
	return result, count, err
}
