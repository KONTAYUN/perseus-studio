package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"webide-backend/internal/store"
	"webide-backend/internal/usersvc"
)

func TestDiagnosticLogsAPIRequiresAdmin(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	users := store.NewUserStore(db)
	if err := users.Migrate(t.Context()); err != nil {
		t.Fatalf("users.Migrate returned error: %v", err)
	}
	userA, err := users.CreateUser(t.Context(), "user-a", "password", "user")
	if err != nil {
		t.Fatalf("CreateUser user-a returned error: %v", err)
	}
	userB, err := users.CreateUser(t.Context(), "user-b", "password", "user")
	if err != nil {
		t.Fatalf("CreateUser user-b returned error: %v", err)
	}
	sessions := store.NewSessionStore(db)
	if err := sessions.Migrate(t.Context()); err != nil {
		t.Fatalf("sessions.Migrate returned error: %v", err)
	}
	diagnostics := store.NewDiagnosticStore(db)
	if err := diagnostics.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	if _, err := diagnostics.Append(t.Context(), store.DiagnosticLogInput{UserID: userA.ID, Stage: "a", Level: "error"}); err != nil {
		t.Fatalf("Append user-a returned error: %v", err)
	}
	if _, err := diagnostics.Append(t.Context(), store.DiagnosticLogInput{UserID: userB.ID, Stage: "b", Level: "error"}); err != nil {
		t.Fatalf("Append user-b returned error: %v", err)
	}

	server := &Server{
		users:       users,
		diagnostics: diagnostics,
		mux:         http.NewServeMux(),
		sessions:    sessions,
	}
	server.register()

	req := httptest.NewRequest(http.MethodGet, "/api/diagnostics/logs?limit=10", nil)
	attachTestSession(t, server, req, "token-a", userA)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected non-admin 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminDiagnosticLogsAPIListsAllUsers(t *testing.T) {
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
	userA, err := users.CreateUser(t.Context(), "user-a", "password", "user")
	if err != nil {
		t.Fatalf("CreateUser user-a returned error: %v", err)
	}
	userB, err := users.CreateUser(t.Context(), "user-b", "password", "user")
	if err != nil {
		t.Fatalf("CreateUser user-b returned error: %v", err)
	}
	sessions := store.NewSessionStore(db)
	if err := sessions.Migrate(t.Context()); err != nil {
		t.Fatalf("sessions.Migrate returned error: %v", err)
	}
	diagnostics := store.NewDiagnosticStore(db)
	if err := diagnostics.Migrate(t.Context()); err != nil {
		t.Fatalf("diagnostics.Migrate returned error: %v", err)
	}
	if _, err := diagnostics.Append(t.Context(), store.DiagnosticLogInput{UserID: userA.ID, Stage: "a", Level: "error"}); err != nil {
		t.Fatalf("Append user-a returned error: %v", err)
	}
	if _, err := diagnostics.Append(t.Context(), store.DiagnosticLogInput{UserID: userB.ID, Stage: "b", Level: "warn"}); err != nil {
		t.Fatalf("Append user-b returned error: %v", err)
	}

	server := &Server{
		users:       users,
		diagnostics: diagnostics,
		mux:         http.NewServeMux(),
		sessions:    sessions,
	}
	server.register()

	forbiddenReq := httptest.NewRequest(http.MethodGet, "/api/admin/diagnostics/logs?limit=10", nil)
	attachTestSession(t, server, forbiddenReq, "user-token", userA)
	forbiddenRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(forbiddenRec, forbiddenReq)
	if forbiddenRec.Code != http.StatusForbidden {
		t.Fatalf("expected non-admin 403, got %d: %s", forbiddenRec.Code, forbiddenRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/diagnostics/logs?limit=10", nil)
	attachTestSession(t, server, req, "admin-token", admin)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Logs  []store.DiagnosticLog            `json:"logs"`
		Users map[string]diagnosticUserSummary `json:"users"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	seen := make(map[string]bool)
	for _, item := range payload.Logs {
		seen[item.UserID] = true
	}
	if len(payload.Logs) != 2 || !seen[userA.ID] || !seen[userB.ID] {
		t.Fatalf("expected logs from both users, got %#v", payload.Logs)
	}
	if payload.Users[userA.ID].Username != "user-a" || payload.Users[userB.ID].Username != "user-b" {
		t.Fatalf("expected user summaries, got %#v", payload.Users)
	}

	exportReq := httptest.NewRequest(http.MethodGet, "/api/admin/diagnostics/export?limit=10", nil)
	attachTestSession(t, server, exportReq, "admin-export-token", admin)
	exportRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(exportRec, exportReq)
	if exportRec.Code != http.StatusOK {
		t.Fatalf("expected export 200, got %d: %s", exportRec.Code, exportRec.Body.String())
	}
	exportText := exportRec.Body.String()
	for _, expected := range []string{"scope: all_users", "username: user-a", "username: user-b"} {
		if !strings.Contains(exportText, expected) {
			t.Fatalf("export missing %q: %s", expected, exportText)
		}
	}
}

func TestDiagnosticExportDoesNotIncludeSecrets(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	users := store.NewUserStore(db)
	if err := users.Migrate(t.Context()); err != nil {
		t.Fatalf("users.Migrate returned error: %v", err)
	}
	userA, err := users.CreateUser(t.Context(), "user-a", "password", "user")
	if err != nil {
		t.Fatalf("CreateUser user-a returned error: %v", err)
	}
	sessions := store.NewSessionStore(db)
	if err := sessions.Migrate(t.Context()); err != nil {
		t.Fatalf("sessions.Migrate returned error: %v", err)
	}
	diagnostics := store.NewDiagnosticStore(db)
	if err := diagnostics.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	if _, err := diagnostics.Append(t.Context(), store.DiagnosticLogInput{
		UserID:          userA.ID,
		Stage:           "provider_error",
		Level:           "error",
		BaseURLHost:     "backend.intelalloc.com",
		FinalURLPath:    "/v1/chat/completions",
		ErrorMessage:    "模型供应商返回非成功响应",
		ResponseSnippet: "errorcode:502",
		RequestSummary:  `{"request_kind":"chat_completions","tool_names":["attempt_completion"],"failure_class":"provider_bad_request_with_tools"}`,
	}); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}

	server := &Server{
		diagnostics: diagnostics,
		mux:         http.NewServeMux(),
		sessions:    sessions,
	}
	server.register()

	req := httptest.NewRequest(http.MethodGet, "/api/admin/diagnostics/export?limit=10", nil)
	attachTestSession(t, server, req, "token-a", userA)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected non-admin 403, got %d: %s", rec.Code, rec.Body.String())
	}

	admin, err := users.CreateUser(t.Context(), "admin", "password", "admin")
	if err != nil {
		t.Fatalf("CreateUser admin returned error: %v", err)
	}
	adminReq := httptest.NewRequest(http.MethodGet, "/api/admin/diagnostics/export?limit=10", nil)
	attachTestSession(t, server, adminReq, "admin-token-a", admin)
	adminRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(adminRec, adminReq)
	if adminRec.Code != http.StatusOK {
		t.Fatalf("expected admin 200, got %d: %s", adminRec.Code, adminRec.Body.String())
	}
	text := adminRec.Body.String()
	for _, forbidden := range []string{"sk-", "api_key", "prompt", "完整文件内容"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("export contains forbidden fragment %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, "backend.intelalloc.com") || !strings.Contains(text, "errorcode:502") {
		t.Fatalf("export missing expected diagnostic fields: %s", text)
	}
	if !strings.Contains(text, `"failure_class": "provider_bad_request_with_tools"`) ||
		!strings.Contains(text, `"tool_names": [`) {
		t.Fatalf("export missing formatted request_summary: %s", text)
	}
}

func TestDiagnosticLogsAPIDeleteSingleAndAdminClearAll(t *testing.T) {
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
	userA, err := users.CreateUser(t.Context(), "user-a", "password", "user")
	if err != nil {
		t.Fatalf("CreateUser user-a returned error: %v", err)
	}
	userB, err := users.CreateUser(t.Context(), "user-b", "password", "user")
	if err != nil {
		t.Fatalf("CreateUser user-b returned error: %v", err)
	}
	sessions := store.NewSessionStore(db)
	if err := sessions.Migrate(t.Context()); err != nil {
		t.Fatalf("sessions.Migrate returned error: %v", err)
	}
	diagnostics := store.NewDiagnosticStore(db)
	if err := diagnostics.Migrate(t.Context()); err != nil {
		t.Fatalf("diagnostics.Migrate returned error: %v", err)
	}
	logA, err := diagnostics.Append(t.Context(), store.DiagnosticLogInput{UserID: userA.ID, Stage: "a", Level: "error"})
	if err != nil {
		t.Fatalf("Append user-a returned error: %v", err)
	}
	logB, err := diagnostics.Append(t.Context(), store.DiagnosticLogInput{UserID: userB.ID, Stage: "b", Level: "warn"})
	if err != nil {
		t.Fatalf("Append user-b returned error: %v", err)
	}

	server := &Server{
		users:       users,
		diagnostics: diagnostics,
		mux:         http.NewServeMux(),
		sessions:    sessions,
	}
	server.register()

	wrongOwnerReq := httptest.NewRequest(http.MethodDelete, "/api/diagnostics/logs/"+logB.ID, nil)
	attachTestSession(t, server, wrongOwnerReq, "user-a-token", userA)
	wrongOwnerRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(wrongOwnerRec, wrongOwnerReq)
	if wrongOwnerRec.Code != http.StatusForbidden {
		t.Fatalf("expected non-admin delete 403, got %d: %s", wrongOwnerRec.Code, wrongOwnerRec.Body.String())
	}
	logs, err := diagnostics.List(t.Context(), store.DiagnosticLogQuery{UserID: userB.ID, Limit: 10})
	if err != nil {
		t.Fatalf("List user-b returned error: %v", err)
	}
	if len(logs) != 1 || logs[0].ID != logB.ID {
		t.Fatalf("wrong owner delete removed user-b log: %#v", logs)
	}

	deleteOwnReq := httptest.NewRequest(http.MethodDelete, "/api/admin/diagnostics/logs/"+logA.ID, nil)
	attachTestSession(t, server, deleteOwnReq, "user-a-token-2", userA)
	deleteOwnRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(deleteOwnRec, deleteOwnReq)
	if deleteOwnRec.Code != http.StatusForbidden {
		t.Fatalf("expected non-admin admin-route delete 403, got %d: %s", deleteOwnRec.Code, deleteOwnRec.Body.String())
	}
	logs, err = diagnostics.List(t.Context(), store.DiagnosticLogQuery{UserID: userA.ID, Limit: 10})
	if err != nil {
		t.Fatalf("List user-a returned error: %v", err)
	}
	if len(logs) != 1 || logs[0].ID != logA.ID {
		t.Fatalf("expected user-a log to remain, got %#v", logs)
	}

	adminDeleteReq := httptest.NewRequest(http.MethodDelete, "/api/admin/diagnostics/logs/"+logB.ID, nil)
	attachTestSession(t, server, adminDeleteReq, "admin-token", admin)
	adminDeleteRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(adminDeleteRec, adminDeleteReq)
	if adminDeleteRec.Code != http.StatusOK {
		t.Fatalf("expected admin delete 200, got %d: %s", adminDeleteRec.Code, adminDeleteRec.Body.String())
	}
	logs, err = diagnostics.List(t.Context(), store.DiagnosticLogQuery{AllUsers: true, Limit: 10})
	if err != nil {
		t.Fatalf("List all returned error: %v", err)
	}
	if len(logs) != 1 || logs[0].ID != logA.ID {
		t.Fatalf("expected admin single delete to remove only log-b, got %#v", logs)
	}

	if _, err := diagnostics.Append(t.Context(), store.DiagnosticLogInput{UserID: userA.ID, Stage: "a2", Level: "error"}); err != nil {
		t.Fatalf("Append user-a again returned error: %v", err)
	}
	if _, err := diagnostics.Append(t.Context(), store.DiagnosticLogInput{UserID: userB.ID, Stage: "b2", Level: "error"}); err != nil {
		t.Fatalf("Append user-b again returned error: %v", err)
	}
	clearAllReq := httptest.NewRequest(http.MethodDelete, "/api/admin/diagnostics/logs", nil)
	attachTestSession(t, server, clearAllReq, "admin-token-2", admin)
	clearAllRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(clearAllRec, clearAllReq)
	if clearAllRec.Code != http.StatusOK {
		t.Fatalf("expected admin clear all 200, got %d: %s", clearAllRec.Code, clearAllRec.Body.String())
	}
	logs, err = diagnostics.List(t.Context(), store.DiagnosticLogQuery{AllUsers: true, Limit: 10})
	if err != nil {
		t.Fatalf("List all after clear returned error: %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("expected admin clear all to delete every log, got %#v", logs)
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
	sessions := store.NewSessionStore(db)
	if err := sessions.Migrate(t.Context()); err != nil {
		t.Fatalf("sessions.Migrate returned error: %v", err)
	}

	server := &Server{
		users:       users,
		diagnostics: diagnostics,
		factory:     usersvc.NewFactory(db, filepath.Join(t.TempDir(), "users"), skills),
		mux:         http.NewServeMux(),
		sessions:    sessions,
	}
	server.register()

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/users/"+target.ID, nil)
	attachTestSession(t, server, req, "admin-token", admin)
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
