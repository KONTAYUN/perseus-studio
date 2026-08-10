package ai

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	nethtml "golang.org/x/net/html"
)

const (
	defaultNoteIndexBlocks = 80
	maxNoteIndexBlocks     = 500
	defaultNoteBlockRunes  = 20000
	maxNoteBlockRunes      = 50000
)

type noteBlockDocument struct {
	DocHash       string
	Blocks        []noteBlock
	Sections      []noteSection
	TotalTextRune int
	TotalRawRune  int
}

type noteBlock struct {
	ID          string `json:"block_id"`
	Tag         string `json:"tag"`
	SectionID   string `json:"section_id,omitempty"`
	HeadingPath string `json:"heading_path,omitempty"`
	TextPreview string `json:"text_preview,omitempty"`
	TextRunes   int    `json:"text_chars"`
	RawRunes    int    `json:"raw_chars"`
	Level       int    `json:"level,omitempty"`
	HasImages   bool   `json:"has_images,omitempty"`
	HasTables   bool   `json:"has_tables,omitempty"`
	HasStyles   bool   `json:"has_styles,omitempty"`
	RawHTML     string `json:"-"`
	Text        string `json:"-"`
}

func noteBlockHash(block noteBlock) string {
	return noteDocHash(block.RawHTML)
}

func noteBlockTextSegments(block noteBlock) []map[string]any {
	root, err := nethtml.Parse(strings.NewReader("<!doctype html><html><body>" + block.RawHTML + "</body></html>"))
	if err != nil {
		return nil
	}
	body := findFirstHTMLNode(root, "body")
	if body == nil {
		return nil
	}
	segments := make([]map[string]any, 0)
	index := 0
	var walk func(*nethtml.Node, []string)
	walk = func(node *nethtml.Node, ancestors []string) {
		if node == nil {
			return
		}
		if node.Type == nethtml.TextNode {
			index++
			segments = append(segments, map[string]any{
				"segment_id":    fmt.Sprintf("%s:s%04d", block.ID, index),
				"block_id":      block.ID,
				"text":          node.Data,
				"text_chars":    len([]rune(node.Data)),
				"ancestor_tags": append([]string(nil), ancestors...),
				"block_hash":    noteBlockHash(block),
			})
			return
		}
		nextAncestors := ancestors
		if node.Type == nethtml.ElementNode {
			nextAncestors = append(append([]string(nil), ancestors...), strings.ToLower(node.Data))
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child, nextAncestors)
		}
	}
	for child := body.FirstChild; child != nil; child = child.NextSibling {
		walk(child, nil)
	}
	return segments
}

type noteIndexBlockPage struct {
	Blocks         []map[string]any
	StartBlock     int
	NextStartBlock int
	HasMore        bool
}

type noteSection struct {
	ID             string `json:"section_id"`
	HeadingBlockID string `json:"heading_block_id,omitempty"`
	Title          string `json:"title"`
	HeadingPath    string `json:"heading_path,omitempty"`
	Level          int    `json:"level"`
	StartBlockID   string `json:"start_block_id,omitempty"`
	EndBlockID     string `json:"end_block_id,omitempty"`
	BlockCount     int    `json:"block_count"`
	TextRunes      int    `json:"text_chars"`
	RawRunes       int    `json:"raw_chars"`
	HasImages      bool   `json:"has_images,omitempty"`
	HasTables      bool   `json:"has_tables,omitempty"`
	HasStyles      bool   `json:"has_styles,omitempty"`
}

