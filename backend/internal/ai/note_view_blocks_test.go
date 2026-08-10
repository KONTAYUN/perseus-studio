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

func TestReadWorkspaceNoteViewIndexPaginatesBlocks(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	var fragment strings.Builder
	for i := 1; i <= 95; i++ {
		fragment.WriteString(fmt.Sprintf("<p>Block %03d</p>", i))
	}
	if _, err := env.files.CreateFile(ctx, "Paged.html", fragment.String()); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}

	firstJSON, _, _, _, err := env.service.executeTool(ctx, "read_workspace_note_view", `{"path":"Paged.html","mode":"index"}`, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("index page 1 returned error: %v", err)
	}
	var firstPayload map[string]any
	if err := json.Unmarshal([]byte(firstJSON), &firstPayload); err != nil {
		t.Fatalf("invalid index json: %v", err)
	}
	firstBlocks, ok := firstPayload["blocks"].([]any)
	if !ok || len(firstBlocks) != defaultNoteIndexBlocks {
		t.Fatalf("expected default page of %d blocks, got %#v", defaultNoteIndexBlocks, firstPayload["blocks"])
	}
	if firstPayload["has_more_blocks"] != true || intFromAny(firstPayload["next_start_block"]) != defaultNoteIndexBlocks+1 {
		t.Fatalf("expected continuation metadata, got %#v", firstPayload)
	}

	secondJSON, _, _, _, err := env.service.executeTool(ctx, "read_workspace_note_view", fmt.Sprintf(`{"path":"Paged.html","mode":"index","start_block":%d}`, defaultNoteIndexBlocks+1), RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("index page 2 returned error: %v", err)
	}
	var secondPayload map[string]any
	if err := json.Unmarshal([]byte(secondJSON), &secondPayload); err != nil {
		t.Fatalf("invalid second index json: %v", err)
	}
	secondBlocks, ok := secondPayload["blocks"].([]any)
	if !ok || len(secondBlocks) != 15 {
		t.Fatalf("expected final page of 15 blocks, got %#v", secondPayload["blocks"])
	}
	if secondPayload["has_more_blocks"] == true || intFromAny(secondPayload["next_start_block"]) != 0 {
		t.Fatalf("did not expect continuation metadata on final page, got %#v", secondPayload)
	}
	firstSecondBlock := secondBlocks[0].(map[string]any)
	if firstSecondBlock["block_id"] != "b0081" {
		t.Fatalf("expected page 2 to start at b0081, got %#v", firstSecondBlock)
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
	segments, ok := blockPayload["text_segments"].([]any)
	if !ok || len(segments) == 0 {
		t.Fatalf("raw block should expose text segments for precise edits: %#v", blockPayload)
	}
	segment, ok := segments[0].(map[string]any)
	if !ok || stringFromAny(segment["segment_id"]) == "" || stringFromAny(segment["block_hash"]) == "" {
		t.Fatalf("raw text segment is missing stable metadata: %#v", segments[0])
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
	if got := stringSliceFromAnyLoose(sectionPayload["section_ids"]); len(got) != 1 || got[0] != "s0001" {
		t.Fatalf("section raw read should expose section coverage: %#v", sectionPayload["section_ids"])
	}
	if covered := stringSliceFromAnyLoose(sectionPayload["covered_block_ids"]); len(covered) != 4 {
		t.Fatalf("section raw read should expose every covered block: %#v", sectionPayload["covered_block_ids"])
	}
}

func TestReadWorkspaceNoteViewRejectsStaleDocHash(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	if _, err := env.files.CreateFile(ctx, "Stale.html", `<p>Alpha</p>`); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}
	_, _, _, _, err := env.service.executeTool(ctx, "read_workspace_note_view", `{"path":"Stale.html","mode":"block","view":"raw","block_id":"b0001","doc_hash":"stale"}`, RunOptions{UserID: testUserID})
	if err == nil || !strings.Contains(err.Error(), "document_version is stale") {
		t.Fatalf("expected stale document_version error, got %v", err)
	}
}

