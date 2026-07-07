package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	htmlstd "html"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"webide-backend/internal/workspace"
)

func wrapHTMLDocument(title, fragment string) string {
	return "<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n<title>" +
		htmlstd.EscapeString(title) +
		"</title>\n</head>\n<body>\n" +
		fragment +
		"\n</body>\n</html>\n"
}

func wrapHTMLFragmentDocument(fragment string) string {
	return "<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n</head>\n<body>\n" +
		fragment +
		"\n</body>\n</html>\n"
}

func (s *Server) handleFilesExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	targetPath := strings.TrimSpace(r.URL.Query().Get("path"))
	versionID := strings.TrimSpace(r.URL.Query().Get("version_id"))
	format := normalizeExportFormat(r.URL.Query().Get("format"))
	if format == "invalid" {
		writeError(w, http.StatusBadRequest, "unsupported export format")
		return
	}
	targetPaths, err := parseExportPaths(r.URL.Query().Get("paths_json"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if format == "" {
		if len(targetPaths) == 0 && (versionID != "" || (targetPath != "" && isNoteLikePath(targetPath))) {
			format = "html"
		} else {
			format = "zip"
		}
	}
	if versionID != "" && len(targetPaths) > 0 {
		writeError(w, http.StatusBadRequest, "version export only supports a single note")
		return
	}
	if len(targetPaths) > 1 && format != "zip" {
		writeError(w, http.StatusBadRequest, "HTML, Markdown, and Word export are only supported for a single note")
		return
	}
	if len(targetPaths) > 1 {
		tree, err := files.Tree()
		if err != nil {
			s.writeFileError(w, err)
			return
		}
		subtree, found := selectExportNodesForPaths(tree, targetPaths)
		if !found {
			writeError(w, http.StatusNotFound, "path does not exist")
			return
		}
		buffer := &bytes.Buffer{}
		archive := zip.NewWriter(buffer)
		if err := writeExportZip(archive, files, "", subtree); err != nil {
			_ = archive.Close()
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := archive.Close(); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", buildAttachmentDisposition("selected-notes.zip"))
		_, _ = w.Write(buffer.Bytes())
		return
	}
	if versionID != "" || (targetPath != "" && isNoteLikePath(targetPath)) {
		var (
			result  workspace.FileReadResult
			readErr error
		)
		if versionID != "" {
			result, readErr = files.ReadFileVersion(r.Context(), targetPath, versionID)
		} else {
			result, readErr = files.ReadFile(targetPath)
		}
		if readErr != nil {
			s.writeFileError(w, readErr)
			return
		}
		if !isNoteLikePath(result.Path) {
			writeError(w, http.StatusBadRequest, "HTML, Markdown, and Word export are only supported for a single note")
			return
		}
		baseName := filepath.Base(result.Path)
		switch format {
		case "html":
			exportHTML := inlineHTMLAssetsForExport(result.Content, result.Path, files)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Content-Disposition", buildAttachmentDisposition(baseName))
			_, _ = w.Write([]byte(wrapHTMLDocument(strings.TrimSuffix(baseName, filepath.Ext(baseName)), exportHTML)))
		case "md":
			markdown, err := newPandocConverter().HTMLToMarkdown(r.Context(), result.Content)
			if err != nil {
				if errors.Is(err, errPandocUnavailable) {
					writeError(w, http.StatusServiceUnavailable, err.Error())
					return
				}
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			mdName := strings.TrimSuffix(baseName, filepath.Ext(baseName)) + ".md"
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			w.Header().Set("Content-Disposition", buildAttachmentDisposition(mdName))
			_, _ = w.Write([]byte(markdown))
		case "docx":
			docxBytes, err := newPandocConverter().HTMLToDocx(r.Context(), files, result.Path, result.Content)
			if err != nil {
				if errors.Is(err, errPandocUnavailable) {
					writeError(w, http.StatusServiceUnavailable, err.Error())
					return
				}
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			docxName := strings.TrimSuffix(baseName, filepath.Ext(baseName)) + ".docx"
			w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
			w.Header().Set("Content-Disposition", buildAttachmentDisposition(docxName))
			_, _ = w.Write(docxBytes)
		default:
			writeError(w, http.StatusBadRequest, "unsupported export format")
		}
		return
	}
	if format != "zip" {
		writeError(w, http.StatusBadRequest, "HTML, Markdown, and Word export are only supported for a single note")
		return
	}

	tree, err := files.Tree()
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	subtree, found := selectExportNodes(tree, targetPath)
	if targetPath != "" && !found {
		writeError(w, http.StatusNotFound, "path does not exist")
		return
	}
	buffer := &bytes.Buffer{}
	archive := zip.NewWriter(buffer)
	if err := writeExportZip(archive, files, targetPath, subtree); err != nil {
		_ = archive.Close()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := archive.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	fileName := exportArchiveName(targetPath)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", buildAttachmentDisposition(fileName))
	_, _ = w.Write(buffer.Bytes())
}

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

func isNoteLikePath(targetPath string) bool {
	lower := strings.ToLower(targetPath)
	return strings.HasSuffix(lower, ".html")
}

func exportArchiveName(targetPath string) string {
	if strings.TrimSpace(targetPath) == "" {
		return "perseus-notes.zip"
	}
	base := filepath.Base(targetPath)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	if base == "" {
		base = "notes"
	}
	return base + ".zip"
}

func selectExportNodes(tree []workspace.TreeNode, targetPath string) ([]workspace.TreeNode, bool) {
	if strings.TrimSpace(targetPath) == "" {
		return tree, true
	}
	for _, item := range tree {
		if item.Path == targetPath {
			return []workspace.TreeNode{item}, true
		}
		if len(item.Children) > 0 {
			if subtree, found := selectExportNodes(item.Children, targetPath); found {
				return subtree, true
			}
		}
	}
	return nil, false
}

func selectExportNodesForPaths(tree []workspace.TreeNode, targetPaths []string) ([]workspace.TreeNode, bool) {
	var selected []workspace.TreeNode
	foundAny := false
	for _, targetPath := range targetPaths {
		subtree, found := selectExportNodes(tree, targetPath)
		if !found {
			continue
		}
		selected = append(selected, subtree...)
		foundAny = true
	}
	return selected, foundAny
}

func writeExportZip(archive *zip.Writer, files *workspace.Service, prefix string, nodes []workspace.TreeNode) error {
	for _, node := range nodes {
		if node.Type == "folder" {
			if err := writeExportZip(archive, files, prefix, node.Children); err != nil {
				return err
			}
			continue
		}
		content, cleanPath, err := files.ReadBytes(node.Path)
		if err != nil {
			return err
		}
		entryPath := cleanPath
		if prefix != "" {
			entryPath = strings.TrimPrefix(cleanPath, strings.TrimSuffix(prefix, "/"))
			entryPath = strings.TrimPrefix(entryPath, "/")
		}
		if entryPath == "" {
			entryPath = filepath.Base(cleanPath)
		}
		writer, err := archive.Create(filepath.ToSlash(entryPath))
		if err != nil {
			return err
		}
		if _, err := writer.Write(content); err != nil {
			return err
		}
	}
	return nil
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

func extractHTMLFragment(content string) string {
	bodyPattern := regexp.MustCompile(`(?is)<body\b[^>]*>(.*?)</body>`)
	if match := bodyPattern.FindStringSubmatch(content); len(match) > 1 {
		return sanitizeHTMLFragment(match[1])
	}
	content = regexp.MustCompile(`(?is)<!doctype[^>]*>`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`(?is)</?html\b[^>]*>`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`(?is)<head\b[^>]*>.*?</head>`).ReplaceAllString(content, "")
	return sanitizeHTMLFragment(content)
}

func sanitizeHTMLFragment(fragment string) string {
	return workspace.SanitizeHTMLFragment(fragment)
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

func parseExportPaths(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var paths []string
	if err := json.Unmarshal([]byte(raw), &paths); err != nil {
		return nil, fmt.Errorf("invalid export paths")
	}
	seen := make(map[string]struct{}, len(paths))
	cleaned := make([]string, 0, len(paths))
	for _, item := range paths {
		cleanPath, err := cleanWorkspaceReferencePath(item)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[cleanPath]; exists {
			continue
		}
		seen[cleanPath] = struct{}{}
		cleaned = append(cleaned, cleanPath)
	}
	return cleaned, nil
}

func normalizeExportFormat(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return ""
	case "html":
		return "html"
	case "md":
		return "md"
	case "docx":
		return "docx"
	case "zip":
		return "zip"
	default:
		return "invalid"
	}
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
