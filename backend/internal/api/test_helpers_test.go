package api

import (
	"net/http"
	"testing"
	"time"

	"webide-backend/internal/store"
)

func attachTestSession(t *testing.T, server *Server, req *http.Request, token string, user store.User) {
	t.Helper()
	if server.sessions == nil {
		t.Fatalf("server.sessions is nil")
	}
	if err := server.sessions.CreateSession(req.Context(), token, user.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: "perseus_session", Value: token})
}
