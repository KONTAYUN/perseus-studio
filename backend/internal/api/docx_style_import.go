package api

import (
	"archive/zip"
	"bytes"
	"fmt"
	htmlstd "html"
	"math"
	"regexp"
	"strconv"
	"strings"

	nethtml "golang.org/x/net/html"
)

func enhanceImportedDocxHTML(docxBytes []byte, htmlFragment string) string {
	plan, err := extractDocxImportStylePlan(docxBytes)
	if err != nil || plan.isEmpty() {
		return htmlFragment
	}
	enhanced, err := applyDocxImportStylePlanToHTML(htmlFragment, plan)
	if err != nil {
		return htmlFragment
	}
	return enhanced
}

func extractDocxImportStylePlan(docxBytes []byte) (docxStylePlan, error) {
	documentXML, ok := readDocxDocumentXMLFromBytes(docxBytes)
	if !ok {
		return docxStylePlan{}, fmt.Errorf("docx missing word/document.xml")
	}
	bodyWidth := wordBodyWidthEMU(documentXML)
	tableRanges := wordElementRanges(documentXML, wordTablePattern)
	var plan docxStylePlan

	for _, loc := range wordParagraphPattern.FindAllStringIndex(documentXML, -1) {
		paragraphXML := documentXML[loc[0]:loc[1]]
		if rangeInsideAny(loc[0], loc[1], tableRanges) {
			continue
		}
		text := wordTextFromXML(paragraphXML)
		if text != "" {
			plan.Blocks = append(plan.Blocks, docxBlockStyle{
				Text:  text,
				Align: extractWordParagraphAlign(paragraphXML),
				Runs:  extractWordRunStyles(paragraphXML),
			})
		}
		plan.Images = append(plan.Images, extractWordImageStyles(paragraphXML, bodyWidth)...)
	}

	for _, loc := range wordTableCellPattern.FindAllStringIndex(documentXML, -1) {
		cellXML := documentXML[loc[0]:loc[1]]
		plan.TableCells = append(plan.TableCells, docxTableCellStyle{
			Text:            wordTextFromXML(cellXML),
			Align:           extractWordTableCellAlign(cellXML),
			BackgroundColor: extractWordTableCellBackground(cellXML),
		})
	}

	return plan, nil
}

func applyDocxImportStylePlanToHTML(htmlFragment string, plan docxStylePlan) (string, error) {
	doc, err := nethtml.Parse(strings.NewReader(wrapHTMLFragmentDocument(htmlFragment)))
	if err != nil {
		return "", err
	}
	body := findHTMLElement(doc, "body")
	if body == nil {
		return htmlFragment, nil
	}
	state := &docxImportApplyState{}
	applyDocxImportStylesToHTML(body, false, plan, state)
	removeDocxImportMarkers(body)
	return renderHTMLChildren(body), nil
}

type docxImportApplyState struct {
	BlockIndex     int
	TableCellIndex int
	ImageIndex     int
}

func applyDocxImportStylesToHTML(node *nethtml.Node, inTable bool, plan docxStylePlan, state *docxImportApplyState) {
	if node == nil || state == nil {
		return
	}
	nextInTable := inTable
	if node.Type == nethtml.ElementNode {
		tag := strings.ToLower(node.Data)
		switch {
		case tag == "table":
			nextInTable = true
		case isDocxBlockTag(tag) && !inTable:
			applyNextImportedBlockStyle(node, plan.Blocks, state)
		case (tag == "td" || tag == "th") && inTable:
			applyNextImportedTableCellStyle(node, plan.TableCells, state)
		case tag == "img":
			applyNextImportedImageStyle(node, plan.Images, state)
		}
	}
	for child := node.FirstChild; child != nil; {
		next := child.NextSibling
		applyDocxImportStylesToHTML(child, nextInTable, plan, state)
		child = next
	}
}

func applyNextImportedBlockStyle(node *nethtml.Node, blocks []docxBlockStyle, state *docxImportApplyState) {
	text := normalizeDocxMatchText(htmlNodeText(node))
	if text == "" {
		return
	}
	style, ok := matchNextBlockStyle(blocks, &state.BlockIndex, text)
	if !ok {
		return
	}
	if style.Align != "" {
		setNodeCSS(node, map[string]string{"text-align": style.Align})
	}
	applyImportedRunStyles(node, style.Runs)
}

