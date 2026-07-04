package workspace

import (
	"regexp"
	"strings"
)

var (
	htmlScriptTagPattern      = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`)
	htmlStyleTagPattern       = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`)
	htmlEventAttributePattern = regexp.MustCompile(`(?is)\s+on[a-z]+\s*=\s*("([^"]*)"|'([^']*)'|[^\s>]+)`)
	htmlJSURLPattern          = regexp.MustCompile(`(?is)\s+(href|src)\s*=\s*("javascript:[^"]*"|'javascript:[^']*'|javascript:[^\s>]+)`)
)

func SanitizeHTMLFragment(fragment string) string {
	fragment = htmlScriptTagPattern.ReplaceAllString(fragment, "")
	fragment = htmlStyleTagPattern.ReplaceAllString(fragment, "")
	fragment = htmlEventAttributePattern.ReplaceAllString(fragment, "")
	fragment = htmlJSURLPattern.ReplaceAllString(fragment, "")
	return strings.TrimSpace(fragment)
}
