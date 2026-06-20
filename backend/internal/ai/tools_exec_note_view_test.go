package ai

import (
	"strings"
	"testing"
)

func TestRenderNoteViewProducesCompactTextStructureAndFormat(t *testing.T) {
	fragment := `<h2>Plan</h2><p data-text-align="center" style="text-align:center;color:#123456;background-color:#ffeeaa;font-size:18px">Hello <strong>world</strong></p><ul><li>First item</li></ul><table><tr><th>A</th></tr><tr><td>B</td></tr></table><img alt="Cover" data-note-src=".assets/Note/cover.png"><script>alert(1)</script><style>.x{display:none}</style>`

	text := renderNoteView(fragment, "text")
	if strings.Contains(text, "<script") || strings.Contains(text, "display:none") || strings.Contains(text, "<strong>") {
		t.Fatalf("text view leaked ignored HTML: %s", text)
	}
	if !strings.Contains(text, "Plan") || !strings.Contains(text, "Hello world") || !strings.Contains(text, "First item") || !strings.Contains(text, "Cover") {
		t.Fatalf("text view missed note content: %s", text)
	}

	structure := renderNoteView(fragment, "structure")
	for _, want := range []string{"## Plan", "Hello world", "- First item", "A", "B", "Cover"} {
		if !strings.Contains(structure, want) {
			t.Fatalf("structure view missing %q: %s", want, structure)
		}
	}

	format := renderNoteView(fragment, "format")
	for _, want := range []string{"paragraph", "align=center", "text-align=center", "font-size=18px", "color=#123456", "background-color=#ffeeaa", "table rows=2", "src=.assets/Note/cover.png"} {
		if !strings.Contains(format, want) {
			t.Fatalf("format view missing %q: %s", want, format)
		}
	}

	raw := renderNoteView(fragment, "raw")
	if !strings.Contains(raw, "<script>alert(1)</script>") {
		t.Fatalf("raw view should preserve exact fragment: %s", raw)
	}
}
