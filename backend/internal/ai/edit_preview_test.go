package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestCompletePartialJSONObjectClosesStringAndContainers(t *testing.T) {
	raw, ok := completePartialJSONObject(`{"path":"draft.html","edits":[{"block_id":"b0001","html":"<p>Live`)
	if !ok {
		t.Fatal("expected partial object to be completable")
	}
	var decoded map[string]any
	if !decodePartialJSONObject(raw, &decoded) {
		t.Fatalf("completed object did not decode: %s", raw)
	}
	if decoded["path"] != "draft.html" {
		t.Fatalf("unexpected decoded path: %#v", decoded)
	}
}

func TestEditPreviewManagerPublishesSafeCreatePreviewWithoutWriting(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	var eventTypes []string
	var payloads []map[string]any
	opts := RunOptions{
		UserID: testUserID,
		RunID:  "run-preview",
		TransientEvent: func(eventType, _ string, payload map[string]any) {
			eventTypes = append(eventTypes, eventType)
			payloads = append(payloads, payload)
		},
		Event: func(eventType, _ string, payload map[string]any) {
			eventTypes = append(eventTypes, eventType)
			payloads = append(payloads, payload)
		},
	}
	manager := newEditPreviewManager(env.service, opts)
	manager.handle(ctx, ToolCallArgumentsDelta{
		Index:         0,
		ToolCallID:    "call-create",
		ToolName:      "create_workspace_note",
		ArgumentsJSON: `{"path":"draft.html","html":"<h1>Live`,
	}, true)

	if len(eventTypes) != 1 || eventTypes[0] != "workspace_edit_preview_started" {
		t.Fatalf("unexpected preview events: %#v", eventTypes)
	}
	if content, _ := payloads[0]["content"].(string); !strings.Contains(content, "Live") {
		t.Fatalf("preview content was not sanitized and published: %#v", payloads[0])
	}
	files, err := env.factory.Workspace(testUserID)
	if err != nil {
		t.Fatalf("Workspace returned error: %v", err)
	}
	if _, err := files.ReadFile("draft.html"); err == nil {
		t.Fatal("preview wrote the staged note to disk")
	}

	manager.complete("call-create", ToolEvent{
		ToolCallID: "call-create", ToolName: "create_workspace_note",
		Status: "success", Path: "draft.html", Hash: "hash-final",
	})
	if len(eventTypes) != 2 || eventTypes[1] != "workspace_edit_preview_completed" {
		t.Fatalf("expected completed preview event, got %#v", eventTypes)
	}
}

func TestAttachEditPreviewManagerConnectsProviderArgumentDeltas(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	providerDeltaCalls := 0
	var eventTypes []string
	opts := RunOptions{
		UserID: testUserID,
		RunID:  "run-provider-preview",
		OnToolCallArgumentsDelta: func(ToolCallArgumentsDelta) {
			providerDeltaCalls++
		},
		TransientEvent: func(eventType, _ string, _ map[string]any) {
			eventTypes = append(eventTypes, eventType)
		},
	}
	runtime := &agentRuntime{opts: opts}
	attachEditPreviewManager(ctx, runtime, env.service, opts)
	runtime.opts.OnToolCallArgumentsDelta(ToolCallArgumentsDelta{
		Index:         0,
		ToolCallID:    "call-provider-create",
		ToolName:      "create_workspace_note",
		ArgumentsJSON: `{"path":"provider-draft.html","html":"<p>streamed preview</p>"}`,
	})

	if providerDeltaCalls != 1 {
		t.Fatalf("previous provider argument hook calls = %d, want 1", providerDeltaCalls)
	}
	if len(eventTypes) != 1 || eventTypes[0] != "workspace_edit_preview_started" {
		t.Fatalf("provider argument delta did not reach preview manager: %#v", eventTypes)
	}
}

