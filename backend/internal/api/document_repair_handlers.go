package api

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleDocumentRepairs(w http.ResponseWriter, r *http.Request) {
	files, err := s.userWorkspace(r)
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	if r.Method == http.MethodGet {
		items, err := files.DocumentRepairCandidates(r.Context(), r.URL.Query().Get("path"))
		if err != nil {
			s.writeFileError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"items": items})
		return
	}
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var req struct {
		ID           string `json:"id"`
		ExpectedHash string `json:"expected_hash"`
		Accept       bool   `json:"accept"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid repair decision")
		return
	}
	result, err := files.DecideDocumentRepair(r.Context(), req.ID, req.ExpectedHash, req.Accept)
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
