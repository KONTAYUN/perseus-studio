package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestRootAliasesCannotBeMutated(t *testing.T) {
	s := newTestService(t)
	if _, err := s.CreateFile(t.Context(), "keep.txt", "keep"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".", "a/..", `a\..`, "./"} {
		if _, err := s.Delete(path); !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("delete %q: %v", path, err)
		}
		if _, err := s.MoveToTrash(path); !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("trash %q: %v", path, err)
		}
		if _, err := s.Rename(path, "moved"); !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("rename %q: %v", path, err)
		}
	}
	if file, err := s.ReadFile("keep.txt"); err != nil || file.Content != "keep" {
		t.Fatalf("workspace changed: %+v %v", file, err)
	}
	for _, path := range []string{"keep.txt:secret", "C:relative.txt"} {
		if _, err := s.WriteFile(t.Context(), path, "bad", "test"); !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("accepted %q: %v", path, err)
		}
	}
}

func TestStalePathMutationsPreserveUserContent(t *testing.T) {
	s := newTestService(t)
	before, err := s.CreateFile(t.Context(), "a.txt", "old")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteFile(t.Context(), "a.txt", "newer", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveToTrashIfUnchanged(t.Context(), "a.txt", before.Hash); !errors.Is(err, ErrFileConflict) {
		t.Fatalf("stale delete: %v", err)
	}
	if _, err := s.RenameIfUnchanged(t.Context(), "a.txt", "b.txt", before.Hash); !errors.Is(err, ErrFileConflict) {
		t.Fatalf("stale move: %v", err)
	}
	if file, err := s.ReadFile("a.txt"); err != nil || file.Content != "newer" {
		t.Fatalf("lost content: %+v %v", file, err)
	}
}

func TestConcurrentSaveAndTrashAdmitOnlyOne(t *testing.T) {
	s := newTestService(t)
	before, err := s.CreateFile(t.Context(), "a.txt", "old")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := s.MoveToTrashIfUnchanged(t.Context(), "a.txt", before.Hash)
		results <- err
	}()
	go func() {
		defer wg.Done()
		_, err := s.WriteFileIfUnchanged(t.Context(), "a.txt", "saved", "test", &before.Hash)
		results <- err
	}()
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrFileConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
}

func TestCanceledWritesDoNotCreateFiles(t *testing.T) {
	s := newTestService(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.CreateTextFile(ctx, "canceled/a.txt", "bad", "test"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.rootAbs, "canceled")); !os.IsNotExist(err) {
		t.Fatalf("created directory after cancel: %v", err)
	}
}

func TestAtomicWritePreservesExecutableMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits")
	}
	s := newTestService(t)
	path := filepath.Join(s.rootAbs, "script.sh")
	if err := os.WriteFile(path, []byte("old"), 0o751); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteFile(t.Context(), "script.sh", "new", "test"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o751 {
		t.Fatalf("mode lost: %v %v", info, err)
	}
}

func TestMetadataFailureRollsBackTrashAndRename(t *testing.T) {
	for _, op := range []string{"trash", "rename"} {
		t.Run(op, func(t *testing.T) {
			s := newTestService(t)
			if _, err := s.CreateFile(t.Context(), "a.txt", "keep"); err != nil {
				t.Fatal(err)
			}
			if err := s.db.Close(); err != nil {
				t.Fatal(err)
			}
			var err error
			if op == "trash" {
				_, err = s.MoveToTrash("a.txt")
			} else {
				_, err = s.Rename("a.txt", "b.txt")
			}
			if err == nil {
				t.Fatal("expected metadata failure")
			}
			if read, err := os.ReadFile(filepath.Join(s.rootAbs, "a.txt")); err != nil || string(read) != "keep" {
				t.Fatalf("mutation not rolled back: %q %v", read, err)
			}
		})
	}
}
