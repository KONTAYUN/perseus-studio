package api

import (
	"net/http"
	"strconv"

	"webide-backend/internal/tasks"
)

func (s *Server) handleTaskEvents(w http.ResponseWriter, r *http.Request, taskID string) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	user := MustUserFromContext(r.Context())
	if _, err := s.store.GetTask(r.Context(), user.UserID, taskID); err != nil {
		status := http.StatusInternalServerError
		if tasks.IsNotFound(err) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	after, limit := int64(0), 200
	var err error
	if raw := r.URL.Query().Get("after_seq"); raw != "" {
		after, err = strconv.ParseInt(raw, 10, 64)
	}
	if err != nil || after < 0 {
		writeError(w, http.StatusBadRequest, "invalid after_seq")
		return
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 1000 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 1000")
			return
		}
	}
	items, err := s.store.ListEventsAfter(r.Context(), user.UserID, taskID, after, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	next := after
	if len(items) > 0 {
		next = items[len(items)-1].Seq
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "next_seq": next})
}
