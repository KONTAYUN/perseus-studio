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
	loginKey := s.clientIP(r) + "\x00" + strings.ToLower(strings.TrimSpace(req.Username))
	if !s.allowLoginRate(w, loginKey) {
		return
	}
	user, err := s.users.Authenticate(r.Context(), req.Username, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	s.createSessionAndRespond(r.Context(), w, r, user)
}

func (s *Server) handleAuthRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if !s.allowRate(w, s.registerLimiter, s.clientIP(r), s.cfg.RegisterRatePerHour, time.Hour, "auth_rate_limited") {
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
	role, err := s.users.DefaultUserRole(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if count == 0 {
		role = store.UserRoleAdmin
	}
	user, err := s.users.CreateUser(r.Context(), req.Username, req.Password, role)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if user.Role == store.UserRolePending {
		writeJSON(w, http.StatusCreated, map[string]any{
			"authenticated": false,
			"pending":       true,
			"user": map[string]any{
				"id":       user.ID,
				"username": user.Username,
				"role":     user.Role,
			},
		})
		return
	}
	s.createSessionAndRespond(r.Context(), w, r, user)
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
	if cookie, err := r.Cookie("perseus_session"); err == nil && cookie.Value != "" && s.sessions != nil {
		_ = s.sessions.DeleteSession(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "perseus_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.sessionCookieSecure(r),
	})
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
}

func (s *Server) createSessionAndRespond(ctx context.Context, w http.ResponseWriter, r *http.Request, user store.User) {
	token, err := randomSessionToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	expires := time.Now().Add(30 * 24 * time.Hour)
	if s.sessions != nil {
		if err := s.sessions.CreateSession(ctx, token, user.ID, expires); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create session")
			return
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "perseus_session",
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.sessionCookieSecure(r),
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

func (s *Server) sessionCookieSecure(r *http.Request) bool {
	switch strings.ToLower(strings.TrimSpace(s.cfg.SessionCookieSecure)) {
	case "true":
		return true
	case "false":
		return false
	default:
		return s.requestIsHTTPS(r)
	}
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
		path == "/api/ready" ||
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
	if s.sessions == nil {
		return nil
	}
	info, ok, err := s.sessions.GetSession(r.Context(), cookie.Value, now)
	if err != nil || !ok {
		return nil
	}
	session := SessionInfo{
		UserID:   info.UserID,
		Username: info.Username,
		Role:     info.Role,
		Expires:  info.Expires,
	}
	return &session
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
