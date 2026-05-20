package ai

import "strings"

var providerReasoningTagNames = []string{"thinking", "think"}

type providerVisibleTextFilter struct {
	buffer    string
	inside    bool
	activeTag string
}

func newProviderVisibleTextFilter() *providerVisibleTextFilter {
	return &providerVisibleTextFilter{}
}

func providerVisibleText(text string) string {
	filter := newProviderVisibleTextFilter()
	return strings.TrimSpace(filter.Feed(text) + filter.Close())
}

func appendProviderVisibleText(content *strings.Builder, deltas *streamDeltaEmitter, filter *providerVisibleTextFilter, text string) string {
	if filter == nil {
		filter = newProviderVisibleTextFilter()
	}
	visible := filter.Feed(text)
	visible = trimLeadingProviderVisibleWhitespace(content, visible)
	if visible == "" {
		return ""
	}
	content.WriteString(visible)
	if deltas != nil {
		deltas.Add(visible)
	}
	return visible
}

func closeProviderVisibleText(content *strings.Builder, deltas *streamDeltaEmitter, filter *providerVisibleTextFilter) string {
	if filter == nil {
		return ""
	}
	visible := filter.Close()
	visible = trimLeadingProviderVisibleWhitespace(content, visible)
	if visible == "" {
		return ""
	}
	content.WriteString(visible)
	if deltas != nil {
		deltas.Add(visible)
	}
	return visible
}

func trimLeadingProviderVisibleWhitespace(content *strings.Builder, visible string) string {
	if content != nil && content.Len() == 0 {
		return strings.TrimLeft(visible, " \t\r\n")
	}
	return visible
}

func (f *providerVisibleTextFilter) Feed(text string) string {
	if f == nil || text == "" {
		return ""
	}
	f.buffer += text
	var out strings.Builder
	for {
		if f.inside {
			if !f.consumeInsideReasoning() {
				return out.String()
			}
			continue
		}
		openIndex, openTag, openLen := findProviderReasoningOpening(f.buffer)
		closeIndex, closeLen := findProviderReasoningClosing(f.buffer)
		if closeIndex >= 0 && (openIndex < 0 || closeIndex < openIndex) {
			if closeIndex > 0 {
				out.WriteString(f.buffer[:closeIndex])
			}
			f.buffer = f.buffer[closeIndex+closeLen:]
			continue
		}
		if openIndex < 0 {
			keep := providerReasoningBoundarySuffixLen(f.buffer, providerReasoningBoundaryTags())
			if keep < len(f.buffer) {
				out.WriteString(f.buffer[:len(f.buffer)-keep])
				f.buffer = f.buffer[len(f.buffer)-keep:]
			}
			return out.String()
		}
		if openIndex > 0 {
			out.WriteString(f.buffer[:openIndex])
		}
		f.buffer = f.buffer[openIndex+openLen:]
		f.inside = true
		f.activeTag = openTag
	}
}

func (f *providerVisibleTextFilter) Close() string {
	if f == nil {
		return ""
	}
	if f.inside {
		f.buffer = ""
		f.inside = false
		f.activeTag = ""
		return ""
	}
	out := f.buffer
	f.buffer = ""
	return out
}

func (f *providerVisibleTextFilter) consumeInsideReasoning() bool {
	closing := "</" + f.activeTag + ">"
	index := strings.Index(strings.ToLower(f.buffer), closing)
	if index < 0 {
		keep := providerReasoningBoundarySuffixLen(f.buffer, []string{closing})
		if keep < len(f.buffer) {
			f.buffer = f.buffer[len(f.buffer)-keep:]
		}
		return false
	}
	f.buffer = f.buffer[index+len(closing):]
	f.inside = false
	f.activeTag = ""
	return true
}

func findProviderReasoningOpening(text string) (int, string, int) {
	lower := strings.ToLower(text)
	bestIndex := -1
	bestTag := ""
	bestLen := 0
	for _, tag := range providerReasoningTagNames {
		opening := "<" + tag + ">"
		index := strings.Index(lower, opening)
		if index >= 0 && (bestIndex < 0 || index < bestIndex) {
			bestIndex = index
			bestTag = tag
			bestLen = len(opening)
		}
	}
	return bestIndex, bestTag, bestLen
}

func findProviderReasoningClosing(text string) (int, int) {
	lower := strings.ToLower(text)
	bestIndex := -1
	bestLen := 0
	for _, tag := range providerReasoningTagNames {
		closing := "</" + tag + ">"
		index := strings.Index(lower, closing)
		if index >= 0 && (bestIndex < 0 || index < bestIndex) {
			bestIndex = index
			bestLen = len(closing)
		}
	}
	return bestIndex, bestLen
}

func providerReasoningBoundaryTags() []string {
	items := make([]string, 0, len(providerReasoningTagNames)*2)
	for _, tag := range providerReasoningTagNames {
		items = append(items, "<"+tag+">", "</"+tag+">")
	}
	return items
}

func providerReasoningBoundarySuffixLen(text string, candidates []string) int {
	lower := strings.ToLower(text)
	best := 0
	for _, candidate := range candidates {
		limit := len(candidate) - 1
		if limit > len(lower) {
			limit = len(lower)
		}
		for size := 1; size <= limit; size++ {
			if strings.HasSuffix(lower, candidate[:size]) && size > best {
				best = size
			}
		}
	}
	return best
}
