package api

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"webide-backend/internal/workspace"
)

func (s *Server) handleFilesImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := r.ParseMultipartForm(128 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart body")
		return
	}
	targetParent := strings.Trim(strings.ReplaceAll(r.FormValue("parent_path"), "\\", "/"), "/")
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing import file")
		return
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, 128<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read import file")
		return
	}

	var imported []string
	name := strings.ToLower(header.Filename)
	switch {
	case strings.HasSuffix(name, ".zip"):
		imported, err = importZipNotes(r.Context(), files, targetParent, content)
	case strings.HasSuffix(name, ".html"), strings.HasSuffix(name, ".htm"):
		imported, err = importHTMLNote(r.Context(), files, targetParent, header.Filename, content)
	case strings.HasSuffix(name, ".md"), strings.HasSuffix(name, ".markdown"):
		imported, err = importMarkdownNote(r.Context(), files, targetParent, header.Filename, content)
	case strings.HasSuffix(name, ".docx"):
		imported, err = importDocxNote(r.Context(), files, targetParent, header.Filename, content)
	default:
		writeError(w, http.StatusBadRequest, "only .html, .md, .markdown, .docx, and .zip note imports are supported")
		return
	}
	if err != nil {
		if errors.Is(err, errPandocUnavailable) {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"imported_count": len(imported),
		"imported_paths": imported,
	})
}

func importHTMLNote(ctx context.Context, files *workspace.Service, parentPath, filename string, content []byte) ([]string, error) {
	name := sanitizeImportedPath(convertImportNoteExtension(filename, ".html"))
	if name == "" {
		name = "Imported Note.html"
	}
	targetPath := joinImportTarget(parentPath, name)
	finalPath, err := writeImportedFile(ctx, files, targetPath, []byte(extractHTMLFragment(string(content))))
	if err != nil {
		return nil, err
	}
	return []string{finalPath}, nil
}

func importMarkdownNote(ctx context.Context, files *workspace.Service, parentPath, filename string, content []byte) ([]string, error) {
	htmlContent, err := newPandocConverter().MarkdownToHTML(ctx, string(content))
	if err != nil {
		return nil, err
	}
	htmlContent = sanitizeHTMLFragment(htmlContent)
	name := sanitizeImportedPath(convertImportNoteExtension(filename, ".html"))
	if name == "" {
		name = "Imported Note.html"
	}
	targetPath := joinImportTarget(parentPath, name)
	finalPath, err := writeImportedFile(ctx, files, targetPath, []byte(htmlContent))
	if err != nil {
		return nil, err
	}
	return []string{finalPath}, nil
}

func importDocxNote(ctx context.Context, files *workspace.Service, parentPath, filename string, content []byte) ([]string, error) {
	converter := newPandocConverter()
	htmlContent, assets, err := converter.DocxToHTML(ctx, content)
	if err != nil {
		return nil, err
	}
	htmlContent = sanitizeHTMLFragment(htmlContent)
	noteName := buildDocxImportNoteName(filename)
	targetPath := joinImportTarget(parentPath, noteName)
	finalPath, err := writeImportedFile(ctx, files, targetPath, []byte(htmlContent))
	if err != nil {
		return nil, err
	}
	noteTitle := strings.TrimSuffix(filepath.Base(finalPath), filepath.Ext(finalPath))
	htmlContent = rewriteImportedHTMLAssets(htmlContent, noteTitle, assets)
	htmlContent = sanitizeHTMLFragment(htmlContent)
	if _, err := files.WriteBytes(ctx, finalPath, []byte(htmlContent), "import_docx"); err != nil {
		return nil, err
	}
	imported := []string{finalPath}
	for relativePath, raw := range assets {
		assetTarget := joinImportTarget(parentPath, filepath.ToSlash(filepath.Join(".assets", noteTitle, relativePath)))
		finalAssetPath, err := writeImportedFile(ctx, files, assetTarget, raw)
		if err != nil {
			return nil, err
		}
		imported = append(imported, finalAssetPath)
	}
	return imported, nil
}

func convertImportNoteExtension(filename, ext string) string {
	base := filepath.Base(filename)
	if base == "." || base == string(filepath.Separator) || strings.TrimSpace(base) == "" {
		return "Imported Note" + ext
	}
	return strings.TrimSuffix(base, filepath.Ext(base)) + ext
}

func importZipNotes(ctx context.Context, files *workspace.Service, parentPath string, content []byte) ([]string, error) {
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, fmt.Errorf("invalid zip archive")
	}
	imported := make([]string, 0, len(reader.File))
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		relative := sanitizeImportedPath(entry.Name)
		if relative == "" {
			continue
		}
		if !shouldImportArchiveEntry(relative) {
			continue
		}
		rc, err := entry.Open()
		if err != nil {
			return nil, err
		}
		raw, err := io.ReadAll(io.LimitReader(rc, 128<<20))
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		if isNoteLikePath(relative) {
			raw = []byte(extractHTMLFragment(string(raw)))
		}
		targetPath := joinImportTarget(parentPath, relative)
		finalPath, err := writeImportedFile(ctx, files, targetPath, raw)
		if err != nil {
			return nil, err
		}
		imported = append(imported, finalPath)
	}
	if len(imported) == 0 {
		return nil, fmt.Errorf("no importable notes or assets were found in the archive")
	}
	return imported, nil
}

func shouldImportArchiveEntry(relative string) bool {
	lower := strings.ToLower(relative)
	if strings.Contains(lower, "__macosx/") || strings.HasSuffix(lower, ".json") || strings.HasSuffix(lower, ".zip") {
		return false
	}
	return true
}

func sanitizeImportedPath(name string) string {
	clean := path.Clean(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	if clean == "." || clean == "" || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return ""
	}
	parts := strings.Split(clean, "/")
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			continue
		}
		filtered = append(filtered, part)
	}
	return strings.Join(filtered, "/")
}

func joinImportTarget(parentPath, relative string) string {
	if parentPath == "" {
		return relative
	}
	return strings.Trim(parentPath+"/"+relative, "/")
}

func writeImportedFile(ctx context.Context, files *workspace.Service, targetPath string, content []byte) (string, error) {
	for attempt := 1; attempt <= 999; attempt++ {
		candidate := targetPath
		if attempt > 1 {
			candidate = appendPathSuffix(targetPath, attempt)
		}
		if _, err := files.CreateBytes(ctx, candidate, content); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "already exists") {
				continue
			}
			return "", err
		}
		return candidate, nil
	}
	return "", fmt.Errorf("unable to find an available import path for %s", targetPath)
}

func appendPathSuffix(targetPath string, attempt int) string {
	ext := path.Ext(targetPath)
	base := strings.TrimSuffix(targetPath, ext)
	return fmt.Sprintf("%s (%d)%s", base, attempt, ext)
}

func buildDocxImportNoteName(filename string) string {
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	base = strings.TrimSpace(base)
	if base == "" {
		base = "Imported Note"
	}
	base = invalidFilenameChars.ReplaceAllString(base, "-")
	base = whitespaceRuns.ReplaceAllString(base, " ")
	base = strings.TrimSpace(base)
	if base == "" {
		base = "Imported Note"
	}
	return base + ".html"
}
