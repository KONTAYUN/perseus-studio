package api

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	htmlstd "html"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"webide-backend/internal/workspace"
)

var errPandocUnavailable = errors.New("服务器未启用 Word 导入导出能力")

type pandocConverter struct{}

func newPandocConverter() *pandocConverter {
	return &pandocConverter{}
}

func (c *pandocConverter) ensureAvailable() error {
	if _, err := exec.LookPath("pandoc"); err != nil {
		return errPandocUnavailable
	}
	return nil
}

func (c *pandocConverter) HTMLToDocx(ctx context.Context, files *workspace.Service, notePath, html string) ([]byte, error) {
	if err := c.ensureAvailable(); err != nil {
		return nil, err
	}
	tempRoot, err := os.MkdirTemp("", "perseus-pandoc-export-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempRoot)

	noteCleanPath, err := cleanWorkspaceReferencePath(notePath)
	if err != nil {
		return nil, err
	}
	html, err = materializeHTMLAssets(html, noteCleanPath, tempRoot, files)
	if err != nil {
		return nil, err
	}

	sourcePath := filepath.Join(tempRoot, strings.TrimSuffix(filepath.FromSlash(noteCleanPath), filepath.Ext(noteCleanPath))+".html")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(sourcePath, []byte(wrapHTMLDocument(filepath.Base(noteCleanPath), html)), 0o644); err != nil {
		return nil, err
	}
	outputPath := strings.TrimSuffix(sourcePath, filepath.Ext(sourcePath)) + ".docx"
	docxCmd := exec.CommandContext(
		ctx,
		"pandoc",
		"--from=html",
		"--to=docx",
		"--output", outputPath,
		sourcePath,
	)
	docxCmd.Dir = tempRoot
	if output, err := docxCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pandoc docx export failed: %s", strings.TrimSpace(string(output)))
	}
	return os.ReadFile(outputPath)
}

func (c *pandocConverter) MarkdownToHTML(ctx context.Context, markdown string) (string, error) {
	if err := c.ensureAvailable(); err != nil {
		return "", err
	}
	tempRoot, err := os.MkdirTemp("", "perseus-pandoc-md-html-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempRoot)

	sourcePath := filepath.Join(tempRoot, "source.md")
	outputPath := filepath.Join(tempRoot, "converted.html")
	if err := os.WriteFile(sourcePath, []byte(markdown), 0o644); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(
		ctx,
		"pandoc",
		"--from=gfm+raw_html+raw_attribute",
		"--to=html",
		"--output", outputPath,
		sourcePath,
	)
	cmd.Dir = tempRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("pandoc markdown import failed: %s", strings.TrimSpace(string(output)))
	}
	rawHTML, err := os.ReadFile(outputPath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(rawHTML)), nil
}

func (c *pandocConverter) HTMLToMarkdown(ctx context.Context, html string) (string, error) {
	if err := c.ensureAvailable(); err != nil {
		return "", err
	}
	tempRoot, err := os.MkdirTemp("", "perseus-pandoc-html-md-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempRoot)

	sourcePath := filepath.Join(tempRoot, "source.html")
	outputPath := filepath.Join(tempRoot, "converted.md")
	if err := os.WriteFile(sourcePath, []byte(wrapHTMLDocument("note", html)), 0o644); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(
		ctx,
		"pandoc",
		"--from=html",
		"--to=gfm+raw_html",
		"--output", outputPath,
		sourcePath,
	)
	cmd.Dir = tempRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("pandoc markdown export failed: %s", strings.TrimSpace(string(output)))
	}
	rawMarkdown, err := os.ReadFile(outputPath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(rawMarkdown)), nil
}

