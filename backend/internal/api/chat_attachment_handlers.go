package api

import (
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func (s *Server) handleChatAttachments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	user := MustUserFromContext(r.Context())
	if !s.allowRate(w, s.uploadLimiter, user.UserID, s.cfg.UploadRatePerMinute, time.Minute, "upload_rate_limited") {
		return
	}
	if err := r.ParseMultipartForm(maxChatImageBytes + (1 << 20)); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart body")
		return
	}
	header, err := firstMultipartFile(r, "file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "image file is required")
		return
	}
	item, err := s.persistStagedChatImage(r.Context(), user.UserID, header)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, chatAttachmentMetadataFromRecord(item))
}

func (s *Server) handleChatAttachmentRoutes(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/chat-attachments/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 1 && r.Method == http.MethodDelete {
		s.handleDeleteStagedChatAttachment(w, r, id)
		return
	}
	if len(parts) != 2 || r.Method != http.MethodGet || (parts[1] != "content" && parts[1] != "download") {
		http.NotFound(w, r)
		return
	}
	s.handleReadChatAttachment(w, r, id, parts[1] == "download")
}

func firstMultipartFile(r *http.Request, field string) (*multipart.FileHeader, error) {
	files := r.MultipartForm.File[field]
	if len(files) == 0 {
		return nil, fmt.Errorf("file is required")
	}
	return files[0], nil
}

func (s *Server) handleReadChatAttachment(w http.ResponseWriter, r *http.Request, id string, download bool) {
	user := MustUserFromContext(r.Context())
	item, err := s.conversations.GetChatAttachment(r.Context(), id)
	if err != nil || (item.UserID != user.UserID && user.Role != "admin") {
		http.NotFound(w, r)
		return
	}
	path, err := s.chatAttachmentDiskPath(item)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", item.MIMEType)
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	w.Header().Set("ETag", `"sha256-`+item.SHA256+`"`)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	disposition := "inline"
	if download {
		disposition = "attachment"
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": item.OriginalName}))
	http.ServeContent(w, r, item.OriginalName, info.ModTime(), file)
}

func (s *Server) handleDeleteStagedChatAttachment(w http.ResponseWriter, r *http.Request, id string) {
	user := MustUserFromContext(r.Context())
	item, err := s.conversations.DeleteStagedChatAttachment(r.Context(), user.UserID, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if path, pathErr := s.chatAttachmentDiskPath(item); pathErr == nil {
		_ = os.Remove(path)
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "deleted": true})
}
