package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateNoteBlockEditObservationRequiresEveryTargetRead(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	path := "Note.html"
	if _, err := env.files.CreateFile(ctx, path, `<h1>Title</h1><p>First</p><p>Second</p>`); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}

	read, err := env.files.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	doc := buildNoteBlockDocument(read.Content)
	if len(doc.Blocks) != 3 {
		t.Fatalf("expected three blocks, got %#v", doc.Blocks)
	}

	runtime := newTestWorkspaceAgentRuntime(env.service, "conversation", "message")
	editCall := agentToolCall{
		ID:   "edit-1",
		Name: "edit_workspace_note_blocks",
		ArgumentsJSON: mustJSON(map[string]any{
			"path":             path,
			"document_version": doc.DocHash,
			"edits": []map[string]any{
				{"block_id": doc.Blocks[1].ID, "operation": "replace_inner_html", "html": "First updated"},
				{"block_id": doc.Blocks[2].ID, "operation": "replace_inner_html", "html": "Second updated"},
			},
		}),
	}

	err = runtime.validateHTMLNoteMutationPrecondition(ctx, editCall)
	assertToolErrorCode(t, err, ToolErrorNoteEditObservation)
	assertMissingBlockIDs(t, err, doc.Blocks[1].ID, doc.Blocks[2].ID)

	firstReadCall := agentToolCall{
		ID:   "read-first",
		Name: "read_workspace_note_view",
		ArgumentsJSON: mustJSON(map[string]any{
			"path":             path,
			"mode":             "block",
			"view":             "raw",
			"block_id":         doc.Blocks[1].ID,
			"document_version": doc.DocHash,
		}),
	}
	firstResult, _, _, _, err := env.service.executeTool(ctx, firstReadCall.Name, firstReadCall.ArgumentsJSON, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("first block read returned error: %v", err)
	}
	runtime.recordKernelToolProgress(firstReadCall, false, ToolEvent{ToolName: firstReadCall.Name, Status: "success", Path: path}, firstResult)

	err = runtime.validateHTMLNoteMutationPrecondition(ctx, editCall)
	assertToolErrorCode(t, err, ToolErrorNoteEditObservation)
	assertMissingBlockIDs(t, err, doc.Blocks[2].ID)

	batchReadCall := agentToolCall{
		ID:   "read-both",
		Name: "read_workspace_note_view",
		ArgumentsJSON: mustJSON(map[string]any{
			"path":             path,
			"mode":             "block",
			"view":             "raw",
			"block_ids":        []string{doc.Blocks[1].ID, doc.Blocks[2].ID},
			"document_version": doc.DocHash,
		}),
	}
	batchResult, _, _, _, err := env.service.executeTool(ctx, batchReadCall.Name, batchReadCall.ArgumentsJSON, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("batch block read returned error: %v", err)
	}
	runtime.recordKernelToolProgress(batchReadCall, false, ToolEvent{ToolName: batchReadCall.Name, Status: "success", Path: path}, batchResult)

	if err := runtime.validateHTMLNoteMutationPrecondition(ctx, editCall); err != nil {
		t.Fatalf("all current target reads should satisfy the edit precondition: %v", err)
	}
}

func TestRawTruncatedNoteReadDoesNotCreateEditCredential(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	path := "Large.html"
	content := `<h1>Title</h1><p>` + strings.Repeat("content ", 2000) + `</p>`
	if _, err := env.files.CreateFile(ctx, path, content); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}
	read, err := env.files.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	doc := buildNoteBlockDocument(read.Content)
	raw, _, _, _, err := env.service.execReadWorkspaceNoteBlockView(path, read.Content, read.Hash, "raw", "block", doc.Blocks[1].ID, "", doc.DocHash, 0, 0, 16)
	if err == nil {
		t.Fatal("expected oversized raw block to be rejected")
	}
	assertToolErrorCode(t, err, ToolErrorNoteBlockTooLarge)
	if raw != "" {
		t.Fatalf("oversized raw read returned content instead of a credential-free error: %q", raw)
	}
}

