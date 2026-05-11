package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(r) {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		users, err := s.users.ListUsers(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": users})
	case http.MethodPost:
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Role     string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		if req.Role == "" {
			req.Role = "user"
		}
		user, err := s.users.CreateUser(r.Context(), req.Username, req.Password, req.Role)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, user)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleAdminUserRoutes(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(r) {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/admin/users/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	userID := parts[0]

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			user, err := s.users.GetUser(r.Context(), userID)
			if err != nil {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, user)
		case http.MethodPatch:
			var req struct {
				Role string `json:"role"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "invalid json body")
				return
			}
			if err := s.users.UpdateRole(r.Context(), userID, req.Role); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			s.invalidateUserSessions(userID)
			user, _ := s.users.GetUser(r.Context(), userID)
			writeJSON(w, http.StatusOK, user)
		case http.MethodDelete:
			if err := s.users.DeleteUser(r.Context(), userID); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			_ = s.diagnostics.ClearUser(r.Context(), userID)
			s.invalidateUserSessions(userID)
			_ = s.factory.RemoveUserData(userID)
			writeJSON(w, http.StatusOK, map[string]any{"id": userID})
		default:
			http.NotFound(w, r)
		}
		return
	}

	if len(parts) == 2 && r.Method == http.MethodPost {
		switch parts[1] {
		case "reset-password":
			var req struct {
				Password string `json:"password"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "invalid json body")
				return
			}
			if err := s.users.ResetPassword(r.Context(), userID, req.Password); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		case "disable":
			if err := s.users.SetDisabled(r.Context(), userID, true); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			s.invalidateUserSessions(userID)
			user, _ := s.users.GetUser(r.Context(), userID)
			writeJSON(w, http.StatusOK, user)
		case "enable":
			if err := s.users.SetDisabled(r.Context(), userID, false); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			user, _ := s.users.GetUser(r.Context(), userID)
			writeJSON(w, http.StatusOK, user)
		default:
			http.NotFound(w, r)
		}
		return
	}
	http.NotFound(w, r)
}

func (s *Server) handleAdminSettings(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(r) {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		allowReg, _ := s.users.IsRegistrationAllowed(r.Context())
		writeJSON(w, http.StatusOK, map[string]any{
			"allow_registration": allowReg,
		})
	case http.MethodPatch:
		var req struct {
			AllowRegistration *bool `json:"allow_registration"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		if req.AllowRegistration != nil {
			if err := s.users.SetRegistrationAllowed(r.Context(), *req.AllowRegistration); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		allowReg, _ := s.users.IsRegistrationAllowed(r.Context())
		writeJSON(w, http.StatusOK, map[string]any{
			"allow_registration": allowReg,
		})
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) invalidateUserSessions(userID string) {
	s.sessionMu.Lock()
	for token, info := range s.sessions {
		if info.UserID == userID {
			delete(s.sessions, token)
		}
	}
	s.sessionMu.Unlock()
}
