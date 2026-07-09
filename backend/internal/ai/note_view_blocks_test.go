package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestReadWorkspaceNoteViewIndexHandlesLongHTMLLines(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	longParagraph := strings.Repeat("long text ", 3200)
	fragment := `<h1 style="text-align:center">Title</h1><p>` + longParagraph + `</p><ul><li data-id="a">List item</li></ul><table><tr><td style="background-color:#ffff00">Cell</td></tr></table><p><img data-note-src=".assets/pic.png" data-width="50%"></p>`
	if _, err := env.files.CreateFile(ctx, "Long.html", fragment); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}

	result, _, _, _, err := env.service.executeTool(ctx, "read_workspace_note_view", `{"path":"Long.html","mode":"index"}`, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("executeTool returned error: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("invalid index payload: %v", err)
	}
	if payload["mode"] != "index" || payload["doc_hash"] == "" {
		t.Fatalf("expected index mode with doc_hash, got %#v", payload)
	}
	if _, ok := payload["content"]; ok {
		t.Fatalf("index mode must not return full content: %#v", payload)
	}
	if payload["truncated"] == true || payload["index_truncated"] == true {
		t.Fatalf("short block index should not truncate, got %#v", payload)
	}
	blocks, ok := payload["blocks"].([]any)
	if !ok || len(blocks) < 4 {
		t.Fatalf("expected indexed blocks, got %#v", payload["blocks"])
	}
	first := blocks[0].(map[string]any)
	if first["block_id"] != "b0001" || first["tag"] != "h1" || first["has_styles"] != true {
		t.Fatalf("unexpected first block: %#v", first)
	}
	foundImageBlock := false
	foundTableBlock := false
	for _, raw := range blocks {
		block := raw.(map[string]any)
		foundImageBlock = foundImageBlock || block["has_images"] == true
		foundTableBlock = foundTableBlock || block["has_tables"] == true
	}
	if !foundImageBlock || !foundTableBlock {
		t.Fatalf("expected image/table metadata in blocks: %#v", blocks)
	}
}

func TestReadWorkspaceNoteViewBlockAndSectionRaw(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	fragment := `<h1>Main</h1><p style="color:#ff0000">Alpha</p><h2>Child</h2><p>Beta</p>`
	if _, err := env.files.CreateFile(ctx, "Note.html", fragment); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}

	indexJSON, _, _, _, err := env.service.executeTool(ctx, "read_workspace_note_view", `{"path":"Note.html","mode":"index"}`, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("index returned error: %v", err)
	}
	var indexPayload map[string]any
	if err := json.Unmarshal([]byte(indexJSON), &indexPayload); err != nil {
		t.Fatalf("invalid index json: %v", err)
	}
	docHash := indexPayload["doc_hash"].(string)

	blockArgs := fmt.Sprintf(`{"path":"Note.html","mode":"block","view":"raw","block_id":"b0002","doc_hash":%q}`, docHash)
	blockJSON, _, _, _, err := env.service.executeTool(ctx, "read_workspace_note_view", blockArgs, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("block read returned error: %v", err)
	}
	var blockPayload map[string]any
	if err := json.Unmarshal([]byte(blockJSON), &blockPayload); err != nil {
		t.Fatalf("invalid block json: %v", err)
	}
	if blockPayload["mode"] != "block" || blockPayload["block_id"] != "b0002" {
		t.Fatalf("unexpected block payload metadata: %#v", blockPayload)
	}
	content := stringFromAny(blockPayload["content"])
	if !strings.Contains(content, `<p style="color:#ff0000">Alpha</p>`) {
		t.Fatalf("expected raw paragraph HTML, got %q", content)
	}

	sectionArgs := fmt.Sprintf(`{"path":"Note.html","mode":"section","view":"raw","section_id":"s0001","doc_hash":%q}`, docHash)
	sectionJSON, _, _, _, err := env.service.executeTool(ctx, "read_workspace_note_view", sectionArgs, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("section read returned error: %v", err)
	}
	var sectionPayload map[string]any
	if err := json.Unmarshal([]byte(sectionJSON), &sectionPayload); err != nil {
		t.Fatalf("invalid section json: %v", err)
	}
	sectionContent := stringFromAny(sectionPayload["content"])
	for _, want := range []string{"<h1>Main</h1>", "<h2>Child</h2>", "<p>Beta</p>"} {
		if !strings.Contains(sectionContent, want) {
			t.Fatalf("section content missing %q: %q", want, sectionContent)
		}
	}
}

func TestReadWorkspaceNoteViewRejectsStaleDocHash(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	if _, err := env.files.CreateFile(ctx, "Stale.html", `<p>Alpha</p>`); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}
	_, _, _, _, err := env.service.executeTool(ctx, "read_workspace_note_view", `{"path":"Stale.html","mode":"block","view":"raw","block_id":"b0001","doc_hash":"stale"}`, RunOptions{UserID: testUserID})
	if err == nil || !strings.Contains(err.Error(), "doc_hash is stale") {
		t.Fatalf("expected stale doc_hash error, got %v", err)
	}
}

func TestBudgetToolResultForModelGivesHTMLBlockContinuationHint(t *testing.T) {
	raw := mustJSON(map[string]any{
		"tool":      "read_workspace_note_view",
		"tool_name": "read_workspace_note_view",
		"path":      "Long.html",
		"view":      "raw",
		"mode":      "full",
		"content":   strings.Repeat("<p>long html</p>", 600),
	})
	budgeted := budgetToolResultForModel("read_workspace_note_view", raw, providerRuntimeProfile{
		MaxToolResultRunes:      1200,
		MaxReadFileContentRunes: 6000,
	})
	var payload map[string]any
	if err := json.Unmarshal([]byte(budgeted), &payload); err != nil {
		t.Fatalf("budgeted payload is not json: %v\n%s", err, budgeted)
	}
	if payload["model_view_truncated"] != true {
		t.Fatalf("expected model_view_truncated, got %#v", payload)
	}
	nextAction := stringFromAny(payload["next_action"])
	if !strings.Contains(nextAction, "mode=index") || strings.Contains(nextAction, "line range") {
		t.Fatalf("expected HTML block continuation hint, got %q", nextAction)
	}
}

func TestProviderFailureClassClassifiesTPMRateLimit(t *testing.T) {
	err := &ProviderResponseError{
		StatusCode:      429,
		Message:         "The request exceeds the rate limit on dimension: tpm.",
		ResponseSnippet: `{"code":"429001"}`,
	}
	if got := providerFailureClass(err, ProviderRequestSummary{}); got != "provider_tpm_rate_limited" {
		t.Fatalf("expected provider_tpm_rate_limited, got %q", got)
	}
}

func TestWorkspacePromptDirectsHTMLTruncationToBlockReads(t *testing.T) {
	for _, want := range []string{"mode=index", "mode=block", "mode=section", "do not tell the user the file was not returned"} {
		if !strings.Contains(workspaceToolSystemPrompt, want) {
			t.Fatalf("workspace prompt missing %q", want)
		}
	}
}
