package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"webide-backend/internal/store"
)

func (s *Server) handleSearchProviders(w http.ResponseWriter, r *http.Request) {
	user := MustUserFromContext(r.Context())
	switch r.Method {
	case http.MethodGet:
		items, err := s.configs.ListSearchProviders(r.Context(), user.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		var req store.SearchProviderInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		item, err := s.configs.CreateSearchProvider(r.Context(), user.UserID, req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, item)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleSearchProviderRoutes(w http.ResponseWriter, r *http.Request) {
	user := MustUserFromContext(r.Context())
	rest := strings.TrimPrefix(r.URL.Path, "/api/search-providers/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 1 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]

	switch r.Method {
	case http.MethodPatch:
		var req store.SearchProviderInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		item, err := s.configs.UpdateSearchProvider(r.Context(), user.UserID, id, req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, item)
	case http.MethodDelete:
		if err := s.configs.DeleteSearchProvider(r.Context(), user.UserID, id); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id})
	default:
		http.NotFound(w, r)
	}
}