func applyNextImportedTableCellStyle(node *nethtml.Node, cells []docxTableCellStyle, state *docxImportApplyState) {
	for state.TableCellIndex < len(cells) && cells[state.TableCellIndex].Text == "" {
		state.TableCellIndex++
	}
	if state.TableCellIndex >= len(cells) {
		return
	}
	cell := cells[state.TableCellIndex]
	state.TableCellIndex++
	updates := make(map[string]string)
	if cell.Align != "" {
		updates["text-align"] = cell.Align
	}
	if cell.BackgroundColor != "" {
		updates["background-color"] = "#" + cell.BackgroundColor
	}
	if len(updates) > 0 {
		setNodeCSS(node, updates)
	}
}

func applyNextImportedImageStyle(node *nethtml.Node, images []docxImageStyle, state *docxImportApplyState) {
	if state.ImageIndex >= len(images) {
		return
	}
	image := images[state.ImageIndex]
	state.ImageIndex++
	updates := make(map[string]string)
	if image.Align != "" {
		setNodeAttr(node, "data-align", image.Align)
		switch image.Align {
		case "left":
			updates["margin-left"] = "0"
			updates["margin-right"] = "auto"
		case "right":
			updates["margin-left"] = "auto"
			updates["margin-right"] = "0"
		default:
			updates["margin-left"] = "auto"
			updates["margin-right"] = "auto"
		}
		updates["display"] = "block"
	}
	if image.WidthPercent > 0 {
		width := strconv.Itoa(image.WidthPercent)
		setNodeAttr(node, "data-width", width)
		updates["width"] = width + "%"
		updates["max-width"] = "100%"
	}
	if len(updates) > 0 {
		setNodeCSS(node, updates)
	}
}

func applyImportedRunStyles(root *nethtml.Node, runs []docxRunStyle) {
	for _, run := range runs {
		if !run.hasStyle() || run.Text == "" {
			continue
		}
		if node := findTextNodeForImportedRun(root, run.Text); node != nil {
			wrapImportedTextRun(node, run)
		}
	}
}

func findTextNodeForImportedRun(root *nethtml.Node, text string) *nethtml.Node {
	if root == nil {
		return nil
	}
	if root.Type == nethtml.TextNode && !hasDocxImportMarker(root) {
		if strings.Contains(normalizeDocxMatchText(root.Data), normalizeDocxMatchText(text)) {
			return root
		}
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if found := findTextNodeForImportedRun(child, text); found != nil {
			return found
		}
	}
	return nil
}

func wrapImportedTextRun(node *nethtml.Node, run docxRunStyle) {
	if node == nil || node.Parent == nil {
		return
	}
	target := normalizeDocxMatchText(run.Text)
	if target == "" {
		return
	}
	index := strings.Index(node.Data, target)
	if index < 0 {
		return
	}
	before := node.Data[:index]
	match := node.Data[index : index+len(target)]
	after := node.Data[index+len(target):]
	parent := node.Parent
	if before != "" {
		parent.InsertBefore(&nethtml.Node{Type: nethtml.TextNode, Data: before}, node)
	}
	span := &nethtml.Node{
		Type: nethtml.ElementNode,
		Data: "span",
		Attr: []nethtml.Attribute{{Key: "data-docx-import-run", Val: "true"}},
	}
	span.AppendChild(&nethtml.Node{Type: nethtml.TextNode, Data: match})
	setNodeCSS(span, importedRunCSS(run))
	parent.InsertBefore(span, node)
	if after != "" {
		parent.InsertBefore(&nethtml.Node{Type: nethtml.TextNode, Data: after}, node)
	}
	parent.RemoveChild(node)
}

