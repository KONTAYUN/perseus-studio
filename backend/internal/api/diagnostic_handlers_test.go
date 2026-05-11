package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"webide-backend/internal/store"
	"webide-backend/internal/usersvc"
)

func TestDiagnosticLogsAPIUsesCurrentUser(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	diagnostics := store.NewDiagnosticStore(db)
	if err := diagnostics.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	if _, err := diagnostics.Append(t.Context(), store.DiagnosticLogInput{UserID: "user-a", Stage: "a", Level: "error"}); err != nil {
		t.Fatalf("Append user-a returned error: %v", err)
	}
	if _, err := diagnostics.Append(t.Context(), store.DiagnosticLogInput{UserID: "user-b", Stage: "b", Level: "error"}); err != nil {
		t.Fatalf("Append user-b returned error: %v", err)
	}

	server := &Server{
		diagnostics: diagnostics,
		mux:         http.NewServeMux(),
		sessions: map[string]SessionInfo{
			"token-a": {UserID: "user-a", Username: "a", Role: "user", Expires: time.Now().Add(time.Hour)},
		},
	}
	server.register()

	req := httptest.NewRequest(http.MethodGet, "/api/diagnostics/logs?limit=10", nil)
	req.AddCookie(&http.Cookie{Name: "perseus_session", Value: "token-a"})
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Logs []store.DiagnosticLog `json:"logs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if len(payload.Logs) != 1 || payload.Logs[0].UserID != "user-a" {
		t.Fatalf("expected only user-a logs, got %#v", payload.Logs)
	}
}

func TestDiagnosticExportDoesNotIncludeSecrets(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	diagnostics := store.NewDiagnosticStore(db)
	if err := diagnostics.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	if _, err := diagnostics.Append(t.Context(), store.DiagnosticLogInput{
		UserID:          "user-a",
		Stage:           "provider_error",
		Level:           "error",
		BaseURLHost:     "backend.intelalloc.com",
		FinalURLPath:    "/v1/chat/completions",
		ErrorMessage:    "模型供应商返回非成功响应",
		ResponseSnippet: "errorcode:502",
	}); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}

	server := &Server{
		diagnostics: diagnostics,
		mux:         http.NewServeMux(),
		sessionMu:   sync.Mutex{},
		sessions: map[string]SessionInfo{
			"token-a": {UserID: "user-a", Username: "a", Role: "user", Expires: time.Now().Add(time.Hour)},
		},
	}
	server.register()

	req := httptest.NewRequest(http.MethodGet, "/api/diagnostics/export?limit=10", nil)
	req.AddCookie(&http.Cookie{Name: "perseus_session", Value: "token-a"})
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	text := rec.Body.String()
	for _, forbidden := range []string{"sk-", "api_key", "prompt", "完整文件内容"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("export contains forbidden fragment %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, "backend.intelalloc.com") || !strings.Contains(text, "errorcode:502") {
		t.Fatalf("export missing expected diagnostic fields: %s", text)
	}
}

func TestAdminDeleteUserClearsDiagnosticLogs(t *testing.T) {
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
	target, err := users.CreateUser(t.Context(), "target", "password", "user")
	if err != nil {
		t.Fatalf("CreateUser target returned error: %v", err)
	}
	diagnostics := store.NewDiagnosticStore(db)
	if err := diagnostics.Migrate(t.Context()); err != nil {
		t.Fatalf("diagnostics.Migrate returned error: %v", err)
	}
	if _, err := diagnostics.Append(t.Context(), store.DiagnosticLogInput{UserID: target.ID, Stage: "s", Level: "error"}); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}
	skills := store.NewSkillStore(db)
	if err := skills.Migrate(t.Context()); err != nil {
		t.Fatalf("skills.Migrate returned error: %v", err)
	}

	server := &Server{
		users:       users,
		diagnostics: diagnostics,
		factory:     usersvc.NewFactory(db, filepath.Join(t.TempDir(), "users"), skills),
		mux:         http.NewServeMux(),
		sessions: map[string]SessionInfo{
			"admin-token": {UserID: admin.ID, Username: admin.Username, Role: "admin", Expires: time.Now().Add(time.Hour)},
		},
	}
	server.register()

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/users/"+target.ID, nil)
	req.AddCookie(&http.Cookie{Name: "perseus_session", Value: "admin-token"})
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	logs, err := diagnostics.List(t.Context(), store.DiagnosticLogQuery{UserID: target.ID, Limit: 10})
	if err != nil {
		t.Fatalf("diagnostics.List returned error: %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("expected target diagnostics cleared, got %#v", logs)
	}
}
