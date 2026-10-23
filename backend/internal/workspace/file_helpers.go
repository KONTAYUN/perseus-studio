package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	"webide-backend/internal/idgen"
)

func (s *Service) createFileVersion(ctx context.Context, path string, content []byte, source string) (string, error) {
	versionID := idgen.New("fv")
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
	mode := os.FileMode(0o600)
	if info, err := os.Stat(target); err == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}
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
	if err := tempFile.Chmod(mode); err != nil {
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

func countTextLines(text string) int {
	if text == "" {
		return 1
	}
	return strings.Count(text, "\n") + 1
}

func sliceTextLines(text string, startLine, endLine int) (string, int, int) {
	lines := strings.SplitAfter(text, "\n")
	if len(lines) == 0 {
		return "", 1, 1
	}
	if startLine <= 0 {
		startLine = 1
	}
	if endLine <= 0 || endLine > len(lines) {
		endLine = len(lines)
	}
	if startLine > len(lines) {
		startLine = len(lines)
	}
	if endLine < startLine {
		endLine = startLine
	}
	return strings.Join(lines[startLine-1:endLine], ""), startLine, endLine
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

func IsSupportedTextPath(path string) bool {
	return isTextPath(path)
}
