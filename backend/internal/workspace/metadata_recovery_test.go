package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRestoreMetadataFailureRestoresBothOriginals(t *testing.T) {
	s := newTestService(t)
	if _, err := s.CreateFile(t.Context(), "a.txt", "old"); err != nil {
		t.Fatal(err)
	}
	trash, err := s.MoveToTrash("a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateFile(t.Context(), "a.txt", "current"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER fail_trash_delete BEFORE DELETE ON trash_items BEGIN SELECT RAISE(ABORT, 'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RestoreTrashItem(trash.NewPath, true); err == nil {
		t.Fatal("expected metadata failure")
	}
	for path, want := range map[string]string{"a.txt": "current", trash.NewPath: "old"} {
		got, err := os.ReadFile(filepath.Join(s.rootAbs, filepath.FromSlash(path)))
		if err != nil || string(got) != want {
			t.Fatalf("lost %s: %q %v", path, got, err)
		}
	}
}

func TestReorderMetadataFailureReversesMove(t *testing.T) {
	s := newTestService(t)
	if _, err := s.CreateFile(t.Context(), "a.txt", "keep"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MkdirNew("target"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER fail_order_insert BEFORE INSERT ON workspace_item_orders BEGIN SELECT RAISE(ABORT, 'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReorderPath("a.txt", "target", "", ""); err == nil {
		t.Fatal("expected metadata failure")
	}
	if raw, err := os.ReadFile(filepath.Join(s.rootAbs, "a.txt")); err != nil || string(raw) != "keep" {
		t.Fatalf("move not compensated: %q %v", raw, err)
	}
}

func TestPathOrderMetadataUsesLiteralCaseSensitivePrefixes(t *testing.T) {
	for _, name := range []string{"a_b", "a%b", "Case", "中文_目录"} {
		t.Run(name, func(t *testing.T) {
			s := newTestService(t)
			// Include wildcard matches, case-only variants, and Unicode descendants.
			parents := []string{name + "/nested", "axb/nested", "case/nested", "中文X目录/nested"}
			for _, parent := range parents {
				if err := s.replaceOrderForParent(parent, []string{"z", "a"}); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.moveOrderMetadata(name, "moved"); err != nil {
				t.Fatal(err)
			}
			if order, _, err := s.loadOrderForParent("moved/nested"); err != nil || order["z"] != 1 {
				t.Fatalf("descendant not moved: %v %v", order, err)
			}
			for _, parent := range parents[1:] {
				if order, _, err := s.loadOrderForParent(parent); err != nil || order["z"] != 1 {
					t.Fatalf("unrelated metadata changed: %s %v %v", parent, order, err)
				}
			}
			if err := s.removeOrderMetadataForPath(name); err != nil {
				t.Fatal(err)
			}
			for _, parent := range parents[1:] {
				if order, _, err := s.loadOrderForParent(parent); err != nil || order["z"] != 1 {
					t.Fatalf("unrelated metadata deleted: %s %v %v", parent, order, err)
				}
			}
		})
	}
}
