package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestAgentEditPreviewUpsertKeepsOnlyLatestRevision(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()

	runs := NewAgentRunStore(db)
	if err := runs.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	run, err := runs.CreateRun(ctx, CreateAgentRunInput{
		UserID: "user-a", ConversationID: "conv-a", ModelProviderID: "model-a",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	input := AgentEditPreviewInput{
		PreviewID: "preview-a", RunID: run.ID, UserID: "user-a",
		ToolCallID: "tool-a", Path: "Note.html", Revision: 1,
		SnapshotJSON: `{"revision":1,"content":"first"}`,
	}
	if err := runs.UpsertEditPreview(ctx, input); err != nil {
		t.Fatalf("UpsertEditPreview returned error: %v", err)
	}
	input.Revision = 2
	input.SnapshotJSON = `{"revision":2,"content":"second"}`
	if err := runs.UpsertEditPreview(ctx, input); err != nil {
		t.Fatalf("UpsertEditPreview revision 2 returned error: %v", err)
	}
	input.Revision = 1
	input.SnapshotJSON = `{"revision":1,"content":"stale"}`
	if err := runs.UpsertEditPreview(ctx, input); err != nil {
		t.Fatalf("stale UpsertEditPreview returned error: %v", err)
	}
	previews, err := runs.ListEditPreviews(ctx, "user-a", run.ID)
	if err != nil {
		t.Fatalf("ListEditPreviews returned error: %v", err)
	}
	if len(previews) != 1 || previews[0].Revision != 2 || previews[0].SnapshotJSON != `{"revision":2,"content":"second"}` {
		t.Fatalf("unexpected previews: %#v", previews)
	}
}

func TestLegacyPreviewCleanupDeletesOnlyTransientPreviewEvents(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()

	runs := NewAgentRunStore(db)
	if err := runs.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	run, err := runs.CreateRun(ctx, CreateAgentRunInput{
		UserID: "user-a", ConversationID: "conv-a", ModelProviderID: "model-a",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM agent_data_migrations WHERE name = ?`, legacyPreviewEventCleanupMigration); err != nil {
		t.Fatalf("reset migration marker: %v", err)
	}
	for _, eventType := range []string{
		"workspace_edit_preview_started",
		"workspace_edit_preview_updated",
		"workspace_edit_preview_completed",
		"tool_call_completed",
	} {
		if _, err := runs.AppendEvent(ctx, AgentEventInput{
			RunID: run.ID, UserID: "user-a", ConversationID: "conv-a",
			Type: eventType, Level: "info", Payload: map[string]any{"type": eventType},
		}); err != nil {
			t.Fatalf("AppendEvent(%s) returned error: %v", eventType, err)
		}
	}
	if err := runs.cleanupLegacyPreviewEvents(ctx); err != nil {
		t.Fatalf("cleanupLegacyPreviewEvents returned error: %v", err)
	}
	if err := runs.cleanupLegacyPreviewEvents(ctx); err != nil {
		t.Fatalf("second cleanupLegacyPreviewEvents returned error: %v", err)
	}
	events, err := runs.ListEventsAfter(ctx, "user-a", run.ID, 0)
	if err != nil {
		t.Fatalf("ListEventsAfter returned error: %v", err)
	}
	if len(events) != 2 || events[0].Type != "workspace_edit_preview_completed" || events[1].Type != "tool_call_completed" {
		t.Fatalf("cleanup removed durable events or kept transient events: %#v", events)
	}
}
