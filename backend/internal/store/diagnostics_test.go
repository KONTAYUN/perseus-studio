package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiagnosticStoreUserIsolationAndLimits(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()

	store := NewDiagnosticStore(db)
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}

	if _, err := store.Append(ctx, DiagnosticLogInput{
		UserID:          "user-a",
		Stage:           "provider_error",
		Level:           "error",
		ErrorMessage:    strings.Repeat("e", 1200),
		ResponseSnippet: strings.Repeat("s", 700),
	}); err != nil {
		t.Fatalf("Append user-a returned error: %v", err)
	}
	if _, err := store.Append(ctx, DiagnosticLogInput{UserID: "user-b", Stage: "provider_error", Level: "error"}); err != nil {
		t.Fatalf("Append user-b returned error: %v", err)
	}

	logs, err := store.List(ctx, DiagnosticLogQuery{UserID: "user-a", Limit: 10})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(logs) != 1 || logs[0].UserID != "user-a" {
		t.Fatalf("expected only user-a logs, got %#v", logs)
	}
	if len([]rune(logs[0].ErrorMessage)) != 1000 {
		t.Fatalf("expected truncated error message, got %d runes", len([]rune(logs[0].ErrorMessage)))
	}
	if len([]rune(logs[0].ResponseSnippet)) != 500 {
		t.Fatalf("expected truncated response snippet, got %d runes", len([]rune(logs[0].ResponseSnippet)))
	}

	if err := store.ClearUser(ctx, "user-a"); err != nil {
		t.Fatalf("ClearUser returned error: %v", err)
	}
	logs, err = store.List(ctx, DiagnosticLogQuery{UserID: "user-a", Limit: 10})
	if err != nil {
		t.Fatalf("List after clear returned error: %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("expected user-a logs cleared, got %#v", logs)
	}
	logs, err = store.List(ctx, DiagnosticLogQuery{UserID: "user-b", Limit: 10})
	if err != nil {
		t.Fatalf("List user-b returned error: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected user-b log to remain, got %#v", logs)
	}
}

func TestDiagnosticStoreKeepsLatest200PerUser(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()

	store := NewDiagnosticStore(db)
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	for i := 0; i < 205; i++ {
		if _, err := store.Append(ctx, DiagnosticLogInput{UserID: "user-a", Stage: "s", Level: "error"}); err != nil {
			t.Fatalf("Append %d returned error: %v", i, err)
		}
	}
	logs, err := store.List(ctx, DiagnosticLogQuery{UserID: "user-a", Limit: 300})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(logs) != 200 {
		t.Fatalf("expected 200 retained logs, got %d", len(logs))
	}
}
