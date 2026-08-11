package maintenance

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"webide-backend/internal/store"
)

func TestArchivePreservesEventEnvelopeAndCompactsPayload(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "app.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.NewAgentRunStore(db).Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.NewDiagnosticStore(db).Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.NewTaskStore(db).Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	ensureRequiredInspectionSchema(t, db)
	old := time.Now().UTC().Add(-72 * time.Hour)
	insertArchiveFixture(t, db, old)

	output := filepath.Join(filepath.Dir(dbPath), "archive.jsonl.gz")
	dryRun, err := Archive(ctx, db, ArchiveOptions{Before: time.Now().UTC().Add(-24 * time.Hour), Output: output})
	if err != nil {
		t.Fatal(err)
	}
	if dryRun.Records["agent_events"] != 1 || dryRun.Applied {
		t.Fatalf("unexpected dry run: %+v", dryRun)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote archive: %v", err)
	}

	manifest, err := Archive(ctx, db, ArchiveOptions{Before: time.Now().UTC().Add(-24 * time.Hour), Output: output, Apply: true})
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Applied || manifest.SHA256 == "" {
		t.Fatalf("archive not applied: %+v", manifest)
	}
	var count, archived int
	var payload string
	if err := db.QueryRow(`SELECT COUNT(*), COUNT(archived_at), MAX(payload_json) FROM agent_events WHERE run_id='run-old'`).Scan(&count, &archived, &payload); err != nil {
		t.Fatal(err)
	}
	if count != 1 || archived != 1 || payload == `{"large":"payload"}` {
		t.Fatalf("event envelope was not compacted safely: count=%d archived=%d payload=%s", count, archived, payload)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM diagnostic_logs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("diagnostics were not archived: %d", count)
	}
	if err := Vacuum(ctx, db); err != nil {
		t.Fatal(err)
	}
	report, err := Inspect(ctx, db, dbPath)
	if err != nil || report.Integrity != "ok" {
		t.Fatalf("inspect after archive: report=%+v err=%v", report, err)
	}
}

func TestArchiveKeepsUnlinkedDiagnostics(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "app.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, migrate := range []func(context.Context) error{
		store.NewAgentRunStore(db).Migrate,
		store.NewDiagnosticStore(db).Migrate,
		store.NewTaskStore(db).Migrate,
	} {
		if err := migrate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().UTC().Add(-72 * time.Hour)
	if _, err := db.Exec(`INSERT INTO diagnostic_logs (id,user_id,stage,level,error_message,created_at) VALUES ('diag-unlinked','user','provider','error','keep',?)`, old); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(filepath.Dir(dbPath), "archive.jsonl.gz")
	manifest, err := Archive(ctx, db, ArchiveOptions{Before: time.Now().UTC().Add(-24 * time.Hour), Output: output, Apply: true})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Records["diagnostic_logs"] != 0 {
		t.Fatalf("unlinked diagnostic was selected: %+v", manifest)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM diagnostic_logs WHERE id='diag-unlinked'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("unlinked diagnostic was removed: %d", count)
	}
}

