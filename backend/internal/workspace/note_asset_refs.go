package workspace

import (
	htmlstd "html"
	"path"
	"regexp"
	"strings"
)

var (
	htmlNoteImageTagPattern = regexp.MustCompile(`(?is)<img\b[^>]*>`)
)

func rewriteMovedHTMLNoteAssetRefs(fragment, oldNotePath, newNotePath string) string {
	if !isHTMLNotePath(oldNotePath) || !isHTMLNotePath(newNotePath) {
		return fragment
	}
	return htmlNoteImageTagPattern.ReplaceAllStringFunc(fragment, func(tag string) string {
		reference := firstHTMLAttrValue(tag, "data-note-src")
		if !isLocalWorkspaceReference(reference) {
			return tag
		}
		assetPath := resolveWorkspaceReferenceFromNote(oldNotePath, reference)
		if assetPath == "" {
			return tag
		}
		nextReference := relativeWorkspaceReference(path.Dir(normalizeWorkspaceReference(newNotePath)), assetPath)
		if nextReference == "" || nextReference == reference {
			return tag
		}
		tag = setHTMLAttrValue(tag, "data-note-src", nextReference)
		if firstHTMLAttrValue(tag, "src") != "" {
			tag = setHTMLAttrValue(tag, "src", nextReference)
		}
		return tag
	})
}

func isHTMLNotePath(notePath string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(notePath)), ".html")
}

func isLocalWorkspaceReference(reference string) bool {
	lower := strings.ToLower(strings.TrimSpace(reference))
	if lower == "" {
		return false
	}
	return !strings.HasPrefix(lower, "http://") &&
		!strings.HasPrefix(lower, "https://") &&
		!strings.HasPrefix(lower, "data:") &&
		!strings.HasPrefix(lower, "mailto:") &&
		!strings.HasPrefix(lower, "#") &&
		!strings.HasPrefix(lower, "/api/")
}

func resolveWorkspaceReferenceFromNote(notePath, reference string) string {
	reference = normalizeWorkspaceReference(reference)
	if reference == "" {
		return ""
	}
	if !strings.HasPrefix(reference, ".") {
		return reference
	}
	base := path.Dir(normalizeWorkspaceReference(notePath))
	if base == "." {
		base = ""
	}
	resolved := normalizeWorkspaceReference(path.Join(base, reference))
	if resolved == "" || resolved == "." || strings.HasPrefix(resolved, "../") || resolved == ".." {
		return ""
	}
	return resolved
}

func relativeWorkspaceReference(fromDir, targetPath string) string {
	fromParts := splitWorkspacePath(fromDir)
	targetParts := splitWorkspacePath(targetPath)
	common := 0
	for common < len(fromParts) && common < len(targetParts) && fromParts[common] == targetParts[common] {
		common++
	}
	parts := make([]string, 0, len(fromParts)-common+len(targetParts)-common)
	for i := common; i < len(fromParts); i++ {
		parts = append(parts, "..")
	}
	parts = append(parts, targetParts[common:]...)
	if len(parts) == 0 {
		return "."
	}
	return strings.Join(parts, "/")
}

func splitWorkspacePath(value string) []string {
	normalized := normalizeWorkspaceReference(value)
	if normalized == "" || normalized == "." {
		return nil
	}
	parts := strings.Split(normalized, "/")
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" && part != "." {
			filtered = append(filtered, part)
		}
	}
	return filtered
}

func normalizeWorkspaceReference(value string) string {
	cleaned := path.Clean(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/"))
	if cleaned == "." {
		return ""
	}
	return strings.Trim(cleaned, "/")
}

func firstHTMLAttrValue(tagOrAttrs, name string) string {
	pattern := regexp.MustCompile(`(?is)\b` + regexp.QuoteMeta(name) + `\s*=\s*("([^"]*)"|'([^']*)'|([^\s>]+))`)
	match := pattern.FindStringSubmatch(tagOrAttrs)
	if len(match) == 0 {
		return ""
	}
	for _, value := range match[2:] {
		if value != "" {
			return htmlstd.UnescapeString(value)
		}
	}
	return ""
}

func setHTMLAttrValue(tag, name, value string) string {
	escaped := htmlstd.EscapeString(value)
	attrPattern := regexp.MustCompile(`(?is)\s+` + regexp.QuoteMeta(name) + `\s*=\s*("([^"]*)"|'([^']*)'|[^\s>]+)`)
	if attrPattern.MatchString(tag) {
		return attrPattern.ReplaceAllStringFunc(tag, func(string) string {
			return ` ` + name + `="` + escaped + `"`
		})
	}
	end := strings.LastIndex(tag, ">")
	if end < 0 {
		return tag
	}
	before := strings.TrimRight(tag[:end], " \t\r\n")
	return before + ` ` + name + `="` + escaped + `"` + tag[end:]
}
