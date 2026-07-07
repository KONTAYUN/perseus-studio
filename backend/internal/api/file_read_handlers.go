package api

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleFilesTree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tree, err := files.Tree()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": tree})
}

func (s *Server) handleFilesSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req searchFilesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	items, err := files.Search(req.Query, req.Limit)
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleFilesRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	result, err := files.ReadFile(r.URL.Query().Get("path"))
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleFilesContent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	content, path, err := files.ReadBytes(r.URL.Query().Get("path"))
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	w.Header().Set("Content-Type", http.DetectContentType(content))
	w.Header().Set("Cache-Control", "private, max-age=60")
	w.Header().Set("X-Content-Path", path)
	_, _ = w.Write(content)
}