func TestReadWorkspaceNoteIndexIgnoresStaleDocHash(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	if _, err := env.files.CreateFile(ctx, "FreshIndex.html", `<h1>Current</h1><p>Body</p>`); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}
	raw, _, _, _, err := env.service.executeTool(
		ctx,
		"read_workspace_note_view",
		`{"path":"FreshIndex.html","mode":"index","doc_hash":"stale"}`,
		RunOptions{UserID: testUserID},
	)
	if err != nil {
		t.Fatalf("mode=index should refresh despite stale hash: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("invalid index payload: %v", err)
	}
	if stringFromAny(payload["doc_hash"]) == "" || stringFromAny(payload["doc_hash"]) == "stale" {
		t.Fatalf("expected current doc_hash, got %#v", payload["doc_hash"])
	}
	documentVersion := stringFromAny(payload["document_version"])
	if len(documentVersion) != 64 || documentVersion != stringFromAny(payload["doc_hash"]) {
		t.Fatalf("expected a full document_version, got %#v", payload)
	}
}

func TestProviderFailureClassClassifiesTPMRateLimit(t *testing.T) {
	err := &ProviderResponseError{
		StatusCode:      429,
		Message:         "The request exceeds the rate limit on dimension: tpm.",
		ResponseSnippet: `{"code":"429001"}`,
	}
	if got := providerFailureClass(err, ProviderRequestSummary{}); got != string(ProviderFailureRateLimited) {
		t.Fatalf("expected generic structured rate limit, got %q", got)
	}
	err.ErrorCode = "tpm_limit_exceeded"
	if got := classifyProviderRateLimitKind(err); got != ProviderRateLimitTPM {
		t.Fatalf("expected structured TPM rate-limit kind, got %q", got)
	}
}

func TestWorkspacePromptDirectsHTMLTruncationToBlockReads(t *testing.T) {
	for _, want := range []string{"mode=index", "mode=block", "mode=section", "do not tell the user the file was not returned"} {
		if !strings.Contains(workspaceToolSystemPrompt, want) {
			t.Fatalf("workspace prompt missing %q", want)
		}
	}
}

func TestWorkspacePromptRequiresCompleteMarkdownCompletionResult(t *testing.T) {
	for _, want := range []string{
		"All user-visible replies are Markdown",
		"attempt_completion.result",
		"fenced code blocks",
		"appropriate language tag",
		"degraded plain-text result",
	} {
		if !strings.Contains(workspaceToolSystemPrompt, want) {
			t.Fatalf("workspace prompt missing Markdown completion rule %q", want)
		}
	}
}

func TestAgentKernelNoteViewBlocksCountAsDistinctProgress(t *testing.T) {
	runtime := &agentRuntime{
		kernel:           newAgentKernelState(),
		appliedMutations: map[string]ToolEvent{},
		chunkWrites:      map[string]workspaceChunkWriteState{},
	}
	event := ToolEvent{ToolName: "read_workspace_note_view", Status: "success", Path: "Note.html"}
	callOne := agentToolCall{Name: "read_workspace_note_view", ArgumentsJSON: `{"path":"Note.html","mode":"block","view":"raw","block_id":"b0001","doc_hash":"abc"}`}
	callTwo := agentToolCall{Name: "read_workspace_note_view", ArgumentsJSON: `{"path":"Note.html","mode":"block","view":"raw","block_id":"b0002","doc_hash":"abc"}`}
	if loopGuardActionKey(callOne, false, event, true) == loopGuardActionKey(callTwo, false, event, true) {
		t.Fatal("different note blocks must not share loop-guard action keys")
	}

	runtime.recordLoopGuardAction(callOne, false, event, mustJSON(map[string]any{
		"tool_name": "read_workspace_note_view",
		"path":      "Note.html",
		"mode":      "block",
		"view":      "raw",
		"block_id":  "b0001",
		"doc_hash":  "abc",
		"ledger_id": "note:Note.html:block:raw:abc:b0001",
		"content":   "<p>one</p>",
	}))
	firstFingerprint := runtime.progressFingerprint()
	runtime.recordLoopGuardAction(callTwo, false, event, mustJSON(map[string]any{
		"tool_name": "read_workspace_note_view",
		"path":      "Note.html",
		"mode":      "block",
		"view":      "raw",
		"block_id":  "b0002",
		"doc_hash":  "abc",
		"ledger_id": "note:Note.html:block:raw:abc:b0002",
		"content":   "<p>two</p>",
	}))
	if runtime.loopGuard.RepeatedActionCount != 1 {
		t.Fatalf("different block reads should reset repeated action count, got %d", runtime.loopGuard.RepeatedActionCount)
	}
	if runtime.progressFingerprint() == firstFingerprint {
		t.Fatal("second block read should advance the progress fingerprint")
	}
	if len(runtime.kernel.ReadTargets) != 2 {
		t.Fatalf("expected two note read targets, got %#v", runtime.kernel.ReadTargets)
	}
}

