package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func createTestDirectoryLink(t *testing.T, link, target string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS != "windows" {
			t.Fatal(err)
		}
		if output, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
			t.Fatalf("create junction: %v: %s", err, output)
		}
	}
}

func TestSameTrashNamesDoNotOverwriteAnotherUsersRecoveryMetadata(t *testing.T) {
	db, root := newTestDBAndRoot(t)
	var services []*Service
	for _, user := range []string{"alice", "bob"} {
		s, err := NewService(db, filepath.Join(root, user), user)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Migrate(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := s.EnsureWorkspace(); err != nil {
			t.Fatal(err)
		}
		path := user + "-nested/same.txt"
		if _, err := s.CreateFile(t.Context(), path, user); err != nil {
			t.Fatal(err)
		}
		if _, err := s.MoveToTrash(path); err != nil {
			t.Fatal(err)
		}
		services = append(services, s)
	}
	for i, user := range []string{"alice", "bob"} {
		items, err := services[i].TrashItems()
		if err != nil || len(items) != 1 || items[0].OriginalPath != user+"-nested/same.txt" {
			t.Fatalf("lost %s metadata: %+v %v", user, items, err)
		}
		if _, err := services[i].RestoreTrashItem(items[0].Path, false); err != nil {
			t.Fatal(err)
		}
		read, err := services[i].ReadFile(user + "-nested/same.txt")
		if err != nil || read.Content != user {
			t.Fatalf("restored wrong user's file: %+v %v", read, err)
		}
	}
}

func TestLegacyMigrationRejectsLinkedTrashDirectories(t *testing.T) {
	for _, linked := range []string{TrashDirName, LegacyTrashDirName} {
		t.Run(linked, func(t *testing.T) {
			s := newTestService(t)
			outside := t.TempDir()
			if err := os.WriteFile(filepath.Join(outside, "keep.txt"), []byte("outside"), 0o600); err != nil {
				t.Fatal(err)
			}
			createTestDirectoryLink(t, filepath.Join(s.rootAbs, linked), outside)
			if err := s.migrateLegacyTrashDir(); err == nil {
				t.Fatal("migration followed linked trash")
			}
			if raw, err := os.ReadFile(filepath.Join(outside, "keep.txt")); err != nil || string(raw) != "outside" {
				t.Fatalf("outside content changed: %q %v", raw, err)
			}
		})
	}
}
