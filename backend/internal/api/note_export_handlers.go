package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"webide-backend/internal/workspace"
)

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