func TestBackupProducesVerifiedDatabaseAndManifest(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "app.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.NewAgentRunStore(db).Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	ensureRequiredInspectionSchema(t, db)
	if _, err := db.Exec(`CREATE TABLE backup_fixture (id INTEGER PRIMARY KEY, value TEXT); INSERT INTO backup_fixture(value) VALUES ('preserve');`); err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(filepath.Dir(dbPath), "backup", "app.db")
	report, err := Backup(ctx, db, dbPath, backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if report.Integrity != "ok" || report.SHA256 == "" || report.BackupBytes == 0 {
		t.Fatalf("invalid backup report: %+v", report)
	}
	if _, err := os.Stat(report.ManifestPath); err != nil {
		t.Fatalf("backup manifest missing: %v", err)
	}
	backupDB, err := sql.Open("sqlite", "file:"+filepath.ToSlash(backupPath)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer backupDB.Close()
	var value string
	if err := backupDB.QueryRow(`SELECT value FROM backup_fixture`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "preserve" {
		t.Fatalf("backup value changed: %q", value)
	}
}

func TestVerifyRestoreComparesProtectedDataAndRemovesTemporaryCopy(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dbPath := filepath.Join(root, "app.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ensureRequiredInspectionSchema(t, db)
	if _, err := db.Exec(`INSERT INTO users(id, username) VALUES ('user-1', 'alice'); INSERT INTO conversations(id) VALUES ('conv-1')`); err != nil {
		t.Fatal(err)
	}
	userRoot := filepath.Join(root, "users")
	if err := os.MkdirAll(filepath.Join(userRoot, "user-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userRoot, "user-1", "note.html"), []byte("<p>verified</p>"), 0o600); err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(root, "backups", "app.db")
	if _, err := Backup(ctx, db, dbPath, backupPath); err != nil {
		t.Fatal(err)
	}
	report, err := VerifyRestore(ctx, db, dbPath, backupPath, userRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !report.RestoreVerified || !report.TemporaryRestoreRemoved || report.Integrity != "ok" || !report.ForeignKeysValid {
		t.Fatalf("unexpected restore report: %+v", report)
	}
	if report.ProtectedTables["users"].Rows != 1 || report.UserData == nil || report.UserData.Files != 1 {
		t.Fatalf("protected data was not verified: %+v", report)
	}
	matches, err := filepath.Glob(filepath.Join(root, "backups", ".restore-verify-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary restore remains: matches=%v err=%v", matches, err)
	}
}

func TestMaintenancePathsStayInsidePersistentDataRoot(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "app.db")
	backup := DefaultBackupPath(dbPath, time.Date(2026, 8, 11, 4, 0, 0, 0, time.UTC))
	if !strings.HasPrefix(backup, filepath.Join(root, "backups")) {
		t.Fatalf("default backup is not persistent: %s", backup)
	}
	if _, err := ValidateMaintenanceOutputPath(dbPath, backup); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(filepath.Dir(root), "ephemeral.db")
	if _, err := ValidateMaintenanceOutputPath(dbPath, outside); err == nil {
		t.Fatalf("outside output was accepted: %s", outside)
	}
	for _, reserved := range []string{dbPath, dbPath + "-wal", dbPath + "-shm", dbPath + "-journal"} {
		if _, err := ValidateMaintenanceOutputPath(dbPath, reserved); err == nil {
			t.Fatalf("reserved database path was accepted: %s", reserved)
		}
	}
}

func TestBackupAndArchiveRefuseExistingOutputs(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dbPath := filepath.Join(root, "app.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ensureRequiredInspectionSchema(t, db)
	existing := filepath.Join(root, "backups", "existing.db")
	if err := os.MkdirAll(filepath.Dir(existing), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(existing, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Backup(ctx, db, dbPath, existing); err == nil {
		t.Fatal("backup overwrote an existing output")
	}
	content, err := os.ReadFile(existing)
	if err != nil || string(content) != "keep" {
		t.Fatalf("existing backup output changed: content=%q err=%v", content, err)
	}
	if _, err := Archive(ctx, db, ArchiveOptions{Before: time.Now().UTC(), Output: existing, Apply: true}); err == nil {
		t.Fatal("archive overwrote an existing output")
	}
}

func TestInspectModes(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "app.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ensureRequiredInspectionSchema(t, db)
	quick, err := InspectWithOptions(ctx, db, dbPath, InspectOptions{})
	if err != nil || quick.IntegrityMode != "not_run" || quick.Integrity != "not_run" || quick.Connectivity != "ok" || len(quick.Tables) != 0 {
		t.Fatalf("quick inspect failed: %+v err=%v", quick, err)
	}
	full, err := InspectWithOptions(ctx, db, dbPath, InspectOptions{FullIntegrity: true})
	if err != nil || full.IntegrityMode != "full" || full.Integrity != "ok" {
		t.Fatalf("full inspect failed: %+v err=%v", full, err)
	}
}

func ensureRequiredInspectionSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	statements := []string{
		`CREATE TABLE IF NOT EXISTS users (id TEXT PRIMARY KEY, username TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS user_sessions (id TEXT PRIMARY KEY)`,
		`CREATE TABLE IF NOT EXISTS conversations (id TEXT PRIMARY KEY)`,
		`CREATE TABLE IF NOT EXISTS messages (id TEXT PRIMARY KEY)`,
		`CREATE TABLE IF NOT EXISTS tool_calls (id TEXT PRIMARY KEY)`,
		`CREATE TABLE IF NOT EXISTS file_versions (id TEXT PRIMARY KEY)`,
		`CREATE TABLE IF NOT EXISTS agent_runs (id TEXT PRIMARY KEY, status TEXT NOT NULL DEFAULT 'queued', event_head_seq INTEGER NOT NULL DEFAULT 0, final_seq INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS agent_events (id TEXT PRIMARY KEY, run_id TEXT NOT NULL DEFAULT '', type TEXT NOT NULL DEFAULT '', seq INTEGER NOT NULL DEFAULT 0, payload_json TEXT NOT NULL DEFAULT '{}')`,
		`CREATE TABLE IF NOT EXISTS agent_turn_items (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL DEFAULT '{}')`,
		`CREATE TABLE IF NOT EXISTS agent_edit_previews (preview_id TEXT PRIMARY KEY, snapshot_json TEXT NOT NULL DEFAULT '{}')`,
		`CREATE TABLE IF NOT EXISTS diagnostic_logs (id TEXT PRIMARY KEY, response_snippet TEXT NOT NULL DEFAULT '', request_summary TEXT NOT NULL DEFAULT '', error_message TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS chat_attachments (id TEXT PRIMARY KEY)`,
		`CREATE TABLE IF NOT EXISTS message_attachments (message_id TEXT NOT NULL, attachment_id TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS trash_items (id TEXT PRIMARY KEY)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
}

func insertArchiveFixture(t *testing.T, db *sql.DB, created time.Time) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO tasks (id,user_id,type,status,title,payload_json,created_at,finished_at) VALUES ('task-old','user','fixture','done','fixture','{}',?,?)`, created, created); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO agent_runs (id,user_id,conversation_id,status,created_at,completed_at,updated_at,event_head_seq,final_seq) VALUES ('run-old','user','conversation','succeeded',?,?,?,1,1)`, created, created, created); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO agent_events (id,run_id,user_id,type,level,seq,payload_json,created_at) VALUES ('event-old','run-old','user','turn_completed','info',1,'{"large":"payload"}',?)`, created); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO diagnostic_logs (id,user_id,task_id,stage,level,error_message,response_snippet,request_summary,created_at) VALUES ('diag-old','user','task-old','provider','error','error','snippet','summary',?)`, created); err != nil {
		t.Fatal(err)
	}
}
