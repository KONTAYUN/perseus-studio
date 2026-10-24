package workspace

import (
	"context"
	"fmt"
	"time"
	"webide-backend/internal/document"
	"webide-backend/internal/idgen"
)

type RepairCandidate struct {
	ID         string `json:"id"`
	RunID      string `json:"run_id"`
	Path       string `json:"path"`
	BaseHash   string `json:"base_hash"`
	Hash       string `json:"hash"`
	Content    string `json:"content"`
	State      string `json:"state"`
	Equivalent bool   `json:"equivalent"`
	Attempt    int    `json:"attempt"`
}

func (s *Service) migrateDocumentRepairs(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS document_repair_candidates(id TEXT PRIMARY KEY,user_id TEXT NOT NULL,run_id TEXT NOT NULL,path TEXT NOT NULL,base_hash TEXT NOT NULL,content_hash TEXT NOT NULL,content TEXT NOT NULL,state TEXT NOT NULL,equivalent INTEGER NOT NULL,attempt INTEGER NOT NULL,created_at DATETIME NOT NULL,UNIQUE(user_id,run_id,attempt));`)
	return err
}
func (s *Service) StageDocumentRepair(ctx context.Context, runID string, action *document.Action, content string) (RepairCandidate, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	var c RepairCandidate
	if action == nil || runID == "" {
		return c, fmt.Errorf("缺少文稿修复授权")
	}
	if err := action.Validate(); err != nil {
		return c, err
	}
	if err := s.checkFileHash(action.Path, action.BaseHash); err != nil {
		return c, err
	}
	before, err := s.ReadFile(action.Path)
	if err != nil {
		return c, err
	}
	var attempts int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM document_repair_candidates WHERE user_id=? AND run_id=?`, s.userID, runID).Scan(&attempts); err != nil {
		return c, err
	}
	if attempts >= 2 {
		return c, fmt.Errorf("本次授权的两次候选已用完，请结束修复")
	}
	result, validationErr := document.ValidateRepair([]byte(before.Content), []byte(content))
	c = RepairCandidate{ID: idgen.New("repair"), RunID: runID, Path: action.Path, BaseHash: action.BaseHash, Hash: result.Hash, Content: content, State: "staged", Equivalent: document.RepairEquivalent([]byte(before.Content), []byte(content)), Attempt: attempts + 1}
	if validationErr != nil {
		c.State = "rejected"
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO document_repair_candidates VALUES(?,?,?,?,?,?,?,?,?,?,?)`, c.ID, s.userID, c.RunID, c.Path, c.BaseHash, c.Hash, c.Content, c.State, c.Equivalent, c.Attempt, time.Now().UTC())
	if err != nil {
		return c, err
	}
	return c, validationErr
}
func scanRepair(row interface{ Scan(...any) error }) (RepairCandidate, error) {
	var c RepairCandidate
	err := row.Scan(&c.ID, &c.RunID, &c.Path, &c.BaseHash, &c.Hash, &c.Content, &c.State, &c.Equivalent, &c.Attempt)
	return c, err
}

const repairColumns = `id,run_id,path,base_hash,content_hash,content,state,equivalent,attempt`

func (s *Service) DocumentRepairCandidates(ctx context.Context, path string) ([]RepairCandidate, error) {
	clean, _, err := s.sanitizePath(path)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+repairColumns+` FROM document_repair_candidates WHERE user_id=? AND path=? AND state IN ('staged','applying') ORDER BY created_at DESC LIMIT 20`, s.userID, clean)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []RepairCandidate{}
	for rows.Next() {
		c, err := scanRepair(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, rows.Err()
}
func (s *Service) DecideDocumentRepair(ctx context.Context, id, expectedHash string, accept bool) (FileWriteResult, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	c, err := scanRepair(s.db.QueryRowContext(ctx, `SELECT `+repairColumns+` FROM document_repair_candidates WHERE user_id=? AND id=?`, s.userID, id))
	if err != nil {
		return FileWriteResult{}, err
	}
	if !accept {
		if c.State != "staged" && c.State != "dismissed" {
			return FileWriteResult{}, fmt.Errorf("已授权应用的建议不能撤销确认，请检查当前版本")
		}
		_, err = s.db.ExecContext(ctx, `UPDATE document_repair_candidates SET state='dismissed' WHERE user_id=? AND id=?`, s.userID, id)
		return FileWriteResult{}, err
	}
	if expectedHash == "" || expectedHash != c.BaseHash {
		return FileWriteResult{}, ErrFileConflict
	}
	if c.State != "staged" && c.State != "applying" && c.State != "applied" {
		return FileWriteResult{}, fmt.Errorf("候选状态不允许应用")
	}
	if c.State == "applying" || c.State == "applied" {
		head, readErr := s.ReadFile(c.Path)
		if readErr != nil {
			return FileWriteResult{}, readErr
		}
		if head.Hash == c.Hash {
			_, err = s.db.ExecContext(ctx, `UPDATE document_repair_candidates SET state='applied' WHERE user_id=? AND id=?`, s.userID, id)
			return FileWriteResult{Path: c.Path, Hash: c.Hash, Size: int64(len(c.Content))}, err
		}
		if c.State == "applied" {
			return FileWriteResult{}, ErrFileConflict
		}
	}
	if err := s.checkFileHash(c.Path, expectedHash); err != nil {
		return FileWriteResult{}, err
	}
	if hashBytes([]byte(c.Content)) != c.Hash {
		return FileWriteResult{}, fmt.Errorf("候选校验失败")
	}
	// Persist authorization before crossing the filesystem boundary. A lost
	// response or post-write database failure can be reconciled by exact hash;
	// a new head never becomes implicit permission to replay an old candidate.
	if _, err = s.db.ExecContext(ctx, `UPDATE document_repair_candidates SET state='applying' WHERE user_id=? AND id=?`, s.userID, id); err != nil {
		return FileWriteResult{}, err
	}
	result, err := s.writeBytes(ctx, c.Path, []byte(c.Content), "document_repair_confirmed")
	if err != nil {
		return result, err
	}
	// Content hash makes a retry after an uncertain DB response detectable. Never
	// reapply against a new head, including after cancellation or server restart.
	_, err = s.db.ExecContext(ctx, `UPDATE document_repair_candidates SET state='applied' WHERE user_id=? AND id=?`, s.userID, id)
	return result, err
}
