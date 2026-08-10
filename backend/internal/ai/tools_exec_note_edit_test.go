package ai

import (
	"strings"
	"testing"
)

func TestApplyNoteBlockEditsSupportsAtomicBlockOperations(t *testing.T) {
	input := `<h1>Title</h1><p>First</p><p>Last</p>`
	output, changed, err := applyNoteBlockEdits(input, []noteBlockEdit{
		{BlockID: "b0001", Operation: "insert_after", HTML: `<p>Before</p>`},
		{BlockID: "b0002", Operation: "replace_html", HTML: `<p style="color:#ff0000">Updated</p>`},
		{BlockID: "b0003", Operation: "insert_after", HTML: `<p>After</p>`},
	})
	if err != nil {
		t.Fatalf("applyNoteBlockEdits returned error: %v", err)
	}
	if len(changed) != 3 {
		t.Fatalf("expected 3 changed blocks, got %d", len(changed))
	}
	for _, want := range []string{"Before", "Updated", "Last", "After"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q: %s", want, output)
		}
	}
	if strings.Contains(output, "First") {
		t.Fatalf("replaced block remained in output: %s", output)
	}
	if !(strings.Index(output, "Before") < strings.Index(output, "Updated") &&
		strings.Index(output, "Updated") < strings.Index(output, "Last") &&
		strings.Index(output, "Last") < strings.Index(output, "After")) {
		t.Fatalf("unexpected block order: %s", output)
	}
}

func TestApplyNoteBlockEditsRejectsWholeBatchBeforeMutation(t *testing.T) {
	input := `<p>First</p><p>Second</p>`
	_, _, err := applyNoteBlockEdits(input, []noteBlockEdit{
		{BlockID: "b0001", Operation: "replace_html", HTML: `<p>Changed</p>`},
		{BlockID: "b9999", Operation: "delete"},
	})
	if err == nil {
		t.Fatal("expected stale block error")
	}
	if normalizeToolError(err).Code != ToolErrorStaleDocument {
		t.Fatalf("expected stale document error, got %v", err)
	}
	if !strings.Contains(input, "First") {
		t.Fatal("test input was unexpectedly mutated")
	}
}

func TestApplyNoteBlockEditsReportsMultiplePreflightErrors(t *testing.T) {
	_, _, err := applyNoteBlockEdits(`<p>First</p><p>Second</p>`, []noteBlockEdit{
		{BlockID: "b9999", Operation: "delete"},
		{BlockID: "b0001", Operation: "replace_html"},
	})
	if err == nil {
		t.Fatal("expected preflight errors")
	}
	typed := normalizeToolError(err)
	if typed.Phase != "preflight" {
		t.Fatalf("expected preflight phase, got %q", typed.Phase)
	}
	errors, ok := typed.Details["errors"].([]map[string]any)
	if !ok || len(errors) != 2 {
		t.Fatalf("expected two deterministic preflight errors, got %#v", typed.Details)
	}
}

func TestApplyNoteBlockEditsSanitizesInsertedHTML(t *testing.T) {
	output, _, err := applyNoteBlockEdits(`<p>Safe</p>`, []noteBlockEdit{{
		BlockID:   "b0001",
		Operation: "insert_after",
		HTML:      `<p onclick="alert(1)">Added<script>alert(2)</script></p>`,
	}})
	if err != nil {
		t.Fatalf("applyNoteBlockEdits returned error: %v", err)
	}
	lower := strings.ToLower(output)
	if strings.Contains(lower, "onclick") || strings.Contains(lower, "<script") {
		t.Fatalf("unsafe HTML survived sanitization: %s", output)
	}
	if !strings.Contains(output, "Added") {
		t.Fatalf("safe text was lost: %s", output)
	}
}

func TestApplyNoteBlockEditsReplaceTextPreservesMarkup(t *testing.T) {
	expected := 1
	output, changed, err := applyNoteBlockEdits(
		`<p style="text-align:center">Version <strong>14.7</strong></p>`,
		[]noteBlockEdit{{
			BlockID: "b0001", Operation: "replace_text",
			Find: "14.7", Replace: "14.8", ExpectedMatches: &expected,
		}},
	)
	if err != nil {
		t.Fatalf("applyNoteBlockEdits returned error: %v", err)
	}
	if len(changed) != 1 || !strings.Contains(output, "<strong>14.8</strong>") {
		t.Fatalf("replace_text did not preserve markup: %s", output)
	}
	if !strings.Contains(output, `text-align:center`) {
		t.Fatalf("replace_text lost block formatting: %s", output)
	}
}

