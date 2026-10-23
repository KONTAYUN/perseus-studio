package workspace

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLargeFilesAreRejectedWithoutChangingDisk(t *testing.T) {
	s := newTestService(t)
	path := filepath.Join(s.rootAbs, "large.txt")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxFileReadBytes + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	file.Close()
	if _, err := s.ReadFile("large.txt"); err == nil {
		t.Fatal("unbounded editor read accepted")
	}
	if _, err := s.ReadTextFile("large.txt", 1000); err == nil {
		t.Fatal("unbounded model read accepted")
	}
	if _, err := s.WriteFile(t.Context(), "large.txt", "replacement", "test"); err == nil {
		t.Fatal("overwrite without bounded backup accepted")
	}
	if info, err := os.Stat(path); err != nil || info.Size() != maxFileReadBytes+1 {
		t.Fatalf("large file changed: %v %v", info, err)
	}
	content := bytes.Repeat([]byte{'x'}, int(maxFileReadBytes+1))
	if _, err := s.CreateBytes(t.Context(), "new/large.bin", content); err == nil {
		t.Fatal("oversized write accepted")
	}
	if _, err := os.Stat(filepath.Join(s.rootAbs, "new")); !os.IsNotExist(err) {
		t.Fatalf("rejected write created directories: %v", err)
	}
}
