package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"webide-backend/internal/failure"
	"webide-backend/internal/idgen"
)

// A journal is a recoverable filesystem operation plan, not an ACID transaction
// spanning SQLite and the filesystem. Each original is durable before any write.
type PatchJournalEntry struct {
	Operation  string `json:"operation"`
	Path       string `json:"path"`
	NewPath    string `json:"new_path,omitempty"`
	Before     string `json:"before"`
	BeforeHash string `json:"before_hash"`
	AfterHash  string `json:"after_hash"`
}

func (s *Service) BeginPatchJournal(ctx context.Context, entries []PatchJournalEntry) (string, error) {
	if err := s.validatePatchJournal(entries); err != nil {
		return "", err
	}
	raw, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}
	if len(raw) > 16<<20 {
		return "", fmt.Errorf("patch recovery plan exceeds 16 MiB")
	}
	id := idgen.New("patch")
	_, err = s.db.ExecContext(ctx, `INSERT INTO workspace_patch_journals (id,user_id,state,plan_json,created_at) VALUES (?,?,'pending',?,?)`, id, s.userID, string(raw), time.Now().UTC())
	return id, err
}

func (s *Service) FinishPatchJournal(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE workspace_patch_journals SET state='completed',plan_json='[]' WHERE id=? AND user_id=?`, id, s.userID)
	return err
}

func (s *Service) LockPatchExecution() func() { s.patchMu.Lock(); return s.patchMu.Unlock }

// Recovery is conservative and idempotent. Never overwrite content not matching
// either the recorded original or the exact intended result. Trash copies are
// retained even when a deleted original must be reconstructed after a crash.
func (s *Service) RecoverPendingPatches(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id,plan_json FROM workspace_patch_journals WHERE user_id=? AND state='pending' ORDER BY created_at`, s.userID)
	if err != nil {
		return err
	}
	type pending struct{ id, raw string }
	var plans []pending
	for rows.Next() {
		var p pending
		if err = rows.Scan(&p.id, &p.raw); err != nil {
			rows.Close()
			return err
		}
		plans = append(plans, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, p := range plans {
		var entries []PatchJournalEntry
		if err = json.Unmarshal([]byte(p.raw), &entries); err != nil {
			return failure.New("workspace_patch_journal_invalid", "补丁恢复日志无法解析；已停止恢复，请核验原始数据", err)
		}
		// Validate every original before reverse recovery can mutate any file.
		if err = s.validatePatchJournal(entries); err != nil {
			return err
		}
		for i := len(entries) - 1; i >= 0; i-- {
			if err = s.recoverPatchEntry(ctx, entries[i]); err != nil {
				return failure.New("workspace_patch_recovery_conflict", fmt.Sprintf("未完成补丁 %s 恢复停止于 %s；原始内容已保留，需核验冲突: %v", p.id, entries[i].Path, err), err)
			}
		}
		if err = s.FinishPatchJournal(ctx, p.id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) validatePatchJournal(entries []PatchJournalEntry) error {
	bad := func() error {
		return failure.New("workspace_patch_journal_invalid", "补丁恢复计划或原文哈希无效；已停止恢复，请核验原始数据", nil)
	}
	if len(entries) == 0 {
		return bad()
	}
	for _, entry := range entries {
		if _, err := s.NormalizePath(entry.Path); err != nil {
			return bad()
		}
		switch entry.Operation {
		case "update", "delete":
			if hashBytes([]byte(entry.Before)) != entry.BeforeHash {
				return bad()
			}
		case "move":
			if _, err := s.NormalizePath(entry.NewPath); err != nil {
				return bad()
			}
		case "create":
		default:
			return bad()
		}
		if entry.Operation != "delete" && !validPatchJournalHash(entry.AfterHash) {
			return bad()
		}
	}
	return nil
}

func validPatchJournalHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}
