package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"webide-backend/internal/workspace"
)

func (s *Server) handleFilesTrash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items, err := files.TrashItems()
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleFilesTrashRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req restoreTrashRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	result, err := files.RestoreTrashItem(req.Path, req.Overwrite)
	if err != nil {
		if errors.Is(err, workspace.ErrPathExists) {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":    "原路径已存在，是否覆盖？",
				"conflict": true,
				"path":     req.Path,
			})
			return
		}
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleFilesTrashDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req pathRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	result, err := files.DeleteTrashItem(req.Path)
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleFilesEmptyTrash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	result, err := files.EmptyTrash()
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
