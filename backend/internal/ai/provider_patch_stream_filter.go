package ai

import "strings"

const (
	patchBeginMarker = "*** Begin Patch"
	patchEndMarker   = "*** End Patch"
)

type providerPatchBlockFilter struct {
	parser WorkspacePatchEnvelopeParser
}

type providerPatchFilteredEmitter struct {
	inner    *toolGatedStreamDeltaEmitter
	filter   *providerPatchBlockFilter
	streamed strings.Builder
}

type WorkspacePatchEnvelopeParser struct {
	buffer     string
	inside     bool
	afterPatch bool
}

func newProviderPatchFilteredEmitter(inner *toolGatedStreamDeltaEmitter) *providerPatchFilteredEmitter {
	return &providerPatchFilteredEmitter{
		inner:  inner,
		filter: &providerPatchBlockFilter{},
	}
}

func (e *providerPatchFilteredEmitter) Add(text string) {
	if e == nil || text == "" {
		return
	}
	visible := e.filter.Feed(text)
	if visible == "" {
		return
	}
	e.streamed.WriteString(visible)
	if e.inner != nil {
		e.inner.Add(visible)
	}
}

func (e *providerPatchFilteredEmitter) MarkToolObserved() {
	if e == nil || e.inner == nil {
		return
	}
	e.inner.MarkToolObserved()
}

func (e *providerPatchFilteredEmitter) Flush() {
	if e == nil {
		return
	}
	if tail := e.filter.Close(); tail != "" {
		e.streamed.WriteString(tail)
		if e.inner != nil {
			e.inner.Add(tail)
		}
	}
	if e.inner != nil {
		e.inner.Flush()
	}
}

func (e *providerPatchFilteredEmitter) StreamedContent() string {
	if e == nil {
		return ""
	}
	return e.streamed.String()
}

func (f *providerPatchBlockFilter) Feed(text string) string {
	if f == nil {
		return ""
	}
	return f.parser.Feed(text)
}

func (f *providerPatchBlockFilter) Close() string {
	if f == nil {
		return ""
	}
	return f.parser.Close()
}

func (f *WorkspacePatchEnvelopeParser) Feed(text string) string {
	if f == nil || text == "" {
		return ""
	}
	f.buffer += text
	var out strings.Builder
	for {
		if f.inside {
			if !f.consumeHiddenPatch() {
				return out.String()
			}
			f.afterPatch = true
			continue
		}
		if f.afterPatch {
			visible, pending := consumePostPatchWrapper(f.buffer)
			if pending {
				return out.String()
			}
			f.buffer = visible
			f.afterPatch = false
			continue
		}
		start := strings.Index(f.buffer, patchBeginMarker)
		if start < 0 {
			visible, carry, pending := splitOutsidePatchVisible(f.buffer)
			out.WriteString(visible)
			f.buffer = carry
			if pending {
				return out.String()
			}
			return out.String()
		}
		if start > 0 {
			visible, _ := splitPrePatchVisible(f.buffer[:start])
			out.WriteString(visible)
		}
		f.buffer = f.buffer[start+len(patchBeginMarker):]
		f.inside = true
	}
}

func (f *WorkspacePatchEnvelopeParser) Close() string {
	if f == nil {
		return ""
	}
	if f.inside {
		f.buffer = ""
		f.inside = false
		f.afterPatch = false
		return ""
	}
	out := ""
	if f.afterPatch {
		visible, pending := consumePostPatchWrapper(f.buffer)
		if !pending {
			visible, _, _ = splitOutsidePatchVisible(visible)
			out = visible
		}
	} else {
		visible, carry, pending := splitOutsidePatchVisible(f.buffer)
		if pending {
			out = visible + carry
		} else {
			out = visible
		}
	}
	f.buffer = ""
	f.afterPatch = false
	return out
}

func patchBoundarySuffixLen(text, marker string) int {
	best := 0
	limit := len(marker) - 1
	if limit > len(text) {
		limit = len(text)
	}
	for size := 1; size <= limit; size++ {
		if strings.HasSuffix(text, marker[:size]) {
			best = size
		}
	}
	return best
}

func (f *WorkspacePatchEnvelopeParser) consumeHiddenPatch() bool {
	end := strings.Index(f.buffer, patchEndMarker)
	if end < 0 {
		keep := patchBoundarySuffixLen(f.buffer, patchEndMarker)
		if keep < len(f.buffer) {
			f.buffer = f.buffer[len(f.buffer)-keep:]
		}
		return false
	}
	f.buffer = f.buffer[end+len(patchEndMarker):]
	f.inside = false
	return true
}

func splitOutsidePatchVisible(text string) (visible string, carry string, pending bool) {
	keep := patchBoundarySuffixLen(text, patchBeginMarker)
	if keep > 0 {
		prefix := text[:len(text)-keep]
		markerPrefix := text[len(text)-keep:]
		visible, wrapper := splitPrePatchVisible(prefix)
		return visible, wrapper + markerPrefix, true
	}
	if isPrePatchWrapperPrefix(text) {
		return "", text, true
	}
	return text, "", false
}

func splitPrePatchVisible(text string) (visible string, wrapper string) {
	if strings.TrimSpace(text) == "" {
		return "", text
	}
	if index, ok := trailingPatchFenceWrapperStart(text); ok {
		beforeFence := text[:index]
		visible = strings.TrimRight(beforeFence, " \t\r\n")
		if visible != "" && len(visible) < len(beforeFence) {
			visible += "\n"
		}
		return visible, text[index:]
	}
	return text, ""
}

func consumePostPatchWrapper(text string) (visible string, pending bool) {
	if strings.TrimSpace(text) == "" {
		return "", true
	}
	trimmedLeft := strings.TrimLeft(text, " \t\r\n")
	if strings.HasPrefix(trimmedLeft, patchEndMarker) {
		rest := trimmedLeft[len(patchEndMarker):]
		if rest == "" {
			return "", true
		}
		rest = strings.TrimLeft(rest, " \t\r\n")
		if rest == "" {
			return "", true
		}
		return rest, false
	}
	if isPatchFencePrefix(strings.TrimSpace(trimmedLeft)) {
		return "", true
	}
	if strings.HasPrefix(trimmedLeft, "```") {
		line, rest, hasNewline := cutFirstLine(trimmedLeft)
		if isPatchFenceLine(strings.TrimSpace(line)) {
			if !hasNewline {
				return "", true
			}
			return rest, false
		}
	}
	return text, false
}

func trailingPatchFenceWrapperStart(text string) (int, bool) {
	for index := strings.LastIndex(text, "```"); index >= 0; {
		if isPatchFenceLine(strings.TrimSpace(text[index:])) {
			return index, true
		}
		if index == 0 {
			break
		}
		next := strings.LastIndex(text[:index], "```")
		if next < 0 {
			break
		}
		index = next
	}
	return -1, false
}

func isPrePatchWrapperPrefix(text string) bool {
	trimmed := strings.TrimSpace(text)
	return trimmed == "" || isPatchFencePrefix(trimmed)
}

func isPatchFenceLine(text string) bool {
	switch text {
	case "```", "```text", "```patch", "```diff":
		return true
	default:
		return false
	}
}

func isPatchFencePrefix(text string) bool {
	if text == "" {
		return true
	}
	for _, line := range []string{"```", "```text", "```patch", "```diff"} {
		if strings.HasPrefix(line, text) {
			return true
		}
	}
	return false
}

func cutFirstLine(text string) (line string, rest string, hasNewline bool) {
	index := strings.IndexByte(text, '\n')
	if index < 0 {
		return text, "", false
	}
	return text[:index], text[index+1:], true
}
