package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func newAgentItemTestStore(t *testing.T) (*ConversationStore, context.Context) {
	t.Helper()
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "agent-items.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	conversations := NewConversationStore(db)
	if err := conversations.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	return conversations, ctx
}

func TestEnsureAgentTurnItemsMigratesLegacyHistoryIdempotently(t *testing.T) {
	conversations, ctx := newAgentItemTestStore(t)
	conversation, err := conversations.CreateConversation(ctx, CreateConversationInput{UserID: "user-a", SelectedModelProviderID: "model"})
	if err != nil {
		t.Fatal(err)
	}
	userMessage, _ := conversations.AppendMessage(ctx, "user-a", conversation.ID, "user", "hello", `[]`)
	assistantMessage, _ := conversations.AppendMessage(ctx, "user-a", conversation.ID, "assistant", "world", `[]`)
	if err := conversations.EnsureAgentTurnItems(ctx, "user-a", conversation.ID); err != nil {
		t.Fatal(err)
	}
	if err := conversations.EnsureAgentTurnItems(ctx, "user-a", conversation.ID); err != nil {
		t.Fatal(err)
	}
	items, err := conversations.ListAgentTurnItems(ctx, "user-a", conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].SourceID != userMessage.ID || items[1].SourceID != assistantMessage.ID {
		t.Fatalf("unexpected idempotent migration: %#v", items)
	}
}

