package maintenance

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type TableStat struct {
	Rows         int64 `json:"rows"`
	PayloadBytes int64 `json:"payload_bytes,omitempty"`
}

type InspectReport struct {
	DatabasePath  string               `json:"database_path"`
	DatabaseBytes int64                `json:"database_bytes"`
	PageCount     int64                `json:"page_count"`
	PageSize      int64                `json:"page_size"`
	FreePages     int64                `json:"free_pages"`
	Integrity     string               `json:"integrity"`
	Tables        map[string]TableStat `json:"tables"`
}

type ArchiveManifest struct {
	Version     int            `json:"version"`
	CreatedAt   time.Time      `json:"created_at"`
	Before      time.Time      `json:"before"`
	ArchivePath string         `json:"archive_path"`
	SHA256      string         `json:"sha256"`
	Records     map[string]int `json:"records"`
	Applied     bool           `json:"applied"`
	Compaction  string         `json:"compaction"`
}

type ArchiveOptions struct {
	Before time.Time
	Output string
	Apply  bool
}

func Inspect(ctx context.Context, db *sql.DB, dbPath string) (InspectReport, error) {
	report := InspectReport{DatabasePath: dbPath, Tables: map[string]TableStat{}}
	if info, err := os.Stat(dbPath); err == nil {
		report.DatabaseBytes = info.Size()
	}
	for query, target := range map[string]*int64{
		"PRAGMA page_count":     &report.PageCount,
		"PRAGMA page_size":      &report.PageSize,
		"PRAGMA freelist_count": &report.FreePages,
	} {
		if err := db.QueryRowContext(ctx, query).Scan(target); err != nil {
			return InspectReport{}, err
		}
	}
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&report.Integrity); err != nil {
		return InspectReport{}, err
	}
	queries := map[string]string{
		"agent_runs":          `SELECT COUNT(*), 0 FROM agent_runs`,
		"agent_events":        `SELECT COUNT(*), COALESCE(SUM(LENGTH(payload_json)), 0) FROM agent_events`,
		"agent_edit_previews": `SELECT COUNT(*), COALESCE(SUM(LENGTH(snapshot_json)), 0) FROM agent_edit_previews`,
		"diagnostic_logs":     `SELECT COUNT(*), COALESCE(SUM(LENGTH(response_snippet) + LENGTH(request_summary) + LENGTH(error_message)), 0) FROM diagnostic_logs`,
		"agent_turn_items":    `SELECT COUNT(*), COALESCE(SUM(LENGTH(payload_json)), 0) FROM agent_turn_items`,
	}
	for table, query := range queries {
		exists, err := tableExists(ctx, db, table)
		if err != nil {
			return InspectReport{}, err
		}
		if !exists {
			continue
		}
		var stat TableStat
		if err := db.QueryRowContext(ctx, query).Scan(&stat.Rows, &stat.PayloadBytes); err != nil {
			return InspectReport{}, err
		}
		report.Tables[table] = stat
	}
	return report, nil
}

func Archive(ctx context.Context, db *sql.DB, options ArchiveOptions) (ArchiveManifest, error) {
	if options.Before.IsZero() {
		return ArchiveManifest{}, fmt.Errorf("before is required")
	}
	if strings.TrimSpace(options.Output) == "" {
		return ArchiveManifest{}, fmt.Errorf("output is required")
	}
	manifest := ArchiveManifest{
		Version: 1, CreatedAt: time.Now().UTC(), Before: options.Before.UTC(),
		ArchivePath: options.Output, Applied: false,
		Compaction: "agent event envelopes retained; archived payloads replaced by structured references",
		Records:    map[string]int{},
	}
	counts, err := archiveCandidateCounts(ctx, db, options.Before)
	if err != nil {
		return ArchiveManifest{}, err
	}
	manifest.Records = counts
	if !options.Apply {
		return manifest, nil
	}
	if err := os.MkdirAll(filepath.Dir(options.Output), 0o755); err != nil {
		return ArchiveManifest{}, err
	}
	tmp := options.Output + ".tmp"
	if err := writeArchive(ctx, db, options.Before, tmp); err != nil {
		_ = os.Remove(tmp)
		return ArchiveManifest{}, err
	}
	checksum, err := fileSHA256(tmp)
	if err != nil {
		_ = os.Remove(tmp)
		return ArchiveManifest{}, err
	}
	manifest.SHA256 = checksum
	if err := os.Rename(tmp, options.Output); err != nil {
		_ = os.Remove(tmp)
		return ArchiveManifest{}, err
	}
	manifestPath := options.Output + ".manifest.json"
	if err := writeManifest(manifestPath, manifest); err != nil {
		return ArchiveManifest{}, err
	}
	if err := compactArchivedData(ctx, db, options.Before, filepath.Base(options.Output), checksum); err != nil {
		return ArchiveManifest{}, err
	}
	manifest.Applied = true
	if err := writeManifest(manifestPath, manifest); err != nil {
		return ArchiveManifest{}, err
	}
	return manifest, nil
}