func buildNoteBlockDocument(fragment string) noteBlockDocument {
	doc := noteBlockDocument{DocHash: noteDocHash(fragment)}
	root, err := nethtml.Parse(strings.NewReader("<!doctype html><html><body>" + fragment + "</body></html>"))
	if err != nil {
		text := htmlFragmentToText(fragment)
		raw := strings.TrimSpace(fragment)
		if raw != "" || text != "" {
			doc.Blocks = []noteBlock{newNoteBlock("b0001", "fragment", "", 0, raw, text)}
		}
		finalizeNoteSections(&doc)
		return doc
	}
	nodes := noteBodyChildren(root)
	var headingStack []string
	var walk func(*nethtml.Node)
	walk = func(n *nethtml.Node) {
		if n == nil {
			return
		}
		if isNoteBlockNode(n) {
			raw := renderHTMLNode(n)
			text := htmlFragmentToText(raw)
			level := headingLevel(n.Data)
			if level > 0 {
				title := firstNonEmpty(text, strings.TrimSpace(n.Data))
				headingStack = updateHeadingStack(headingStack, level, title)
			}
			headingPath := strings.Join(headingStack, " > ")
			id := fmt.Sprintf("b%04d", len(doc.Blocks)+1)
			block := newNoteBlock(id, strings.ToLower(n.Data), headingPath, level, raw, text)
			block.HasImages = strings.EqualFold(n.Data, "img") || hasDescendantTag(n, "img")
			block.HasTables = strings.EqualFold(n.Data, "table") || hasDescendantTag(n, "table")
			block.HasStyles = hasStylingAttribute(n)
			doc.Blocks = append(doc.Blocks, block)
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	for _, node := range nodes {
		walk(node)
	}
	if rawBlocks := collectRawNoteBlockHTML(fragment); len(rawBlocks) == len(doc.Blocks) {
		for index, raw := range rawBlocks {
			replaceNoteBlockRaw(&doc.Blocks[index], raw)
		}
	}
	finalizeNoteSections(&doc)
	return doc
}

func noteBodyChildren(root *nethtml.Node) []*nethtml.Node {
	body := findFirstHTMLNode(root, "body")
	if body == nil {
		body = root
	}
	var nodes []*nethtml.Node
	for child := body.FirstChild; child != nil; child = child.NextSibling {
		nodes = append(nodes, child)
	}
	return nodes
}

func findFirstHTMLNode(root *nethtml.Node, tag string) *nethtml.Node {
	if root == nil {
		return nil
	}
	if root.Type == nethtml.ElementNode && strings.EqualFold(root.Data, tag) {
		return root
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if found := findFirstHTMLNode(child, tag); found != nil {
			return found
		}
	}
	return nil
}

func newNoteBlock(id, tag, headingPath string, level int, rawHTML, text string) noteBlock {
	text = normalizeNoteViewText(text)
	return noteBlock{
		ID:          id,
		Tag:         strings.ToLower(strings.TrimSpace(tag)),
		HeadingPath: headingPath,
		TextPreview: truncatePlainRunes(text, 160),
		TextRunes:   len([]rune(text)),
		RawRunes:    len([]rune(rawHTML)),
		Level:       level,
		RawHTML:     strings.TrimSpace(rawHTML),
		Text:        text,
	}
}

func noteDocHash(fragment string) string {
	sum := sha256.Sum256([]byte(fragment))
	return hex.EncodeToString(sum[:])
}

func noteDocumentVersionMatches(expected, current string) bool {
	expected = strings.TrimSpace(expected)
	current = strings.TrimSpace(current)
	return expected != "" && expected == current
}

func isNoteBlockNode(n *nethtml.Node) bool {
	if n == nil || n.Type != nethtml.ElementNode {
		return false
	}
	return isNoteBlockTag(n.Data)
}

func isNoteBlockTag(tag string) bool {
	switch strings.ToLower(strings.TrimSpace(tag)) {
	case "h1", "h2", "h3", "h4", "h5", "h6", "p", "blockquote", "li", "pre", "table", "img":
		return true
	default:
		return false
	}
}

type noteRawBlockCapture struct {
	tag   string
	depth int
	raw   strings.Builder
}

func collectRawNoteBlockHTML(fragment string) []string {
	tokenizer := nethtml.NewTokenizer(strings.NewReader(fragment))
	var blocks []string
	var capture *noteRawBlockCapture
	for {
		tokenType := tokenizer.Next()
		if tokenType == nethtml.ErrorToken {
			if tokenizer.Err() == io.EOF {
				break
			}
			return nil
		}
		raw := string(tokenizer.Raw())
		token := tokenizer.Token()
		tag := strings.ToLower(strings.TrimSpace(token.Data))
		if capture != nil {
			capture.raw.WriteString(raw)
			switch tokenType {
			case nethtml.StartTagToken:
				if tag == capture.tag && !isHTMLVoidElement(tag) {
					capture.depth++
				}
			case nethtml.EndTagToken:
				if tag == capture.tag {
					capture.depth--
					if capture.depth <= 0 {
						blocks = append(blocks, capture.raw.String())
						capture = nil
					}
				}
			}
			continue
		}
		if (tokenType == nethtml.StartTagToken || tokenType == nethtml.SelfClosingTagToken) && isNoteBlockTag(tag) {
			capture = &noteRawBlockCapture{tag: tag, depth: 1}
			capture.raw.WriteString(raw)
			if tokenType == nethtml.SelfClosingTagToken || isHTMLVoidElement(tag) {
				blocks = append(blocks, capture.raw.String())
				capture = nil
			}
		}
	}
	if capture != nil && strings.TrimSpace(capture.raw.String()) != "" {
		blocks = append(blocks, capture.raw.String())
	}
	return blocks
}

func replaceNoteBlockRaw(block *noteBlock, raw string) {
	if block == nil {
		return
	}
	raw = strings.TrimSpace(raw)
	text := normalizeNoteViewText(htmlFragmentToText(raw))
	block.RawHTML = raw
	block.Text = text
	block.TextPreview = truncatePlainRunes(text, 160)
	block.TextRunes = len([]rune(text))
	block.RawRunes = len([]rune(raw))
	block.HasImages = strings.EqualFold(block.Tag, "img") || noteRawHasTag(raw, "img")
	block.HasTables = strings.EqualFold(block.Tag, "table") || noteRawHasTag(raw, "table")
	block.HasStyles = noteRawHasStyles(raw)
}

func noteRawHasTag(raw, tag string) bool {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return false
	}
	tokenizer := nethtml.NewTokenizer(strings.NewReader(raw))
	for {
		tokenType := tokenizer.Next()
		if tokenType == nethtml.ErrorToken {
			return false
		}
		if tokenType != nethtml.StartTagToken && tokenType != nethtml.SelfClosingTagToken {
			continue
		}
		if strings.EqualFold(tokenizer.Token().Data, tag) {
			return true
		}
	}
}

func noteRawHasStyles(raw string) bool {
	tokenizer := nethtml.NewTokenizer(strings.NewReader(raw))
	for {
		tokenType := tokenizer.Next()
		if tokenType == nethtml.ErrorToken {
			return false
		}
		if tokenType != nethtml.StartTagToken && tokenType != nethtml.SelfClosingTagToken {
			continue
		}
		for _, attr := range tokenizer.Token().Attr {
			key := strings.ToLower(strings.TrimSpace(attr.Key))
			if (key == "style" || key == "class" || strings.HasPrefix(key, "data-")) && strings.TrimSpace(attr.Val) != "" {
				return true
			}
		}
	}
}

func isHTMLVoidElement(tag string) bool {
	switch strings.ToLower(strings.TrimSpace(tag)) {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr":
		return true
	default:
		return false
	}
}

func headingLevel(tag string) int {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if len(tag) != 2 || tag[0] != 'h' || tag[1] < '1' || tag[1] > '6' {
		return 0
	}
	return int(tag[1] - '0')
}

func updateHeadingStack(stack []string, level int, title string) []string {
	if level <= 0 {
		return stack
	}
	if len(stack) < level {
		next := make([]string, level)
		copy(next, stack)
		stack = next
	} else {
		stack = stack[:level]
	}
	stack[level-1] = strings.TrimSpace(title)
	return stack
}

func renderHTMLNode(n *nethtml.Node) string {
	if n == nil {
		return ""
	}
	var buf bytes.Buffer
	if err := nethtml.Render(&buf, n); err != nil {
		return ""
	}
	return buf.String()
}

func hasDescendantTag(n *nethtml.Node, tag string) bool {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" || n == nil {
		return false
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == nethtml.ElementNode && strings.EqualFold(child.Data, tag) {
			return true
		}
		if hasDescendantTag(child, tag) {
			return true
		}
	}
	return false
}

