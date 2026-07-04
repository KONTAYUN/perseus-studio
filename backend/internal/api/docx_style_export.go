package api

import (
	"archive/zip"
	"bytes"
	"fmt"
	htmlstd "html"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"

	nethtml "golang.org/x/net/html"
)

const defaultDocxBodyWidthEMU int64 = 9360 * 635

type docxStylePlan struct {
	Blocks     []docxBlockStyle
	TableCells []docxTableCellStyle
	Images     []docxImageStyle
}

type docxBlockStyle struct {
	Text  string
	Align string
	Runs  []docxRunStyle
}

type docxTableCellStyle struct {
	Text            string
	Align           string
	BackgroundColor string
}

type docxImageStyle struct {
	Align        string
	WidthPercent int
}

type docxRunStyle struct {
	Text               string
	Color              string
	BackgroundColor    string
	FontSizeHalfPoints int
}

func enhanceDocxRichTextStyles(docxBytes []byte, htmlFragment string) []byte {
	plan, err := extractDocxStylePlan(htmlFragment)
	if err != nil || plan.isEmpty() {
		return docxBytes
	}
	enhanced, ok := rewriteDocxDocumentXML(docxBytes, func(documentXML string) string {
		return applyDocxStylePlan(documentXML, plan)
	})
	if !ok {
		return docxBytes
	}
	return enhanced
}

func (plan docxStylePlan) isEmpty() bool {
	return len(plan.Blocks) == 0 && len(plan.TableCells) == 0 && len(plan.Images) == 0
}

func extractDocxStylePlan(fragment string) (docxStylePlan, error) {
	doc, err := nethtml.Parse(strings.NewReader(wrapHTMLFragmentDocument(fragment)))
	if err != nil {
		return docxStylePlan{}, err
	}
	body := findHTMLElement(doc, "body")
	if body == nil {
		body = doc
	}
	var plan docxStylePlan
	walkDocxStyleHTML(body, false, &plan)
	return plan, nil
}