func Vacuum(ctx context.Context, db *sql.DB) error {
	var integrity string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return fmt.Errorf("integrity check failed before vacuum: %s", integrity)
	}
	if _, err := db.ExecContext(ctx, "VACUUM"); err != nil {
		return err
	}
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return fmt.Errorf("integrity check failed after vacuum: %s", integrity)
	}
	return nil
}

func archiveCandidateCounts(ctx context.Context, db *sql.DB, before time.Time) (map[string]int, error) {
	queries := map[string]string{
		"agent_events":        `SELECT COUNT(*) FROM agent_events e JOIN agent_runs r ON r.id=e.run_id WHERE r.status IN ('succeeded','failed','canceled','delegated') AND r.completed_at < ? AND e.archived_at IS NULL`,
		"diagnostic_logs":     `SELECT COUNT(*) FROM diagnostic_logs WHERE created_at < ?`,
		"agent_edit_previews": `SELECT COUNT(*) FROM agent_edit_previews p JOIN agent_runs r ON r.id=p.run_id WHERE r.status IN ('succeeded','failed','canceled','delegated') AND r.completed_at < ?`,
	}
	counts := map[string]int{}
	for name, query := range queries {
		var count int
		if err := db.QueryRowContext(ctx, query, before.UTC()).Scan(&count); err != nil {
			return nil, err
		}
		counts[name] = count
	}
	return counts, nil
}

func tableExists(ctx context.Context, db *sql.DB, table string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count)
	return count == 1, err
}

func writeManifest(path string, manifest ArchiveManifest) error {
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeArchive(ctx context.Context, db *sql.DB, before time.Time, path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	gz := gzip.NewWriter(file)
	writer := bufio.NewWriterSize(gz, 128*1024)
	encode := func(value any) error {
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		_, err = writer.Write(append(raw, '\n'))
		return err
	}
	if err := encode(map[string]any{"kind": "archive_header", "version": 1, "before": before.UTC()}); err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, `SELECT e.id,e.run_id,e.seq,e.type,e.level,e.payload_json,e.created_at FROM agent_events e JOIN agent_runs r ON r.id=e.run_id WHERE r.status IN ('succeeded','failed','canceled','delegated') AND r.completed_at < ? AND e.archived_at IS NULL ORDER BY e.run_id,e.seq`, before.UTC())
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, runID, eventType, level, payload string
		var seq int64
		var created time.Time
		if err := rows.Scan(&id, &runID, &seq, &eventType, &level, &payload, &created); err != nil {
			rows.Close()
			return err
		}
		if err := encode(map[string]any{"kind": "agent_event", "id": id, "run_id": runID, "seq": seq, "type": eventType, "level": level, "payload_json": json.RawMessage(payload), "created_at": created}); err != nil {
			rows.Close()
			return err
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	rows, err = db.QueryContext(ctx, `SELECT id,user_id,conversation_id,message_id,task_id,stage,level,http_status,error_message,response_snippet,request_summary,created_at FROM diagnostic_logs WHERE created_at < ? ORDER BY created_at,id`, before.UTC())
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, userID, conversationID, messageID, taskID, stage, level, errorMessage, snippet, summary string
		var status int
		var created time.Time
		if err := rows.Scan(&id, &userID, &conversationID, &messageID, &taskID, &stage, &level, &status, &errorMessage, &snippet, &summary, &created); err != nil {
			rows.Close()
			return err
		}
		if err := encode(map[string]any{"kind": "diagnostic_log", "id": id, "user_id": userID, "conversation_id": conversationID, "message_id": messageID, "task_id": taskID, "stage": stage, "level": level, "http_status": status, "error_message": errorMessage, "response_snippet": snippet, "request_summary": summary, "created_at": created}); err != nil {
			rows.Close()
			return err
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	return file.Sync()
}

func compactArchivedData(ctx context.Context, db *sql.DB, before time.Time, archiveName, checksum string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	refRaw, _ := json.Marshal(map[string]any{"archived": true, "archive": archiveName, "sha256": checksum})
	if _, err := tx.ExecContext(ctx, `UPDATE agent_events SET payload_json=?, archived_at=? WHERE id IN (SELECT e.id FROM agent_events e JOIN agent_runs r ON r.id=e.run_id WHERE r.status IN ('succeeded','failed','canceled','delegated') AND r.completed_at < ? AND e.archived_at IS NULL)`, string(refRaw), time.Now().UTC(), before.UTC()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM diagnostic_logs WHERE created_at < ?`, before.UTC()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM agent_edit_previews WHERE preview_id IN (SELECT p.preview_id FROM agent_edit_previews p JOIN agent_runs r ON r.id=p.run_id WHERE r.status IN ('succeeded','failed','canceled','delegated') AND r.completed_at < ?)`, before.UTC()); err != nil {
		return err
	}
	return tx.Commit()
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func ParseAge(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if strings.HasSuffix(value, "d") {
		days, err := strconv.ParseFloat(strings.TrimSuffix(value, "d"), 64)
		if err != nil || days <= 0 {
			return 0, fmt.Errorf("invalid age %q", value)
		}
		return time.Duration(days * float64(24*time.Hour)), nil
	}
	return time.ParseDuration(value)
}
