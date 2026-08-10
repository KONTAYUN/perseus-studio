package ai

import (
	"strings"
	"testing"
)

func TestReplaceInnerHTMLPreservesContainerAttributes(t *testing.T) {
	input := `<h1>Title</h1><p class="kept" style="color:red">Old <strong>text</strong></p>`
	output, changed, err := applyNoteBlockEdits(input, []noteBlockEdit{{
		BlockID: "b0002", Operation: "replace_inner_html", HTML: `New <em>text</em>`,
	}})
	if err != nil {
		t.Fatalf("applyNoteBlockEdits returned error: %v", err)
	}
	if len(changed) != 1 || changed[0]["operation"] != "replace_inner_html" {
		t.Fatalf("unexpected changed blocks: %#v", changed)
	}
	if !strings.Contains(output, `class="kept"`) ||
		!strings.Contains(output, `style="color:red"`) ||
		!strings.Contains(output, `New <em>text</em>`) {
		t.Fatalf("container attributes or replacement content were lost: %s", output)
	}
}

func TestDeleteBlockRangeRemovesInclusiveContiguousBlocks(t *testing.T) {
	input := `<h1>Title</h1><p>One</p><p>Two</p><p>Three</p><h2>Keep</h2>`
	output, changed, err := applyNoteBlockEdits(input, []noteBlockEdit{{
		BlockID: "b0002", EndBlockID: "b0004", Operation: "delete_block_range",
	}})
	if err != nil {
		t.Fatalf("applyNoteBlockEdits returned error: %v", err)
	}
	if len(changed) != 1 || changed[0]["end_block_id"] != "b0004" {
		t.Fatalf("unexpected changed blocks: %#v", changed)
	}
	if strings.Contains(output, "One") || strings.Contains(output, "Two") || strings.Contains(output, "Three") {
		t.Fatalf("range content was not fully removed: %s", output)
	}
	if !strings.Contains(output, "Title") || !strings.Contains(output, "Keep") {
		t.Fatalf("content outside the range changed: %s", output)
	}
}
