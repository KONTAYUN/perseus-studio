package api

import (
	"mime"
	"net/http"
	"path"
	"strings"
)

// ReadFile retains arbitrary bytes in its Go string. Serve them directly,
// without the UTF-8 replacement performed by JSON encoding.
func (s *Server) handleFilesDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	file, err := files.ReadFile(r.URL.Query().Get("path"))
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": path.Base(file.Path)}))
	http.ServeContent(w, r, path.Base(file.Path), file.ModifiedAt, strings.NewReader(file.Content))
}