func TestEditPreviewManagerRevertsActivePreview(t *testing.T) {
	env := newTestAIEnv(t)
	var eventTypes []string
	manager := newEditPreviewManager(env.service, RunOptions{
		UserID: testUserID,
		RunID:  "run-revert",
		TransientEvent: func(eventType, _ string, _ map[string]any) {
			eventTypes = append(eventTypes, eventType)
		},
		Event: func(eventType, _ string, _ map[string]any) {
			eventTypes = append(eventTypes, eventType)
		},
	})
	manager.handle(context.Background(), ToolCallArgumentsDelta{
		ToolCallID: "call-revert", ToolName: "create_workspace_note",
		ArgumentsJSON: `{"path":"draft.html","html":"<p>candidate</p>"}`,
	}, true)
	manager.revertAll("run_interrupted")
	if len(eventTypes) != 2 || eventTypes[1] != "workspace_edit_preview_reverted" {
		t.Fatalf("expected preview revert lifecycle, got %#v", eventTypes)
	}
}

func TestEditPreviewManagerPublishesBlockChangesWithoutWholeDocument(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	if _, err := env.files.CreateFile(ctx, "note.html", `<h1>Title</h1><p>Old text</p>`); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}
	read, err := env.files.ReadFile("note.html")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	documentVersion := noteDocHash(read.Content)
	var payloads []map[string]any
	manager := newEditPreviewManager(env.service, RunOptions{
		UserID: testUserID,
		RunID:  "run-block-preview",
		TransientEvent: func(_ string, _ string, payload map[string]any) {
			payloads = append(payloads, payload)
		},
		Event: func(_ string, _ string, payload map[string]any) {
			payloads = append(payloads, payload)
		},
	})
	arguments, err := json.Marshal(noteBlockEditArgs{
		Path:            "note.html",
		DocumentVersion: documentVersion,
		Edits: []noteBlockEdit{{
			BlockID: "b0002", Operation: "replace_text",
			Find: "Old text", Replace: "New text",
		}},
	})
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	manager.handle(ctx, ToolCallArgumentsDelta{
		ToolCallID:    "call-block-preview",
		ToolName:      "edit_workspace_note_blocks",
		ArgumentsJSON: string(arguments),
	}, true)

	if len(payloads) != 1 {
		t.Fatalf("expected one preview payload, got %d", len(payloads))
	}
	payload := payloads[0]
	if _, exists := payload["content"]; exists {
		t.Fatalf("block preview must not broadcast whole-document content: %#v", payload)
	}
	if payload["document_version"] != documentVersion {
		t.Fatalf("unexpected document version: %#v", payload)
	}
	changes, ok := payload["changes"].([]map[string]any)
	if !ok || len(changes) != 1 {
		t.Fatalf("expected one block change, got %#v", payload["changes"])
	}
	if changes[0]["block_id"] != "b0002" || changes[0]["operation"] != "replace_text" {
		t.Fatalf("unexpected block change: %#v", changes[0])
	}
}

func TestStableEditPreviewIDUsesRunCallAndPath(t *testing.T) {
	first := stableEditPreviewID("run-1", "call-1", "note.html")
	if first == "" || first != stableEditPreviewID("run-1", "call-1", "note.html") {
		t.Fatalf("preview ID must be stable: %q", first)
	}
	if first == stableEditPreviewID("run-1", "call-2", "note.html") {
		t.Fatal("different tool calls must not reuse a preview ID")
	}
	if first == stableEditPreviewID("run-1", "call-1", "other.html") {
		t.Fatal("different paths must not reuse a preview ID")
	}
}

func TestDiffPreviewChangesReturnsOnlyChangedAndRemovedBlocks(t *testing.T) {
	previous := []map[string]any{
		{"block_id": "b0001", "operation": "replace_text", "replace": "old"},
		{"block_id": "b0002", "operation": "delete"},
	}
	next := []map[string]any{
		{"block_id": "b0001", "operation": "replace_text", "replace": "new"},
		{"block_id": "b0003", "operation": "replace_html", "html": "<p>new</p>"},
	}
	changed, removed := diffPreviewChanges(previous, next)
	if len(changed) != 2 {
		t.Fatalf("expected two changed blocks, got %#v", changed)
	}
	if len(removed) != 1 || removed[0] != "b0002" {
		t.Fatalf("expected b0002 removal, got %#v", removed)
	}
}
