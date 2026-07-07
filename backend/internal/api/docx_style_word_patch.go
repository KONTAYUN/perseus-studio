package api

import (
	"regexp"
	"strconv"
	"strings"
)

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
