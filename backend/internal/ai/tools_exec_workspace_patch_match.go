package ai

import (
	"fmt"
	"sort"
	"strings"
)

func applyWorkspacePatchUpdate(content string, hunks []workspacePatchHunk) (string, error) {
	if len(hunks) == 0 {
		return "", patchApplyError("update operation contains no hunks", "", 0, 0, "update operation contains no hunks", nil)
	}
	eol := dominantEOL(content)
	lines := splitWorkspacePatchContentLines(normalizePatchNewlines(content))
	type replacement struct {
		start int
		oldN  int
		new   []string
	}
	var replacements []replacement
	searchStart := 0
	for index, hunk := range hunks {
		oldLines := trimPatchLineSuffixes(hunk.OldLines)
		newLines := trimPatchLineSuffixes(hunk.NewLines)
		if len(oldLines) == 0 {
			if content == "" && len(hunks) == 1 && len(hunk.Contexts) == 0 {
				return restoreEOL(strings.Join(newLines, "\n")+"\n", eol), nil
			}
			return "", patchApplyError(fmt.Sprintf("hunk %d has no removable/context lines", index+1), "", 0, index+1, "hunk has no removable/context lines", nil)
		}
		for _, context := range hunk.Contexts {
			context = strings.TrimSpace(context)
			if context == "" {
				continue
			}
			contextIndex, matches := seekWorkspacePatchSequence(lines, []string{context}, searchStart)
			if matches == 0 {
				candidates := similarWorkspacePatchCandidates(lines, []string{context}, 3)
				return "", patchApplyError(fmt.Sprintf("update hunk %d context %q did not match", index+1, context), "", 0, index+1, "context did not match", candidates)
			}
			if matches > 1 {
				return "", patchApplyError(fmt.Sprintf("update hunk %d context %q matched %d times; add more context", index+1, context, matches), "", 0, index+1, "context matched multiple times", nil)
			}
			searchStart = contextIndex + 1
		}
		found, matches := seekWorkspacePatchSequence(lines, oldLines, searchStart, hunk.EndOfFile)
		if matches == 0 {
			return "", workspacePatchHunkError(lines, oldLines, index+1, "did not match")
		}
		if matches > 1 {
			return "", patchApplyError(fmt.Sprintf("update hunk %d matched %d times; add more context", index+1, matches), "", 0, index+1, "hunk matched multiple times", nil)
		}
		replacements = append(replacements, replacement{start: found, oldN: len(oldLines), new: newLines})
		searchStart = found + len(oldLines)
	}
	for index := len(replacements) - 1; index >= 0; index-- {
		item := replacements[index]
		next := make([]string, 0, len(lines)-item.oldN+len(item.new))
		next = append(next, lines[:item.start]...)
		next = append(next, item.new...)
		next = append(next, lines[item.start+item.oldN:]...)
		lines = next
	}
	if len(lines) == 0 || lines[len(lines)-1] != "" {
		lines = append(lines, "")
	}
	return restoreEOL(strings.Join(lines, "\n"), eol), nil
}

func splitWorkspacePatchContentLines(content string) []string {
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func trimPatchLineSuffixes(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, strings.TrimSuffix(line, "\n"))
	}
	return out
}

func normalizeWorkspacePatchEnvelope(input string) (string, error) {
	text := normalizePatchNewlines(input)
	blocks, err := extractWorkspacePatchEnvelopes(text)
	if err != nil {
		return "", err
	}
	if len(blocks) == 0 {
		return "", parsePatchApplyError("patch apply failed: missing *** Begin Patch", "missing *** Begin Patch", 0, nil, workspacePatchCandidatePathsLoose(text))
	}
	if len(blocks) == 1 {
		return blocks[0], nil
	}
	var builder strings.Builder
	builder.WriteString("*** Begin Patch\n")
	for _, block := range blocks {
		lines := splitPatchLines(block)
		for index, line := range lines {
			header := strings.TrimSpace(strings.TrimSuffix(line, "\n"))
			if index == 0 && header == "*** Begin Patch" {
				continue
			}
			if header == "*** End Patch" {
				continue
			}
			builder.WriteString(line)
		}
		if !strings.HasSuffix(builder.String(), "\n") {
			builder.WriteString("\n")
		}
	}
	builder.WriteString("*** End Patch")
	return builder.String(), nil
}

