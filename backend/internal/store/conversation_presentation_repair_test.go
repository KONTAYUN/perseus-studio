package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestRepairMessagePresentationEventsKeepsOnlyOriginalQuestion(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "message-presentation.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewConversationStore(db)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	conversation, err := store.CreateConversation(ctx, CreateConversationInput{UserID: "user-a", SelectedModelProviderID: "provider-a"})
	if err != nil {
		t.Fatal(err)
	}
	question := `[{"tool_call_id":"call-1","tool_name":"ask_followup_question","status":"answered","question":"Run it?","options":["Yes"]}]`
	original, err := store.AppendMessageWithMeta(ctx, MessageInput{
		UserID: "user-a", ConversationID: conversation.ID, Role: "assistant", Content: "Run it?",
		AttachmentsJSON: question, Status: "final", MetaJSON: `{"kind":"assistant_segment","run_id":"run-1"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	completion, err := store.AppendMessageWithMeta(ctx, MessageInput{
		UserID: "user-a", ConversationID: conversation.ID, Role: "assistant", Content: "The check is complete.",
		AttachmentsJSON: question, Status: "final", MetaJSON: `{"kind":"followup_question","run_id":"run-1"}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	report, err := store.RepairMessagePresentationEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.RemovedDuplicates != 1 || report.UpdatedMessages != 2 {
		t.Fatalf("unexpected repair report: %#v", report)
	}
	storedOriginal, err := store.GetMessage(ctx, "user-a", original.ID)
	if err != nil {
		t.Fatal(err)
	}
	storedCompletion, err := store.GetMessage(ctx, "user-a", completion.ID)
	if err != nil {
		t.Fatal(err)
	}
	if messageKind(t, storedOriginal.Meta) != "followup_question" {
		t.Fatalf("original question kind was not repaired: %s", storedOriginal.Meta)
	}
	if messageKind(t, storedCompletion.Meta) != "assistant_segment" {
		t.Fatalf("completion kind was not repaired: %s", storedCompletion.Meta)
	}
	if string(storedCompletion.Attachments) != "[]" || storedCompletion.Content != "The check is complete." {
		t.Fatalf("completion was polluted after repair: %#v", storedCompletion)
	}

	second, err := store.RepairMessagePresentationEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second.UpdatedMessages != 0 || second.RemovedDuplicates != 0 {
		t.Fatalf("repair must be idempotent: %#v", second)
	}
}

func TestRepairMessagePresentationEventsLeavesAmbiguousAttachmentsUntouched(t *testing.T) {
	store, ctx := newAgentItemTestStore(t)
	conversation, err := store.CreateConversation(ctx, CreateConversationInput{UserID: "user-a", SelectedModelProviderID: "provider-a"})
	if err != nil {
		t.Fatal(err)
	}
	message, err := store.AppendMessageWithMeta(ctx, MessageInput{
		UserID: "user-a", ConversationID: conversation.ID, Role: "assistant", Content: "Question",
		AttachmentsJSON: `[{"tool_name":"ask_followup_question","question":"Question","options":["A"]}]`,
		Status:          "final", MetaJSON: `{"kind":"assistant_segment"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := store.RepairMessagePresentationEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.AmbiguousAttachments != 1 || report.UpdatedMessages != 0 {
		t.Fatalf("unexpected ambiguous report: %#v", report)
	}
	stored, err := store.GetMessage(ctx, "user-a", message.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(stored.Attachments) != message.AttachmentsJSON {
		t.Fatalf("ambiguous attachment must remain unchanged: %s", stored.Attachments)
	}
}

func messageKind(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatal(err)
	}
	value, _ := meta["kind"].(string)
	return value
}
