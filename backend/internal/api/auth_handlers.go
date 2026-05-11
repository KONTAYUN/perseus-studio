package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"webide-backend/internal/store"
)

type contextKey string

const ctxSessionKey contextKey = "session"

func UserFromContext(ctx context.Context) *SessionInfo {
	v, _ := ctx.Value(ctxSessionKey).(*SessionInfo)
	return v
}

func MustUserFromContext(ctx context.Context) SessionInfo {
	v := UserFromContext(ctx)
	if v == nil {
		return SessionInfo{}
	}
	return *v
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	user, err := s.users.Authenticate(r.Context(), req.Username, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	s.createSessionAndRespond(w, user)
}

func (s *Server) handleAuthRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	count, err := s.users.CountUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if count > 0 {
		allowed, err := s.users.IsRegistrationAllowed(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !allowed {
			writeError(w, http.StatusForbidden, "registration is disabled")
			return
		}
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	role := "user"
	if count == 0 {
		role = "admin"
	}
	user, err := s.users.CreateUser(r.Context(), req.Username, req.Password, role)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.createSessionAndRespond(w, user)
}

func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	session := UserFromContext(r.Context())
	if session == nil {
		count, _ := s.users.CountUsers(r.Context())
		allowReg := true
		if count > 0 {
			allowReg, _ = s.users.IsRegistrationAllowed(r.Context())
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"authenticated":     false,
			"has_users":         count > 0,
			"registration_open": allowReg || count == 0,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"user": map[string]any{
			"id":       session.UserID,
			"username": session.Username,
			"role":     session.Role,
		},
	})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if cookie, err := r.Cookie("perseus_session"); err == nil {
		s.sessionMu.Lock()
		delete(s.sessions, cookie.Value)
		s.sessionMu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "perseus_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
}

func (s *Server) createSessionAndRespond(w http.ResponseWriter, user store.User) {
	token, err := randomSessionToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	expires := time.Now().Add(30 * 24 * time.Hour)
	info := SessionInfo{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		Expires:  expires,
	}
	s.sessionMu.Lock()
	s.sessions[token] = info
	s.sessionMu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     "perseus_session",
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"user": map[string]any{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
		},
	})
}

func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions || !strings.HasPrefix(r.URL.Path, "/api/") || s.isPublicAPI(r.URL.Path) {
			session := s.getSession(r)
			if session != nil {
				r = r.WithContext(context.WithValue(r.Context(), ctxSessionKey, session))
			}
			next.ServeHTTP(w, r)
			return
		}
		session := s.getSession(r)
		if session == nil {
			writeError(w, http.StatusUnauthorized, "请先登录 Perseus Studio")
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), ctxSessionKey, session))
		next.ServeHTTP(w, r)
	})
}

func (s *Server) isPublicAPI(path string) bool {
	return path == "/api/health" ||
		path == "/api/version" ||
		path == "/api/auth/login" ||
		path == "/api/auth/register" ||
		path == "/api/auth/me" ||
		path == "/api/auth/logout"
}

func (s *Server) getSession(r *http.Request) *SessionInfo {
	cookie, err := r.Cookie("perseus_session")
	if err != nil || cookie.Value == "" {
		return nil
	}
	now := time.Now()
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	info, ok := s.sessions[cookie.Value]
	if !ok {
		return nil
	}
	if now.After(info.Expires) {
		delete(s.sessions, cookie.Value)
		return nil
	}
	return &info
}

func (s *Server) requireAdmin(r *http.Request) bool {
	session := UserFromContext(r.Context())
	return session != nil && session.Role == "admin"
}

func randomSessionToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
