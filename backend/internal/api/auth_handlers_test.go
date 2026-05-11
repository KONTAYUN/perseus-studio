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