func importedRunCSS(run docxRunStyle) map[string]string {
	styles := make(map[string]string)
	if run.Color != "" {
		styles["color"] = "#" + run.Color
	}
	if run.BackgroundColor != "" {
		styles["background-color"] = "#" + run.BackgroundColor
	}
	if run.FontSizeHalfPoints > 0 {
		styles["font-size"] = strconv.Itoa(docxHalfPointsToPixels(run.FontSizeHalfPoints)) + "px"
	}
	return styles
}

func removeDocxImportMarkers(node *nethtml.Node) {
	if node == nil {
		return
	}
	removeNodeAttr(node, "data-docx-import-run")
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		removeDocxImportMarkers(child)
	}
}

func hasDocxImportMarker(node *nethtml.Node) bool {
	for current := node; current != nil; current = current.Parent {
		if nodeAttr(current, "data-docx-import-run") == "true" {
			return true
		}
	}
	return false
}

func extractWordRunStyles(paragraphXML string) []docxRunStyle {
	var runs []docxRunStyle
	for _, loc := range wordRunPattern.FindAllStringIndex(paragraphXML, -1) {
		runXML := paragraphXML[loc[0]:loc[1]]
		text := wordTextFromXML(runXML)
		if text == "" {
			continue
		}
		runs = append(runs, docxRunStyle{
			Text:               text,
			Color:              extractWordRunColor(runXML),
			BackgroundColor:    extractWordRunBackground(runXML),
			FontSizeHalfPoints: extractWordRunFontSizeHalfPoints(runXML),
		})
	}
	return runs
}

func extractWordImageStyles(paragraphXML string, bodyWidthEMU int64) []docxImageStyle {
	align := extractWordParagraphAlign(paragraphXML)
	var images []docxImageStyle
	for _, loc := range wordDrawingPattern.FindAllStringIndex(paragraphXML, -1) {
		drawingXML := paragraphXML[loc[0]:loc[1]]
		widthPercent := 0
		if cx, _, ok := firstDrawingExtent(drawingXML); ok && bodyWidthEMU > 0 {
			widthPercent = clampDocxImportPercent(int(math.Round(float64(cx) * 100 / float64(bodyWidthEMU))))
		}
		if align != "" || widthPercent > 0 {
			images = append(images, docxImageStyle{
				Align:        align,
				WidthPercent: widthPercent,
			})
		}
	}
	return images
}

func extractWordParagraphAlign(paragraphXML string) string {
	match := regexp.MustCompile(`(?s)<w:pPr\b[^>]*>.*?</w:pPr>`).FindString(paragraphXML)
	if match == "" {
		return ""
	}
	return normalizeDocxAlign(wordXMLAttr(regexp.MustCompile(`<w:jc\b[^>]*/?>`).FindString(match), "w:val"))
}

func extractWordTableCellAlign(cellXML string) string {
	for _, loc := range wordParagraphPattern.FindAllStringIndex(cellXML, -1) {
		if align := extractWordParagraphAlign(cellXML[loc[0]:loc[1]]); align != "" {
			return align
		}
	}
	return ""
}

func extractWordTableCellBackground(cellXML string) string {
	tcPr := regexp.MustCompile(`(?s)<w:tcPr\b[^>]*>.*?</w:tcPr>`).FindString(cellXML)
	return extractWordShadingFill(tcPr)
}

func extractWordRunColor(runXML string) string {
	color := wordXMLAttr(regexp.MustCompile(`<w:color\b[^>]*/?>`).FindString(runXML), "w:val")
	return normalizeDocxHexColor("#" + strings.TrimSpace(color))
}

func extractWordRunBackground(runXML string) string {
	if background := extractWordShadingFill(runXML); background != "" {
		return background
	}
	highlight := wordXMLAttr(regexp.MustCompile(`<w:highlight\b[^>]*/?>`).FindString(runXML), "w:val")
	return wordHighlightColor(highlight)
}

func extractWordRunFontSizeHalfPoints(runXML string) int {
	value := wordXMLAttr(regexp.MustCompile(`<w:sz\b[^>]*/?>`).FindString(runXML), "w:val")
	size, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || size < 12 || size > 192 {
		return 0
	}
	return size
}

