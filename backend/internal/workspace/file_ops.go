package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Service) ReadFile(path string) (FileReadResult, error) {
	cleanPath, absPath, err := s.sanitizePath(path)
	if err != nil {
		return FileReadResult{}, err
	}
	if isAnyTrashPath(cleanPath) {
		return FileReadResult{}, trashManagedOnlyError()
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return FileReadResult{}, err
	}
	if info.IsDir() {
		return FileReadResult{}, fmt.Errorf("path is a directory")
	}
	content, err := os.ReadFile(absPath)
	if err != nil {
		return FileReadResult{}, err
	}
	return FileReadResult{
		Path:       cleanPath,
		Content:    string(content),
		Size:       int64(len(content)),
		Hash:       hashBytes(content),
		ModifiedAt: info.ModTime().UTC(),
	}, nil
}

func (s *Service) ReadTextFile(path string, maxChars int) (TextReadResult, error) {
	if maxChars <= 0 {
		maxChars = 20000
	}
	cleanPath, absPath, err := s.sanitizePath(path)
	if err != nil {
		return TextReadResult{}, err
	}
	if isAnyTrashPath(cleanPath) {
		return TextReadResult{}, trashManagedOnlyError()
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return TextReadResult{}, err
	}
	if info.IsDir() {
		return TextReadResult{}, fmt.Errorf("path is a directory")
	}
	if !isTextPath(cleanPath) {
		return TextReadResult{}, fmt.Errorf("unsupported file type")
	}
	content, err := os.ReadFile(absPath)
	if err != nil {
		return TextReadResult{}, err
	}
	text := string(content)
	runes := []rune(text)
	truncated := false
	if len(runes) > maxChars {
		text = string(runes[:maxChars])
		truncated = true
	}
	return TextReadResult{Path: cleanPath, Content: text, Truncated: truncated}, nil
}

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

func (s *Service) Mkdir(path string) (map[string]string, error) {
	cleanPath, absPath, err := s.sanitizePath(path)
	if err != nil {
		return nil, err
	}
	if isAnyTrashPath(cleanPath) {
		return nil, trashManagedOnlyError()
	}
	if err := os.MkdirAll(absPath, 0o755); err != nil {
		return nil, err
	}
	return map[string]string{"path": cleanPath}, nil
}

func (s *Service) MkdirNew(path string) (PathChangeResult, error) {
	cleanPath, absPath, err := s.sanitizePath(path)
	if err != nil {
		return PathChangeResult{}, err
	}
	if isAnyTrashPath(cleanPath) {
		return PathChangeResult{}, trashManagedOnlyError()
	}
	if _, err := os.Stat(absPath); err == nil {
		return PathChangeResult{}, fmt.Errorf("path already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return PathChangeResult{}, err
	}
	if err := os.MkdirAll(absPath, 0o755); err != nil {
		return PathChangeResult{}, err
	}
	return PathChangeResult{
		Path:         cleanPath,
		ChangedPaths: []string{cleanPath},
	}, nil
}

func (s *Service) RenameWithNewName(oldPath, newName string) (PathChangeResult, error) {
	cleanOld, _, err := s.sanitizePath(oldPath)
	if err != nil {
		return PathChangeResult{}, err
	}
	if isAnyTrashPath(cleanOld) {
		return PathChangeResult{}, trashManagedOnlyError()
	}
	cleanName, err := sanitizeLeafName(newName)
	if err != nil {
		return PathChangeResult{}, err
	}

	parent := getParentPath(cleanOld)
	cleanNew := cleanName
	if parent != "" {
		cleanNew = parent + "/" + cleanName
	}
	if isAnyTrashPath(cleanNew) {
		return PathChangeResult{}, trashManagedOnlyError()
	}
	if cleanNew == cleanOld {
		return PathChangeResult{}, fmt.Errorf("new name must be different")
	}
	return s.renameValidated(cleanOld, cleanNew)
}

func (s *Service) MovePath(oldPath, newParentPath string) (PathChangeResult, error) {
	cleanOld, absOld, err := s.sanitizePath(oldPath)
	if err != nil {
		return PathChangeResult{}, err
	}
	if isAnyTrashPath(cleanOld) {
		return PathChangeResult{}, trashManagedOnlyError()
	}

	cleanParent, absParent, err := s.sanitizeOptionalPath(newParentPath)
	if err != nil {
		return PathChangeResult{}, err
	}
	if isAnyTrashPath(cleanParent) {
		return PathChangeResult{}, trashManagedOnlyError()
	}
	parentInfo, err := os.Stat(absParent)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PathChangeResult{}, fmt.Errorf("target parent folder does not exist")
		}
		return PathChangeResult{}, err
	}
	if !parentInfo.IsDir() {
		return PathChangeResult{}, fmt.Errorf("target parent path is not a folder")
	}
	sourceInfo, err := os.Stat(absOld)
	if err != nil {
		return PathChangeResult{}, err
	}
	if sourceInfo.IsDir() && cleanParent != "" && (cleanParent == cleanOld || strings.HasPrefix(cleanParent, cleanOld+"/")) {
		return PathChangeResult{}, ErrMoveIntoSelf
	}

	baseName := filepath.Base(cleanOld)
	cleanNew := baseName
	if cleanParent != "" {
		cleanNew = cleanParent + "/" + baseName
	}
	if cleanNew == cleanOld {
		return PathChangeResult{}, fmt.Errorf("source and destination are the same")
	}
	return s.renameValidated(cleanOld, cleanNew)
}

