package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"webide-backend/internal/store"
)

const DocumentImageDir = ".perseus-images"
const DocumentImageGrace = 7 * 24 * time.Hour

// Old editor uploads used UUID-prefixed names in 图片/. Other user images
// remain ordinary workspace files and are never garbage collected.
var legacyDocumentImage = regexp.MustCompile(`(?i)^图片/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})-[^/]+\.(png|jpe?g|gif|webp|avif)$`)
var managedDocumentImage = regexp.MustCompile(`^\.perseus-images/([0-9a-f]{32})\.(png|jpg|gif|webp|avif)$`)

type DocumentImage struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Src  string `json:"src"`
}

func documentImageToken(path string) string {
	for _, pattern := range []*regexp.Regexp{managedDocumentImage, legacyDocumentImage} {
		if match := pattern.FindStringSubmatch(path); match != nil {
			return strings.ToLower(match[1])
		}
	}
	return ""
}

func isDocumentImageFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif":
		return true
	}
	return false
}

func (s *Service) migrateDocumentImages(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS document_image_leases(user_id TEXT NOT NULL,path TEXT NOT NULL,touched_at INTEGER NOT NULL,orphan_since INTEGER NOT NULL,PRIMARY KEY(user_id,path))`)
	if err != nil {
		return err
	}
	return store.AddColumnIfNotExists(ctx, s.db, "document_image_leases", "name", "TEXT NOT NULL DEFAULT ''")
}

func (s *Service) SaveDocumentImage(ctx context.Context, data []byte, extension, name string) (DocumentImage, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if !isDocumentImageFile("image"+extension) || extension == ".jpeg" || len(data) > 16<<20 {
		return DocumentImage{}, fmt.Errorf("invalid image upload")
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return DocumentImage{}, err
	}
	path := DocumentImageDir + "/" + hex.EncodeToString(id) + extension
	_, abs, err := s.sanitizePath(path)
	if err != nil {
		return DocumentImage{}, err
	}
	if err = os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return DocumentImage{}, err
	}
	if err = writeAtomic(abs, data); err != nil {
		return DocumentImage{}, err
	}
	if err = s.touchDocumentImage(ctx, path, time.Now()); err != nil {
		return DocumentImage{}, err
	}
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if _, err = s.db.ExecContext(ctx, `UPDATE document_image_leases SET name=? WHERE user_id=? AND path=?`, name, s.userID, path); err != nil {
		return DocumentImage{}, err
	}
	return DocumentImage{Path: path, Name: name, Src: "/" + path}, nil
}

func (s *Service) touchDocumentImage(ctx context.Context, path string, now time.Time) error {
	if documentImageToken(path) == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO document_image_leases(user_id,path,touched_at,orphan_since) VALUES(?,?,?,?) ON CONFLICT(user_id,path) DO UPDATE SET touched_at=excluded.touched_at`, s.userID, path, now.Unix(), now.Unix())
	return err
}

// Displaying an image renews its lease, including images in open drafts.
func (s *Service) ReadDocumentImage(ctx context.Context, path string) (FileReadResult, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	file, err := s.ReadFile(path)
	if err == nil {
		err = s.touchDocumentImage(ctx, file.Path, time.Now())
	}
	return file, err
}

func (s *Service) DocumentImages(ctx context.Context) ([]DocumentImage, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	items := []DocumentImage{}
	names := map[string]string{}
	rows, err := s.db.QueryContext(ctx, `SELECT path,name FROM document_image_leases WHERE user_id=?`, s.userID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var path, name string
		if err := rows.Scan(&path, &name); err != nil {
			rows.Close()
			return nil, err
		}
		names[path] = name
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	err = filepath.WalkDir(s.rootAbs, func(abs string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(s.rootAbs, abs)
		if err != nil {
			return err
		}
		path := filepath.ToSlash(rel)
		if isAnyTrashPath(path) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() || !isDocumentImageFile(path) {
			return nil
		}
		if _, _, err := s.sanitizePath(path); err != nil {
			return err
		}
		name := entry.Name()
		if match := legacyDocumentImage.FindStringSubmatch(path); match != nil {
			name = name[37:]
		}
		if original := names[path]; original != "" {
			name = original
		}
		items = append(items, DocumentImage{Path: path, Name: name, Src: "/" + path})
		if len(items) >= 500 {
			return fs.SkipAll
		}
		return nil
	})
	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	return items, err
}

func (s *Service) hideDocumentImageEntry(parent, name string) bool {
	path := name
	if parent != "" {
		path = parent + "/" + name
	}
	if path == DocumentImageDir || strings.HasPrefix(path, DocumentImageDir+"/") || documentImageToken(path) != "" {
		return true
	}
	if path != "图片" {
		return false
	}
	entries, err := os.ReadDir(filepath.Join(s.rootAbs, "图片"))
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() || documentImageToken("图片/"+entry.Name()) == "" {
			return false
		}
	}
	return true
}