func extractWordShadingFill(xml string) string {
	fill := wordXMLAttr(regexp.MustCompile(`<w:shd\b[^>]*/?>`).FindString(xml), "w:fill")
	fill = strings.TrimSpace(fill)
	if strings.EqualFold(fill, "auto") || fill == "" {
		return ""
	}
	return normalizeDocxHexColor("#" + fill)
}

func wordHighlightColor(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "black":
		return "000000"
	case "blue":
		return "0000FF"
	case "cyan":
		return "00FFFF"
	case "green":
		return "00FF00"
	case "magenta":
		return "FF00FF"
	case "red":
		return "FF0000"
	case "yellow":
		return "FFFF00"
	case "white":
		return "FFFFFF"
	case "darkblue":
		return "000080"
	case "darkcyan":
		return "008080"
	case "darkgreen":
		return "008000"
	case "darkmagenta":
		return "800080"
	case "darkred":
		return "800000"
	case "darkyellow":
		return "808000"
	case "darkgray":
		return "808080"
	case "lightgray":
		return "C0C0C0"
	default:
		return ""
	}
}

func wordXMLAttr(tag, name string) string {
	if tag == "" || name == "" {
		return ""
	}
	pattern := regexp.MustCompile(regexp.QuoteMeta(name) + `="([^"]*)"`)
	match := pattern.FindStringSubmatch(tag)
	if len(match) < 2 {
		return ""
	}
	return htmlstd.UnescapeString(match[1])
}

func docxHalfPointsToPixels(halfPoints int) int {
	return int(math.Round(float64(halfPoints) * 2 / 3))
}

func clampDocxImportPercent(value int) int {
	if value <= 0 {
		return 0
	}
	if value < 20 {
		return 20
	}
	if value > 100 {
		return 100
	}
	return value
}

func setNodeCSS(node *nethtml.Node, updates map[string]string) {
	if node == nil || len(updates) == 0 {
		return
	}
	styles := parseCSSDeclarations(nodeAttr(node, "style"))
	for key, value := range updates {
		if strings.TrimSpace(value) != "" {
			styles[strings.ToLower(key)] = value
		}
	}
	setNodeAttr(node, "style", serializeCSSDeclarations(styles))
}

func serializeCSSDeclarations(styles map[string]string) string {
	order := []string{"text-align", "color", "background-color", "font-size", "width", "max-width", "display", "margin-left", "margin-right"}
	var parts []string
	seen := make(map[string]struct{}, len(styles))
	for _, key := range order {
		if value := strings.TrimSpace(styles[key]); value != "" {
			parts = append(parts, key+":"+value)
			seen[key] = struct{}{}
		}
	}
	for key, value := range styles {
		if _, ok := seen[key]; ok || strings.TrimSpace(value) == "" {
			continue
		}
		parts = append(parts, key+":"+strings.TrimSpace(value))
	}
	return strings.Join(parts, ";")
}

func setNodeAttr(node *nethtml.Node, name, value string) {
	if node == nil {
		return
	}
	for index := range node.Attr {
		if strings.EqualFold(node.Attr[index].Key, name) {
			node.Attr[index].Val = value
			return
		}
	}
	node.Attr = append(node.Attr, nethtml.Attribute{Key: name, Val: value})
}

func removeNodeAttr(node *nethtml.Node, name string) {
	if node == nil {
		return
	}
	filtered := node.Attr[:0]
	for _, attr := range node.Attr {
		if !strings.EqualFold(attr.Key, name) {
			filtered = append(filtered, attr)
		}
	}
	node.Attr = filtered
}

func renderHTMLChildren(node *nethtml.Node) string {
	if node == nil {
		return ""
	}
	var builder strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		var buffer bytes.Buffer
		if err := nethtml.Render(&buffer, child); err == nil {
			builder.WriteString(buffer.String())
		}
	}
	return strings.TrimSpace(builder.String())
}

func readDocxDocumentXMLFromBytes(docxBytes []byte) (string, bool) {
	reader, err := zip.NewReader(bytes.NewReader(docxBytes), int64(len(docxBytes)))
	if err != nil {
		return "", false
	}
	for _, file := range reader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		raw, err := readZipFile(file)
		if err != nil {
			return "", false
		}
		return string(raw), true
	}
	return "", false
}
