package api

import (
	"fmt"
	htmlstd "html"
	"math"
	"regexp"
	"strconv"
	"strings"
)

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