func (c *pandocConverter) DocxToHTML(ctx context.Context, content []byte) (string, map[string][]byte, error) {
	if err := c.ensureAvailable(); err != nil {
		return "", nil, err
	}
	tempRoot, err := os.MkdirTemp("", "perseus-pandoc-import-*")
	if err != nil {
		return "", nil, err
	}
	defer os.RemoveAll(tempRoot)

	sourcePath := filepath.Join(tempRoot, "source.docx")
	if err := os.WriteFile(sourcePath, content, 0o644); err != nil {
		return "", nil, err
	}
	mediaDir := filepath.Join(tempRoot, "media")
	outputPath := filepath.Join(tempRoot, "converted.html")
	cmd := exec.CommandContext(
		ctx,
		"pandoc",
		"--from=docx",
		"--to=html",
		"--extract-media=media",
		"--output", outputPath,
		sourcePath,
	)
	cmd.Dir = tempRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", nil, fmt.Errorf("pandoc import failed: %s", strings.TrimSpace(string(output)))
	}
	rawHTML, err := os.ReadFile(outputPath)
	if err != nil {
		return "", nil, err
	}
	assets := make(map[string][]byte)
	if info, err := os.Stat(mediaDir); err == nil && info.IsDir() {
		err = filepath.Walk(mediaDir, func(current string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() {
				return nil
			}
			relative, err := filepath.Rel(mediaDir, current)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(current)
			if err != nil {
				return err
			}
			assets[filepath.ToSlash(relative)] = data
			return nil
		})
		if err != nil {
			return "", nil, err
		}
	}
	return strings.TrimSpace(string(rawHTML)), assets, nil
}

func materializeHTMLAssets(html, notePath, tempRoot string, files *workspace.Service) (string, error) {
	replacements := make(map[string]string)
	materializedPaths := make(map[string]struct{})
	matches := noteImageTagPattern.FindAllStringSubmatch(html, -1)
	for _, match := range matches {
		tag := match[0]
		dataNoteSrc := strings.TrimSpace(match[1])
		if dataNoteSrc == "" {
			continue
		}
		assetPath := resolveWorkspaceAssetPath(notePath, dataNoteSrc)
		if assetPath == "" {
			continue
		}
		if err := materializeWorkspaceAsset(tempRoot, files, assetPath, materializedPaths); err != nil {
			continue
		}
		replacements[tag] = setHTMLAttribute(tag, "src", dataNoteSrc)
	}
	for oldTag, newTag := range replacements {
		html = strings.ReplaceAll(html, oldTag, newTag)
	}
	return html, nil
}

func inlineHTMLAssetsForExport(html, notePath string, files *workspace.Service) string {
	noteCleanPath, err := cleanWorkspaceReferencePath(notePath)
	if err != nil {
		return html
	}
	replacements := make(map[string]string)
	matches := noteImageTagPattern.FindAllStringSubmatch(html, -1)
	for _, match := range matches {
		tag := match[0]
		dataNoteSrc := strings.TrimSpace(match[1])
		if !isLocalAssetReference(dataNoteSrc) {
			continue
		}
		assetPath := resolveWorkspaceAssetPath(noteCleanPath, dataNoteSrc)
		if assetPath == "" {
			continue
		}
		assetBytes, cleanAssetPath, err := files.ReadBytes(assetPath)
		if err != nil {
			continue
		}
		mimeType := detectAttachmentMIME(cleanAssetPath, assetBytes)
		dataURI := fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(assetBytes))
		replacements[tag] = setHTMLAttribute(tag, "src", dataURI)
	}
	for oldTag, newTag := range replacements {
		html = strings.ReplaceAll(html, oldTag, newTag)
	}
	return html
}

func materializeWorkspaceAsset(tempRoot string, files *workspace.Service, assetPath string, materializedPaths map[string]struct{}) error {
	assetBytes, cleanAssetPath, err := files.ReadBytes(assetPath)
	if err != nil {
		return err
	}
	if _, exists := materializedPaths[cleanAssetPath]; exists {
		return nil
	}
	targetPath := filepath.Join(tempRoot, filepath.FromSlash(cleanAssetPath))
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(targetPath, assetBytes, 0o644); err != nil {
		return err
	}
	materializedPaths[cleanAssetPath] = struct{}{}
	return nil
}

