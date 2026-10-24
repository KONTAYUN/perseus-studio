package workspace

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// Collection is conservative: every textual mention, including links, HTML,
// snapshots and trash, retains an asset. Incomplete scans never delete files.
// The seven-day persisted lease also protects uploads canceled before insertion.
func (s *Service) CollectDocumentImages(ctx context.Context, now time.Time) (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	candidates := map[string]string{}
	for _, dir := range []string{DocumentImageDir, "图片"} {
		_, abs, err := s.sanitizePath(dir)
		if err != nil {
			return 0, err
		}
		entries, err := os.ReadDir(abs)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return 0, err
		}
		for _, entry := range entries {
			path := dir + "/" + entry.Name()
			if token := documentImageToken(path); token != "" && entry.Type().IsRegular() {
				candidates[path] = token
			}
		}
	}
	if len(candidates) == 0 {
		return 0, nil
	}
	referenced := map[string]bool{}
	budget := int64(128 << 20)
	retain := func(data []byte) error {
		budget -= int64(len(data))
		if budget < 0 {
			return fmt.Errorf("image reference scan exceeds 128 MiB; collection skipped")
		}
		if !utf8.Valid(data) {
			return nil
		}
		text := strings.ToLower(string(data))
		if decoded, err := url.PathUnescape(text); err == nil {
			text = strings.ToLower(decoded)
		}
		for path, token := range candidates {
			if strings.Contains(text, token) {
				referenced[path] = true
			}
		}
		return nil
	}
	err := filepath.WalkDir(s.rootAbs, func(abs string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if abs == s.rootAbs {
			return nil
		}
		rel, err := filepath.Rel(s.rootAbs, abs)
		if err != nil {
			return err
		}
		path := filepath.ToSlash(rel)
		// Include 回收站 and .trash: restoring a document must restore its images.
		if path == DocumentImageDir && entry.IsDir() {
			return filepath.SkipDir
		}
		if entry.IsDir() || isDocumentImageFile(path) {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("linked workspace entry; collection skipped")
		}
		if _, _, err := s.sanitizePath(path); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular workspace entry; collection skipped")
		}
		if info.Size() > budget {
			return fmt.Errorf("image reference scan limit; collection skipped")
		}
		data, err := readFileBounded(abs, budget)
		if err != nil {
			return err
		}
		return retain(data)
	})
	if err != nil {
		return 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT COALESCE(content_text,'') FROM file_versions WHERE user_id=?
UNION ALL SELECT content FROM document_repair_candidates WHERE user_id=?
UNION ALL SELECT content FROM document_chunk_drafts WHERE user_id=?
UNION ALL SELECT plan_json FROM workspace_patch_journals WHERE user_id=? AND state='pending'`, s.userID, s.userID, s.userID, s.userID)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			rows.Close()
			return 0, err
		}
		if err := retain([]byte(content)); err != nil {
			rows.Close()
			return 0, err
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	removed := 0
	cutoff := now.Add(-DocumentImageGrace).Unix()
	for path := range candidates {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		if _, err := s.db.ExecContext(ctx, `INSERT INTO document_image_leases(user_id,path,touched_at,orphan_since) VALUES(?,?,?,?) ON CONFLICT(user_id,path) DO NOTHING`, s.userID, path, now.Unix(), now.Unix()); err != nil {
			return removed, err
		}
		if referenced[path] {
			if _, err := s.db.ExecContext(ctx, `UPDATE document_image_leases SET orphan_since=? WHERE user_id=? AND path=?`, now.Unix(), s.userID, path); err != nil {
				return removed, err
			}
			continue
		}
		var touched, orphan int64
		if err := s.db.QueryRowContext(ctx, `SELECT touched_at,orphan_since FROM document_image_leases WHERE user_id=? AND path=?`, s.userID, path).Scan(&touched, &orphan); err != nil {
			return removed, err
		}
		if touched > cutoff || orphan > cutoff {
			continue
		}
		_, abs, err := s.sanitizePath(path)
		if err != nil {
			return removed, err
		}
		if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
			return removed, err
		}
		if _, err := s.db.ExecContext(ctx, `DELETE FROM document_image_leases WHERE user_id=? AND path=?`, s.userID, path); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}
