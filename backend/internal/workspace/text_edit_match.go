package workspace

import (
	"fmt"
	"strings"
)

var flexibleWhitespacePattern = strings.NewReplacer("\t", " ")

type textReplacementPlan struct {
	content      string
	replacements int
	matchKind    TextEditMatchKind
}

func planTextReplacement(content, oldText, newText string, replaceAll bool) (textReplacementPlan, error) {
	if count := strings.Count(content, oldText); count > 0 {
		if !replaceAll && count > 1 {
			return textReplacementPlan{}, fmt.Errorf("%w: old_text matched %d times; set replace_all=true or provide a more specific old_text", ErrTextMatchMany, count)
		}
		limit := 1
		if replaceAll {
			limit = -1
		}
		return textReplacementPlan{
			content:      strings.Replace(content, oldText, newText, limit),
			replacements: count,
			matchKind:    TextEditMatchExact,
		}, nil
	}
	if plan, ok, err := planTrimmedLineReplacement(content, oldText, newText, replaceAll); err != nil || ok {
		return plan, err
	}
	if plan, ok, err := planIndentationFlexibleReplacement(content, oldText, newText, replaceAll); err != nil || ok {
		return plan, err
	}
	return textReplacementPlan{}, ErrTextMatchMissing
}

func planTrimmedLineReplacement(content, oldText, newText string, replaceAll bool) (textReplacementPlan, bool, error) {
	lines := splitTextPreserveLines(content)
	oldLines := splitTextPreserveLines(oldText)
	if len(oldLines) == 0 {
		return textReplacementPlan{}, false, nil
	}
	target := trimmedLineSignature(oldLines)
	if len(target) == 0 {
		return textReplacementPlan{}, false, nil
	}
	matches := findLineWindowMatches(lines, len(oldLines), func(window []string) bool {
		return equalStringSlices(trimmedLineSignature(window), target)
	})
	if len(matches) == 0 {
		return textReplacementPlan{}, false, nil
	}
	if !replaceAll && len(matches) > 1 {
		return textReplacementPlan{}, true, fmt.Errorf("%w: old_text matched %d times after trimming line whitespace; provide more context", ErrTextMatchMany, len(matches))
	}
	next := replaceLineWindows(lines, matches, oldLines, newText, replaceAll)
	return textReplacementPlan{content: next, replacements: len(matches), matchKind: TextEditMatchTrimmedLines}, true, nil
}

func planIndentationFlexibleReplacement(content, oldText, newText string, replaceAll bool) (textReplacementPlan, bool, error) {
	lines := splitTextPreserveLines(content)
	oldLines := splitTextPreserveLines(oldText)
	if len(oldLines) == 0 {
		return textReplacementPlan{}, false, nil
	}
	target := indentationFlexibleSignature(oldLines)
	if len(target) == 0 {
		return textReplacementPlan{}, false, nil
	}
	matches := findLineWindowMatches(lines, len(oldLines), func(window []string) bool {
		return equalStringSlices(indentationFlexibleSignature(window), target)
	})
	if len(matches) == 0 {
		return textReplacementPlan{}, false, nil
	}
	if !replaceAll && len(matches) > 1 {
		return textReplacementPlan{}, true, fmt.Errorf("%w: old_text matched %d times with flexible indentation; provide more context", ErrTextMatchMany, len(matches))
	}
	next := replaceLineWindows(lines, matches, oldLines, newText, replaceAll)
	return textReplacementPlan{content: next, replacements: len(matches), matchKind: TextEditMatchIndentationFlexible}, true, nil
}

func splitTextPreserveLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.SplitAfter(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func trimmedLineSignature(lines []string) []string {
	items := make([]string, 0, len(lines))
	for _, line := range lines {
		items = append(items, strings.TrimSpace(strings.TrimSuffix(line, "\n")))
	}
	return items
}

func indentationFlexibleSignature(lines []string) []string {
	items := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSuffix(line, "\n")
		line = strings.Join(strings.Fields(flexibleWhitespacePattern.Replace(line)), " ")
		items = append(items, line)
	}
	return items
}

func findLineWindowMatches(lines []string, size int, match func([]string) bool) []int {
	if size <= 0 || len(lines) < size {
		return nil
	}
	var matches []int
	for index := 0; index+size <= len(lines); index++ {
		if match(lines[index : index+size]) {
			matches = append(matches, index)
		}
	}
	return matches
}

func replaceLineWindows(lines []string, matches []int, oldLines []string, newText string, replaceAll bool) string {
	matchSet := map[int]bool{}
	for _, index := range matches {
		matchSet[index] = true
		if !replaceAll {
			break
		}
	}
	var out strings.Builder
	for index := 0; index < len(lines); {
		if matchSet[index] {
			out.WriteString(newText)
			index += len(oldLines)
			continue
		}
		out.WriteString(lines[index])
		index++
	}
	return out.String()
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

func looksLikeOmittedText(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}
	patterns := []string{
		"... rest",
		"rest omitted",
		"remaining unchanged",
		"unchanged ...",
		"其余不变",
		"其余省略",
		"内容省略",
		"下文同上",
	}
	for _, pattern := range patterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}
