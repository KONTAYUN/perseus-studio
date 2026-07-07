package api

import (
	htmlstd "html"
	"regexp"
	"strings"
	"webide-backend/internal/workspace"
)

func wrapHTMLDocument(title, fragment string) string {
	return "<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n<title>" +
		htmlstd.EscapeString(title) +
		"</title>\n</head>\n<body>\n" +
		fragment +
		"\n</body>\n</html>\n"
}

func wrapHTMLFragmentDocument(fragment string) string {
	return "<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n</head>\n<body>\n" +
		fragment +
		"\n</body>\n</html>\n"
}

func isNoteLikePath(targetPath string) bool {
	lower := strings.ToLower(targetPath)
	return strings.HasSuffix(lower, ".html")
}

func extractHTMLFragment(content string) string {
	bodyPattern := regexp.MustCompile(`(?is)<body\b[^>]*>(.*?)</body>`)
	if match := bodyPattern.FindStringSubmatch(content); len(match) > 1 {
		return sanitizeHTMLFragment(match[1])
	}
	content = regexp.MustCompile(`(?is)<!doctype[^>]*>`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`(?is)</?html\b[^>]*>`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`(?is)<head\b[^>]*>.*?</head>`).ReplaceAllString(content, "")
	return sanitizeHTMLFragment(content)
}

func sanitizeHTMLFragment(fragment string) string {
	return workspace.SanitizeHTMLFragment(fragment)
}
