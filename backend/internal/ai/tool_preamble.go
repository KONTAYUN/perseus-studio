package ai

import (
	"strings"
	"unicode/utf8"
)

func cleanToolPreamble(content string) string {
	content = providerVisibleText(content)
	content = strings.TrimSpace(content)
	if isToolPreamblePlaceholder(content) {
		return ""
	}
	return content
}

func isToolPreamblePlaceholder(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" || utf8.RuneCountInString(content) > 8 {
		return false
	}
	for _, item := range content {
		switch item {
		case '.', '…', '。':
			continue
		default:
			return false
		}
	}
	return true
}