func (s *Service) Rename(oldPath, newPath string) (map[string]string, error) {
	cleanOld, _, err := s.sanitizePath(oldPath)
	if err != nil {
		return nil, err
	}
	cleanNew, _, err := s.sanitizePath(newPath)
	if err != nil {
		return nil, err
	}
	if isAnyTrashPath(cleanOld) || isAnyTrashPath(cleanNew) {
		return nil, trashManagedOnlyError()
	}
	result, err := s.renameValidated(cleanOld, cleanNew)
	if err != nil {
		return nil, err
	}
	return map[string]string{"old_path": result.OldPath, "new_path": result.NewPath}, nil
}

func (s *Service) Delete(path string) (map[string]string, error) {
	cleanPath, absPath, err := s.sanitizePath(path)
	if err != nil {
		return nil, err
	}
	if isAnyTrashPath(cleanPath) {
		return nil, trashManagedOnlyError()
	}
	if err := os.RemoveAll(absPath); err != nil {
		return nil, err
	}
	if err := s.removeOrderMetadataForPath(cleanPath); err != nil {
		return nil, err
	}
	return map[string]string{"path": cleanPath}, nil
}

func (s *Service) renameValidated(cleanOld, cleanNew string) (PathChangeResult, error) {
	if isAnyTrashPath(cleanOld) || isAnyTrashPath(cleanNew) {
		return PathChangeResult{}, trashManagedOnlyError()
	}
	_, absOld, err := s.sanitizePath(cleanOld)
	if err != nil {
		return PathChangeResult{}, err
	}
	_, absNew, err := s.sanitizePath(cleanNew)
	if err != nil {
		return PathChangeResult{}, err
	}
	if _, err := os.Stat(absNew); err == nil {
		return PathChangeResult{}, fmt.Errorf("target path already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return PathChangeResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(absNew), 0o755); err != nil {
		return PathChangeResult{}, err
	}
	if err := os.Rename(absOld, absNew); err != nil {
		return PathChangeResult{}, err
	}
	if err := s.moveOrderMetadata(cleanOld, cleanNew); err != nil {
		return PathChangeResult{}, err
	}
	return PathChangeResult{
		Path:         cleanNew,
		OldPath:      cleanOld,
		NewPath:      cleanNew,
		ChangedPaths: []string{cleanOld, cleanNew},
	}, nil
}

func (s *Service) createFileVersion(ctx context.Context, path string, content []byte, source string) (string, error) {
	versionID := fmt.Sprintf("fv_%d", time.Now().UTC().UnixNano())
	_, err := s.db.ExecContext(ctx, `
INSERT INTO file_versions (id, user_id, path, content_text, size_bytes, content_hash, source, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, versionID, s.userID, path, string(content), len(content), hashBytes(content), source, time.Now().UTC())
	if err != nil {
		return "", err
	}
	return versionID, nil
}

func writeAtomic(target string, content []byte) error {
	tempFile, err := os.CreateTemp(filepath.Dir(target), ".tmp-*")
	if err != nil {
		return err
	}
	tempName := tempFile.Name()
	defer os.Remove(tempName)

	if _, err := tempFile.Write(content); err != nil {
		tempFile.Close()
		return err
	}
	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, target)
}

func hashBytes(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func isTextPath(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	textExtensions := map[string]bool{
		"txt": true, "md": true, "markdown": true, "json": true, "js": true, "jsx": true, "ts": true, "tsx": true,
		"css": true, "html": true, "go": true, "py": true, "yaml": true, "yml": true, "toml": true,
		"env": true, "gitignore": true, "xml": true, "csv": true, "log": true, "sh": true,
	}
	return name == ".env" || name == ".gitignore" || textExtensions[extension]
}
