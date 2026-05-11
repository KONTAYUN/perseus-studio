package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"webide-backend/internal/config"
	"webide-backend/internal/version"
)

func TestVersionEndpointIsPublic(t *testing.T) {
	oldVersion, oldCommit, oldBuiltAt := version.Version, version.Commit, version.BuiltAt
	version.Version, version.Commit, version.BuiltAt = "test-version", "test-commit", "test-built-at"
	t.Cleanup(func() {
		version.Version, version.Commit, version.BuiltAt = oldVersion, oldCommit, oldBuiltAt
	})

	server := &Server{mux: http.NewServeMux(), sessions: map[string]SessionInfo{}}
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
	server := &Server{
		cfg: config.Config{EnableDebugAPI: false},
		mux: http.NewServeMux(),
		sessions: map[string]SessionInfo{
			"admin-token": {UserID: "admin", Username: "admin", Role: "admin", Expires: time.Now().Add(time.Hour)},
		},
	}
	server.register()

	req := httptest.NewRequest(http.MethodPost, "/api/debug/model-tool-test", nil)
	req.AddCookie(&http.Cookie{Name: "perseus_session", Value: "admin-token"})
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for disabled debug endpoint, got %d: %s", rec.Code, rec.Body.String())
	}
}
