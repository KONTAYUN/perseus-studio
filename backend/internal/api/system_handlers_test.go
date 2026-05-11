package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"webide-backend/internal/config"
	"webide-backend/internal/store"
	"webide-backend/internal/version"
)

func TestVersionEndpointIsPublic(t *testing.T) {
	oldVersion, oldCommit, oldBuiltAt := version.Version, version.Commit, version.BuiltAt
	version.Version, version.Commit, version.BuiltAt = "test-version", "test-commit", "test-built-at"
	t.Cleanup(func() {
		version.Version, version.Commit, version.BuiltAt = oldVersion, oldCommit, oldBuiltAt
	})

	server := &Server{mux: http.NewServeMux()}
	server.register()

	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if payload["version"] != "test-version" || payload["commit"] != "test-commit" || payload["built_at"] != "test-built-at" {
		t.Fatalf("unexpected version payload: %#v", payload)
	}
}

func TestDebugEndpointDisabledByDefault(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	users := store.NewUserStore(db)
	if err := users.Migrate(t.Context()); err != nil {
		t.Fatalf("users.Migrate returned error: %v", err)
	}
	admin, err := users.CreateUser(t.Context(), "admin", "password", "admin")
	if err != nil {
		t.Fatalf("CreateUser admin returned error: %v", err)
	}
	sessions := store.NewSessionStore(db)
	if err := sessions.Migrate(t.Context()); err != nil {
		t.Fatalf("sessions.Migrate returned error: %v", err)
	}
	server := &Server{
		cfg:      config.Config{EnableDebugAPI: false},
		mux:      http.NewServeMux(),
		sessions: sessions,
	}
	server.register()

	req := httptest.NewRequest(http.MethodPost, "/api/debug/model-tool-test", nil)
	attachTestSession(t, server, req, "admin-token", admin)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for disabled debug endpoint, got %d: %s", rec.Code, rec.Body.String())
	}
}
