package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChangeRevertRestoresEachOperationAndIsIdempotent(t *testing.T) {
	for _, operation := range []string{"create", "update", "move", "delete"} {
		t.Run(operation, func(t *testing.T) {
			s := newTestService(t)
			before, err := s.CreateFile(t.Context(), "a.txt", "before")
			if err != nil {
				t.Fatal(err)
			}
			change := FileChange{Operation: operation, Path: "a.txt", BeforeHash: before.Hash}
			switch operation {
			case "create":
				change.BeforeHash, change.Hash = "", before.Hash
			case "update", "move":
				write, err := s.WriteFile(t.Context(), "a.txt", "after", "ai_apply_patch")
				if err != nil {
					t.Fatal(err)
				}
				change.Hash, change.VersionID = write.Hash, write.VersionID
				if operation == "move" {
					if _, err := s.Rename("a.txt", "b.txt"); err != nil {
						t.Fatal(err)
					}
					change.OldPath, change.Path = "a.txt", "b.txt"
				}
			case "delete":
				trash, err := s.MoveToTrash("a.txt")
				if err != nil {
					t.Fatal(err)
				}
				change.TrashPath = trash.NewPath
			}
			preview, err := s.PreviewChange(t.Context(), "undo", change)
			if err != nil || !preview.CanRevert {
				t.Fatalf("cannot review: %+v %v", preview, err)
			}
			inverse, err := s.RevertChange(t.Context(), "undo", change)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.VerifyChanges([]FileChange{inverse}); err != nil {
				t.Fatal(err)
			}
			if operation == "create" {
				if _, err := s.CreateFile(t.Context(), "a.txt", "user recreated"); err != nil {
					t.Fatal(err)
				}
			} else {
				read, err := s.ReadFile("a.txt")
				if err != nil || read.Content != "before" {
					t.Fatalf("not restored: %+v %v", read, err)
				}
				if _, err := s.WriteFile(t.Context(), "a.txt", "user recreated", "test"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.RevertChange(t.Context(), "undo", change); err != nil {
				t.Fatal(err)
			}
			read, err := s.ReadFile("a.txt")
			if err != nil || read.Content != "user recreated" {
				t.Fatalf("replayed undo destroyed newer file: %+v %v", read, err)
			}
		})
	}
}

func TestChangeRevertRefusesNewerContentAfterPreview(t *testing.T) {
	s := newTestService(t)
	write, err := s.CreateFile(t.Context(), "a.txt", "agent")
	if err != nil {
		t.Fatal(err)
	}
	change := FileChange{Operation: "create", Path: "a.txt", Hash: write.Hash}
	if _, err := s.PreviewChange(t.Context(), "undo", change); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteFile(t.Context(), "a.txt", "user edited", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RevertChange(t.Context(), "undo", change); !errors.Is(err, ErrFileConflict) {
		t.Fatalf("expected conflict: %v", err)
	}
	if read, err := s.ReadFile("a.txt"); err != nil || read.Content != "user edited" {
		t.Fatalf("user edit lost: %+v %v", read, err)
	}
}

func TestAmbiguousRevertIsNeverReplayed(t *testing.T) {
	s := newTestService(t)
	before, err := s.CreateFile(t.Context(), "a.txt", "before")
	if err != nil {
		t.Fatal(err)
	}
	write, err := s.WriteFile(t.Context(), "a.txt", "after", "test")
	if err != nil {
		t.Fatal(err)
	}
	change := FileChange{Operation: "update", Path: "a.txt", BeforeHash: before.Hash, Hash: write.Hash, VersionID: write.VersionID}
	if _, err := s.db.Exec(`CREATE TRIGGER reject_revert_completion BEFORE UPDATE ON workspace_change_reverts BEGIN SELECT RAISE(ABORT, 'injected completion failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RevertChange(t.Context(), "undo", change); err == nil {
		t.Fatal("expected completion failure")
	}
	if read, err := s.ReadFile("a.txt"); err != nil || read.Content != "before" {
		t.Fatalf("disk not restored: %+v %v", read, err)
	}
	if _, err := s.db.Exec(`DROP TRIGGER reject_revert_completion`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteFile(t.Context(), "a.txt", "after", "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RevertChange(t.Context(), "undo", change); err == nil || !strings.Contains(err.Error(), "不会重复执行") {
		t.Fatalf("ambiguous undo retried: %v", err)
	}
	if read, err := s.ReadFile("a.txt"); err != nil || read.Content != "after" {
		t.Fatalf("ambiguous replay mutated disk: %+v %v", read, err)
	}
}

func TestChangePreviewBoundsFileSize(t *testing.T) {
	s := newTestService(t)
	if err := os.WriteFile(filepath.Join(s.rootAbs, "large.txt"), []byte(strings.Repeat("x", maxChangePreviewBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	preview, err := s.PreviewChange(t.Context(), "undo", FileChange{Operation: "create", Path: "large.txt", Hash: "untrusted"})
	if err != nil || preview.CanRevert || len(preview.Current) != 0 {
		t.Fatalf("unbounded preview: %+v %v", preview, err)
	}
}
func TestCorruptBaselineCannotBeRestored(t *testing.T) {
	s := newTestService(t)
	before, err := s.CreateFile(t.Context(), "a.txt", "before")
	if err != nil {
		t.Fatal(err)
	}
	after, err := s.WriteFile(t.Context(), "a.txt", "after", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE file_versions SET content_text = 'corrupt' WHERE id = ?`, after.VersionID); err != nil {
		t.Fatal(err)
	}
	change := FileChange{Operation: "update", Path: "a.txt", Hash: after.Hash, BeforeHash: before.Hash, VersionID: after.VersionID}
	if _, err := s.RevertChange(t.Context(), "corrupt-revert", change); err == nil {
		t.Fatal("corrupt baseline restored")
	}
	if file, err := s.ReadFile("a.txt"); err != nil || file.Content != "after" {
		t.Fatalf("current content lost: %+v %v", file, err)
	}
}
