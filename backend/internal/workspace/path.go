package workspace

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
)

func sanitizeLeafName(value string) (string, error) {
	if filepath.IsAbs(value) {
		return "", ErrInvalidPath
	}
	clean := filepath.Clean(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/"))
	if clean == "" || clean == "." || clean == ".." {
		return "", ErrInvalidPath
	}
	if strings.Contains(clean, "/") || strings.HasPrefix(clean, "../") || clean == ".." {
		return "", ErrPathEscape
	}
	return clean, nil
}

func getParentPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) <= 1 {
		return ""
	}
	return strings.Join(parts[:len(parts)-1], "/")
}

func (s *Service) sanitizePath(path string) (string, string, error) {
	if path == "" {
		return "", "", ErrInvalidPath
	}
	return s.sanitizeOptionalPath(path)
}

func (s *Service) sanitizeOptionalPath(path string) (string, string, error) {
	if filepath.IsAbs(path) {
		return "", "", ErrInvalidPath
	}
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "\\") {
		return "", "", ErrInvalidPath
	}

	clean := filepath.Clean(strings.ReplaceAll(path, "\\", "/"))
	if clean == "." || clean == "" {
		return "", s.rootAbs, nil
	}
	if strings.HasPrefix(clean, "../") || clean == ".." {
		return "", "", ErrPathEscape
	}

	absPath := filepath.Clean(filepath.Join(s.rootAbs, filepath.FromSlash(clean)))
	rel, err := filepath.Rel(s.rootAbs, absPath)
	if err != nil {
		return "", "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", ErrPathEscape
	}
	return filepath.ToSlash(clean), absPath, nil
}

func IsPathError(err error) bool {
	return errors.Is(err, ErrInvalidPath) || errors.Is(err, ErrPathEscape) || errors.Is(err, ErrTrashManagedOnly) || errors.Is(err, ErrMoveIntoSelf)
}

func IsNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, ErrPathNotFound)
}
