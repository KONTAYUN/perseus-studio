package workspace

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestConcurrentEditorSavesDoNotOverwriteAgentChanges(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	initial, err := s.WriteFile(ctx, "draft.md", "original", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteFile(ctx, "draft.md", "agent change", "ai_patch"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteFileIfUnchanged(ctx, "draft.md", "stale autosave", "write", &initial.Hash); !errors.Is(err, ErrFileConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	file, err := s.ReadFile("draft.md")
	if err != nil {
		t.Fatal(err)
	}
	if file.Content != "agent change" {
		t.Fatalf("lost agent change: %q", file.Content)
	}

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, content := range []string{"save one", "save two"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.WriteFileIfUnchanged(ctx, "draft.md", content, "write", &file.Hash)
			results <- err
		}()
	}
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
