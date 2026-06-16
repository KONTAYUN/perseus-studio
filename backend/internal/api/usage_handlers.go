package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"webide-backend/internal/store"
)

func (s *Server) handleAdminUsageSummary(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(r) {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	if s.usage == nil {
		writeError(w, http.StatusInternalServerError, "usage store is not available")
		return
	}
	summary, err := s.usage.Summary(r.Context(), usageQueryFromRequest(r, 0))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	users, err := s.diagnosticUserSummaries(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	providers, err := s.configs.ListAllModelProviders(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"summary":   summary,
		"users":     users,
		"providers": modelProvidersForResponse("admin", providers),
	})
}

func (s *Server) handleAdminUsageEvents(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(r) {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	if s.usage == nil {
		writeError(w, http.StatusInternalServerError, "usage store is not available")
		return
	}
	events, err := s.usage.ListEvents(r.Context(), usageQueryFromRequest(r, 100))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (s *Server) handleAdminUsagePrices(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(r) {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}
	if s.usage == nil {
		writeError(w, http.StatusInternalServerError, "usage store is not available")
		return
	}
	switch r.Method {
	case http.MethodGet:
		prices, err := s.usage.ListPrices(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": prices})
	case http.MethodPost:
		var req store.LLMModelPriceInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		item, err := s.usage.CreatePrice(r.Context(), req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, item)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleAdminUsagePriceRoutes(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(r) {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}
	if s.usage == nil {
		writeError(w, http.StatusInternalServerError, "usage store is not available")
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/usage/prices/"), "/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var req store.LLMModelPriceInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		item, err := s.usage.UpdatePrice(r.Context(), id, req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, item)
	case http.MethodDelete:
		if err := s.usage.DeletePrice(r.Context(), id); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		http.NotFound(w, r)
	}
}

func usageQueryFromRequest(r *http.Request, defaultLimit int) store.UsageQuery {
	q := r.URL.Query()
	from, to := usageRangeFromRequest(q.Get("range"), q.Get("from"), q.Get("to"))
	return store.UsageQuery{
		From:            from,
		To:              to,
		UserID:          q.Get("user_id"),
		ModelProviderID: q.Get("model_provider_id"),
		ModelID:         q.Get("model_id"),
		Stage:           q.Get("stage"),
		Limit:           parseLimit(q.Get("limit"), defaultLimit),
		Offset:          parseOffset(q.Get("offset")),
	}
}

func usageRangeFromRequest(rangeValue, fromValue, toValue string) (time.Time, time.Time) {
	now := time.Now().UTC()
	to := now
	switch strings.TrimSpace(rangeValue) {
	case "today":
		localNow := time.Now()
		localStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, localNow.Location())
		return localStart.UTC(), to
	case "7d":
		return now.AddDate(0, 0, -7), to
	case "30d", "":
		return now.AddDate(0, 0, -30), to
	default:
		if parsedFrom, err := time.Parse(time.RFC3339, strings.TrimSpace(fromValue)); err == nil {
			if parsedTo, err := time.Parse(time.RFC3339, strings.TrimSpace(toValue)); err == nil {
				return parsedFrom.UTC(), parsedTo.UTC()
			}
			return parsedFrom.UTC(), to
		}
		return now.AddDate(0, 0, -30), to
	}
}

func parseOffset(raw string) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 0 {
		return 0
	}
	return value
}