func walkDocxStyleHTML(node *nethtml.Node, inTable bool, plan *docxStylePlan) {
	if node == nil {
		return
	}
	nextInTable := inTable
	if node.Type == nethtml.ElementNode {
		tag := strings.ToLower(node.Data)
		switch {
		case tag == "table":
			nextInTable = true
		case isDocxBlockTag(tag) && !inTable:
			plan.Blocks = append(plan.Blocks, extractDocxBlockStyle(node))
		case (tag == "td" || tag == "th") && inTable:
			plan.TableCells = append(plan.TableCells, extractDocxTableCellStyle(node))
		case tag == "img":
			if image := extractDocxImageStyle(node); image.Align != "" || image.WidthPercent > 0 {
				plan.Images = append(plan.Images, image)
			}
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walkDocxStyleHTML(child, nextInTable, plan)
	}
}

func isDocxBlockTag(tag string) bool {
	switch tag {
	case "p", "blockquote", "h1", "h2", "h3", "h4", "h5", "h6":
		return true
	default:
		return false
	}
}

func extractDocxBlockStyle(node *nethtml.Node) docxBlockStyle {
	runs := collectDocxRunStyles(node, docxRunStyle{})
	return docxBlockStyle{
		Text:  normalizeDocxMatchText(htmlNodeText(node)),
		Align: extractNodeTextAlign(node),
		Runs:  runs,
	}
}

func extractDocxTableCellStyle(node *nethtml.Node) docxTableCellStyle {
	cell := docxTableCellStyle{
		Text:            normalizeDocxMatchText(htmlNodeText(node)),
		Align:           extractNodeTextAlign(node),
		BackgroundColor: extractNodeBackgroundColor(node),
	}
	if cell.Align == "" {
		if descendant := firstDescendantWithAlign(node); descendant != nil {
			cell.Align = extractNodeTextAlign(descendant)
		}
	}
	return cell
}

func extractDocxImageStyle(node *nethtml.Node) docxImageStyle {
	style := parseCSSDeclarations(nodeAttr(node, "style"))
	align := normalizeDocxAlign(nodeAttr(node, "data-align"))
	if align == "" {
		align = normalizeDocxImageAlignFromCSS(style)
	}
	width := normalizeDocxPercent(nodeAttr(node, "data-width"))
	if width == 0 {
		width = normalizeDocxPercent(style["width"])
	}
	return docxImageStyle{
		Align:        align,
		WidthPercent: width,
	}
}

func collectDocxRunStyles(node *nethtml.Node, inherited docxRunStyle) []docxRunStyle {
	var runs []docxRunStyle
	collectDocxRunStylesInto(node, inherited, &runs)
	return runs
}

func collectDocxRunStylesInto(node *nethtml.Node, inherited docxRunStyle, runs *[]docxRunStyle) {
	if node == nil {
		return
	}
	current := inherited
	if node.Type == nethtml.ElementNode {
		current = mergeDocxRunStyle(current, extractNodeRunStyle(node))
	}
	if node.Type == nethtml.TextNode {
		text := normalizeDocxMatchText(node.Data)
		if text != "" {
			appendDocxRunStyle(runs, docxRunStyle{
				Text:               text,
				Color:              current.Color,
				BackgroundColor:    current.BackgroundColor,
				FontSizeHalfPoints: current.FontSizeHalfPoints,
			})
		}
		return
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		collectDocxRunStylesInto(child, current, runs)
	}
}

func appendDocxRunStyle(runs *[]docxRunStyle, next docxRunStyle) {
	if next.Text == "" {
		return
	}
	if len(*runs) > 0 {
		last := &(*runs)[len(*runs)-1]
		if last.Color == next.Color &&
			last.BackgroundColor == next.BackgroundColor &&
			last.FontSizeHalfPoints == next.FontSizeHalfPoints {
			last.Text = normalizeDocxMatchText(last.Text + " " + next.Text)
			return
		}
	}
	*runs = append(*runs, next)
}

func mergeDocxRunStyle(base, next docxRunStyle) docxRunStyle {
	if next.Color != "" {
		base.Color = next.Color
	}
	if next.BackgroundColor != "" {
		base.BackgroundColor = next.BackgroundColor
	}
	if next.FontSizeHalfPoints > 0 {
		base.FontSizeHalfPoints = next.FontSizeHalfPoints
	}
	return base
}

func extractNodeRunStyle(node *nethtml.Node) docxRunStyle {
	style := parseCSSDeclarations(nodeAttr(node, "style"))
	background := normalizeDocxHexColor(firstNonEmpty(
		style["background-color"],
		nodeAttr(node, "data-color"),
		nodeAttr(node, "data-background-color"),
	))
	return docxRunStyle{
		Color:              normalizeDocxHexColor(style["color"]),
		BackgroundColor:    background,
		FontSizeHalfPoints: normalizeDocxFontSizeHalfPoints(style["font-size"]),
	}
}

func extractNodeTextAlign(node *nethtml.Node) string {
	style := parseCSSDeclarations(nodeAttr(node, "style"))
	return normalizeDocxAlign(firstNonEmpty(
		nodeAttr(node, "data-text-align"),
		nodeAttr(node, "align"),
		style["text-align"],
	))
}

func extractNodeBackgroundColor(node *nethtml.Node) string {
	style := parseCSSDeclarations(nodeAttr(node, "style"))
	return normalizeDocxHexColor(firstNonEmpty(
		style["background-color"],
		nodeAttr(node, "data-background-color"),
	))
}

func firstDescendantWithAlign(node *nethtml.Node) *nethtml.Node {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == nethtml.ElementNode && extractNodeTextAlign(child) != "" {
			return child
		}
		if match := firstDescendantWithAlign(child); match != nil {
			return match
		}
	}
	return nil
}

func findHTMLElement(node *nethtml.Node, tag string) *nethtml.Node {
	if node == nil {
		return nil
	}
	if node.Type == nethtml.ElementNode && strings.EqualFold(node.Data, tag) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findHTMLElement(child, tag); found != nil {
			return found
		}
	}
	return nil
}

