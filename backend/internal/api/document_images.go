package api

import (
	"encoding/binary"
	"errors"
	"io"
	"net/http"
)

func (s *Server) handleDocumentImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	if r.Method == http.MethodGet {
		items, err := files.DocumentImages(r.Context())
		if err != nil {
			s.writeFileError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"items": items})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, (16<<20)+(64<<10))
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		if r.MultipartForm != nil {
			r.MultipartForm.RemoveAll()
		}
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			writeError(w, 413, "图片不能超过 16 MB")
			return
		}
		writeError(w, 400, "无法读取上传图片")
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, 400, "请选择图片")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (16<<20)+1))
	if err != nil {
		writeError(w, 400, "无法读取上传图片")
		return
	}
	if len(data) > 16<<20 {
		writeError(w, 413, "图片不能超过 16 MB")
		return
	}
	extension := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp", "image/avif": ".avif"}[documentImageMIME(data)]
	if extension == "" {
		writeError(w, 415, "请选择 PNG、JPG、GIF、WebP 或 AVIF 图片")
		return
	}
	result, err := files.SaveDocumentImage(r.Context(), data, extension, header.Filename)
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, 201, result)
}

// DetectContentType does not recognize ISO-BMFF AVIF. Inspect only the bounded
// ftyp box; never trust a path extension or a client-supplied MIME type.
func documentImageMIME(data []byte) string {
	mime := http.DetectContentType(data)
	if len(data) < 16 || string(data[4:8]) != "ftyp" {
		return mime
	}
	size := int64(binary.BigEndian.Uint32(data[:4]))
	if size < 16 || size > 4096 || size > int64(len(data)) || size%4 != 0 {
		return mime
	}
	for offset := 8; offset+4 <= int(size); offset += 4 {
		if offset == 12 { // minor version is not a brand
			continue
		}
		brand := string(data[offset : offset+4])
		if brand == "avif" || brand == "avis" {
			return "image/avif"
		}
	}
	return mime
}
