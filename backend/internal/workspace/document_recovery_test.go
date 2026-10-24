package workspace

import (
	"errors"
	"testing"
	"webide-backend/internal/document"
)

func TestDocumentRepairReconcilesLostDatabaseAcknowledgementWithoutReapplying(t *testing.T) {
	s := newTestService(t)
	ctx := t.Context()
	head, err := s.CreateFile(ctx, "recover.md", "before")
	if err != nil {
		t.Fatal(err)
	}
	action := &document.Action{Type: "format_repair", Path: head.Path, BaseHash: head.Hash, Profile: document.Version, Diagnostics: []document.Issue{{Code: "test", Message: "test"}}}
	c, err := s.StageDocumentRepair(ctx, "r", action, "after")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.db.Exec(`CREATE TRIGGER fail_applied BEFORE UPDATE OF state ON document_repair_candidates WHEN NEW.state='applied' BEGIN SELECT RAISE(ABORT,'lost acknowledgement'); END`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DecideDocumentRepair(ctx, c.ID, head.Hash, true); err == nil {
		t.Fatal("fault not injected")
	}
	actual, _ := s.ReadFile(head.Path)
	if actual.Hash != c.Hash {
		t.Fatal("write did not reach disk")
	}
	_, err = s.db.Exec(`DROP TRIGGER fail_applied`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DecideDocumentRepair(ctx, c.ID, head.Hash, true); err != nil {
		t.Fatal(err)
	}
	versions, err := s.DocumentVersions(ctx, head.Path)
	if err != nil || len(versions) != 1 {
		t.Fatal("retry created duplicate snapshot", err, len(versions))
	}
	if _, err = s.WriteFile(ctx, head.Path, "new user head", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DecideDocumentRepair(ctx, c.ID, head.Hash, true); !errors.Is(err, ErrFileConflict) {
		t.Fatal("stale retry allowed", err)
	}
}

func TestDocumentChunkReceiptsAndFinalizeAreDurableAndIdempotent(t *testing.T) {
	s := newTestService(t)
	ctx := t.Context()
	first, err := s.StageDocumentChunk(ctx, "r", "chunks.md", "create", "first\n", 1)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.StageDocumentChunk(ctx, "r", "chunks.md", "append", "last\n", 2)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.VerifyDocumentChunk(ctx, "r", c.Path, first.Hash, 1); err != nil {
		t.Fatal("append invalidated earlier receipt", err)
	}
	_, err = s.db.Exec(`CREATE TRIGGER fail_chunk_applied BEFORE UPDATE OF state ON document_chunk_drafts WHEN NEW.state='applied' BEGIN SELECT RAISE(ABORT,'lost acknowledgement'); END`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CommitDocumentChunk(ctx, "r", c.Path, c.Hash); err == nil {
		t.Fatal("fault not injected")
	}
	if _, err = s.db.Exec(`DROP TRIGGER fail_chunk_applied`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CommitDocumentChunk(ctx, "r", c.Path, c.Hash); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CommitDocumentChunk(ctx, "r", c.Path, c.Hash); err != nil {
		t.Fatal("finalize not idempotent", err)
	}
	if _, err = s.StageDocumentChunk(ctx, "r", c.Path, "append", "extra", 3); err == nil {
		t.Fatal("append after commit allowed")
	}
	if _, err = s.WriteFile(ctx, c.Path, "new user head", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CommitDocumentChunk(ctx, "r", c.Path, c.Hash); !errors.Is(err, ErrFileConflict) {
		t.Fatal("stale commit replayed", err)
	}
}
