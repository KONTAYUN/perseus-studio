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
		items, err := s.configs.ListModelProvidersForUser(r.Context(), user.UserID, user.Role)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": modelProvidersForResponse(user.Role, items)})
	case http.MethodPost:
		if !s.requireAdminOrWriteForbidden(w, r) {
			return
		}
		var req struct {
			store.ModelProviderInput
			CredentialProviderID string `json:"credential_provider_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		if req.APIKey == "" && req.CredentialProviderID != "" {
			existing, err := s.configs.GetModelProviderAny(r.Context(), req.CredentialProviderID)
			if err != nil {
				writeError(w, http.StatusBadRequest, "保存的凭据不可用，请重新选择模型配置或输入 API Key")
				return
			}
			req.APIKey = existing.APIKey
		}
		item, err := s.configs.CreateModelProvider(r.Context(), user.UserID, req.ModelProviderInput)
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
		if !s.requireAdminOrWriteForbidden(w, r) {
			return
		}
		item, err := s.configs.SetDefaultModelProviderAny(r.Context(), id)
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
		if !s.requireAdminOrWriteForbidden(w, r) {
			return
		}
		existing, err := s.configs.GetModelProviderAny(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		var req store.ModelProviderInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		item, err := s.configs.UpdateModelProvider(r.Context(), existing.UserID, id, req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, item)
	case http.MethodDelete:
		if !s.requireAdminOrWriteForbidden(w, r) {
			return
		}
		existing, err := s.configs.GetModelProviderAny(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.configs.DeleteModelProvider(r.Context(), existing.UserID, id); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id})
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleDiscoverModelProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if !s.requireAdminOrWriteForbidden(w, r) {
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
		existing, err := s.configs.GetModelProviderAny(r.Context(), req.ProviderID)
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
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if !s.requireAdminOrWriteForbidden(w, r) {
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
		existing, err := s.configs.GetModelProviderAny(r.Context(), req.ProviderID)
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
		if strings.TrimSpace(req.ReasoningCapability) == "" {
			req.ReasoningCapability = existing.ReasoningCapability
		}
		if strings.TrimSpace(req.ReasoningCapabilitySource) == "" {
			req.ReasoningCapabilitySource = existing.ReasoningCapabilitySource
		}
		if strings.TrimSpace(req.ReasoningProfileLevels) == "" {
			req.ReasoningProfileLevels = existing.ReasoningProfileLevels
		}
	}
	report := runModelProviderTest(r, req)
	writeJSON(w, http.StatusOK, report)
}