func htmlNodeText(node *nethtml.Node) string {
	if node == nil {
		return ""
	}
	if node.Type == nethtml.TextNode {
		return node.Data
	}
	var builder strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		text := htmlNodeText(child)
		if strings.TrimSpace(text) == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteByte(' ')
		}
		builder.WriteString(text)
	}
	return builder.String()
}

func nodeAttr(node *nethtml.Node, name string) string {
	if node == nil {
		return ""
	}
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, name) {
			return htmlstd.UnescapeString(attr.Val)
		}
	}
	return ""
}

func parseCSSDeclarations(style string) map[string]string {
	values := make(map[string]string)
	for _, part := range strings.Split(style, ";") {
		name, value, ok := strings.Cut(part, ":")
		if !ok {
			continue
		}
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		values[name] = cleanCSSValue(value)
	}
	return values
}

func cleanCSSValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, "!important")
	return strings.TrimSpace(value)
}

func normalizeDocxAlign(value string) string {
	switch strings.ToLower(cleanCSSValue(value)) {
	case "left", "start":
		return "left"
	case "center", "middle":
		return "center"
	case "right", "end":
		return "right"
	default:
		return ""
	}
}

func normalizeDocxImageAlignFromCSS(style map[string]string) string {
	left := strings.ToLower(style["margin-left"])
	right := strings.ToLower(style["margin-right"])
	switch {
	case left == "auto" && right == "auto":
		return "center"
	case left == "auto":
		return "right"
	case right == "auto":
		return "left"
	default:
		return ""
	}
}

func normalizeDocxHexColor(value string) string {
	value = strings.ToLower(cleanCSSValue(value))
	switch value {
	case "", "inherit", "initial", "currentcolor", "transparent", "none":
		return ""
	}
	if strings.HasPrefix(value, "#") {
		raw := strings.TrimPrefix(value, "#")
		if len(raw) == 3 && isHexColor(raw) {
			return strings.ToUpper(string([]byte{raw[0], raw[0], raw[1], raw[1], raw[2], raw[2]}))
		}
		if len(raw) == 6 && isHexColor(raw) {
			return strings.ToUpper(raw)
		}
		return ""
	}
	if strings.HasPrefix(value, "rgb(") && strings.HasSuffix(value, ")") {
		parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(value, "rgb("), ")"), ",")
		if len(parts) != 3 {
			return ""
		}
		var channels [3]int
		for index, part := range parts {
			channel, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil || channel < 0 || channel > 255 {
				return ""
			}
			channels[index] = channel
		}
		return fmt.Sprintf("%02X%02X%02X", channels[0], channels[1], channels[2])
	}
	return ""
}

func isHexColor(value string) bool {
	for _, char := range value {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}
	return true
}

func normalizeDocxFontSizeHalfPoints(value string) int {
	value = strings.ToLower(cleanCSSValue(value))
	if value == "" {
		return 0
	}
	unit := "px"
	if strings.HasSuffix(value, "pt") {
		unit = "pt"
		value = strings.TrimSuffix(value, "pt")
	} else {
		value = strings.TrimSuffix(value, "px")
	}
	size, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || size < 8 || size > 96 {
		return 0
	}
	if unit == "pt" {
		return int(math.Round(size * 2))
	}
	return int(math.Round(size * 1.5))
}

func normalizeDocxPercent(value string) int {
	value = cleanCSSValue(value)
	value = strings.TrimSuffix(value, "%")
	percent, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || percent < 20 || percent > 100 {
		return 0
	}
	return int(math.Round(percent))
}

func normalizeDocxMatchText(value string) string {
	value = htmlstd.UnescapeString(value)
	value = strings.ReplaceAll(value, "\u00a0", " ")
	value = docxWhitespacePattern.ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}

func applyDocxStylePlan(documentXML string, plan docxStylePlan) string {
	documentXML = patchWordTableCells(documentXML, plan.TableCells)
	documentXML = patchWordDrawings(documentXML, plan.Images)
	documentXML = patchWordParagraphs(documentXML, plan.Blocks, plan.Images)
	return documentXML
}