func extractWorkspacePatchEnvelopes(text string) ([]string, error) {
	lines := splitPatchLines(text)
	var blocks []string
	var current strings.Builder
	inPatch := false
	startLine := 0
	for index, line := range lines {
		header := strings.TrimSpace(strings.TrimSuffix(line, "\n"))
		switch header {
		case "*** Begin Patch":
			if inPatch {
				return nil, parsePatchApplyError("patch apply failed: nested *** Begin Patch", "nested *** Begin Patch", index+1, patchDiagnosticLineWindow(lines, index), workspacePatchCandidatePathsLoose(text))
			}
			inPatch = true
			startLine = index + 1
			current.Reset()
			current.WriteString("*** Begin Patch\n")
		case "*** End Patch":
			if !inPatch {
				return nil, parsePatchApplyError("patch apply failed: *** End Patch before *** Begin Patch", "*** End Patch before *** Begin Patch", index+1, patchDiagnosticLineWindow(lines, index), workspacePatchCandidatePathsLoose(text))
			}
			current.WriteString("*** End Patch")
			blocks = append(blocks, current.String())
			inPatch = false
			current.Reset()
		default:
			if inPatch {
				current.WriteString(line)
			}
		}
	}
	if inPatch {
		return nil, parsePatchApplyError("patch apply failed: missing *** End Patch", "missing *** End Patch", startLine, patchDiagnosticLineWindow(lines, len(lines)-1), workspacePatchCandidatePathsLoose(text))
	}
	return blocks, nil
}

func seekWorkspacePatchSequence(lines, pattern []string, start int, eof ...bool) (int, int) {
	if len(pattern) == 0 || len(pattern) > len(lines) {
		return -1, 0
	}
	if start < 0 {
		start = 0
	}
	best := -1
	matches := 0
	for _, matcher := range []func(string, string) bool{
		func(a, b string) bool { return a == b },
	} {
		best = -1
		matches = 0
		for index := start; index+len(pattern) <= len(lines); index++ {
			if len(eof) > 0 && eof[0] && index+len(pattern) != len(lines) {
				continue
			}
			if workspacePatchLinesMatch(lines[index:index+len(pattern)], pattern, matcher) {
				best = index
				matches++
			}
		}
		if matches > 0 {
			return best, matches
		}
	}
	return -1, 0
}

func workspacePatchHunkError(lines, pattern []string, hunkIndex int, reason string) error {
	candidates := similarWorkspacePatchCandidates(lines, pattern, 3)
	if len(candidates) == 0 {
		return patchApplyError(fmt.Sprintf("update hunk %d %s; read the target range again and submit a corrected patch", hunkIndex, reason), "", 0, hunkIndex, "hunk "+reason, nil)
	}
	return patchApplyError(fmt.Sprintf("update hunk %d %s; nearby candidates: %s", hunkIndex, reason, strings.Join(candidates, " | ")), "", 0, hunkIndex, "hunk "+reason, candidates)
}

func similarWorkspacePatchCandidates(lines, pattern []string, limit int) []string {
	if limit <= 0 || len(lines) == 0 || len(pattern) == 0 {
		return nil
	}
	size := len(pattern)
	if size > len(lines) {
		size = len(lines)
	}
	needle := normalizeWorkspacePatchCandidate(strings.Join(pattern, "\n"))
	type candidate struct {
		line  int
		text  string
		score int
	}
	var candidates []candidate
	for index := 0; index+size <= len(lines); index++ {
		text := strings.Join(lines[index:index+size], "\n")
		score := workspacePatchCandidateScore(needle, normalizeWorkspacePatchCandidate(text))
		if score <= 0 {
			continue
		}
		candidates = append(candidates, candidate{line: index + 1, text: truncateRunesForPatch(text, 180), score: score})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].line < candidates[j].line
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	out := make([]string, 0, len(candidates))
	for _, item := range candidates {
		out = append(out, fmt.Sprintf("line %d: %q", item.line, item.text))
	}
	return out
}

func normalizeWorkspacePatchCandidate(value string) string {
	return strings.Join(strings.Fields(normalizeWorkspacePatchUnicode(value)), " ")
}

func workspacePatchCandidateScore(needle, value string) int {
	if needle == "" || value == "" {
		return 0
	}
	if needle == value {
		return len(needle) + 1000
	}
	score := 0
	for _, token := range strings.Fields(needle) {
		if len([]rune(token)) < 2 {
			continue
		}
		if strings.Contains(value, token) {
			score += len([]rune(token))
		}
	}
	return score
}

func truncateRunesForPatch(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
}

func workspacePatchLinesMatch(lines, pattern []string, matcher func(string, string) bool) bool {
	for index := range pattern {
		if !matcher(lines[index], pattern[index]) {
			return false
		}
	}
	return true
}

