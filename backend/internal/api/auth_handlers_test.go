package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"webide-backend/internal/store"
)

func TestAuthSessionSurvivesServerRecreation(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	users := store.NewUserStore(db)
	if err := users.Migrate(t.Context()); err != nil {
		t.Fatalf("users.Migrate returned error: %v", err)
	}
	if _, err := users.CreateUser(t.Context(), "admin", "password", "admin"); err != nil {
		t.Fatalf("CreateUser returned error: %v", err)
	}
	sessions := store.NewSessionStore(db)
	if err := sessions.Migrate(t.Context()); err != nil {
		t.Fatalf("sessions.Migrate returned error: %v", err)
	}

	server := &Server{users: users, sessions: sessions, mux: http.NewServeMux()}
	server.register()
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"username":"admin","password":"password"}`))
	loginRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d: %s", loginRec.Code, loginRec.Body.String())
	}
	var sessionCookie *http.Cookie
	for _, cookie := range loginRec.Result().Cookies() {
		if cookie.Name == "perseus_session" {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatalf("expected session cookie, got %#v", loginRec.Result().Cookies())
	}

	recreatedServer := &Server{users: users, sessions: store.NewSessionStore(db), mux: http.NewServeMux()}
	recreatedServer.register()
	meReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meReq.AddCookie(sessionCookie)
	meRec := httptest.NewRecorder()
	recreatedServer.Routes().ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("expected me 200, got %d: %s", meRec.Code, meRec.Body.String())
	}
	var payload struct {
		Authenticated bool `json:"authenticated"`
		User          struct {
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"user"`
	}
	if err := json.Unmarshal(meRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if !payload.Authenticated || payload.User.Username != "admin" || payload.User.Role != "admin" {
		t.Fatalf("unexpected auth payload: %#v", payload)
	}
}

func TestRegisterUsesDefaultPendingRoleWithoutSession(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	users := store.NewUserStore(db)
	if err := users.Migrate(t.Context()); err != nil {
		t.Fatalf("users.Migrate returned error: %v", err)
	}
	if _, err := users.CreateUser(t.Context(), "admin", "password", store.UserRoleAdmin); err != nil {
		t.Fatalf("CreateUser admin returned error: %v", err)
	}
	if err := users.SetDefaultUserRole(t.Context(), store.UserRolePending); err != nil {
		t.Fatalf("SetDefaultUserRole returned error: %v", err)
	}
	sessions := store.NewSessionStore(db)
	if err := sessions.Migrate(t.Context()); err != nil {
		t.Fatalf("sessions.Migrate returned error: %v", err)
	}
	server := &Server{users: users, sessions: sessions, mux: http.NewServeMux()}
	server.register()

	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewBufferString(`{"username":"new-user","password":"password"}`))
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected register 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Authenticated bool `json:"authenticated"`
		Pending       bool `json:"pending"`
		User          struct {
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if payload.Authenticated || !payload.Pending || payload.User.Role != store.UserRolePending {
		t.Fatalf("unexpected register payload: %#v", payload)
	}
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == "perseus_session" && cookie.Value != "" {
			t.Fatalf("pending registration must not receive session cookie")
		}
	}
	user, err := users.GetUserByUsername(t.Context(), "new-user")
	if err != nil {
		t.Fatalf("GetUserByUsername returned error: %v", err)
	}
	if user.Role != store.UserRolePending {
		t.Fatalf("expected stored pending role, got %q", user.Role)
	}
}

func TestFirstRegisteredUserIsAdminWhenDefaultRolePending(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	users := store.NewUserStore(db)
	if err := users.Migrate(t.Context()); err != nil {
		t.Fatalf("users.Migrate returned error: %v", err)
	}
	if err := users.SetDefaultUserRole(t.Context(), store.UserRolePending); err != nil {
		t.Fatalf("SetDefaultUserRole returned error: %v", err)
	}
	sessions := store.NewSessionStore(db)
	if err := sessions.Migrate(t.Context()); err != nil {
		t.Fatalf("sessions.Migrate returned error: %v", err)
	}
	server := &Server{users: users, sessions: sessions, mux: http.NewServeMux()}
	server.register()

	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewBufferString(`{"username":"owner","password":"password"}`))
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected register 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Authenticated bool `json:"authenticated"`
		User          struct {
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if !payload.Authenticated || payload.User.Role != store.UserRoleAdmin {
		t.Fatalf("unexpected first user payload: %#v", payload)
	}
}