func hasStylingAttribute(n *nethtml.Node) bool {
	if n == nil {
		return false
	}
	if n.Type == nethtml.ElementNode {
		for _, attr := range n.Attr {
			key := strings.ToLower(strings.TrimSpace(attr.Key))
			if key == "style" || key == "class" || strings.HasPrefix(key, "data-") {
				if strings.TrimSpace(attr.Val) != "" {
					return true
				}
			}
		}
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if hasStylingAttribute(child) {
			return true
		}
	}
	return false
}

func finalizeNoteSections(doc *noteBlockDocument) {
	if doc == nil || len(doc.Blocks) == 0 {
		return
	}
	for index := range doc.Blocks {
		doc.TotalTextRune += doc.Blocks[index].TextRunes
		doc.TotalRawRune += doc.Blocks[index].RawRunes
	}
	root := noteSection{
		ID:           "s0000",
		Title:        "document",
		Level:        0,
		StartBlockID: doc.Blocks[0].ID,
		EndBlockID:   doc.Blocks[len(doc.Blocks)-1].ID,
	}
	for index := range doc.Blocks {
		addBlockToSection(&root, doc.Blocks[index])
	}
	doc.Sections = append(doc.Sections, root)
	headingOrdinal := 0
	for index, block := range doc.Blocks {
		if block.Level <= 0 {
			continue
		}
		headingOrdinal++
		end := len(doc.Blocks) - 1
		for next := index + 1; next < len(doc.Blocks); next++ {
			if doc.Blocks[next].Level > 0 && doc.Blocks[next].Level <= block.Level {
				end = next - 1
				break
			}
		}
		section := noteSection{
			ID:             fmt.Sprintf("s%04d", headingOrdinal),
			HeadingBlockID: block.ID,
			Title:          firstNonEmpty(block.Text, block.TextPreview, block.ID),
			HeadingPath:    block.HeadingPath,
			Level:          block.Level,
			StartBlockID:   block.ID,
			EndBlockID:     doc.Blocks[end].ID,
		}
		for blockIndex := index; blockIndex <= end; blockIndex++ {
			addBlockToSection(&section, doc.Blocks[blockIndex])
		}
		doc.Sections = append(doc.Sections, section)
	}
	assignNearestSections(doc)
}

