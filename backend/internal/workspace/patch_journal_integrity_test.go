package workspace

import (
	"encoding/json"
	"testing"

	"webide-backend/internal/failure"
)

func TestCorruptPatchOriginalStopsRecoveryBeforeAnyWrite(t *testing.T) {
	db, root := newTestDBAndRoot(t)
	s := newTestServiceWithDBAndRoot(t, db, root)
	for _, path := range []string{"a.md", "b.md"} {
		if _, err := s.CreateTextFile(t.Context(), path, "after", "test"); err != nil {
			t.Fatal(err)
		}
	}
	entries := []PatchJournalEntry{
		{Operation: "update", Path: "a.md", Before: "original", BeforeHash: journalHash("original"), AfterHash: journalHash("after")},
		{Operation: "update", Path: "b.md", Before: "original", BeforeHash: journalHash("original"), AfterHash: journalHash("after")},
	}
	id, err := s.BeginPatchJournal(t.Context(), entries)
	if err != nil {
		t.Fatal(err)
	}
	// Valid JSON with a damaged original must not restore unverified bytes.
	// b.md appears later, so reverse recovery must validate a.md before touching b.md.
	entries[0].Before = "damaged recovery content"
	raw, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(t.Context(), `UPDATE workspace_patch_journals SET plan_json=? WHERE id=?`, string(raw), id); err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverPendingPatches(t.Context()); failure.Code(err) != "workspace_patch_journal_invalid" {
		t.Fatalf("corruption was not rejected: %v", err)
	}
	for _, path := range []string{"a.md", "b.md"} {
		file, err := s.ReadFile(path)
		if err != nil || file.Content != "after" {
			t.Fatalf("unverified recovery wrote %s: %#v %v", path, file, err)
		}
	}
	var state string
	if err = db.QueryRowContext(t.Context(), `SELECT state FROM workspace_patch_journals WHERE id=?`, id).Scan(&state); err != nil || state != "pending" {
		t.Fatal("lost recovery evidence")
	}
}