func normalizeWorkspacePatchUnicode(value string) string {
	replacer := strings.NewReplacer(
		"\u2010", "-", "\u2011", "-", "\u2012", "-", "\u2013", "-", "\u2014", "-", "\u2015", "-", "\u2212", "-",
		"\u2018", "'", "\u2019", "'", "\u201A", "'", "\u201B", "'",
		"\u201C", "\"", "\u201D", "\"", "\u201E", "\"", "\u201F", "\"",
		"\u00A0", " ", "\u2002", " ", "\u2003", " ", "\u2004", " ", "\u2005", " ", "\u2006", " ", "\u2007", " ",
		"\u2008", " ", "\u2009", " ", "\u200A", " ", "\u202F", " ", "\u205F", " ", "\u3000", " ",
	)
	return strings.TrimSpace(replacer.Replace(value))
}

func splitPatchLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.SplitAfter(text, "\n")
}

func countWorkspacePatchBlocks(text string) int {
	blocks, err := extractWorkspacePatchEnvelopes(normalizePatchNewlines(text))
	if err != nil {
		return strings.Count(text, "*** Begin Patch")
	}
	return len(blocks)
}

func patchDiagnosticHead(text string) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > 600 {
		runes = runes[:600]
	}
	return string(runes)
}

func patchDiagnosticTail(text string) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > 600 {
		runes = runes[len(runes)-600:]
	}
	return string(runes)
}

func patchDiagnosticLineWindow(lines []string, index int) []string {
	if len(lines) == 0 || index < 0 {
		return nil
	}
	start := index - 2
	if start < 0 {
		start = 0
	}
	end := index + 3
	if end > len(lines) {
		end = len(lines)
	}
	out := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		out = append(out, fmt.Sprintf("%d: %s", i+1, strings.TrimSuffix(lines[i], "\n")))
	}
	return trimPatchDiagnosticStrings(out)
}

func trimPatchDiagnosticStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, truncateRunes(value, 300))
	}
	return out
}

func normalizePatchNewlines(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

func dominantEOL(text string) string {
	crlf := strings.Count(text, "\r\n")
	lfOnly := strings.Count(text, "\n") - crlf
	if crlf > lfOnly {
		return "\r\n"
	}
	return "\n"
}

func restoreEOL(text, eol string) string {
	if eol == "\r\n" {
		return strings.ReplaceAll(text, "\n", "\r\n")
	}
	return text
}

func workspacePatchDiffSummary(added, updated, deleted int, changedPaths []string) string {
	parts := make([]string, 0, 4)
	if added > 0 {
		parts = append(parts, fmt.Sprintf("added %d file(s)", added))
	}
	if updated > 0 {
		parts = append(parts, fmt.Sprintf("updated %d file(s)", updated))
	}
	if deleted > 0 {
		parts = append(parts, fmt.Sprintf("deleted %d file(s)", deleted))
	}
	if len(parts) == 0 {
		parts = append(parts, "applied patch")
	}
	if len(changedPaths) > 0 {
		parts = append(parts, fmt.Sprintf("changed %d path(s)", len(changedPaths)))
	}
	return strings.Join(parts, "; ")
}

func firstPatchPathFromText(patchText string) string {
	patch, err := parseWorkspacePatch(patchText)
	if err != nil || len(patch.Operations) == 0 {
		return ""
	}
	return patch.Operations[0].Path
}

func workspacePatchCandidatePaths(patchText string) []string {
	patch, err := parseWorkspacePatch(patchText)
	if err != nil {
		return workspacePatchCandidatePathsLoose(patchText)
	}
	var paths []string
	for _, op := range patch.Operations {
		paths = append(paths, op.Path)
		if strings.TrimSpace(op.MovePath) != "" {
			paths = append(paths, op.MovePath)
		}
	}
	return normalizeCandidatePaths(paths...)
}

func workspacePatchCandidatePathsLoose(patchText string) []string {
	lines := splitPatchLines(normalizePatchNewlines(patchText))
	var paths []string
	for _, line := range lines {
		header := strings.TrimSpace(strings.TrimSuffix(line, "\n"))
		for _, prefix := range []string{"*** Add File: ", "*** Replace File: ", "*** Update File: ", "*** Delete File: ", "*** Move to: "} {
			if strings.HasPrefix(header, prefix) {
				paths = append(paths, strings.TrimSpace(strings.TrimPrefix(header, prefix)))
				break
			}
		}
	}
	return normalizeCandidatePaths(paths...)
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := map[string]bool{}
	next := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		next = append(next, value)
	}
	return next
}