func patchWordTableCells(documentXML string, cells []docxTableCellStyle) string {
	if len(cells) == 0 {
		return documentXML
	}
	matches := wordTableCellPattern.FindAllStringIndex(documentXML, -1)
	if len(matches) == 0 {
		return documentXML
	}
	var builder strings.Builder
	builder.Grow(len(documentXML))
	last := 0
	cellIndex := 0
	for _, loc := range matches {
		builder.WriteString(documentXML[last:loc[0]])
		cellXML := documentXML[loc[0]:loc[1]]
		if cellIndex < len(cells) {
			cellXML = applyTableCellStyle(cellXML, cells[cellIndex])
			cellIndex++
		}
		builder.WriteString(cellXML)
		last = loc[1]
	}
	builder.WriteString(documentXML[last:])
	return builder.String()
}

func applyTableCellStyle(cellXML string, style docxTableCellStyle) string {
	if style.BackgroundColor != "" {
		cellXML = setWordProperty(cellXML, "w:tc", "w:tcPr", "w:shd", wordShadingXML(style.BackgroundColor))
	}
	if style.Align == "" {
		return cellXML
	}
	return patchParagraphsInFragment(cellXML, func(paragraphXML string) string {
		if wordTextFromXML(paragraphXML) == "" {
			return paragraphXML
		}
		return setParagraphAlignment(paragraphXML, style.Align)
	})
}

func patchWordDrawings(documentXML string, images []docxImageStyle) string {
	if len(images) == 0 {
		return documentXML
	}
	bodyWidthEMU := wordBodyWidthEMU(documentXML)
	matches := wordDrawingPattern.FindAllStringIndex(documentXML, -1)
	if len(matches) == 0 {
		return documentXML
	}
	var builder strings.Builder
	builder.Grow(len(documentXML))
	last := 0
	imageIndex := 0
	for _, loc := range matches {
		builder.WriteString(documentXML[last:loc[0]])
		drawingXML := documentXML[loc[0]:loc[1]]
		if imageIndex < len(images) && images[imageIndex].WidthPercent > 0 {
			drawingXML = patchDrawingWidth(drawingXML, images[imageIndex], bodyWidthEMU)
		}
		imageIndex++
		builder.WriteString(drawingXML)
		last = loc[1]
	}
	builder.WriteString(documentXML[last:])
	return builder.String()
}

func patchDrawingWidth(drawingXML string, image docxImageStyle, bodyWidthEMU int64) string {
	currentCx, currentCy, ok := firstDrawingExtent(drawingXML)
	if !ok || currentCx <= 0 || currentCy <= 0 {
		return drawingXML
	}
	nextCx := bodyWidthEMU * int64(image.WidthPercent) / 100
	nextCy := currentCy * nextCx / currentCx
	if nextCx <= 0 || nextCy <= 0 {
		return drawingXML
	}
	return wordExtentPattern.ReplaceAllStringFunc(drawingXML, func(tag string) string {
		tag = setXMLIntegerAttr(tag, "cx", nextCx)
		tag = setXMLIntegerAttr(tag, "cy", nextCy)
		return tag
	})
}

