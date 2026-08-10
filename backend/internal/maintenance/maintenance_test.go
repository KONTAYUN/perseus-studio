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

func insertArchiveFixture(t *testing.T, db *sql.DB, created time.Time) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO agent_runs (id,user_id,conversation_id,status,created_at,completed_at,updated_at,event_head_seq,final_seq) VALUES ('run-old','user','conversation','succeeded',?,?,?,1,1)`, created, created, created); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO agent_events (id,run_id,user_id,type,level,seq,payload_json,created_at) VALUES ('event-old','run-old','user','turn_completed','info',1,'{"large":"payload"}',?)`, created); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO diagnostic_logs (id,user_id,stage,level,error_message,response_snippet,request_summary,created_at) VALUES ('diag-old','user','provider','error','error','snippet','summary',?)`, created); err != nil {
		t.Fatal(err)
	}
}
