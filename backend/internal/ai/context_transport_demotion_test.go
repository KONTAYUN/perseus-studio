package ai

import (
	"strings"
	"testing"

	"webide-backend/internal/store"
)

func TestDemoteConsumedTransportPayloadsKeepsLedgerButShrinksProjection(t *testing.T) {
	largeHTML := strings.Repeat("<p>updated</p>", 600)
	items := []store.AgentTurnItem{
		contextTestItem(1, "turn-1", "tool_call", "assistant", map[string]any{
			"tool_call_id": "edit-1", "tool_name": "edit_workspace_note_blocks",
			"arguments_json": `{"path":"Note.html","doc_hash":"old","edits":[{"block_id":"b0001","operation":"replace_html","html":"` + largeHTML + `"}]}`,
		}),
		contextTestItem(2, "turn-1", "tool_result", "tool", map[string]any{
			"tool_call_id": "edit-1", "tool_name": "edit_workspace_note_blocks",
			"path": "Note.html", "content": `{"status":"success","path":"Note.html","doc_hash":"new"}`,
		}),
	}
	result, count, removed := demoteConsumedTransportPayloads(items)
	if count != 1 || removed <= 0 {
		t.Fatalf("expected one demoted payload, count=%d removed=%d", count, removed)
	}
	if len(contextItemArgumentsJSON(result[0])) >= len(contextItemArgumentsJSON(items[0])) {
		t.Fatal("model projection arguments were not reduced")
	}
	if len(items[0].PayloadJSON) <= len(result[0].PayloadJSON) {
		t.Fatal("test fixture should retain the larger original persisted item")
	}
}

func TestDemoteConsumedTransportPayloadsMarksReadStaleAfterFileChange(t *testing.T) {
	items := []store.AgentTurnItem{
		contextTestItem(1, "turn-1", "tool_call", "assistant", map[string]any{
			"tool_call_id": "read-1", "tool_name": "read_workspace_note_view",
			"arguments_json": `{"path":"Note.html","mode":"block"}`,
		}),
		contextTestItem(2, "turn-1", "tool_result", "tool", map[string]any{
			"tool_call_id": "read-1", "tool_name": "read_workspace_note_view",
			"path": "Note.html", "doc_hash": "old", "content": strings.Repeat("old source ", 1000),
		}),
		contextTestItem(3, "turn-1", "file_change", "system", map[string]any{
			"path": "Note.html", "changed_paths": []string{"Note.html"}, "hash": "new",
		}),
	}
	result, count, removed := demoteConsumedTransportPayloads(items)
	if count != 1 || removed <= 0 {
		t.Fatalf("expected stale read demotion, count=%d removed=%d", count, removed)
	}
	payload := decodeContextItemPayload(result[1])
	if payload["stale_due_to_file_change"] != true {
		t.Fatalf("expected stale marker, got %#v", payload)
	}
}
