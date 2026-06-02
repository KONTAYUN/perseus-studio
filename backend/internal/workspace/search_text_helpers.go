package workspace

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

func searchTextFile(absPath, relPath, query string, pattern *regexp.Regexp, limit, contextLines int) ([]TextSearchResult, error) {
	if limit <= 0 {
		return nil, nil
	}
	file, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var results []TextSearchResult
	var previous []string
	type pendingAfterContext struct {
		Index int
		Lines []string
	}
	var pending []pendingAfterContext
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if contextLines > 0 && len(pending) > 0 {
			nextPending := pending[:0]
			for _, item := range pending {
				item.Lines = append(item.Lines, line)
				if len(item.Lines) >= contextLines {
					results[item.Index].After = strings.Join(item.Lines, "\n")
				} else {
					nextPending = append(nextPending, item)
				}
			}
			pending = nextPending
		}
		column := searchLineColumn(line, query, pattern)
		if column > 0 {
			result := TextSearchResult{
				Path:   relPath,
				Line:   lineNumber,
				Column: column,
				Match:  truncateRunes(line, 300),
			}
			if contextLines > 0 && len(previous) > 0 {
				start := len(previous) - contextLines
				if start < 0 {
					start = 0
				}
				result.Before = strings.Join(previous[start:], "\n")
			}
			results = append(results, result)
			if contextLines > 0 {
				pending = append(pending, pendingAfterContext{Index: len(results) - 1})
			}
			if len(results) >= limit {
				results[len(results)-1].Truncated = true
				return results, nil
			}
		}
		previous = append(previous, line)
		if len(previous) > contextLines {
			previous = previous[len(previous)-contextLines:]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func searchLineColumn(line, query string, pattern *regexp.Regexp) int {
	if pattern != nil {
		loc := pattern.FindStringIndex(line)
		if loc == nil {
			return 0
		}
		return len([]rune(line[:loc[0]])) + 1
	}
	index := strings.Index(strings.ToLower(line), strings.ToLower(query))
	if index < 0 {
		return 0
	}
	return len([]rune(line[:index])) + 1
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}
