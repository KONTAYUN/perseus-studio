package workspace

import (
	"errors"
	"testing"
	"webide-backend/internal/document"
)

func TestDocumentVersionsFollowIdentityAndRestoreChecksHash(t *testing.T) {
	s := newTestService(t)
	ctx := t.Context()
	a, err := s.CreateFile(ctx, "稿件.md", "第一版")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.WriteFileIfUnchanged(ctx, "稿件.md", "第二版", "test", &a.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Rename("稿件.md", "新名字.md"); err != nil {
		t.Fatal(err)
	}
	versions, err := s.DocumentVersions(ctx, "新名字.md")
	if err != nil || len(versions) != 1 {
		t.Fatalf("%+v %v", versions, err)
	}
	id := versions[0].DocumentID
	if id == "" {
		t.Fatal("missing stable identity")
	}
	if _, err = s.RestoreDocumentVersion(ctx, "新名字.md", versions[0].ID, "stale"); !errors.Is(err, ErrFileConflict) {
		t.Fatal(err)
	}
	r, err := s.RestoreDocumentVersion(ctx, "新名字.md", versions[0].ID, b.Hash)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.ReadFile("新名字.md")
	if err != nil || got.Content != "第一版" || got.Hash != r.Hash {
		t.Fatalf("%+v %v", got, err)
	}
	versions, err = s.DocumentVersions(ctx, "新名字.md")
	if err != nil || len(versions) != 2 || versions[0].DocumentID != id {
		t.Fatalf("%+v %v", versions, err)
	}
	trash, err := s.MoveToTrash("新名字.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateFile(ctx, "新名字.md", "同名新文稿"); err != nil {
		t.Fatal(err)
	}
	fresh, err := s.DocumentVersions(ctx, "新名字.md")
	if err != nil || len(fresh) != 0 {
		t.Fatalf("history leaked to new document: %+v %v", fresh, err)
	}
	if _, err = s.RestoreTrashItem(trash.NewPath, true); err != nil {
		t.Fatal(err)
	}
	restored, err := s.DocumentVersions(ctx, "新名字.md")
	if err != nil || len(restored) != 2 || restored[0].DocumentID != id {
		t.Fatalf("identity lost: %+v %v", restored, err)
	}
}
func TestDocumentRejectsInvalidWriteAndCorruptHistory(t *testing.T) {
	s := newTestService(t)
	ctx := t.Context()
	a, err := s.CreateFile(ctx, "a.md", "保留原稿")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.WriteBytes(ctx, "a.md", []byte{0xff}, "test"); err == nil {
		t.Fatal("invalid UTF8 written")
	} else {
		var v *document.ValidationError
		if !errors.As(err, &v) {
			t.Fatal(err)
		}
	}
	got, _ := s.ReadFile("a.md")
	if got.Hash != a.Hash {
		t.Fatal("changed rejected file")
	}
	b, err := s.WriteFile(ctx, "a.md", "新稿", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`UPDATE file_versions SET content_text='损坏' WHERE id=?`, b.VersionID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RestoreDocumentVersion(ctx, "a.md", b.VersionID, b.Hash); err == nil {
		t.Fatal("restored corrupt version")
	}
	got, _ = s.ReadFile("a.md")
	if got.Hash != b.Hash {
		t.Fatal("corruption overwrote current file")
	}
}
