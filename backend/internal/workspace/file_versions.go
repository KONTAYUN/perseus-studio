package workspace

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Service) ListFileVersions(ctx context.Context, path string, limit int) ([]FileVersion, error) {
	cleanPath, _, err := s.sanitizePath(path)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}

	current, currentErr := s.ReadFile(cleanPath)
	if currentErr != nil && !IsNotExist(currentErr) {
		return nil, currentErr
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT id, path, size_bytes, content_hash, source, created_at
FROM file_versions
WHERE user_id = ? AND path = ?
ORDER BY created_at DESC
LIMIT ?
`, s.userID, cleanPath, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []FileVersion
	for rows.Next() {
		var item FileVersion
		if err := rows.Scan(&item.ID, &item.Path, &item.Size, &item.Hash, &item.Source, &item.CreatedAt); err != nil {
			return nil, err
		}
		versions = append(versions, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if currentErr == nil {
		versions = append([]FileVersion{{
			ID:        "current",
			Path:      cleanPath,
			Size:      current.Size,
			Hash:      current.Hash,
			Source:    "current",
			CreatedAt: current.ModifiedAt,
			IsCurrent: true,
		}}, versions...)
	}
	return versions, nil
}

func (s *Service) RestoreFileVersion(ctx context.Context, path, versionID string) (FileWriteResult, error) {
	cleanPath, absPath, err := s.sanitizePath(path)
	if err != nil {
		return FileWriteResult{}, err
	}
	versionID = strings.TrimSpace(versionID)
	if versionID == "" || versionID == "current" {
		return FileWriteResult{}, ErrInvalidPath
	}

	var versionPath string
	var content sql.NullString
	err = s.db.QueryRowContext(ctx, `
SELECT path, content_text
FROM file_versions
WHERE user_id = ? AND id = ?
`, s.userID, versionID).Scan(&versionPath, &content)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return FileWriteResult{}, fs.ErrNotExist
		}
		return FileWriteResult{}, err
	}
	if versionPath != cleanPath {
		return FileWriteResult{}, ErrInvalidPath
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return FileWriteResult{}, err
	}
	return s.WriteFile(ctx, cleanPath, content.String, "restore_version")
}

func (s *Service) ReadFileVersion(ctx context.Context, path, versionID string) (FileReadResult, error) {
	cleanPath, _, err := s.sanitizePath(path)
	if err != nil {
		return FileReadResult{}, err
	}
	versionID = strings.TrimSpace(versionID)
	if versionID == "" || versionID == "current" {
		return s.ReadFile(cleanPath)
	}

	var (
		versionPath string
		content     sql.NullString
		size        int64
		hash        string
		createdAt   time.Time
	)
	err = s.db.QueryRowContext(ctx, `
SELECT path, content_text, size_bytes, content_hash, created_at
FROM file_versions
WHERE user_id = ? AND id = ?
`, s.userID, versionID).Scan(&versionPath, &content, &size, &hash, &createdAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return FileReadResult{}, fs.ErrNotExist
		}
		return FileReadResult{}, err
	}
	if versionPath != cleanPath {
		return FileReadResult{}, ErrInvalidPath
	}
	return FileReadResult{
		Path:       cleanPath,
		Content:    content.String,
		Size:       size,
		Hash:       hash,
		ModifiedAt: createdAt.UTC(),
	}, nil
}

func (s *Service) DeleteFileVersion(ctx context.Context, path, versionID string) error {
	cleanPath, _, err := s.sanitizePath(path)
	if err != nil {
		return err
	}
	versionID = strings.TrimSpace(versionID)
	if versionID == "" || versionID == "current" {
		return ErrInvalidPath
	}

	result, err := s.db.ExecContext(ctx, `
DELETE FROM file_versions
WHERE user_id = ? AND path = ? AND id = ?
`, s.userID, cleanPath, versionID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fs.ErrNotExist
	}
	return nil
}

func (s *Service) DeleteAllFileVersions(ctx context.Context, path string) error {
	cleanPath, _, err := s.sanitizePath(path)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
DELETE FROM file_versions
WHERE user_id = ? AND path = ?
`, s.userID, cleanPath)
	return err
}

func (s *Service) rewriteMovedHTMLNoteVersions(oldNotePath, newNotePath string) error {
	if !isHTMLNotePath(oldNotePath) || !isHTMLNotePath(newNotePath) {
		return nil
	}

	rows, err := s.db.Query(`
SELECT id, content_text
FROM file_versions
WHERE user_id = ? AND path = ?
`, s.userID, oldNotePath)
	if err != nil {
		return err
	}
	defer rows.Close()

	type versionUpdate struct {
		id      string
		content string
	}
	var updates []versionUpdate
	for rows.Next() {
		var (
			id      string
			content sql.NullString
		)
		if err := rows.Scan(&id, &content); err != nil {
			return err
		}
		if !content.Valid {
			continue
		}
		next := rewriteMovedHTMLNoteAssetRefs(content.String, oldNotePath, newNotePath)
		if next != content.String {
			updates = append(updates, versionUpdate{id: id, content: next})
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(updates) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
UPDATE file_versions
SET content_text = ?, size_bytes = ?, content_hash = ?
WHERE user_id = ? AND id = ?
`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, update := range updates {
		content := []byte(update.content)
		if _, err := stmt.Exec(update.content, len(content), hashBytes(content), s.userID, update.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
