package workspace

import (
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
		if isAnyTrashPath(relPath) || s.hideDocumentImageEntry(getParentPath(relPath), entry.Name()) {
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

type TextSearchOptions struct {
	PathScope    string
	IncludeGlobs []string
	ExcludeGlobs []string
}

func (s *Service) SearchText(query string, regex bool, maxResults int, contextLines int) ([]TextSearchResult, bool, error) {
	return s.SearchTextScoped(query, regex, maxResults, contextLines, TextSearchOptions{})
}

func (s *Service) SearchTextScoped(query string, regex bool, maxResults int, contextLines int, opts TextSearchOptions) ([]TextSearchResult, bool, error) {
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
	pathScope := strings.Trim(strings.ReplaceAll(opts.PathScope, "\\", "/"), "/")
	if pathScope != "" {
		cleanScope, _, err := s.sanitizePath(pathScope)
		if err != nil {
			return nil, false, err
		}
		pathScope = cleanScope
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
		if pathScope != "" && !searchPathWithinScope(relPath, pathScope) {
			if entry.IsDir() && !searchScopeMayContain(pathScope, relPath) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || !isTextPath(relPath) {
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		if !searchPathMatchesAnyInclude(relPath, opts.IncludeGlobs) || searchPathMatchesAnyExclude(relPath, opts.ExcludeGlobs) {
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

func searchPathWithinScope(path, scope string) bool {
	path = strings.Trim(strings.ReplaceAll(path, "\\", "/"), "/")
	scope = strings.Trim(strings.ReplaceAll(scope, "\\", "/"), "/")
	return scope == "" || path == scope || strings.HasPrefix(path, scope+"/")
}

func searchScopeMayContain(scope, dir string) bool {
	scope = strings.Trim(strings.ReplaceAll(scope, "\\", "/"), "/")
	dir = strings.Trim(strings.ReplaceAll(dir, "\\", "/"), "/")
	return dir == "" || scope == dir || strings.HasPrefix(scope, dir+"/")
}

func searchPathMatchesAnyInclude(path string, patterns []string) bool {
	patterns = normalizeSearchGlobs(patterns)
	if len(patterns) == 0 {
		return true
	}
	for _, pattern := range patterns {
		matched, err := pathMatchesPattern(path, pattern)
		if err == nil && matched {
			return true
		}
	}
	return false
}

func searchPathMatchesAnyExclude(path string, patterns []string) bool {
	for _, pattern := range normalizeSearchGlobs(patterns) {
		matched, err := pathMatchesPattern(path, pattern)
		if err == nil && matched {
			return true
		}
	}
	return false
}

func normalizeSearchGlobs(patterns []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(strings.ReplaceAll(pattern, "\\", "/"))
		if pattern == "" || seen[pattern] {
			continue
		}
		seen[pattern] = true
		out = append(out, pattern)
	}
	return out
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
	if isAnyTrashPath(clean) || clean == DocumentImageDir || strings.HasPrefix(clean, DocumentImageDir+"/") || documentImageToken(clean) != "" {
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
