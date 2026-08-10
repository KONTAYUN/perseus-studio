package maintenance

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
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

func TestInspectModes(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "app.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	quick, err := InspectWithOptions(ctx, db, dbPath, InspectOptions{})
	if err != nil || quick.IntegrityMode != "quick" || quick.Integrity != "ok" {
		t.Fatalf("quick inspect failed: %+v err=%v", quick, err)
	}
	full, err := InspectWithOptions(ctx, db, dbPath, InspectOptions{FullIntegrity: true})
	if err != nil || full.IntegrityMode != "full" || full.Integrity != "ok" {
		t.Fatalf("full inspect failed: %+v err=%v", full, err)
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