func rewriteImportedHTMLAssets(html, noteTitle string, assets map[string][]byte) string {
	if len(assets) == 0 {
		return strings.TrimSpace(html)
	}
	normalizedTitle := sanitizeNoteTitle(noteTitle)
	if normalizedTitle == "" {
		normalizedTitle = "note"
	}
	for relativePath := range assets {
		targetPath := filepath.ToSlash(filepath.Join(".assets", normalizedTitle, relativePath))
		html = strings.ReplaceAll(html, filepath.ToSlash(relativePath), targetPath)
		html = strings.ReplaceAll(html, strings.ReplaceAll(relativePath, "/", `\`), targetPath)
	}
	return strings.TrimSpace(html)
}

func resolveWorkspaceAssetPath(notePath, assetReferencePath string) string {
	reference := normalizeWorkspacePath(assetReferencePath)
	if reference == "" {
		return ""
	}
	if !strings.HasPrefix(reference, ".") {
		return reference
	}
	baseParts := strings.Split(normalizeWorkspacePath(filepath.ToSlash(filepath.Dir(notePath))), "/")
	parts := strings.Split(reference, "/")
	resolved := make([]string, 0, len(baseParts)+len(parts))
	for _, part := range baseParts {
		if part != "" {
			resolved = append(resolved, part)
		}
	}
	for _, part := range parts {
		switch part {
		case "", ".":
			continue
		case "..":
			if len(resolved) > 0 {
				resolved = resolved[:len(resolved)-1]
			}
		default:
			resolved = append(resolved, part)
		}
	}
	return strings.Join(resolved, "/")
}

func isLocalAssetReference(reference string) bool {
	lower := strings.ToLower(strings.TrimSpace(reference))
	if lower == "" {
		return false
	}
	return !strings.HasPrefix(lower, "http://") &&
		!strings.HasPrefix(lower, "https://") &&
		!strings.HasPrefix(lower, "data:") &&
		!strings.HasPrefix(lower, "mailto:") &&
		!strings.HasPrefix(lower, "#") &&
		!strings.HasPrefix(lower, "/api/")
}

func normalizeWorkspacePath(path string) string {
	return strings.Trim(strings.ReplaceAll(path, "\\", "/"), "/")
}

func sanitizeNoteTitle(name string) string {
	cleaned := invalidFilenameChars.ReplaceAllString(strings.TrimSpace(name), "-")
	cleaned = whitespaceRuns.ReplaceAllString(cleaned, " ")
	return strings.TrimSpace(cleaned)
}

func asciiFallbackFileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "download"
	}
	var builder strings.Builder
	for _, r := range name {
		switch {
		case r >= 32 && r <= 126 && r != '"' && r != '\\':
			builder.WriteRune(r)
		case r == '/' || r == ':' || r == '*' || r == '?' || r == '<' || r == '>' || r == '|':
			builder.WriteByte('-')
		default:
			builder.WriteByte('_')
		}
	}
	fallback := strings.TrimSpace(builder.String())
	if fallback == "" {
		return "download"
	}
	return fallback
}

func buildAttachmentDisposition(fileName string) string {
	fallback := asciiFallbackFileName(fileName)
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, fallback, url.PathEscape(fileName))
}

func setHTMLAttribute(tag, name, value string) string {
	escaped := htmlstd.EscapeString(value)
	attrPattern := regexp.MustCompile(`(?is)\s+` + regexp.QuoteMeta(name) + `\s*=\s*("([^"]*)"|'([^']*)'|[^\s>]+)`)
	if attrPattern.MatchString(tag) {
		return attrPattern.ReplaceAllStringFunc(tag, func(string) string {
			return ` ` + name + `="` + escaped + `"`
		})
	}
	end := strings.LastIndex(tag, ">")
	if end < 0 {
		return tag
	}
	before := strings.TrimRight(tag[:end], " \t\r\n")
	return before + ` ` + name + `="` + escaped + `"` + tag[end:]
}

var (
	noteImageTagPattern  = regexp.MustCompile(`(?is)<img\b[^>]*\bdata-note-src="([^"]+)"[^>]*>`)
	invalidFilenameChars = regexp.MustCompile(`[\\/:*?"<>|]+`)
	whitespaceRuns       = regexp.MustCompile(`\s+`)
)
