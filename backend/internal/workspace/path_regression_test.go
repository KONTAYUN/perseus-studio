package workspace

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLeafNameCannotContainDirectories(t *testing.T) {
	for _, path := range []string{"nested/child.md", `nested\child.md`, "../other.md", `..\other.md`} {
		if _, err := sanitizeLeafName(path); err == nil {
			t.Errorf("accepted a path as a leaf name: %q", path)
		}
	}
}

func TestWorkspaceRejectsSymlinkEscapes(t *testing.T) {
	s := newTestService(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(s.rootAbs, "escape")
	if err := os.Symlink(outside, link); err != nil {
		if runtime.GOOS != "windows" {
			t.Fatal(err)
		}
		if output, junctionErr := exec.Command("cmd", "/c", "mklink", "/J", link, outside).CombinedOutput(); junctionErr != nil {
			t.Fatalf("cannot create test junction: %v: %s", junctionErr, output)
		}
	}
	if _, err := s.ReadFile("escape/secret.md"); !errors.Is(err, ErrPathEscape) {
		t.Fatalf("symlink escaped workspace on read: %v", err)
	}
	if _, err := s.WriteFile(context.Background(), "escape/new.md", "modified", "test"); !errors.Is(err, ErrPathEscape) {
		t.Fatalf("symlink escaped workspace on write: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "new.md")); !os.IsNotExist(err) {
		t.Fatalf("outside file was created: %v", err)
	}
	results, _, err := s.SearchText("private", false, 20, 0)
	if err != nil || len(results) != 0 {
		t.Fatalf("search followed a linked directory: %#v %v", results, err)
	}
	snapshot, err := s.commandWorkspaceSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := snapshot["escape/secret.md"]; exists {
		t.Fatal("command snapshot followed a linked directory")
	}
}