func addBlockToSection(section *noteSection, block noteBlock) {
	section.BlockCount++
	section.TextRunes += block.TextRunes
	section.RawRunes += block.RawRunes
	section.HasImages = section.HasImages || block.HasImages
	section.HasTables = section.HasTables || block.HasTables
	section.HasStyles = section.HasStyles || block.HasStyles
}

func assignNearestSections(doc *noteBlockDocument) {
	for blockIndex := range doc.Blocks {
		selected := "s0000"
		selectedStart := -1
		for _, section := range doc.Sections {
			if section.ID == "s0000" {
				continue
			}
			start := blockIndexByID(doc.Blocks, section.StartBlockID)
			end := blockIndexByID(doc.Blocks, section.EndBlockID)
			if start >= 0 && end >= start && blockIndex >= start && blockIndex <= end && start >= selectedStart {
				selected = section.ID
				selectedStart = start
			}
		}
		doc.Blocks[blockIndex].SectionID = selected
	}
}

func blockIndexByID(blocks []noteBlock, id string) int {
	for index, block := range blocks {
		if block.ID == id {
			return index
		}
	}
	return -1
}

func noteIndexPage(blocks []noteBlock, startBlock, maxBlocks int) noteIndexBlockPage {
	if maxBlocks <= 0 {
		maxBlocks = defaultNoteIndexBlocks
	}
	if maxBlocks > maxNoteIndexBlocks {
		maxBlocks = maxNoteIndexBlocks
	}
	if startBlock <= 0 {
		startBlock = 1
	}
	startIndex := startBlock - 1
	if startIndex > len(blocks) {
		startIndex = len(blocks)
	}
	endIndex := startIndex + maxBlocks
	if endIndex > len(blocks) {
		endIndex = len(blocks)
	}
	items, _ := noteIndexPayloadBlocks(blocks[startIndex:endIndex], maxBlocks)
	nextStart := 0
	hasMore := endIndex < len(blocks)
	if hasMore {
		nextStart = endIndex + 1
	}
	return noteIndexBlockPage{
		Blocks:         items,
		StartBlock:     startIndex + 1,
		NextStartBlock: nextStart,
		HasMore:        hasMore,
	}
}

