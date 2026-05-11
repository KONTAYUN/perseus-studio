package api

import "net/http"

func (s *Server) handleDebugModelToolTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if !s.cfg.EnableDebugAPI {
		http.NotFound(w, r)
		return
	}
	if !s.requireAdmin(r) {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}
	user := MustUserFromContext(r.Context())
	result, err := s.ai.DebugModelToolTest(r.Context(), user.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
