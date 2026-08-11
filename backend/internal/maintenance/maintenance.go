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

	_ "modernc.org/sqlite"
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
	Connectivity  string               `json:"connectivity"`
	Schema        string               `json:"schema"`
	IntegrityMode string               `json:"integrity_mode"`
	Integrity     string               `json:"integrity"`
	MissingTables []string             `json:"missing_tables,omitempty"`
	Tables        map[string]TableStat `json:"tables"`
	Consistency   EventConsistency     `json:"event_consistency"`
}

type EventConsistency struct {
	SequenceGapRuns          int64 `json:"sequence_gap_runs"`
	HeadMismatchRuns         int64 `json:"head_mismatch_runs"`
	FinalBarrierMismatchRuns int64 `json:"final_barrier_mismatch_runs"`
	DuplicateMutationIDs     int64 `json:"duplicate_mutation_ids"`
}

type InspectOptions struct {
	FullIntegrity bool
}

type BackupReport struct {
	SourcePath   string    `json:"source_path"`
	BackupPath   string    `json:"backup_path"`
	ManifestPath string    `json:"manifest_path"`
	SourceBytes  int64     `json:"source_bytes"`
	BackupBytes  int64     `json:"backup_bytes"`
	SHA256       string    `json:"sha256"`
	Integrity    string    `json:"integrity"`
	CreatedAt    time.Time `json:"created_at"`
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
	return InspectWithOptions(ctx, db, dbPath, InspectOptions{FullIntegrity: true})
}