func noteIndexPayloadBlocks(blocks []noteBlock, maxBlocks int) ([]map[string]any, bool) {
	if maxBlocks <= 0 {
		maxBlocks = defaultNoteIndexBlocks
	}
	if maxBlocks > maxNoteIndexBlocks {
		maxBlocks = maxNoteIndexBlocks
	}
	truncated := len(blocks) > maxBlocks
	if truncated {
		blocks = blocks[:maxBlocks]
	}
	items := make([]map[string]any, 0, len(blocks))
	for _, block := range blocks {
		items = append(items, map[string]any{
			"block_id":     block.ID,
			"tag":          block.Tag,
			"section_id":   block.SectionID,
			"heading_path": block.HeadingPath,
			"text_preview": block.TextPreview,
			"text_chars":   block.TextRunes,
			"raw_chars":    block.RawRunes,
			"level":        block.Level,
			"has_images":   block.HasImages,
			"has_tables":   block.HasTables,
			"has_styles":   block.HasStyles,
		})
	}
	return items, truncated
}

func noteIndexPayloadSections(sections []noteSection) []map[string]any {
	items := make([]map[string]any, 0, len(sections))
	for _, section := range sections {
		items = append(items, map[string]any{
			"section_id":       section.ID,
			"heading_block_id": section.HeadingBlockID,
			"title":            section.Title,
			"heading_path":     section.HeadingPath,
			"level":            section.Level,
			"start_block_id":   section.StartBlockID,
			"end_block_id":     section.EndBlockID,
			"block_count":      section.BlockCount,
			"text_chars":       section.TextRunes,
			"raw_chars":        section.RawRunes,
			"has_images":       section.HasImages,
			"has_tables":       section.HasTables,
			"has_styles":       section.HasStyles,
		})
	}
	return items
}

func findNoteBlock(blocks []noteBlock, id string) (noteBlock, bool) {
	for _, block := range blocks {
		if block.ID == id {
			return block, true
		}
	}
	return noteBlock{}, false
}

func findNoteSection(doc noteBlockDocument, id string) (noteSection, []noteBlock, bool) {
	for _, section := range doc.Sections {
		if section.ID != id {
			continue
		}
		start := blockIndexByID(doc.Blocks, section.StartBlockID)
		end := blockIndexByID(doc.Blocks, section.EndBlockID)
		if start < 0 || end < start {
			return section, nil, true
		}
		return section, doc.Blocks[start : end+1], true
	}
	return noteSection{}, nil, false
}

func renderNoteBlocks(blocks []noteBlock, view string) string {
	if len(blocks) == 0 {
		return ""
	}
	var builder strings.Builder
	for index, block := range blocks {
		if index > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(block.RawHTML)
	}
	return renderNoteView(builder.String(), view)
}

func limitNoteContent(content string, maxRunes int) (string, bool) {
	if maxRunes <= 0 {
		maxRunes = defaultNoteBlockRunes
	}
	if maxRunes > maxNoteBlockRunes {
		maxRunes = maxNoteBlockRunes
	}
	runes := []rune(content)
	if len(runes) <= maxRunes {
		return content, false
	}
	return string(runes[:maxRunes]), true
}