func TestBatchNoteReadPropagatesTruncation(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	path := "Batch.html"
	content := `<p>First block with enough text</p><p>Second block with enough text</p>`
	if _, err := env.files.CreateFile(ctx, path, content); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}
	read, err := env.files.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	doc := buildNoteBlockDocument(read.Content)
	raw, _, _, _, err := env.service.execReadWorkspaceNoteBatchView(path, read.Content, read.Hash, "text", "block", []string{doc.Blocks[0].ID, doc.Blocks[1].ID}, nil, doc.DocHash, 4)
	if err != nil {
		t.Fatalf("batch read returned error: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("decode batch payload: %v", err)
	}
	if !boolFromAny(payload["truncated"]) || !boolFromAny(payload["source_truncated"]) {
		t.Fatalf("batch truncation was not propagated: %#v", payload)
	}
	if complete := stringSliceFromAnyLoose(payload["complete_block_ids"]); len(complete) != 0 {
		t.Fatalf("truncated batch must not issue complete block credentials: %#v", complete)
	}
}

func TestNoteEditPreflightRunsBeforePreview(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	path := "Preflight.html"
	if _, err := env.files.CreateFile(ctx, path, `<h1>Title</h1><p>Body</p>`); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}
	read, err := env.files.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	doc := buildNoteBlockDocument(read.Content)
	runtime := newTestWorkspaceAgentRuntime(env.service, "conversation", "message")
	runtime.editPreviews = newEditPreviewManager(env.service, RunOptions{UserID: testUserID, RunID: "run-preflight"})
	readCall := agentToolCall{
		ID: "read-preflight", Name: "read_workspace_note_view",
		ArgumentsJSON: mustJSON(map[string]any{
			"path": path, "mode": "block", "view": "raw", "block_id": doc.Blocks[1].ID,
			"document_version": doc.DocHash,
		}),
	}
	readResult, _, _, _, err := env.service.executeTool(ctx, readCall.Name, readCall.ArgumentsJSON, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("raw read returned error: %v", err)
	}
	runtime.recordKernelToolProgress(readCall, false, ToolEvent{ToolName: readCall.Name, Status: "success", Path: path}, readResult)

	var previewEvents []string
	runtime.editPreviews.opts.Event = func(eventType, _ string, _ map[string]any) { previewEvents = append(previewEvents, eventType) }
	badCall := agentToolCall{
		ID: "edit-preflight", Name: "edit_workspace_note_blocks",
		ArgumentsJSON: mustJSON(map[string]any{
			"path": path, "document_version": doc.DocHash,
			"edits": []map[string]any{{"block_id": doc.Blocks[1].ID, "operation": "replace_html"}},
		}),
	}
	_, isError, _, _, err := runtime.processOneToolCall(ctx, badCall)
	if err != nil || !isError {
		t.Fatalf("invalid edit should be rejected during preflight: error=%v isError=%v", err, isError)
	}
	for _, eventType := range previewEvents {
		if eventType == "workspace_edit_preview_started" {
			t.Fatalf("preflight rejection started a preview: %#v", previewEvents)
		}
	}
}

func assertToolErrorCode(t *testing.T, err error, want ToolErrorCode) *ToolError {
	t.Helper()
	if err == nil {
		t.Fatalf("expected tool error %q", want)
	}
	typed := normalizeToolError(err)
	if typed.Code != want {
		t.Fatalf("expected tool error %q, got %q (%v)", want, typed.Code, err)
	}
	return typed
}

func assertMissingBlockIDs(t *testing.T, err error, want ...string) {
	t.Helper()
	typed := assertToolErrorCode(t, err, ToolErrorNoteEditObservation)
	var details struct {
		Missing []string `json:"missing_block_ids"`
	}
	raw, marshalErr := json.Marshal(typed.Details)
	if marshalErr != nil {
		t.Fatalf("marshal tool error details: %v", marshalErr)
	}
	if unmarshalErr := json.Unmarshal(raw, &details); unmarshalErr != nil {
		t.Fatalf("unmarshal tool error details: %v", unmarshalErr)
	}
	for _, id := range want {
		found := false
		for _, actual := range details.Missing {
			if actual == id {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected missing block %q in %#v", id, details.Missing)
		}
	}
}
