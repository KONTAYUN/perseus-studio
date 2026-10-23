package workspace

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func sanitizeLeafName(value string) (string, error) {
	if filepath.IsAbs(value) {
		return "", ErrInvalidPath
	}
	clean := filepath.ToSlash(filepath.Clean(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")))
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
	clean, abs, err := s.sanitizeOptionalPath(path)
	if err == nil && clean == "" {
		return "", "", ErrInvalidPath
	}
	return clean, abs, err
}

// NormalizePath validates a workspace path before a multi-file operation starts.
func (s *Service) NormalizePath(path string) (string, error) {
	clean, _, err := s.sanitizePath(path)
	return clean, err
}

func (s *Service) sanitizeOptionalPath(path string) (string, string, error) {
	// Colons include Windows drive-relative paths and NTFS alternate streams.
	// Use one portable path contract on every host.
	if strings.Contains(path, ":") || strings.ContainsRune(path, '\x00') {
		return "", "", ErrInvalidPath
	}
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
	// Lexical containment alone does not stop links/junctions from reaching
	// another user's files. Reject linked components, including existing parents
	// of paths that are about to be created.
	probe := s.rootAbs
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		probe = filepath.Join(probe, part)
		info, err := os.Lstat(probe)
		if errors.Is(err, fs.ErrNotExist) {
			break
		}
		if err != nil {
			return "", "", err
		}
		if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return "", "", ErrPathEscape
		}
	}
	return filepath.ToSlash(clean), absPath, nil
}

func IsPathError(err error) bool {
	return errors.Is(err, ErrInvalidPath) || errors.Is(err, ErrPathEscape) || errors.Is(err, ErrTrashManagedOnly) || errors.Is(err, ErrMoveIntoSelf)
}

func IsNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}
