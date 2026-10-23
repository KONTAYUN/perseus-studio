package api

import (
	"net/http"
	"strings"
)

// ParseMultipartForm limits memory, not total bytes. Apply a transport-level
// ceiling to every API body, including chunked requests without Content-Length.
func (s *Server) withBodyLimits(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") || r.Body == nil {
			next.ServeHTTP(w, r)
			return
		}
		limit := int64(2 << 20)
		switch {
		case r.URL.Path == "/api/files/upload" || r.URL.Path == "/api/files/write" || r.URL.Path == "/api/files/create":
			limit = 65 << 20
		case strings.HasPrefix(r.URL.Path, "/api/conversations/") && strings.HasSuffix(r.URL.Path, "/messages") && strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data"):
			limit = 80 << 20
		case r.URL.Path == "/api/skills/upload":
			limit = 27 << 20
		}
		if r.ContentLength > limit {
			writeError(w, http.StatusRequestEntityTooLarge, "request body is too large")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}
