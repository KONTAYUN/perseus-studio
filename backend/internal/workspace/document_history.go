package workspace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"
	"webide-backend/internal/document"
	"webide-backend/internal/idgen"
	"webide-backend/internal/store"
)

func (s *Service) migrateDocuments(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workspace_documents (id TEXT PRIMARY KEY,user_id TEXT NOT NULL,path TEXT,created_at DATETIME NOT NULL,UNIQUE(user_id,path));`)
	if err != nil {
		return err
	}
	if err = store.AddColumnIfNotExists(ctx, s.db, "file_versions", "document_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_file_versions_document ON file_versions(user_id,document_id,created_at DESC)`)
	return err
}

func (s *Service) documentIdentity(ctx context.Context, path string) (string, error) {
	if !document.IsMarkdown(path) {
		return "", nil
	}
	id := idgen.New("doc")
	_, err := s.db.ExecContext(ctx, `INSERT INTO workspace_documents(id,user_id,path,created_at) VALUES(?,?,?,?) ON CONFLICT(user_id,path) DO NOTHING`, id, s.userID, path, time.Now().UTC())
	if err != nil {
		return "", err
	}
	err = s.db.QueryRowContext(ctx, `SELECT id FROM workspace_documents WHERE user_id=? AND path=?`, s.userID, path).Scan(&id)
	return id, err
}

// Paths are metadata. Snapshot bodies and original paths are never rewritten.
func (s *Service) moveDocumentIdentity(oldPath, newPath string) error {
	n := utf8.RuneCountInString(oldPath) + 1
	_, err := s.db.Exec(`UPDATE workspace_documents SET path=?||substr(path,?) WHERE user_id=? AND (path=? OR substr(path,1,?)=?)`, newPath, n, s.userID, oldPath, n, oldPath+"/")
	return err
}
func (s *Service) retireDocumentIdentity(path string) error {
	_, err := s.db.Exec(`UPDATE workspace_documents SET path=NULL WHERE user_id=? AND (path=? OR substr(path,1,?)=?)`, s.userID, path, utf8.RuneCountInString(path)+1, path+"/")
	return err
}

type DocumentVersion struct {
	ID         string    `json:"id"`
	DocumentID string    `json:"document_id"`
	Path       string    `json:"path"`
	Hash       string    `json:"hash"`
	Size       int64     `json:"size"`
	Source     string    `json:"source"`
	CreatedAt  time.Time `json:"created_at"`
	Legacy     bool      `json:"legacy"`
	Content    *string   `json:"content,omitempty"`
}

func (s *Service) DocumentVersions(ctx context.Context, path string) ([]DocumentVersion, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	current, err := s.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !document.IsMarkdown(current.Path) {
		return nil, fmt.Errorf("仅 Markdown 文稿支持文稿历史")
	}
	id, err := s.documentIdentity(ctx, current.Path)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,document_id,path,content_hash,size_bytes,source,created_at FROM file_versions WHERE user_id=? AND (document_id=? OR (document_id='' AND path=?)) ORDER BY created_at DESC,id DESC LIMIT 100`, s.userID, id, current.Path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []DocumentVersion{}
	for rows.Next() {
		var v DocumentVersion
		if err := rows.Scan(&v.ID, &v.DocumentID, &v.Path, &v.Hash, &v.Size, &v.Source, &v.CreatedAt); err != nil {
			return nil, err
		}
		v.Legacy = v.DocumentID == ""
		items = append(items, v)
	}
	return items, rows.Err()
}

func (s *Service) readDocumentVersion(ctx context.Context, path, id string) (DocumentVersion, error) {
	current, err := s.ReadFile(path)
	if err != nil {
		return DocumentVersion{}, err
	}
	var documentID string
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM workspace_documents WHERE user_id=? AND path=?`, s.userID, current.Path).Scan(&documentID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return DocumentVersion{}, err
	}
	var v DocumentVersion
	var body string
	err = s.db.QueryRowContext(ctx, `SELECT id,document_id,path,content_hash,size_bytes,source,created_at,content_text FROM file_versions WHERE user_id=? AND id=? AND ((document_id<>'' AND document_id=?) OR (document_id='' AND path=?)) AND size_bytes<=?`, s.userID, id, documentID, current.Path, document.MaxBytes).Scan(&v.ID, &v.DocumentID, &v.Path, &v.Hash, &v.Size, &v.Source, &v.CreatedAt, &body)
	if err != nil {
		return v, err
	}
	if hashBytes([]byte(body)) != v.Hash || int64(len(body)) != v.Size {
		return v, fmt.Errorf("历史版本校验失败，原文未修改")
	}
	v.Content = &body
	v.Legacy = v.DocumentID == ""
	return v, nil
}
func (s *Service) DocumentVersion(ctx context.Context, path, id string) (DocumentVersion, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.readDocumentVersion(ctx, path, id)
}
func (s *Service) RestoreDocumentVersion(ctx context.Context, path, id, expectedHash string) (FileWriteResult, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if expectedHash == "" {
		return FileWriteResult{}, ErrFileConflict
	}
	if err := s.checkFileHash(path, expectedHash); err != nil {
		return FileWriteResult{}, err
	}
	version, err := s.readDocumentVersion(ctx, path, id)
	if err != nil {
		return FileWriteResult{}, err
	}
	return s.writeBytes(ctx, path, []byte(*version.Content), "document_restore")
}
