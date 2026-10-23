package api

import (
	"context"
	"net/http"
	"time"
)

func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	if s.runs == nil || s.runQueue == nil || s.runs.Ready(ctx) != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "reason": "storage_unavailable"})
		return
	}
	state := s.runQueue.Snapshot()
	if !state.Accepting {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "reason": "stopping"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "scheduler": state})
}
