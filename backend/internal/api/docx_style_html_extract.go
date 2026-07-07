package api

import (
	"fmt"
	htmlstd "html"
	"math"
	"strconv"
	"strings"

	nethtml "golang.org/x/net/html"
)

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
