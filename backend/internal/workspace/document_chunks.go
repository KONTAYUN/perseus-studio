package workspace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"webide-backend/internal/document"
	"webide-backend/internal/store"
)

type DocumentChunk struct {
	Path, BaseHash, Content, Hash, Mode, State string
	Index                                      int
}

func (s *Service) migrateDocumentChunks(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS document_chunk_drafts(user_id TEXT NOT NULL,run_id TEXT NOT NULL,path TEXT NOT NULL,base_hash TEXT NOT NULL,content TEXT NOT NULL,content_hash TEXT NOT NULL,mode TEXT NOT NULL,chunk_index INTEGER NOT NULL,last_chunk_hash TEXT NOT NULL,PRIMARY KEY(user_id,run_id,path))`)
	if err != nil {
		return err
	}
	if err = store.AddColumnIfNotExists(ctx, s.db, "document_chunk_drafts", "state", "TEXT NOT NULL DEFAULT 'staged'"); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS document_chunk_receipts(user_id TEXT NOT NULL,run_id TEXT NOT NULL,path TEXT NOT NULL,chunk_index INTEGER NOT NULL,content_hash TEXT NOT NULL,byte_length INTEGER NOT NULL,PRIMARY KEY(user_id,run_id,path,chunk_index))`)
	return err
}
func (s *Service) readDocumentChunk(ctx context.Context, run, path string) (DocumentChunk, string, error) {
	var c DocumentChunk
	var last string
	err := s.db.QueryRowContext(ctx, `SELECT path,base_hash,content,content_hash,mode,chunk_index,last_chunk_hash,state FROM document_chunk_drafts WHERE user_id=? AND run_id=? AND path=?`, s.userID, run, path).Scan(&c.Path, &c.BaseHash, &c.Content, &c.Hash, &c.Mode, &c.Index, &last, &c.State)
	return c, last, err
}
func (s *Service) StageDocumentChunk(ctx context.Context, run, path, mode, body string, index int) (DocumentChunk, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	clean, _, err := s.sanitizePath(path)
	if err != nil {
		return DocumentChunk{}, err
	}
	if run == "" || !document.IsMarkdown(clean) || isAnyTrashPath(clean) || index < 1 {
		return DocumentChunk{}, fmt.Errorf("invalid document chunk scope")
	}
	c, last, err := s.readDocumentChunk(ctx, run, clean)
	if err == nil && c.Index == index && last == hashBytes([]byte(body)) {
		return c, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return c, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		if index != 1 || (mode != "create" && mode != "overwrite") {
			return c, fmt.Errorf("first chunk requires create/overwrite and index 1")
		}
		before, readErr := s.ReadFile(clean)
		if mode == "create" && readErr == nil {
			return c, ErrPathExists
		}
		if mode == "create" && !errors.Is(readErr, os.ErrNotExist) {
			return c, readErr
		}
		if mode == "overwrite" && readErr != nil {
			return c, readErr
		}
		c = DocumentChunk{Path: clean, BaseHash: before.Hash, Mode: mode, State: "staged"}
	} else if c.State != "staged" || mode != "append" || index != c.Index+1 {
		return c, fmt.Errorf("chunk sequence mismatch")
	}
	c.Content += body
	c.Index = index
	c.Hash = hashBytes([]byte(c.Content))
	if len(c.Content) > document.MaxBytes {
		return c, fmt.Errorf("文稿超过 8 MiB 上限")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return c, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO document_chunk_drafts(user_id,run_id,path,base_hash,content,content_hash,mode,chunk_index,last_chunk_hash) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(user_id,run_id,path) DO UPDATE SET content=excluded.content,content_hash=excluded.content_hash,chunk_index=excluded.chunk_index,last_chunk_hash=excluded.last_chunk_hash`, s.userID, run, clean, c.BaseHash, c.Content, c.Hash, c.Mode, c.Index, hashBytes([]byte(body)))
	if err != nil {
		return c, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO document_chunk_receipts VALUES(?,?,?,?,?,?)`, s.userID, run, clean, c.Index, c.Hash, len(c.Content))
	if err != nil {
		return c, err
	}
	return c, tx.Commit()
}
func (s *Service) DocumentChunk(ctx context.Context, run, path string) (DocumentChunk, error) {
	c, _, err := s.readDocumentChunk(ctx, run, path)
	return c, err
}
func (s *Service) CommitDocumentChunk(ctx context.Context, run, path, hash string) (FileWriteResult, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	c, _, err := s.readDocumentChunk(ctx, run, path)
	if err != nil {
		return FileWriteResult{}, err
	}
	if hash == "" || c.Hash != hash || hashBytes([]byte(c.Content)) != hash {
		return FileWriteResult{}, fmt.Errorf("chunk candidate hash mismatch")
	}
	if c.State == "committing" || c.State == "applied" {
		head, readErr := s.ReadFile(c.Path)
		if readErr == nil && head.Hash == hash {
			_, err = s.db.ExecContext(ctx, `UPDATE document_chunk_drafts SET state='applied' WHERE user_id=? AND run_id=? AND path=?`, s.userID, run, path)
			return FileWriteResult{Path: path, Hash: hash, Size: int64(len(c.Content))}, err
		}
		if c.State == "applied" {
			return FileWriteResult{}, ErrFileConflict
		}
	}
	if c.Mode == "create" {
		if _, err := s.StatPath(c.Path); err == nil {
			return FileWriteResult{}, ErrFileConflict
		} else if !errors.Is(err, os.ErrNotExist) {
			return FileWriteResult{}, err
		}
	} else if err := s.checkFileHash(c.Path, c.BaseHash); err != nil {
		return FileWriteResult{}, err
	}
	if _, err = s.db.ExecContext(ctx, `UPDATE document_chunk_drafts SET state='committing' WHERE user_id=? AND run_id=? AND path=?`, s.userID, run, path); err != nil {
		return FileWriteResult{}, err
	}
	result, err := s.writeBytes(ctx, c.Path, []byte(c.Content), "ai_chunk_finalize")
	if err != nil {
		return result, err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE document_chunk_drafts SET state='applied' WHERE user_id=? AND run_id=? AND path=?`, s.userID, run, path)
	return result, err
}

// A later append must not invalidate the verified receipt for an earlier chunk.
func (s *Service) VerifyDocumentChunk(ctx context.Context, run, path, hash string, index int) error {
	c, err := s.DocumentChunk(ctx, run, path)
	if err != nil {
		return err
	}
	var expected string
	var length int
	if err = s.db.QueryRowContext(ctx, `SELECT content_hash,byte_length FROM document_chunk_receipts WHERE user_id=? AND run_id=? AND path=? AND chunk_index=?`, s.userID, run, path, index).Scan(&expected, &length); err != nil {
		return err
	}
	if expected != hash || length < 0 || length > len(c.Content) || hashBytes([]byte(c.Content[:length])) != hash {
		return fmt.Errorf("staged chunk receipt mismatch")
	}
	return nil
}