func InspectWithOptions(ctx context.Context, db *sql.DB, dbPath string, options InspectOptions) (InspectReport, error) {
	report := InspectReport{
		DatabasePath: dbPath, Tables: map[string]TableStat{},
		Connectivity: "ok", Schema: "ok", IntegrityMode: "not_run", Integrity: "not_run",
	}
	if err := db.PingContext(ctx); err != nil {
		return InspectReport{}, err
	}
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
	for _, table := range requiredTables {
		exists, err := tableExists(ctx, db, table)
		if err != nil {
			return InspectReport{}, err
		}
		if !exists {
			report.MissingTables = append(report.MissingTables, table)
		}
	}
	if len(report.MissingTables) > 0 {
		report.Schema = "missing_required_tables"
		return report, fmt.Errorf("required database tables are missing: %s", strings.Join(report.MissingTables, ", "))
	}
	if !options.FullIntegrity {
		return report, nil
	}
	report.IntegrityMode = "full"
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&report.Integrity); err != nil {
		return InspectReport{}, err
	}
	consistencyQueries := map[string]*int64{
		`SELECT COUNT(*) FROM (
			SELECT run_id
			FROM agent_events
			GROUP BY run_id
			HAVING MIN(seq) <> 1 OR MAX(seq) <> COUNT(*) OR COUNT(DISTINCT seq) <> COUNT(*)
		)`: &report.Consistency.SequenceGapRuns,
		`SELECT COUNT(*)
		 FROM agent_runs r
		 WHERE r.event_head_seq <> COALESCE((SELECT MAX(e.seq) FROM agent_events e WHERE e.run_id = r.id), 0)`: &report.Consistency.HeadMismatchRuns,
		`SELECT COUNT(*)
		 FROM agent_runs r
		 WHERE (
			r.status IN ('succeeded','failed','canceled','delegated','waiting_for_user')
			AND (r.final_seq <= 0 OR r.final_seq <> r.event_head_seq OR NOT EXISTS (
				SELECT 1 FROM agent_events e WHERE e.run_id = r.id AND e.seq = r.final_seq
			))
		 ) OR (
			r.status NOT IN ('succeeded','failed','canceled','delegated','waiting_for_user')
			AND r.final_seq <> 0
		 )`: &report.Consistency.FinalBarrierMismatchRuns,
		`SELECT COUNT(*) FROM (
			SELECT run_id, json_extract(payload_json, '$.mutation_id') AS mutation_id
			FROM agent_events
			WHERE type = 'workspace_file_changed'
			  AND json_valid(payload_json)
			  AND COALESCE(json_extract(payload_json, '$.mutation_id'), '') <> ''
			GROUP BY run_id, mutation_id
			HAVING COUNT(*) > 1
		)`: &report.Consistency.DuplicateMutationIDs,
	}
	for query, target := range consistencyQueries {
		if err := db.QueryRowContext(ctx, query).Scan(target); err != nil {
			return InspectReport{}, err
		}
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

var requiredTables = []string{
	"users", "user_sessions", "conversations", "messages", "agent_runs",
	"agent_events", "agent_turn_items", "tool_calls", "file_versions",
}

// Backup creates a consistent offline SQLite backup using SQLite itself. It is
// intentionally separate from raw file copying so a WAL/journal cannot be
// captured halfway through a transaction.
func Backup(ctx context.Context, db *sql.DB, sourcePath, outputPath string) (BackupReport, error) {
	sourcePath = filepath.Clean(sourcePath)
	outputPath = filepath.Clean(outputPath)
	if sourcePath == outputPath {
		return BackupReport{}, fmt.Errorf("backup output must differ from source")
	}
	if strings.TrimSpace(outputPath) == "" {
		return BackupReport{}, fmt.Errorf("backup output is required")
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return BackupReport{}, err
	}
	if err := requireNewOutput(outputPath, outputPath+".manifest.json"); err != nil {
		return BackupReport{}, err
	}
	var integrity string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return BackupReport{}, err
	}
	if integrity != "ok" {
		return BackupReport{}, fmt.Errorf("integrity check failed before backup: %s", integrity)
	}
	if _, err := db.ExecContext(ctx, "VACUUM INTO "+sqliteStringLiteral(outputPath)); err != nil {
		_ = os.Remove(outputPath)
		return BackupReport{}, err
	}
	checksum, err := fileSHA256(outputPath)
	if err != nil {
		return BackupReport{}, err
	}
	backupDB, err := sql.Open("sqlite", "file:"+filepath.ToSlash(outputPath)+"?mode=ro")
	if err != nil {
		return BackupReport{}, err
	}
	defer backupDB.Close()
	var backupIntegrity string
	if err := backupDB.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&backupIntegrity); err != nil {
		return BackupReport{}, err
	}
	if backupIntegrity != "ok" {
		return BackupReport{}, fmt.Errorf("integrity check failed for backup: %s", backupIntegrity)
	}
	sourceBytes, _ := fileSize(sourcePath)
	backupBytes, _ := fileSize(outputPath)
	report := BackupReport{
		SourcePath: sourcePath, BackupPath: outputPath,
		ManifestPath: outputPath + ".manifest.json",
		SourceBytes:  sourceBytes, BackupBytes: backupBytes,
		SHA256: checksum, Integrity: backupIntegrity, CreatedAt: time.Now().UTC(),
	}
	if err := writeJSONManifest(report.ManifestPath, report); err != nil {
		return BackupReport{}, err
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
	if err := requireNewOutput(options.Output, options.Output+".manifest.json", options.Output+".tmp"); err != nil {
		return ArchiveManifest{}, err
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

func requireNewOutput(paths ...string) error {
	for _, path := range paths {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("maintenance output already exists: %s", path)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
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
	counts["diagnostic_logs"] = 0
	hasTasks, err := tableExists(ctx, db, "tasks")
	if err != nil {
		return nil, err
	}
	if hasTasks {
		var diagnosticCount int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM diagnostic_logs d JOIN tasks t ON t.id=d.task_id WHERE d.task_id <> '' AND t.status IN ('done','failed','canceled') AND t.finished_at < ?`, before.UTC()).Scan(&diagnosticCount); err != nil {
			return nil, err
		}
		counts["diagnostic_logs"] = diagnosticCount
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
	hasTasks, err := tableExists(ctx, db, "tasks")
	if err != nil {
		return err
	}
	if hasTasks {
		rows, err = db.QueryContext(ctx, `SELECT d.id,d.user_id,d.conversation_id,d.message_id,d.task_id,d.stage,d.level,d.http_status,d.error_message,d.response_snippet,d.request_summary,d.created_at FROM diagnostic_logs d JOIN tasks t ON t.id=d.task_id WHERE d.task_id <> '' AND t.status IN ('done','failed','canceled') AND t.finished_at < ? ORDER BY d.created_at,d.id`, before.UTC())
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
	hasTasks, err := tableExists(ctx, db, "tasks")
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	refRaw, _ := json.Marshal(map[string]any{"archived": true, "archive": archiveName, "sha256": checksum})
	if _, err := tx.ExecContext(ctx, `UPDATE agent_events SET payload_json=?, archived_at=? WHERE id IN (SELECT e.id FROM agent_events e JOIN agent_runs r ON r.id=e.run_id WHERE r.status IN ('succeeded','failed','canceled','delegated') AND r.completed_at < ? AND e.archived_at IS NULL)`, string(refRaw), time.Now().UTC(), before.UTC()); err != nil {
		return err
	}
	if hasTasks {
		if _, err := tx.ExecContext(ctx, `DELETE FROM diagnostic_logs WHERE id IN (SELECT d.id FROM diagnostic_logs d JOIN tasks t ON t.id=d.task_id WHERE d.task_id <> '' AND t.status IN ('done','failed','canceled') AND t.finished_at < ?)`, before.UTC()); err != nil {
			return err
		}
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

func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func sqliteStringLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func writeJSONManifest(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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
