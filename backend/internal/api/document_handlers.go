package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"webide-backend/internal/document"
	"webide-backend/internal/workspace"
)

func (s *Server) handleDocumentProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, 200, document.Profile())
}
func (s *Server) handleDocumentValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var req struct {
		Content string `json:"content"`
		Profile string `json:"profile_version"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, document.MaxBytes*6+1024)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid document request")
		return
	}
	result, err := document.Validate([]byte(req.Content), req.Profile)
	status := 200
	if err != nil {
		status = 422
	}
	writeJSON(w, status, result)
}
func (s *Server) handleDocumentVersions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	path := r.URL.Query().Get("path")
	if id := r.URL.Query().Get("version_id"); id != "" {
		item, err := files.DocumentVersion(r.Context(), path, id)
		if err != nil {
			s.writeFileError(w, err)
			return
		}
		writeJSON(w, 200, item)
		return
	}
	items, err := files.DocumentVersions(r.Context(), path)
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) handleDocumentRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var req struct {
		Path         string `json:"path"`
		VersionID    string `json:"version_id"`
		ExpectedHash string `json:"expected_hash"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid restore request")
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	result, err := files.RestoreDocumentVersion(r.Context(), req.Path, req.VersionID, req.ExpectedHash)
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) handleFilesImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	file, err := files.ReadDocumentImage(r.Context(), r.URL.Query().Get("path"))
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	mime := documentImageMIME([]byte(file.Content))
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/avif":
	default:
		writeError(w, 415, "仅支持安全的位图图片")
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	http.ServeContent(w, r, file.Path, file.ModifiedAt, strings.NewReader(file.Content))
}
func writeDocumentError(w http.ResponseWriter, err error) bool {
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "文稿版本不存在")
		return true
	}
	if errors.Is(err, workspace.ErrFileConflict) {
		writeError(w, 409, err.Error())
		return true
	}
	var validation *document.ValidationError
	if !errors.As(err, &validation) {
		return false
	}
	writeJSON(w, 422, map[string]any{"error": err.Error(), "document": validation.Result})
	return true
}
