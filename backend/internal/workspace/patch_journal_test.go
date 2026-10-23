package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"webide-backend/internal/failure"
	"webide-backend/internal/store"
)

func journalHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func TestPatchJournalSurvivesAbruptProcessExitAtEveryStage(t *testing.T) {
	if root := os.Getenv("PERSEUS_TEST_PATCH_CRASH_ROOT"); root != "" {
		db, err := store.Open(filepath.Join(root, "test.db"))
		if err != nil {
			panic(err)
		}
		s, err := NewService(db, filepath.Join(root, "workspace"), "test-user")
		if err != nil {
			panic(err)
		}
		stage := os.Getenv("PERSEUS_TEST_PATCH_CRASH_STAGE")
		if _, err = s.WriteFileIfUnchanged(context.Background(), "a.md", "after", "test", nil); err != nil {
			panic(err)
		}
		if stage == "write" {
			os.Exit(71)
		}
		if _, err = s.CreateTextFile(context.Background(), "created.md", "created", "test"); err != nil {
			panic(err)
		}
		if stage == "create" {
			os.Exit(71)
		}
		if _, err = s.RenameIfUnchanged(context.Background(), "a.md", "moved.md", journalHash("after")); err != nil {
			panic(err)
		}
		if stage == "move" {
			os.Exit(71)
		}
		if _, err = s.MoveToTrashIfUnchanged(context.Background(), "deleted.md", journalHash("original")); err != nil {
			panic(err)
		}
		os.Exit(71) // Simulates a crash before completion/ledger/checkpoint.
	}
	for _, stage := range []string{"write", "create", "move", "delete"} {
		t.Run(stage, func(t *testing.T) {
			root := t.TempDir()
			db, err := store.Open(filepath.Join(root, "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			s := newTestServiceWithDBAndRoot(t, db, filepath.Join(root, "workspace"))
			for _, path := range []string{"a.md", "deleted.md"} {
				if _, err = s.CreateTextFile(t.Context(), path, "original", "test"); err != nil {
					t.Fatal(err)
				}
			}
			entries := []PatchJournalEntry{
				{Operation: "update", Path: "a.md", Before: "original", BeforeHash: journalHash("original"), AfterHash: journalHash("after")},
				{Operation: "create", Path: "created.md", AfterHash: journalHash("created")},
				{Operation: "move", Path: "a.md", NewPath: "moved.md", AfterHash: journalHash("after")},
				{Operation: "delete", Path: "deleted.md", Before: "original", BeforeHash: journalHash("original")},
			}
			if _, err = s.BeginPatchJournal(t.Context(), entries); err != nil {
				t.Fatal(err)
			}
			if err = db.Close(); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestPatchJournalSurvivesAbruptProcessExitAtEveryStage$")
			cmd.Env = append(os.Environ(), "PERSEUS_TEST_PATCH_CRASH_ROOT="+root, "PERSEUS_TEST_PATCH_CRASH_STAGE="+stage)
			output, crashErr := cmd.CombinedOutput()
			exitErr, ok := crashErr.(*exec.ExitError)
			if !ok || exitErr.ExitCode() != 71 {
				t.Fatalf("crash helper failed unexpectedly: %v %s", crashErr, output)
			}
			db, err = store.Open(filepath.Join(root, "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			s = newTestServiceWithDBAndRoot(t, db, filepath.Join(root, "workspace"))
			if err = s.RecoverPendingPatches(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err = s.RecoverPendingPatches(t.Context()); err != nil {
				t.Fatal("recovery not idempotent", err)
			}
			for _, path := range []string{"a.md", "deleted.md"} {
				read, err := s.ReadFile(path)
				if err != nil || read.Content != "original" {
					t.Fatalf("lost original %s: %q %v", path, read.Content, err)
				}
			}
			for _, path := range []string{"created.md", "moved.md"} {
				if _, err = s.StatPath(path); !os.IsNotExist(err) {
					t.Fatalf("partial result remains %s: %v", path, err)
				}
			}
		})
	}
}

func TestPatchRecoveryPreservesSubsequentUserEdits(t *testing.T) {
	db, root := newTestDBAndRoot(t)
	s := newTestServiceWithDBAndRoot(t, db, root)
	if _, err := s.CreateTextFile(t.Context(), "a.md", "user content", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginPatchJournal(t.Context(), []PatchJournalEntry{{Operation: "update", Path: "a.md", Before: "original", BeforeHash: journalHash("original"), AfterHash: journalHash("after")}}); err != nil {
		t.Fatal(err)
	}
	err := s.RecoverPendingPatches(t.Context())
	if failure.Code(err) != "workspace_patch_recovery_conflict" {
		t.Fatalf("expected conflict, got %v", err)
	}
	read, _ := s.ReadFile("a.md")
	if read.Content != "user content" {
		t.Fatal("overwrote user content")
	}
}

func TestCommandObservationIsBoundedCancelableAndReportsReadErrors(t *testing.T) {
	db, root := newTestDBAndRoot(t)
	s := newTestServiceWithDBAndRoot(t, db, root)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.commandWorkspaceSnapshotContext(ctx); err == nil {
		t.Fatal("ignored cancellation")
	}
	if _, err := hashFileForCommandSnapshot(t.Context(), filepath.Join(root, "missing"), 20); err == nil {
		t.Fatal("swallowed read error")
	}
	file, err := os.Create(filepath.Join(root, "large.bin"))
	if err != nil {
		t.Fatal(err)
	}
	err = file.Truncate(257 << 20)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.commandWorkspaceSnapshotContext(t.Context()); err == nil {
		t.Fatal("ignored byte budget")
	}
	t.Setenv("PERSEUS_COMMAND_EXECUTION", "")
	if _, err := s.RunCommand(t.Context(), fmt.Sprint("echo test"), "", 1); failure.Code(err) != "command_disabled" {
		t.Fatalf("not default closed: %v", err)
	}
}
