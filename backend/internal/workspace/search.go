package workspace

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func (s *Service) Search(query string, maxResults int) ([]SearchResult, error) {
	if err := s.migrateLegacyTrashDir(); err != nil {
		return nil, err
	}
	needle := normalizeSearchText(query)
	if needle == "" {
		return []SearchResult{}, nil
	}
	if maxResults <= 0 || maxResults > 200 {
		maxResults = 80
	}

	results := make([]SearchResult, 0, maxResults)
	err := filepath.WalkDir(s.rootAbs, func(absPath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if absPath == s.rootAbs {
			return nil
		}

		rel, err := filepath.Rel(s.rootAbs, absPath)
		if err != nil {
			return err
		}
		relPath := filepath.ToSlash(rel)
		if isAnyTrashPath(relPath) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		name := entry.Name()
		pathMatch := strings.Contains(normalizeSearchText(relPath), needle)
		nameMatch := strings.Contains(normalizeSearchText(name), needle)
		if !pathMatch && !nameMatch {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		itemType := "file"
		if entry.IsDir() {
			itemType = "folder"
		}
		size := int64(0)
		if !entry.IsDir() {
			size = info.Size()
		}
		results = append(results, SearchResult{
			Name:       name,
			Path:       relPath,
			ParentPath: getParentPath(relPath),
			Type:       itemType,
			Size:       size,
			ModifiedAt: info.ModTime().UTC(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.SliceStable(results, func(i, j int) bool {
		leftScore := searchScore(results[i], needle)
		rightScore := searchScore(results[j], needle)
		if leftScore != rightScore {
			return leftScore < rightScore
		}
		return strings.ToLower(results[i].Path) < strings.ToLower(results[j].Path)
	})
	if len(results) > maxResults {
		results = results[:maxResults]
	}
	return results, nil
}

func (s *Service) FindFiles(query, pattern string, maxResults int) ([]SearchResult, error) {
	if err := s.migrateLegacyTrashDir(); err != nil {
		return nil, err
	}
	needle := normalizeSearchText(query)
	pattern = strings.TrimSpace(strings.ReplaceAll(pattern, "\\", "/"))
	if needle == "" && pattern == "" {
		return []SearchResult{}, nil
	}
	if maxResults <= 0 || maxResults > 200 {
		maxResults = 80
	}

	results := make([]SearchResult, 0, maxResults)
	err := filepath.WalkDir(s.rootAbs, func(absPath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if absPath == s.rootAbs {
			return nil
		}
		rel, err := filepath.Rel(s.rootAbs, absPath)
		if err != nil {
			return err
		}
		relPath := filepath.ToSlash(rel)
		if shouldSkipSearchPath(relPath, entry.IsDir()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if needle != "" {
			name := normalizeSearchText(entry.Name())
			path := normalizeSearchText(relPath)
			if !strings.Contains(name, needle) && !strings.Contains(path, needle) {
				return nil
			}
		}
		if pattern != "" {
			matched, err := pathMatchesPattern(relPath, pattern)
			if err != nil || !matched {
				return err
			}
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		itemType := "file"
		if entry.IsDir() {
			itemType = "folder"
		}
		size := int64(0)
		if !entry.IsDir() {
			size = info.Size()
		}
		results = append(results, SearchResult{
			Name:       entry.Name(),
			Path:       relPath,
			ParentPath: getParentPath(relPath),
			Type:       itemType,
			Size:       size,
			ModifiedAt: info.ModTime().UTC(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(results, func(i, j int) bool {
		leftScore := searchScore(results[i], needle)
		rightScore := searchScore(results[j], needle)
		if leftScore != rightScore {
			return leftScore < rightScore
		}
		return strings.ToLower(results[i].Path) < strings.ToLower(results[j].Path)
	})
	if len(results) > maxResults {
		results = results[:maxResults]
	}
	return results, nil
}

func (s *Service) SearchText(query string, regex bool, maxResults int, contextLines int) ([]TextSearchResult, bool, error) {
	if err := s.migrateLegacyTrashDir(); err != nil {
		return nil, false, err
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return []TextSearchResult{}, false, nil
	}
	if maxResults <= 0 || maxResults > 200 {
		maxResults = 80
	}
	if contextLines < 0 {
		contextLines = 0
	}
	if contextLines > 3 {
		contextLines = 3
	}
	var pattern *regexp.Regexp
	if regex {
		compiled, err := regexp.Compile(query)
		if err != nil {
			return nil, false, err
		}
		pattern = compiled
	}

	results := make([]TextSearchResult, 0, maxResults)
	limited := false
	err := filepath.WalkDir(s.rootAbs, func(absPath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if absPath == s.rootAbs {
			return nil
		}
		rel, err := filepath.Rel(s.rootAbs, absPath)
		if err != nil {
			return err
		}
		relPath := filepath.ToSlash(rel)
		if shouldSkipSearchPath(relPath, entry.IsDir()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || !isTextPath(relPath) {
			return nil
		}
		fileResults, err := searchTextFile(absPath, relPath, query, pattern, maxResults-len(results), contextLines)
		if err != nil {
			return err
		}
		results = append(results, fileResults...)
		if len(results) >= maxResults {
			limited = true
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return results, limited, nil
}

func normalizeSearchText(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, "\\", "/")))
}

func searchScore(result SearchResult, needle string) int {
	name := normalizeSearchText(result.Name)
	path := normalizeSearchText(result.Path)
	switch {
	case name == needle:
		return 0
	case strings.HasPrefix(name, needle):
		return 1
	case strings.Contains(name, needle):
		return 2
	case strings.HasPrefix(path, needle):
		return 3
	default:
		return 4
	}
}

func shouldSkipSearchPath(path string, isDir bool) bool {
	clean := filepath.ToSlash(filepath.Clean(strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")))
	if clean == "." || clean == "" {
		return false
	}
	if isAnyTrashPath(clean) {
		return true
	}
	if !isDir {
		return false
	}
	switch filepath.Base(clean) {
	case ".git", "node_modules", "vendor", "dist", "build", ".cache", ".vite", "__pycache__":
		return true
	default:
		return false
	}
}

func pathMatchesPattern(path, pattern string) (bool, error) {
	if pattern == "" {
		return true, nil
	}
	matched, err := filepath.Match(pattern, path)
	if err == nil && matched {
		return true, nil
	}
	if strings.Contains(pattern, "/") {
		return matched, err
	}
	baseMatched, baseErr := filepath.Match(pattern, filepath.Base(path))
	if err != nil && baseErr == nil {
		err = nil
	}
	return baseMatched, err
}

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