func TestAgentKernelTextRangesCountAsDistinctTargets(t *testing.T) {
	event := ToolEvent{ToolName: "read_workspace_text", Status: "success", Path: "draft.txt"}
	first := agentToolCall{Name: "read_workspace_text", ArgumentsJSON: `{"path":"draft.txt","start_line":1,"end_line":80}`}
	second := agentToolCall{Name: "read_workspace_text", ArgumentsJSON: `{"path":"draft.txt","start_line":81,"end_line":160}`}
	if loopGuardActionKey(first, false, event, true) == loopGuardActionKey(second, false, event, true) {
		t.Fatal("different text ranges must not share loop-guard action keys")
	}

	resultTarget := kernelReadTargetFromToolResult("read_workspace_text", mustJSON(map[string]any{
		"tool_name": "read_workspace_text",
		"path":      "draft.txt",
	}))
	argsTarget := kernelReadTargetFromArguments("read_workspace_text", first.ArgumentsJSON)
	merged := mergeKernelReadTarget(resultTarget, argsTarget)
	if merged.LedgerID != "draft.txt:1-80" {
		t.Fatalf("expected argument line range to fill missing result ledger id, got %#v", merged)
	}
}

func TestAgentRuntimeCheckpointPreservesKernelProgress(t *testing.T) {
	runtime := &agentRuntime{
		transcript:       newAgentTranscript(),
		appliedMutations: map[string]ToolEvent{},
		chunkWrites:      map[string]workspaceChunkWriteState{},
		loadedToolsets:   map[string]bool{"workspace": true},
		kernel:           newAgentKernelState(),
		actionProgress:   ActionProgressLedger{LastProgressRound: 3, LastReadProgressRound: 3},
		loopGuard:        AgentLoopGuard{LastActionKey: "read", RepeatedActionCount: 2, LastProgressFingerprint: "fp"},
	}
	runtime.kernel.ReadTargets["read_workspace_note_view\x00Note.html\x00block\x00raw\x00abc\x00b0001\x00"] = AgentReadTarget{
		ToolName: "read_workspace_note_view",
		Path:     "Note.html",
		Mode:     "block",
		View:     "raw",
		BlockID:  "b0001",
		DocHash:  "abc",
	}
	checkpoint := runtime.checkpoint()
	restored := &agentRuntime{}
	restored.restoreCheckpoint(checkpoint)
	if restored.kernel.Version != agentKernelV2Version || len(restored.kernel.ReadTargets) != 1 {
		t.Fatalf("expected kernel state restored, got %#v", restored.kernel)
	}
	if restored.actionProgress.LastReadProgressRound != 3 || restored.loopGuard.RepeatedActionCount != 2 {
		t.Fatalf("expected progress and loop guard restored, got %#v %#v", restored.actionProgress, restored.loopGuard)
	}
}

func TestCurrentReadWorkingSetKeepsDistinctNoteBlocks(t *testing.T) {
	transcript := newAgentTranscript()
	for _, blockID := range []string{"b0001", "b0002"} {
		callID := "read_" + blockID
		transcript.AppendAssistantTurn(agentModelTurn{ToolCalls: []agentToolCall{{
			ID:            callID,
			Name:          "read_workspace_note_view",
			ArgumentsJSON: fmt.Sprintf(`{"path":"Note.html","mode":"block","view":"raw","block_id":%q,"doc_hash":"abc"}`, blockID),
		}}})
		transcript.AppendToolResults([]TranscriptToolResult{{
			ToolCallID: callID,
			ToolName:   "read_workspace_note_view",
			Content: mustJSON(map[string]any{
				"path":      "Note.html",
				"mode":      "block",
				"view":      "raw",
				"block_id":  blockID,
				"doc_hash":  "abc",
				"ledger_id": "note:Note.html:block:raw:abc:" + blockID,
				"content":   "<p>" + blockID + "</p>",
			}),
			Event: ToolEvent{ToolName: "read_workspace_note_view", Status: "success", Path: "Note.html"},
		}})
	}
	workingSet := currentReadWorkingSetFromTranscript(transcript, 4)
	if len(workingSet.Spans) != 2 {
		t.Fatalf("expected two note block spans, got %#v", workingSet.Spans)
	}
	labels := currentReadWorkingSetLabels(workingSet)
	if !containsContextAction(labels, "Note.html mode=block view=raw block=b0001 doc=abc") ||
		!containsContextAction(labels, "Note.html mode=block view=raw block=b0002 doc=abc") {
		t.Fatalf("expected block labels, got %#v", labels)
	}
}
