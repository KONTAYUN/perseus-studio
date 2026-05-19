package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"webide-backend/internal/store"
)

func (s *Server) handleModelProviders(w http.ResponseWriter, r *http.Request) {
	user := MustUserFromContext(r.Context())
	switch r.Method {
	case http.MethodGet:
		items, err := s.configs.ListModelProviders(r.Context(), user.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		var req store.ModelProviderInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		item, err := s.configs.CreateModelProvider(r.Context(), user.UserID, req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, item)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleModelProviderRoutes(w http.ResponseWriter, r *http.Request) {
	user := MustUserFromContext(r.Context())
	rest := strings.TrimPrefix(r.URL.Path, "/api/model-providers/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]

	if len(parts) == 2 && parts[1] == "set-default" {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		item, err := s.configs.SetDefaultModelProvider(r.Context(), user.UserID, id)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}

	if len(parts) != 1 {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodPatch:
		var req store.ModelProviderInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		item, err := s.configs.UpdateModelProvider(r.Context(), user.UserID, id, req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, item)
	case http.MethodDelete:
		if err := s.configs.DeleteModelProvider(r.Context(), user.UserID, id); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id})
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleDiscoverModelProviders(w http.ResponseWriter, r *http.Request) {
	user := MustUserFromContext(r.Context())
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var req discoverModelsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	req.Protocol = strings.ToLower(strings.TrimSpace(req.Protocol))
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	req.APIKey = strings.TrimSpace(req.APIKey)

	if req.ProviderID != "" && req.APIKey == "" {
		existing, err := s.configs.GetModelProvider(r.Context(), user.UserID, req.ProviderID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		req.APIKey = existing.APIKey
		if req.BaseURL == "" {
			req.BaseURL = existing.BaseURL
		}
		if req.Protocol == "" {
			req.Protocol = existing.Protocol
		}
	}

	models, err := discoverModels(r, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

func (s *Server) handleTestModelProvider(w http.ResponseWriter, r *http.Request) {
	user := MustUserFromContext(r.Context())
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var req testModelProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	req.Protocol = strings.ToLower(strings.TrimSpace(req.Protocol))
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	req.APIKey = strings.TrimSpace(req.APIKey)
	req.ModelID = strings.TrimSpace(req.ModelID)
	if req.ProviderID != "" && req.APIKey == "" {
		existing, err := s.configs.GetModelProvider(r.Context(), user.UserID, req.ProviderID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		req.APIKey = existing.APIKey
		if req.BaseURL == "" {
			req.BaseURL = existing.BaseURL
		}
		if req.Protocol == "" {
			req.Protocol = existing.Protocol
		}
		if req.ModelID == "" {
			req.ModelID = existing.ModelID
		}
	}
	report := runModelProviderTest(r, req)
	writeJSON(w, http.StatusOK, report)
}