func TestApplyNoteBlockEditsReplaceTextRejectsUnexpectedMatchCount(t *testing.T) {
	expected := 1
	_, _, err := applyNoteBlockEdits(`<p>old old</p>`, []noteBlockEdit{{
		BlockID: "b0001", Operation: "replace_text",
		Find: "old", Replace: "new", ExpectedMatches: &expected,
	}})
	if err == nil {
		t.Fatal("expected match-count verification error")
	}
	typed := normalizeToolError(err)
	if typed.Code != ToolErrorVerificationFailed {
		t.Fatalf("expected verification error, got %v", err)
	}
	if typed.Details["edit_index"] != 0 || typed.Details["block_id"] != "b0001" ||
		typed.Details["expected_matches"] != 1 || typed.Details["actual_matches"] != 2 {
		t.Fatalf("expected deterministic edit failure details, got %#v", typed.Details)
	}
}

func TestApplyNoteBlockEditsRejectsTextSpanningInlineNodes(t *testing.T) {
	expected := 1
	_, _, err := applyNoteBlockEdits(`<p>前<strong>中</strong>后</p>`, []noteBlockEdit{{
		BlockID: "b0001", Operation: "replace_text", Find: "前中后", Replace: "替换", ExpectedMatches: &expected,
	}})
	if err == nil {
		t.Fatal("expected cross-node text replacement to be rejected")
	}
	typed := normalizeToolError(err)
	if typed.Code != ToolErrorTextSpansMultipleNodes {
		t.Fatalf("expected text_spans_multiple_nodes, got %v", typed.Code)
	}
	if typed.Details["recommended_operation"] != "replace_inner_html" {
		t.Fatalf("expected structured replacement recommendation, got %#v", typed.Details)
	}
	segmentIDs, ok := typed.Details["segment_ids"].([]string)
	if !ok || len(segmentIDs) < 2 {
		t.Fatalf("expected all involved text segments, got %#v", typed.Details["segment_ids"])
	}
}

func TestApplyNoteBlockEditsReplaceTextCanTargetTextSegment(t *testing.T) {
	block := buildNoteBlockDocument(`<p>前<strong>中</strong>后</p>`).Blocks[0]
	segments := noteBlockTextSegments(block)
	if len(segments) < 3 {
		t.Fatalf("expected three text segments, got %#v", segments)
	}
	segmentID := stringFromAny(segments[1]["segment_id"])
	expected := 1
	output, _, err := applyNoteBlockEdits(`<p>前<strong>中</strong>后</p>`, []noteBlockEdit{{
		BlockID: "b0001", SegmentID: segmentID, Operation: "replace_text", Find: "中", Replace: "央", ExpectedMatches: &expected,
	}})
	if err != nil {
		t.Fatalf("segment-targeted replacement returned error: %v", err)
	}
	if !strings.Contains(output, `<strong>央</strong>`) || strings.Contains(output, "<strong>中</strong>") {
		t.Fatalf("segment-targeted replacement did not preserve inline markup: %s", output)
	}
}

func TestValidateNoteEditStructureRejectsNewEmptyList(t *testing.T) {
	before := `<h1>Title</h1><p>Body</p>`
	after := `<h1>Title</h1><ul></ul><p>Body</p>`
	_, err := validateNoteEditStructure(before, after, []noteBlockEdit{{
		BlockID: "b0001", Operation: "insert_after", HTML: `<ul></ul>`,
	}})
	if err == nil || normalizeToolError(err).Code != ToolErrorVerificationFailed {
		t.Fatalf("expected empty-list validation failure, got %v", err)
	}
}

func TestValidateNoteEditStructureRejectsUndeclaredBlockChanges(t *testing.T) {
	before := `<h1>Title</h1><p>First</p><p>Second</p>`
	after := `<h1>Title</h1><p>Changed</p><p>Also changed</p>`
	_, err := validateNoteEditStructure(before, after, []noteBlockEdit{{
		BlockID: "b0002", Operation: "replace_html", HTML: `<p>Changed</p>`,
	}})
	if err == nil || normalizeToolError(err).Code != ToolErrorVerificationFailed {
		t.Fatalf("expected undeclared-block validation failure, got %v", err)
	}
}