func firstDrawingExtent(drawingXML string) (int64, int64, bool) {
	match := wordExtentPattern.FindStringSubmatch(drawingXML)
	if len(match) < 3 {
		return 0, 0, false
	}
	cx, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	cy, err := strconv.ParseInt(match[2], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	return cx, cy, true
}

func patchWordParagraphs(documentXML string, blocks []docxBlockStyle, images []docxImageStyle) string {
	if len(blocks) == 0 && len(images) == 0 {
		return documentXML
	}
	tableRanges := wordElementRanges(documentXML, wordTablePattern)
	matches := wordParagraphPattern.FindAllStringIndex(documentXML, -1)
	if len(matches) == 0 {
		return documentXML
	}
	return patchDocumentParagraphs(documentXML, matches, blocks, images, tableRanges)
}

func patchParagraphsInFragment(fragment string, transform func(string) string) string {
	matches := wordParagraphPattern.FindAllStringIndex(fragment, -1)
	if len(matches) == 0 {
		return fragment
	}

	var builder strings.Builder
	builder.Grow(len(fragment))
	last := 0
	for _, loc := range matches {
		builder.WriteString(fragment[last:loc[0]])
		builder.WriteString(transform(fragment[loc[0]:loc[1]]))
		last = loc[1]
	}
	builder.WriteString(fragment[last:])
	return builder.String()
}

func patchDocumentParagraphs(fragment string, matches [][]int, blocks []docxBlockStyle, images []docxImageStyle, tableRanges []xmlRange) string {
	var builder strings.Builder
	builder.Grow(len(fragment))
	last := 0
	blockIndex := 0
	imageIndex := 0
	for _, loc := range matches {
		builder.WriteString(fragment[last:loc[0]])
		paragraphXML := fragment[loc[0]:loc[1]]
		if !rangeInsideAny(loc[0], loc[1], tableRanges) {
			if text := wordTextFromXML(paragraphXML); text != "" {
				if style, ok := matchNextBlockStyle(blocks, &blockIndex, text); ok {
					paragraphXML = applyBlockStyle(paragraphXML, style)
				}
			}
			if strings.Contains(paragraphXML, "<w:drawing") {
				drawingCount := strings.Count(paragraphXML, "<w:drawing")
				if imageIndex < len(images) && images[imageIndex].Align != "" {
					paragraphXML = setParagraphAlignment(paragraphXML, images[imageIndex].Align)
				}
				imageIndex += drawingCount
			}
		}
		builder.WriteString(paragraphXML)
		last = loc[1]
	}
	builder.WriteString(fragment[last:])
	return builder.String()
}

func matchNextBlockStyle(blocks []docxBlockStyle, blockIndex *int, paragraphText string) (docxBlockStyle, bool) {
	if blockIndex == nil {
		return docxBlockStyle{}, false
	}
	for *blockIndex < len(blocks) && blocks[*blockIndex].Text == "" {
		*blockIndex = *blockIndex + 1
	}
	if *blockIndex >= len(blocks) {
		return docxBlockStyle{}, false
	}
	block := blocks[*blockIndex]
	if !docxTextMatches(paragraphText, block.Text) {
		return docxBlockStyle{}, false
	}
	*blockIndex = *blockIndex + 1
	return block, true
}

func applyBlockStyle(paragraphXML string, style docxBlockStyle) string {
	if style.Align != "" {
		paragraphXML = setParagraphAlignment(paragraphXML, style.Align)
	}
	if len(style.Runs) > 0 {
		paragraphXML = patchWordRuns(paragraphXML, style.Runs)
	}
	return paragraphXML
}

func patchWordRuns(paragraphXML string, runs []docxRunStyle) string {
	matches := wordRunPattern.FindAllStringIndex(paragraphXML, -1)
	if len(matches) == 0 {
		return paragraphXML
	}
	var builder strings.Builder
	builder.Grow(len(paragraphXML))
	last := 0
	runIndex := 0
	for _, loc := range matches {
		builder.WriteString(paragraphXML[last:loc[0]])
		runXML := paragraphXML[loc[0]:loc[1]]
		runText := wordTextFromXML(runXML)
		if runText != "" {
			if style, ok := matchNextRunStyle(runs, &runIndex, runText); ok && style.hasStyle() {
				runXML = applyRunStyle(runXML, style)
			}
		}
		builder.WriteString(runXML)
		last = loc[1]
	}
	builder.WriteString(paragraphXML[last:])
	return builder.String()
}

func matchNextRunStyle(runs []docxRunStyle, runIndex *int, runText string) (docxRunStyle, bool) {
	if runIndex == nil {
		return docxRunStyle{}, false
	}
	for index := *runIndex; index < len(runs) && index < *runIndex+8; index++ {
		if docxTextMatches(runText, runs[index].Text) {
			*runIndex = index + 1
			return runs[index], true
		}
	}
	return docxRunStyle{}, false
}

func (style docxRunStyle) hasStyle() bool {
	return style.Color != "" || style.BackgroundColor != "" || style.FontSizeHalfPoints > 0
}

func applyRunStyle(runXML string, style docxRunStyle) string {
	if style.Color != "" {
		runXML = setWordProperty(runXML, "w:r", "w:rPr", "w:color", `<w:color w:val="`+style.Color+`" />`)
	}
	if style.BackgroundColor != "" {
		runXML = setWordProperty(runXML, "w:r", "w:rPr", "w:shd", wordShadingXML(style.BackgroundColor))
	}
	if style.FontSizeHalfPoints > 0 {
		size := strconv.Itoa(style.FontSizeHalfPoints)
		runXML = setWordProperty(runXML, "w:r", "w:rPr", "w:sz", `<w:sz w:val="`+size+`" />`)
		runXML = setWordProperty(runXML, "w:r", "w:rPr", "w:szCs", `<w:szCs w:val="`+size+`" />`)
	}
	return runXML
}

func setParagraphAlignment(paragraphXML, align string) string {
	align = normalizeDocxAlign(align)
	if align == "" {
		return paragraphXML
	}
	return setWordProperty(paragraphXML, "w:p", "w:pPr", "w:jc", `<w:jc w:val="`+align+`" />`)
}

func setWordProperty(containerXML, containerTag, propertiesTag, childTag, childXML string) string {
	propPattern := regexp.MustCompile(`(?s)<` + regexp.QuoteMeta(propertiesTag) + `\b[^>]*>.*?</` + regexp.QuoteMeta(propertiesTag) + `>`)
	if loc := propPattern.FindStringIndex(containerXML); loc != nil {
		propertiesXML := containerXML[loc[0]:loc[1]]
		propertiesXML = removeWordPropertyChild(propertiesXML, childTag)
		closeTag := "</" + propertiesTag + ">"
		closeIndex := strings.LastIndex(propertiesXML, closeTag)
		if closeIndex < 0 {
			return containerXML
		}
		propertiesXML = propertiesXML[:closeIndex] + childXML + propertiesXML[closeIndex:]
		return containerXML[:loc[0]] + propertiesXML + containerXML[loc[1]:]
	}

	openPattern := regexp.MustCompile(`(?s)<` + regexp.QuoteMeta(containerTag) + `\b[^>]*>`)
	loc := openPattern.FindStringIndex(containerXML)
	if loc == nil {
		return containerXML
	}
	propertiesXML := "<" + propertiesTag + ">" + childXML + "</" + propertiesTag + ">"
	return containerXML[:loc[1]] + propertiesXML + containerXML[loc[1]:]
}

func removeWordPropertyChild(propertiesXML, childTag string) string {
	childPattern := regexp.MustCompile(`(?s)<` + regexp.QuoteMeta(childTag) + `\b[^>]*/>|<` + regexp.QuoteMeta(childTag) + `\b[^>]*>.*?</` + regexp.QuoteMeta(childTag) + `>`)
	return childPattern.ReplaceAllString(propertiesXML, "")
}

func wordShadingXML(color string) string {
	return `<w:shd w:val="clear" w:color="auto" w:fill="` + color + `" />`
}

func wordTextFromXML(xmlFragment string) string {
	matches := wordTextPattern.FindAllStringSubmatch(xmlFragment, -1)
	if len(matches) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, match := range matches {
		if len(match) > 1 {
			if builder.Len() > 0 {
				builder.WriteByte(' ')
			}
			builder.WriteString(match[1])
		}
	}
	return normalizeDocxMatchText(builder.String())
}

func docxTextMatches(left, right string) bool {
	left = normalizeDocxMatchText(left)
	right = normalizeDocxMatchText(right)
	if left == "" || right == "" {
		return false
	}
	return left == right || strings.Contains(left, right) || strings.Contains(right, left)
}

func wordBodyWidthEMU(documentXML string) int64 {
	pageWidthTwips := int64(12240)
	leftMarginTwips := int64(1440)
	rightMarginTwips := int64(1440)
	if match := regexp.MustCompile(`<w:pgSz\b[^>]*\bw:w="(\d+)"[^>]*/?>`).FindStringSubmatch(documentXML); len(match) > 1 {
		if value, err := strconv.ParseInt(match[1], 10, 64); err == nil && value > 0 {
			pageWidthTwips = value
		}
	}
	if match := regexp.MustCompile(`<w:pgMar\b[^>]*>`).FindString(documentXML); match != "" {
		if value := wordIntegerAttr(match, "w:left"); value > 0 {
			leftMarginTwips = value
		}
		if value := wordIntegerAttr(match, "w:right"); value > 0 {
			rightMarginTwips = value
		}
	}
	bodyWidthTwips := pageWidthTwips - leftMarginTwips - rightMarginTwips
	if bodyWidthTwips <= 0 {
		return defaultDocxBodyWidthEMU
	}
	return bodyWidthTwips * 635
}

func wordIntegerAttr(tag, attr string) int64 {
	pattern := regexp.MustCompile(regexp.QuoteMeta(attr) + `="(\d+)"`)
	match := pattern.FindStringSubmatch(tag)
	if len(match) < 2 {
		return 0
	}
	value, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return 0
	}
	return value
}

