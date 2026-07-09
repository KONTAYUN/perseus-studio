package ai

import (
	"context"
	"encoding/json"
	"fmt"
	htmlstd "html"
	"regexp"
	"strings"
)

const maxNoteViewRunes = 50000

func (s *Service) execReadWorkspaceNoteView(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在读取笔记视图...", "", err
	}
	var args struct {
		Path      string `json:"path"`
		View      string `json:"view"`
		Mode      string `json:"mode"`
		BlockID   string `json:"block_id"`
		SectionID string `json:"section_id"`
		DocHash   string `json:"doc_hash"`
		MaxBlocks int    `json:"max_blocks"`
		MaxChars  int    `json:"max_chars"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在读取笔记视图...", "", fmt.Errorf("invalid tool arguments")
	}
	args.View = strings.ToLower(strings.TrimSpace(args.View))
	if args.View == "" {
		args.View = "structure"
	}
	args.Mode = strings.ToLower(strings.TrimSpace(args.Mode))
	if args.Mode == "" {
		args.Mode = "full"
	}
	switch args.View {
	case "text", "structure", "format", "raw":
	default:
		return "", "", "正在读取笔记视图...", args.Path, fmt.Errorf("unsupported note view: %s", args.View)
	}
	switch args.Mode {
	case "full", "index", "block", "section":
	default:
		return "", "", "正在读取笔记视图...", args.Path, fmt.Errorf("unsupported note view mode: %s", args.Mode)
	}
	result, err := files.ReadFile(args.Path)
	if err != nil {
		return "", "", "正在读取笔记视图...", args.Path, err
	}
	if !strings.HasSuffix(strings.ToLower(result.Path), ".html") {
		return "", "", "正在读取笔记视图...", result.Path, fmt.Errorf("read_workspace_note_view only supports .html notes")
	}

	if args.Mode != "full" {
		return s.execReadWorkspaceNoteBlockView(result.Path, result.Content, result.Hash, args.View, args.Mode, args.BlockID, args.SectionID, args.DocHash, args.MaxBlocks, args.MaxChars)
	}

	viewContent := renderNoteView(result.Content, args.View)
	truncated := false
	if len([]rune(viewContent)) > maxNoteViewRunes {
		viewContent = string([]rune(viewContent)[:maxNoteViewRunes])
		truncated = true
	}
	payload := workspaceToolPayload("read_workspace_note_view", result.Path, map[string]any{
		"path":      result.Path,
		"view":      args.View,
		"mode":      args.Mode,
		"content":   viewContent,
		"truncated": truncated,
		"hash":      result.Hash,
		"doc_hash":  noteDocHash(result.Content),
		"next_hint": "Use text/structure for content work and format for lightweight style awareness. For exact HTML, call mode=index first, then mode=block or mode=section with raw.",
	})
	summary := fmt.Sprintf("read %s note view for %s (%d chars, truncated=%t)", args.View, result.Path, len([]rune(viewContent)), truncated)
	return mustJSON(payload), summary, "正在读取笔记视图...", result.Path, nil
}

func (s *Service) execReadWorkspaceNoteBlockView(path, fragment, fileHash, view, mode, blockID, sectionID, expectedDocHash string, maxBlocks, maxChars int) (string, string, string, string, error) {
	doc := buildNoteBlockDocument(fragment)
	if expectedDocHash = strings.TrimSpace(expectedDocHash); expectedDocHash != "" && expectedDocHash != doc.DocHash {
		return "", "", "正在读取笔记视图...", path, fmt.Errorf("note view doc_hash is stale; call read_workspace_note_view with mode=index again")
	}
	base := map[string]any{
		"path":      path,
		"view":      view,
		"mode":      mode,
		"hash":      fileHash,
		"doc_hash":  doc.DocHash,
		"next_hint": "Use mode=index to locate HTML blocks, then mode=block or mode=section for targeted raw HTML before precise formatting edits.",
	}
	switch mode {
	case "index":
		blocks, indexTruncated := noteIndexPayloadBlocks(doc.Blocks, maxBlocks)
		base["blocks"] = blocks
		base["sections"] = noteIndexPayloadSections(doc.Sections)
		base["total_blocks"] = len(doc.Blocks)
		base["returned_blocks"] = len(blocks)
		base["index_truncated"] = indexTruncated
		base["total_text_chars"] = doc.TotalTextRune
		base["total_raw_chars"] = doc.TotalRawRune
		base["truncated"] = indexTruncated
		base["next_action"] = "Choose a block_id or section_id from this index and call read_workspace_note_view with mode=block or mode=section. Use view=raw only for exact HTML edits."
		payload := workspaceToolPayload("read_workspace_note_view", path, base)
		summary := fmt.Sprintf("indexed %s note blocks for %s (%d blocks)", view, path, len(doc.Blocks))
		return mustJSON(payload), summary, "正在读取笔记视图...", path, nil
	case "block":
		blockID = strings.TrimSpace(blockID)
		if blockID == "" {
			return "", "", "正在读取笔记视图...", path, fmt.Errorf("block_id is required for note view mode=block")
		}
		block, ok := findNoteBlock(doc.Blocks, blockID)
		if !ok {
			return "", "", "正在读取笔记视图...", path, fmt.Errorf("note block_id not found; call read_workspace_note_view with mode=index again")
		}
		content := renderNoteBlocks([]noteBlock{block}, view)
		content, truncated := limitNoteContent(content, maxChars)
		base["block_id"] = block.ID
		base["block"] = map[string]any{
			"block_id":     block.ID,
			"tag":          block.Tag,
			"section_id":   block.SectionID,
			"heading_path": block.HeadingPath,
			"text_preview": block.TextPreview,
			"text_chars":   block.TextRunes,
			"raw_chars":    block.RawRunes,
			"has_images":   block.HasImages,
			"has_tables":   block.HasTables,
			"has_styles":   block.HasStyles,
		}
		base["content"] = content
		base["truncated"] = truncated
		if truncated {
			base["next_action"] = "This block is still large. Re-read the note with mode=index and choose a smaller neighboring block or a more specific section."
		}
		payload := workspaceToolPayload("read_workspace_note_view", path, base)
		summary := fmt.Sprintf("read %s note block %s for %s (%d chars, truncated=%t)", view, block.ID, path, len([]rune(content)), truncated)
		return mustJSON(payload), summary, "正在读取笔记视图...", path, nil
	case "section":
		sectionID = strings.TrimSpace(sectionID)
		if sectionID == "" {
			return "", "", "正在读取笔记视图...", path, fmt.Errorf("section_id is required for note view mode=section")
		}
		section, blocks, ok := findNoteSection(doc, sectionID)
		if !ok {
			return "", "", "正在读取笔记视图...", path, fmt.Errorf("note section_id not found; call read_workspace_note_view with mode=index again")
		}
		content := renderNoteBlocks(blocks, view)
		content, truncated := limitNoteContent(content, maxChars)
		base["section_id"] = section.ID
		base["section"] = section
		base["content"] = content
		base["truncated"] = truncated
		if truncated {
			base["next_action"] = "This section is still large. Use mode=index and read the specific block_id values inside this section."
		}
		payload := workspaceToolPayload("read_workspace_note_view", path, base)
		summary := fmt.Sprintf("read %s note section %s for %s (%d chars, truncated=%t)", view, section.ID, path, len([]rune(content)), truncated)
		return mustJSON(payload), summary, "正在读取笔记视图...", path, nil
	default:
		return "", "", "正在读取笔记视图...", path, fmt.Errorf("unsupported note view mode: %s", mode)
	}
}

func renderNoteView(fragment, view string) string {
	switch view {
	case "raw":
		return strings.TrimSpace(fragment)
	case "text":
		return htmlFragmentToText(fragment)
	case "format":
		return htmlFragmentToFormatView(fragment)
	default:
		return htmlFragmentToStructure(fragment)
	}
}

func htmlFragmentToText(fragment string) string {
	text := stripIgnoredHTML(fragment)
	text = imageTagPattern.ReplaceAllStringFunc(text, func(tag string) string {
		label := firstHTMLAttr(tag, "alt")
		if label == "" {
			label = firstHTMLAttr(tag, "data-note-src")
		}
		if label == "" {
			label = firstHTMLAttr(tag, "src")
		}
		if label == "" {
			label = "图片"
		}
		return "\n[图片: " + label + "]\n"
	})
	text = regexp.MustCompile(`(?i)<br\s*/?>`).ReplaceAllString(text, "\n")
	text = regexp.MustCompile(`(?i)</(p|div|h[1-6]|li|tr|table|blockquote|pre)>`).ReplaceAllString(text, "\n")
	text = regexp.MustCompile(`(?i)</(td|th)>`).ReplaceAllString(text, "\t")
	text = htmlTagPattern.ReplaceAllString(text, "")
	text = htmlstd.UnescapeString(text)
	return normalizeNoteViewText(text)
}

func htmlFragmentToStructure(fragment string) string {
	cleaned := stripIgnoredHTML(fragment)
	var lines []string
	consumed := map[string]struct{}{}

	for _, match := range headingPattern.FindAllStringSubmatch(cleaned, -1) {
		text := htmlFragmentToText(match[3])
		if text == "" {
			continue
		}
		level := match[1]
		lines = append(lines, strings.Repeat("#", max(1, atoi(level)))+" "+text)
		consumed[match[0]] = struct{}{}
	}
	for _, match := range paragraphPattern.FindAllStringSubmatch(cleaned, -1) {
		if _, ok := consumed[match[0]]; ok {
			continue
		}
		text := htmlFragmentToText(match[2])
		if text != "" {
			lines = append(lines, text)
		}
	}
	for _, match := range listItemPattern.FindAllStringSubmatch(cleaned, -1) {
		text := htmlFragmentToText(match[1])
		if text != "" {
			lines = append(lines, "- "+text)
		}
	}
	for _, table := range tablePattern.FindAllStringSubmatch(cleaned, -1) {
		lines = append(lines, "[表格]")
		for _, row := range tableRowPattern.FindAllStringSubmatch(table[1], -1) {
			var cells []string
			for _, cell := range tableCellPattern.FindAllStringSubmatch(row[1], -1) {
				cells = append(cells, htmlFragmentToText(cell[1]))
			}
			if len(cells) > 0 {
				lines = append(lines, strings.Join(cells, " | "))
			}
		}
	}
	for _, match := range imageTagPattern.FindAllStringSubmatch(cleaned, -1) {
		label := firstHTMLAttr(match[0], "alt")
		if label == "" {
			label = firstHTMLAttr(match[0], "data-note-src")
		}
		if label == "" {
			label = firstHTMLAttr(match[0], "src")
		}
		lines = append(lines, "[图片: "+firstNonEmpty(label, "未命名")+"]")
	}
	if len(lines) == 0 {
		return htmlFragmentToText(fragment)
	}
	return normalizeNoteViewText(strings.Join(lines, "\n"))
}

func htmlFragmentToFormatView(fragment string) string {
	cleaned := stripIgnoredHTML(fragment)
	var lines []string
	appendBlock := func(kind, tag, attrs, content string) {
		text := htmlFragmentToText(content)
		if text == "" {
			return
		}
		format := summarizeHTMLAttributes(attrs)
		if format == "" {
			format = "default"
		}
		lines = append(lines, fmt.Sprintf("%s %s: %s", kind, format, truncatePlainRunes(text, 140)))
	}
	for _, match := range headingPattern.FindAllStringSubmatch(cleaned, -1) {
		appendBlock("heading-"+match[1], "h"+match[1], match[2], match[3])
	}
	for _, match := range paragraphPattern.FindAllStringSubmatch(cleaned, -1) {
		appendBlock("paragraph", "p", match[1], match[2])
	}
	for _, match := range tablePattern.FindAllStringSubmatch(cleaned, -1) {
		rows := len(tableRowPattern.FindAllStringSubmatch(match[1], -1))
		lines = append(lines, fmt.Sprintf("table rows=%d", rows))
	}
	for _, match := range imageTagPattern.FindAllStringSubmatch(cleaned, -1) {
		lines = append(lines, "image "+summarizeHTMLAttributes(match[1]))
	}
	if len(lines) == 0 {
		return "no explicit formatting detected"
	}
	return strings.Join(lines, "\n")
}

func stripIgnoredHTML(fragment string) string {
	text := scriptStylePattern.ReplaceAllString(fragment, "")
	text = regexp.MustCompile(`(?is)<!--.*?-->`).ReplaceAllString(text, "")
	return text
}

func normalizeNoteViewText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.TrimSpace(regexp.MustCompile(`[ \t]+`).ReplaceAllString(line, " "))
		if line == "" {
			if !blank && len(out) > 0 {
				out = append(out, "")
			}
			blank = true
			continue
		}
		out = append(out, line)
		blank = false
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func summarizeHTMLAttributes(attrs string) string {
	attrs = strings.TrimSpace(attrs)
	if attrs == "" {
		return ""
	}
	var parts []string
	if align := firstHTMLAttr(attrs, "data-text-align"); align != "" {
		parts = append(parts, "align="+align)
	}
	if className := firstHTMLAttr(attrs, "class"); className != "" {
		parts = append(parts, "class="+className)
	}
	if style := firstHTMLAttr(attrs, "style"); style != "" {
		for _, key := range []string{"text-align", "font-size", "color", "background-color"} {
			if value := cssStyleValue(style, key); value != "" {
				parts = append(parts, key+"="+value)
			}
		}
	}
	if src := firstHTMLAttr(attrs, "data-note-src"); src != "" {
		parts = append(parts, "src="+src)
	}
	return strings.Join(parts, ", ")
}

func firstHTMLAttr(tagOrAttrs, name string) string {
	pattern := regexp.MustCompile(`(?is)\b` + regexp.QuoteMeta(name) + `\s*=\s*("([^"]*)"|'([^']*)'|([^\s>]+))`)
	match := pattern.FindStringSubmatch(tagOrAttrs)
	if len(match) == 0 {
		return ""
	}
	for _, value := range match[2:] {
		if value != "" {
			return htmlstd.UnescapeString(value)
		}
	}
	return ""
}

func cssStyleValue(style, key string) string {
	for _, part := range strings.Split(style, ";") {
		name, value, ok := strings.Cut(part, ":")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(name), key) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func atoi(value string) int {
	var n int
	for _, r := range value {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var (
	scriptStylePattern = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>|<style\b[^>]*>.*?</style>`)
	htmlTagPattern     = regexp.MustCompile(`(?is)<[^>]+>`)
	imageTagPattern    = regexp.MustCompile(`(?is)<img\b([^>]*)>`)
	headingPattern     = regexp.MustCompile(`(?is)<h([1-6])\b([^>]*)>(.*?)</h[1-6]>`)
	paragraphPattern   = regexp.MustCompile(`(?is)<p\b([^>]*)>(.*?)</p>`)
	listItemPattern    = regexp.MustCompile(`(?is)<li\b[^>]*>(.*?)</li>`)
	tablePattern       = regexp.MustCompile(`(?is)<table\b[^>]*>(.*?)</table>`)
	tableRowPattern    = regexp.MustCompile(`(?is)<tr\b[^>]*>(.*?)</tr>`)
	tableCellPattern   = regexp.MustCompile(`(?is)<(?:td|th)\b[^>]*>(.*?)</(?:td|th)>`)
)
