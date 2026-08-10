package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenReadOnlyQueriesWithoutAllowingWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "readonly.db")
	writable, err := Open(path)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	if _, err := writable.ExecContext(context.Background(), `CREATE TABLE evidence (value TEXT); INSERT INTO evidence(value) VALUES ('preserved')`); err != nil {
		writable.Close()
		t.Fatalf("seed database: %v", err)
	}
	if err := writable.Close(); err != nil {
		t.Fatalf("close writable database: %v", err)
	}

	readonly, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("OpenReadOnly returned error: %v", err)
	}
	defer readonly.Close()
	var value string
	if err := readonly.QueryRowContext(context.Background(), `SELECT value FROM evidence`).Scan(&value); err != nil {
		t.Fatalf("read query failed: %v", err)
	}
	if value != "preserved" {
		t.Fatalf("read value = %q, want preserved", value)
	}
	if _, err := readonly.ExecContext(context.Background(), `INSERT INTO evidence(value) VALUES ('forbidden')`); err == nil {
		t.Fatal("read-only database accepted a write")
	}
}
