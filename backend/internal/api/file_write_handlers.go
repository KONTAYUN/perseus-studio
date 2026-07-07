package api

import (
	"encoding/json"
	"io"
	"net/http"
	"webide-backend/internal/workspace"
)

func (s *Server) handleFilesUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart body")
		return
	}

	targetPath := r.FormValue("path")
	overwrite := r.FormValue("overwrite") == "true"
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file")
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read file")
		return
	}

	var result workspace.FileWriteResult
	if overwrite {
		result, err = files.WriteBytes(r.Context(), targetPath, content, "upload")
	} else {
		result, err = files.CreateBytes(r.Context(), targetPath, content)
	}
	if err != nil {
		s.writeFileError(w, err)
		return
	}

	status := http.StatusCreated
	if overwrite {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}

func (s *Server) handleFilesWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req writeFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if isNoteLikePath(req.Path) {
		req.Content = sanitizeHTMLFragment(req.Content)
	}
	result, err := files.WriteFile(r.Context(), req.Path, req.Content, "write")
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleFilesCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req writeFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if isNoteLikePath(req.Path) {
		req.Content = sanitizeHTMLFragment(req.Content)
	}
	result, err := files.CreateFile(r.Context(), req.Path, req.Content)
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleFilesMkdir(w http.ResponseWriter, r *http.Request) {
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
	result, err := files.MkdirNew(req.Path)
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleFilesRename(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req renameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	result, err := files.Rename(req.OldPath, req.NewPath)
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleFilesReorder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req reorderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	result, err := files.ReorderPath(req.Path, req.TargetParentPath, req.PreviousPath, req.NextPath)
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleFilesDelete(w http.ResponseWriter, r *http.Request) {
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
	result, err := files.MoveToTrash(req.Path)
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