func TestPendingUserCannotLoginUntilActivated(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	users := store.NewUserStore(db)
	if err := users.Migrate(t.Context()); err != nil {
		t.Fatalf("users.Migrate returned error: %v", err)
	}
	admin, err := users.CreateUser(t.Context(), "admin", "password", store.UserRoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser admin returned error: %v", err)
	}
	pending, err := users.CreateUser(t.Context(), "pending-user", "password", store.UserRolePending)
	if err != nil {
		t.Fatalf("CreateUser pending returned error: %v", err)
	}
	sessions := store.NewSessionStore(db)
	if err := sessions.Migrate(t.Context()); err != nil {
		t.Fatalf("sessions.Migrate returned error: %v", err)
	}
	server := &Server{users: users, sessions: sessions, mux: http.NewServeMux()}
	server.register()

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"username":"pending-user","password":"password"}`))
	loginRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected pending login 401, got %d: %s", loginRec.Code, loginRec.Body.String())
	}

	activateReq := httptest.NewRequest(http.MethodPatch, "/api/admin/users/"+pending.ID, bytes.NewBufferString(`{"role":"user"}`))
	attachTestSession(t, server, activateReq, "admin-token", admin)
	activateRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(activateRec, activateReq)
	if activateRec.Code != http.StatusOK {
		t.Fatalf("expected activate 200, got %d: %s", activateRec.Code, activateRec.Body.String())
	}

	loginReq = httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"username":"pending-user","password":"password"}`))
	loginRec = httptest.NewRecorder()
	server.Routes().ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected activated login 200, got %d: %s", loginRec.Code, loginRec.Body.String())
	}
}

func TestAdminSettingsDefaultUserRoleRoundTrip(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	users := store.NewUserStore(db)
	if err := users.Migrate(t.Context()); err != nil {
		t.Fatalf("users.Migrate returned error: %v", err)
	}
	admin, err := users.CreateUser(t.Context(), "admin", "password", store.UserRoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser admin returned error: %v", err)
	}
	sessions := store.NewSessionStore(db)
	if err := sessions.Migrate(t.Context()); err != nil {
		t.Fatalf("sessions.Migrate returned error: %v", err)
	}
	server := &Server{users: users, sessions: sessions, mux: http.NewServeMux()}
	server.register()

	getReq := httptest.NewRequest(http.MethodGet, "/api/admin/settings", nil)
	attachTestSession(t, server, getReq, "admin-token-a", admin)
	getRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected settings 200, got %d: %s", getRec.Code, getRec.Body.String())
	}
	var payload struct {
		DefaultUserRole string `json:"default_user_role"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal settings returned error: %v", err)
	}
	if payload.DefaultUserRole != store.UserRolePending {
		t.Fatalf("expected default pending, got %q", payload.DefaultUserRole)
	}

	patchReq := httptest.NewRequest(http.MethodPatch, "/api/admin/settings", bytes.NewBufferString(`{"default_user_role":"user"}`))
	attachTestSession(t, server, patchReq, "admin-token-b", admin)
	patchRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("expected settings patch 200, got %d: %s", patchRec.Code, patchRec.Body.String())
	}
	if err := json.Unmarshal(patchRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal patched settings returned error: %v", err)
	}
	if payload.DefaultUserRole != store.UserRoleUser {
		t.Fatalf("expected default user, got %q", payload.DefaultUserRole)
	}
}

func TestLastAdminCannotBecomePending(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	users := store.NewUserStore(db)
	if err := users.Migrate(t.Context()); err != nil {
		t.Fatalf("users.Migrate returned error: %v", err)
	}
	admin, err := users.CreateUser(t.Context(), "admin", "password", store.UserRoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser admin returned error: %v", err)
	}
	sessions := store.NewSessionStore(db)
	if err := sessions.Migrate(t.Context()); err != nil {
		t.Fatalf("sessions.Migrate returned error: %v", err)
	}
	server := &Server{users: users, sessions: sessions, mux: http.NewServeMux()}
	server.register()

	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/"+admin.ID, bytes.NewBufferString(`{"role":"pending"}`))
	attachTestSession(t, server, req, "admin-token", admin)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected last admin role change 400, got %d: %s", rec.Code, rec.Body.String())
	}
	user, err := users.GetUser(t.Context(), admin.ID)
	if err != nil {
		t.Fatalf("GetUser returned error: %v", err)
	}
	if user.Role != store.UserRoleAdmin {
		t.Fatalf("expected last admin to remain admin, got %q", user.Role)
	}
}