func TestRepairDanglingAgentToolCallsPersistsAbortedResult(t *testing.T) {
	conversations, ctx := newAgentItemTestStore(t)
	conversation, _ := conversations.CreateConversation(ctx, CreateConversationInput{UserID: "user-a", SelectedModelProviderID: "model"})
	if err := conversations.EnsureAgentTurnItems(ctx, "user-a", conversation.ID); err != nil {
		t.Fatal(err)
	}
	_, err := conversations.AppendAgentTurnItem(ctx, AgentTurnItemInput{
		UserID: "user-a", ConversationID: conversation.ID, TurnID: "turn-1", Kind: "tool_call",
		Role: "assistant", Status: "started", Visibility: AgentItemVisibilityModel, SourceID: "call-item",
		PayloadJSON: `{"tool_call_id":"call-1","tool_name":"read_workspace_text","arguments_json":"{}"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	repaired, err := conversations.RepairDanglingAgentToolCalls(ctx, "user-a", conversation.ID)
	if err != nil || repaired != 1 {
		t.Fatalf("repair failed: count=%d err=%v", repaired, err)
	}
	repaired, err = conversations.RepairDanglingAgentToolCalls(ctx, "user-a", conversation.ID)
	if err != nil || repaired != 0 {
		t.Fatalf("repair must be idempotent: count=%d err=%v", repaired, err)
	}
	items, _ := conversations.ListAgentTurnItems(ctx, "user-a", conversation.ID)
	if len(items) != 2 || items[1].Kind != "aborted" || items[1].Status != "aborted" {
		t.Fatalf("unexpected repaired history: %#v", items)
	}
}

func TestAppendAgentTurnItemReturnsPersistedSequenceAndExistingSource(t *testing.T) {
	conversations, ctx := newAgentItemTestStore(t)
	conversation, _ := conversations.CreateConversation(ctx, CreateConversationInput{UserID: "user-a", SelectedModelProviderID: "model"})
	if err := conversations.EnsureAgentTurnItems(ctx, "user-a", conversation.ID); err != nil {
		t.Fatal(err)
	}
	input := AgentTurnItemInput{
		UserID: "user-a", ConversationID: conversation.ID, TurnID: "turn-1",
		Kind: "user_message", Role: "user", Visibility: AgentItemVisibilityModel,
		SourceID: "stable-source", PayloadJSON: `{"content":"hello"}`,
	}
	first, err := conversations.AppendAgentTurnItem(ctx, input)
	if err != nil || first.Seq <= 0 {
		t.Fatalf("append did not return the persisted row: item=%#v err=%v", first, err)
	}
	input.ID = "different-generated-id"
	second, err := conversations.AppendAgentTurnItem(ctx, input)
	if err != nil || second.ID != first.ID || second.Seq != first.Seq {
		t.Fatalf("idempotent source did not return the existing row: first=%#v second=%#v err=%v", first, second, err)
	}
}

func TestAppendAgentTurnItemRejectsDifferentContentForSameSource(t *testing.T) {
	conversations, ctx := newAgentItemTestStore(t)
	conversation, _ := conversations.CreateConversation(ctx, CreateConversationInput{UserID: "user-a", SelectedModelProviderID: "model"})
	if err := conversations.EnsureAgentTurnItems(ctx, "user-a", conversation.ID); err != nil {
		t.Fatal(err)
	}
	input := AgentTurnItemInput{
		UserID: "user-a", ConversationID: conversation.ID, RunID: "run-1", TurnID: "turn-1",
		Kind: "assistant_message", Role: "assistant", Status: "completed", Visibility: AgentItemVisibilityModel,
		SourceID: "run-1:final", PayloadJSON: `{"content":"first"}`,
	}
	if _, err := conversations.AppendAgentTurnItem(ctx, input); err != nil {
		t.Fatal(err)
	}
	input.ID = "another-id"
	input.PayloadJSON = `{"content":"different"}`
	_, err := conversations.AppendAgentTurnItem(ctx, input)
	var conflict *AgentTurnItemConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected explicit idempotency conflict, got %v", err)
	}
}

func TestAgentFinalItemsRemainUniqueAcrossTwentyTurns(t *testing.T) {
	conversations, ctx := newAgentItemTestStore(t)
	conversation, _ := conversations.CreateConversation(ctx, CreateConversationInput{UserID: "user-a", SelectedModelProviderID: "model"})
	if err := conversations.EnsureAgentTurnItems(ctx, "user-a", conversation.ID); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 20; index++ {
		turnID := fmt.Sprintf("turn-%02d", index)
		runID := fmt.Sprintf("run-%02d", index)
		if _, err := conversations.AppendAgentTurnItem(ctx, AgentTurnItemInput{
			UserID: "user-a", ConversationID: conversation.ID, RunID: runID, TurnID: turnID,
			Kind: "assistant_message", Role: "assistant", Status: "completed", Visibility: AgentItemVisibilityModel,
			SourceID: runID + ":final", PayloadJSON: fmt.Sprintf(`{"content":"answer-%02d"}`, index),
		}); err != nil {
			t.Fatalf("append final %d: %v", index, err)
		}
	}
	items, err := conversations.ListAgentTurnItems(ctx, "user-a", conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 20 {
		t.Fatalf("expected 20 canonical finals, got %d", len(items))
	}
	seenIDs := map[string]bool{}
	for _, item := range items {
		if seenIDs[item.ID] {
			t.Fatalf("duplicate canonical item ID: %s", item.ID)
		}
		seenIDs[item.ID] = true
	}
}

func TestAgentResumeUsesNewRunWithinSameTurn(t *testing.T) {
	conversations, ctx := newAgentItemTestStore(t)
	conversation, _ := conversations.CreateConversation(ctx, CreateConversationInput{UserID: "user-a", SelectedModelProviderID: "model"})
	if err := conversations.EnsureAgentTurnItems(ctx, "user-a", conversation.ID); err != nil {
		t.Fatal(err)
	}
	for _, runID := range []string{"run-before-interrupt", "run-after-resume"} {
		if _, err := conversations.AppendAgentTurnItem(ctx, AgentTurnItemInput{
			UserID: "user-a", ConversationID: conversation.ID, RunID: runID, TurnID: "turn-stable",
			Kind: "workspace_observation", Role: "system", Status: "completed", Visibility: AgentItemVisibilityModel,
			SourceID: runID + ":observation", PayloadJSON: fmt.Sprintf(`{"content":"%s"}`, runID),
		}); err != nil {
			t.Fatal(err)
		}
	}
	items, _ := conversations.ListAgentTurnItems(ctx, "user-a", conversation.ID)
	if len(items) != 2 || items[0].TurnID != items[1].TurnID || items[0].RunID == items[1].RunID {
		t.Fatalf("resume did not preserve turn while changing run: %#v", items)
	}
}

func TestRepairAgentTurnItemHistoryReconcilesLegacyFinalMessageID(t *testing.T) {
	conversations, ctx := newAgentItemTestStore(t)
	conversation, _ := conversations.CreateConversation(ctx, CreateConversationInput{UserID: "user-a", SelectedModelProviderID: "model"})
	_, _ = conversations.AppendMessage(ctx, "user-a", conversation.ID, "user", "first question", `[]`)
	_, _ = conversations.AppendMessage(ctx, "user-a", conversation.ID, "assistant", "first answer", `[]`)
	if err := conversations.EnsureAgentTurnItems(ctx, "user-a", conversation.ID); err != nil {
		t.Fatal(err)
	}
	secondUser, _ := conversations.AppendMessage(ctx, "user-a", conversation.ID, "user", "second question", `[]`)
	secondAssistant, _ := conversations.AppendMessageWithMeta(ctx, MessageInput{
		UserID: "user-a", ConversationID: conversation.ID, Role: "assistant", Content: "second answer",
		AttachmentsJSON: `[]`, Status: "final", MetaJSON: `{"run_id":"run-2"}`,
	})
	if _, err := conversations.AppendAgentTurnItem(ctx, AgentTurnItemInput{
		ID: "final_message:1:1", UserID: "user-a", ConversationID: conversation.ID,
		RunID: "run-2", TurnID: "run-2", Kind: "assistant_message", Role: "assistant",
		Status: "completed", Visibility: AgentItemVisibilityModel, SourceID: "final_message:1:1",
		PayloadJSON: `{"content":"second answer"}`, CreatedAt: secondAssistant.CreatedAt,
	}); err != nil {
		t.Fatal(err)
	}
	report, err := conversations.RepairAgentTurnItemHistory(ctx, "user-a", conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Reconciled != 1 || report.Inserted != 1 {
		// The second user message is also missing from canonical history.
		t.Fatalf("unexpected repair report: %#v", report)
	}
	items, _ := conversations.ListAgentTurnItems(ctx, "user-a", conversation.ID)
	found := false
	for _, item := range items {
		if item.Kind == "assistant_message" && item.SourceID == secondAssistant.ID {
			found = item.TurnID == secondUser.ID && item.RunID == "run-2"
		}
	}
	if !found {
		t.Fatalf("legacy final message was not reconciled to the real UI message: %#v", items)
	}
}

func TestRepairAgentTurnItemHistoryRestoresEightVisibleFinals(t *testing.T) {
	conversations, ctx := newAgentItemTestStore(t)
	conversation, _ := conversations.CreateConversation(ctx, CreateConversationInput{UserID: "user-a", SelectedModelProviderID: "model"})
	_, _ = conversations.AppendMessage(ctx, "user-a", conversation.ID, "user", "question-1", `[]`)
	_, _ = conversations.AppendMessage(ctx, "user-a", conversation.ID, "assistant", "answer-1", `[]`)
	if err := conversations.EnsureAgentTurnItems(ctx, "user-a", conversation.ID); err != nil {
		t.Fatal(err)
	}
	var secondAssistant Message
	for index := 2; index <= 8; index++ {
		_, _ = conversations.AppendMessage(ctx, "user-a", conversation.ID, "user", fmt.Sprintf("question-%d", index), `[]`)
		assistant, err := conversations.AppendMessageWithMeta(ctx, MessageInput{
			UserID: "user-a", ConversationID: conversation.ID, Role: "assistant",
			Content: fmt.Sprintf("answer-%d", index), AttachmentsJSON: `[]`, Status: "final",
			MetaJSON: fmt.Sprintf(`{"run_id":"run-%d"}`, index),
		})
		if err != nil {
			t.Fatal(err)
		}
		if index == 2 {
			secondAssistant = assistant
		}
	}
	if _, err := conversations.AppendAgentTurnItem(ctx, AgentTurnItemInput{
		ID: "final_message:1:1", UserID: "user-a", ConversationID: conversation.ID,
		RunID: "run-2", TurnID: "run-2", Kind: "assistant_message", Role: "assistant",
		Status: "completed", Visibility: AgentItemVisibilityModel, SourceID: "final_message:1:1",
		PayloadJSON: `{"content":"answer-2"}`, CreatedAt: secondAssistant.CreatedAt,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := conversations.RepairAgentTurnItemHistory(ctx, "user-a", conversation.ID); err != nil {
		t.Fatal(err)
	}
	items, err := conversations.ListAgentTurnItems(ctx, "user-a", conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	assistantCount := 0
	sources := map[string]bool{}
	for _, item := range items {
		if item.Kind != "assistant_message" {
			continue
		}
		assistantCount++
		sources[item.SourceID] = true
	}
	if assistantCount != 8 || len(sources) != 8 {
		t.Fatalf("expected 8 model-visible assistant finals, got count=%d sources=%d items=%#v", assistantCount, len(sources), items)
	}
}

func TestDeleteMessagesAfterTruncatesCanonicalItemsAtSameBoundary(t *testing.T) {
	conversations, ctx := newAgentItemTestStore(t)
	conversation, _ := conversations.CreateConversation(ctx, CreateConversationInput{UserID: "user-a", SelectedModelProviderID: "model"})
	userMessage, _ := conversations.AppendMessage(ctx, "user-a", conversation.ID, "user", "question", `[]`)
	_, _ = conversations.AppendMessage(ctx, "user-a", conversation.ID, "assistant", "old reply", `[]`)
	if err := conversations.EnsureAgentTurnItems(ctx, "user-a", conversation.ID); err != nil {
		t.Fatal(err)
	}
	_, _ = conversations.AppendAgentTurnItem(ctx, AgentTurnItemInput{
		UserID: "user-a", ConversationID: conversation.ID, TurnID: userMessage.ID, Kind: "workspace_observation",
		Role: "system", Status: "completed", Visibility: AgentItemVisibilityModel, SourceID: "late-observation",
		PayloadJSON: `{"path":"v14.7.html"}`,
	})
	if err := conversations.DeleteMessagesAfter(ctx, "user-a", conversation.ID, userMessage.ID); err != nil {
		t.Fatal(err)
	}
	messages, _ := conversations.ListMessages(ctx, "user-a", conversation.ID)
	items, _ := conversations.ListAgentTurnItems(ctx, "user-a", conversation.ID)
	if len(messages) != 1 || len(items) != 1 || items[0].SourceID != userMessage.ID {
		t.Fatalf("UI and canonical histories diverged: messages=%#v items=%#v", messages, items)
	}
}

func TestBranchConversationCopiesCanonicalItemPrefix(t *testing.T) {
	conversations, ctx := newAgentItemTestStore(t)
	source, _ := conversations.CreateConversation(ctx, CreateConversationInput{UserID: "user-a", Title: "source", SelectedModelProviderID: "model"})
	userMessage, _ := conversations.AppendMessage(ctx, "user-a", source.ID, "user", "question", `[]`)
	assistantMessage, _ := conversations.AppendMessage(ctx, "user-a", source.ID, "assistant", "answer", `[]`)
	_, _ = conversations.AppendMessage(ctx, "user-a", source.ID, "user", "later", `[]`)
	if err := conversations.EnsureAgentTurnItems(ctx, "user-a", source.ID); err != nil {
		t.Fatal(err)
	}
	branch, branchMessages, err := conversations.BranchConversation(ctx, "user-a", source.ID, assistantMessage.ID)
	if err != nil {
		t.Fatal(err)
	}
	items, err := conversations.ListAgentTurnItems(ctx, "user-a", branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(branchMessages) != 2 || len(items) != 2 {
		t.Fatalf("branch histories differ: messages=%#v items=%#v", branchMessages, items)
	}
	if items[0].SourceID != branchMessages[0].ID || items[0].TurnID != branchMessages[0].ID || items[1].SourceID != branchMessages[1].ID {
		t.Fatalf("branch message IDs were not remapped: source_user=%s messages=%#v items=%#v", userMessage.ID, branchMessages, items)
	}
}

func TestCompactionItemPreservesRawRows(t *testing.T) {
	conversations, ctx := newAgentItemTestStore(t)
	conversation, _ := conversations.CreateConversation(ctx, CreateConversationInput{UserID: "user-a", SelectedModelProviderID: "model"})
	if err := conversations.EnsureAgentTurnItems(ctx, "user-a", conversation.ID); err != nil {
		t.Fatal(err)
	}
	for _, input := range []AgentTurnItemInput{
		{UserID: "user-a", ConversationID: conversation.ID, TurnID: "turn-1", Kind: "user_message", Role: "user", Visibility: AgentItemVisibilityModel, SourceID: "u1", PayloadJSON: `{"content":"old"}`},
		{UserID: "user-a", ConversationID: conversation.ID, TurnID: "turn-1", Kind: "assistant_message", Role: "assistant", Visibility: AgentItemVisibilityModel, SourceID: "a1", PayloadJSON: `{"content":"old reply"}`},
		{UserID: "user-a", ConversationID: conversation.ID, TurnID: "turn-2", Kind: "compaction", Role: "system", Visibility: AgentItemVisibilityModel, SourceID: "compact", PayloadJSON: `{"summary":"memory","replaces_through_seq":2}`},
	} {
		if _, err := conversations.AppendAgentTurnItem(ctx, input); err != nil {
			t.Fatal(err)
		}
	}
	items, _ := conversations.ListAgentTurnItems(ctx, "user-a", conversation.ID)
	if len(items) != 3 || items[0].PayloadJSON != `{"content":"old"}` || items[2].Kind != "compaction" {
		t.Fatalf("compaction destroyed raw history: %#v", items)
	}
}
