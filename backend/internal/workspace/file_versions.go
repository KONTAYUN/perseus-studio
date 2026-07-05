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
	path = strings.TrimSpace(path)
	var (
		cleanPath string
		err       error
	)
	if path != "" {
		cleanPath, _, err = s.sanitizePath(path)
		if err != nil {
			return nil, err
		}
	}
	if limit <= 0 || limit > maxGlobalFileVersions {
		limit = maxGlobalFileVersions
	}

	query := `
SELECT id, path, size_bytes, content_hash, source, created_at
FROM file_versions
WHERE user_id = ?
`
	args := []any{s.userID}
	if cleanPath != "" {
		query += ` AND path = ?`
		args = append(args, cleanPath)
	}
	query += `
ORDER BY created_at DESC, id DESC
LIMIT ?
`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
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
	return versions, nil
}

func (s *Service) RestoreFileVersion(ctx context.Context, path, versionID string) (FileWriteResult, error) {
	path = strings.TrimSpace(path)
	versionID = strings.TrimSpace(versionID)
	if versionID == "" || versionID == "current" {
		return FileWriteResult{}, ErrInvalidPath
	}

	var versionPath string
	var content sql.NullString
	err := s.db.QueryRowContext(ctx, `
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
	targetPath := versionPath
	if path != "" {
		cleanPath, _, err := s.sanitizePath(path)
		if err != nil {
			return FileWriteResult{}, err
		}
		if versionPath != cleanPath {
			return FileWriteResult{}, ErrInvalidPath
		}
		targetPath = cleanPath
	}
	cleanTargetPath, absPath, err := s.sanitizePath(targetPath)
	if err != nil {
		return FileWriteResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return FileWriteResult{}, err
	}
	return s.WriteFile(ctx, cleanTargetPath, content.String, "restore_version")
}

func (s *Service) ReadFileVersion(ctx context.Context, path, versionID string) (FileReadResult, error) {
	path = strings.TrimSpace(path)
	versionID = strings.TrimSpace(versionID)
	if versionID == "" || versionID == "current" {
		if path == "" {
			return FileReadResult{}, ErrInvalidPath
		}
		cleanPath, _, err := s.sanitizePath(path)
		if err != nil {
			return FileReadResult{}, err
		}
		return s.ReadFile(cleanPath)
	}

	var (
		versionPath string
		content     sql.NullString
		size        int64
		hash        string
		createdAt   time.Time
	)
	err := s.db.QueryRowContext(ctx, `
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
	if path != "" {
		cleanPath, _, err := s.sanitizePath(path)
		if err != nil {
			return FileReadResult{}, err
		}
		if versionPath != cleanPath {
			return FileReadResult{}, ErrInvalidPath
		}
	}
	return FileReadResult{
		Path:       versionPath,
		Content:    content.String,
		Size:       size,
		Hash:       hash,
		ModifiedAt: createdAt.UTC(),
	}, nil
}

func (s *Service) DeleteFileVersion(ctx context.Context, path, versionID string) error {
	path = strings.TrimSpace(path)
	versionID = strings.TrimSpace(versionID)
	if versionID == "" || versionID == "current" {
		return ErrInvalidPath
	}

	query := `
DELETE FROM file_versions
WHERE user_id = ? AND id = ?
`
	args := []any{s.userID, versionID}
	if path != "" {
		cleanPath, _, err := s.sanitizePath(path)
		if err != nil {
			return err
		}
		query += ` AND path = ?`
		args = append(args, cleanPath)
	}
	result, err := s.db.ExecContext(ctx, query, args...)
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
	path = strings.TrimSpace(path)
	if path == "" {
		_, err := s.db.ExecContext(ctx, `
DELETE FROM file_versions
WHERE user_id = ?
`, s.userID)
		return err
	}
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