func setXMLIntegerAttr(tag, attr string, value int64) string {
	pattern := regexp.MustCompile(regexp.QuoteMeta(attr) + `="\d+"`)
	return pattern.ReplaceAllString(tag, attr+`="`+strconv.FormatInt(value, 10)+`"`)
}

type xmlRange struct {
	Start int
	End   int
}

func wordElementRanges(documentXML string, pattern *regexp.Regexp) []xmlRange {
	matches := pattern.FindAllStringIndex(documentXML, -1)
	ranges := make([]xmlRange, 0, len(matches))
	for _, loc := range matches {
		ranges = append(ranges, xmlRange{Start: loc[0], End: loc[1]})
	}
	return ranges
}

func rangeInsideAny(start, end int, ranges []xmlRange) bool {
	for _, candidate := range ranges {
		if start >= candidate.Start && end <= candidate.End {
			return true
		}
	}
	return false
}

func rewriteDocxDocumentXML(docxBytes []byte, transform func(string) string) ([]byte, bool) {
	reader, err := zip.NewReader(bytes.NewReader(docxBytes), int64(len(docxBytes)))
	if err != nil {
		return nil, false
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	foundDocument := false
	for _, file := range reader.File {
		header := file.FileHeader
		header.CRC32 = 0
		header.CompressedSize = 0
		header.UncompressedSize = 0
		header.CompressedSize64 = 0
		header.UncompressedSize64 = 0
		entry, err := writer.CreateHeader(&header)
		if err != nil {
			_ = writer.Close()
			return nil, false
		}
		content, err := readZipFile(file)
		if err != nil {
			_ = writer.Close()
			return nil, false
		}
		if file.Name == "word/document.xml" {
			foundDocument = true
			content = []byte(transform(string(content)))
		}
		if _, err := entry.Write(content); err != nil {
			_ = writer.Close()
			return nil, false
		}
	}
	if err := writer.Close(); err != nil {
		return nil, false
	}
	if !foundDocument {
		return nil, false
	}
	return buffer.Bytes(), true
}

func readZipFile(file *zip.File) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

var (
	docxWhitespacePattern = regexp.MustCompile(`\s+`)
	wordParagraphPattern  = regexp.MustCompile(`(?s)<w:p\b[^>]*>.*?</w:p>`)
	wordRunPattern        = regexp.MustCompile(`(?s)<w:r\b[^>]*>.*?</w:r>`)
	wordTextPattern       = regexp.MustCompile(`(?s)<w:t\b[^>]*>(.*?)</w:t>`)
	wordTablePattern      = regexp.MustCompile(`(?s)<w:tbl\b[^>]*>.*?</w:tbl>`)
	wordTableCellPattern  = regexp.MustCompile(`(?s)<w:tc\b[^>]*>.*?</w:tc>`)
	wordDrawingPattern    = regexp.MustCompile(`(?s)<w:drawing\b[^>]*>.*?</w:drawing>`)
	wordExtentPattern     = regexp.MustCompile(`(?s)<(?:wp:extent|a:ext)\b[^>]*\bcx="(\d+)"[^>]*\bcy="(\d+)"[^>]*/?>`)
)
