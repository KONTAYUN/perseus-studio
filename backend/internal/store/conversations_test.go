package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestBranchConversationCopiesMessagesThroughSelectedMessage(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()

	store := NewConversationStore(db)
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	conversation, err := store.CreateConversation(ctx, CreateConversationInput{
		UserID:                  "user-a",
		Title:                   "原对话",
		SelectedModelProviderID: "model-1",
	})
	if err != nil {
		t.Fatalf("CreateConversation returned error: %v", err)
	}
	first, _ := store.AppendMessage(ctx, "user-a", conversation.ID, "user", "one", `[]`)
	time.Sleep(time.Millisecond)
	second, _ := store.AppendMessage(ctx, "user-a", conversation.ID, "assistant", "two", `[{"type":"tool"}]`)
	time.Sleep(time.Millisecond)
	_, _ = store.AppendMessage(ctx, "user-a", conversation.ID, "user", "three", `[]`)

	branched, messages, err := store.BranchConversation(ctx, "user-a", conversation.ID, second.ID)
	if err != nil {
		t.Fatalf("BranchConversation returned error: %v", err)
	}
	if branched.ID == conversation.ID {
		t.Fatalf("expected a new conversation id")
	}
	if branched.Title != "原对话 - 分支" {
		t.Fatalf("unexpected branch title: %q", branched.Title)
	}
	if branched.SelectedModelProviderID != "model-1" {
		t.Fatalf("expected selected model copied, got %q", branched.SelectedModelProviderID)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 copied messages, got %d", len(messages))
	}
	if messages[0].ID == first.ID || messages[1].ID == second.ID {
		t.Fatalf("expected copied messages to have new ids: %#v", messages)
	}
	if messages[0].Content != "one" || messages[1].Content != "two" {
		t.Fatalf("unexpected copied message content: %#v", messages)
	}
	if string(messages[1].Attachments) != `[{"type":"tool"}]` {
		t.Fatalf("expected attachments copied, got %s", string(messages[1].Attachments))
	}
}

func TestDeleteMessageIsUserScoped(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()

	store := NewConversationStore(db)
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	conversation, err := store.CreateConversation(ctx, CreateConversationInput{
		UserID:                  "user-a",
		SelectedModelProviderID: "model-1",
	})
	if err != nil {
		t.Fatalf("CreateConversation returned error: %v", err)
	}
	message, err := store.AppendMessage(ctx, "user-a", conversation.ID, "assistant", "reply", `[]`)
	if err != nil {
		t.Fatalf("AppendMessage returned error: %v", err)
	}
	if err := store.DeleteMessage(ctx, "user-b", conversation.ID, message.ID); err == nil {
		t.Fatalf("expected delete by another user to fail")
	}
	if err := store.DeleteMessage(ctx, "user-a", conversation.ID, message.ID); err != nil {
		t.Fatalf("DeleteMessage returned error: %v", err)
	}
	messages, err := store.ListMessages(ctx, "user-a", conversation.ID)
	if err != nil {
		t.Fatalf("ListMessages returned error: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected message deleted, got %#v", messages)
	}
}

func TestDeleteMessagesAfterRemovesAssistantTail(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()

	store := NewConversationStore(db)
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	conversation, err := store.CreateConversation(ctx, CreateConversationInput{
		UserID:                  "user-a",
		SelectedModelProviderID: "model-1",
	})
	if err != nil {
		t.Fatalf("CreateConversation returned error: %v", err)
	}
	userMessage, err := store.AppendMessage(ctx, "user-a", conversation.ID, "user", "one", `[]`)
	if err != nil {
		t.Fatalf("AppendMessage user returned error: %v", err)
	}
	if _, err := store.AppendMessage(ctx, "user-a", conversation.ID, "assistant", "two", `[]`); err != nil {
		t.Fatalf("AppendMessage assistant returned error: %v", err)
	}
	if _, err := store.AppendMessage(ctx, "user-a", conversation.ID, "assistant", "three", `[]`); err != nil {
		t.Fatalf("AppendMessage assistant returned error: %v", err)
	}
	if err := store.DeleteMessagesAfter(ctx, "user-a", conversation.ID, userMessage.ID); err != nil {
		t.Fatalf("DeleteMessagesAfter returned error: %v", err)
	}
	messages, err := store.ListMessages(ctx, "user-a", conversation.ID)
	if err != nil {
		t.Fatalf("ListMessages returned error: %v", err)
	}
	if len(messages) != 1 || messages[0].ID != userMessage.ID {
		t.Fatalf("expected only user message to remain, got %#v", messages)
	}
}

func TestConversationSummaryIsUserScopedAndPersistent(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()

	store := NewConversationStore(db)
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	conversation, err := store.CreateConversation(ctx, CreateConversationInput{
		UserID:                  "user-a",
		SelectedModelProviderID: "model-1",
	})
	if err != nil {
		t.Fatalf("CreateConversation returned error: %v", err)
	}
	if err := store.UpdateConversationSummary(ctx, "user-b", conversation.ID, "bad", "msg-bad"); err != nil {
		t.Fatalf("UpdateConversationSummary for another user should be a no-op without error: %v", err)
	}
	summary, err := store.GetConversationSummary(ctx, "user-a", conversation.ID)
	if err != nil {
		t.Fatalf("GetConversationSummary returned error: %v", err)
	}
	if summary.Summary != "" || summary.UntilMessageID != "" {
		t.Fatalf("summary should not be updated by another user, got %#v", summary)
	}
	if err := store.UpdateConversationSummary(ctx, "user-a", conversation.ID, "用户偏好：稳定优先。", "msg-1"); err != nil {
		t.Fatalf("UpdateConversationSummary returned error: %v", err)
	}
	summary, err = store.GetConversationSummary(ctx, "user-a", conversation.ID)
	if err != nil {
		t.Fatalf("GetConversationSummary returned error: %v", err)
	}
	if summary.Summary != "用户偏好：稳定优先。" || summary.UntilMessageID != "msg-1" {
		t.Fatalf("unexpected summary: %#v", summary)
	}
}

func TestAgentRunResumeClearsTerminalStateAndListsEventsAfterSeq(t *testing.T) {
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
		UserID:          "user-a",
		ConversationID:  "conv-a",
		UserMessageID:   "msg-a",
		ModelProviderID: "mp-a",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	first, err := runs.AppendEvent(ctx, AgentEventInput{
		RunID:          run.ID,
		UserID:         "user-a",
		ConversationID: "conv-a",
		MessageID:      "msg-a",
		Type:           "run_failed",
		Level:          "error",
		Payload:        map[string]any{"error": "boom"},
	})
	if err != nil {
		t.Fatalf("AppendEvent returned error: %v", err)
	}
	failedMessage := "boom"
	if _, err := runs.UpdateRun(ctx, "user-a", run.ID, UpdateAgentRunInput{
		Status:          AgentRunFailed,
		Stage:           "failed",
		ErrorMessage:    &failedMessage,
		DiagnosticLogID: "diag-a",
		SetCompletedAt:  true,
	}); err != nil {
		t.Fatalf("UpdateRun returned error: %v", err)
	}
	resumed, err := runs.ResumeRun(ctx, "user-a", run.ID)
	if err != nil {
		t.Fatalf("ResumeRun returned error: %v", err)
	}
	if resumed.Status != AgentRunQueued || resumed.ErrorMessage.Valid || resumed.DiagnosticLogID.Valid || resumed.CompletedAt.Valid {
		t.Fatalf("unexpected resumed run: %#v", resumed)
	}
	second, err := runs.AppendEvent(ctx, AgentEventInput{
		RunID:          run.ID,
		UserID:         "user-a",
		ConversationID: "conv-a",
		MessageID:      "msg-a",
		Type:           "run_resumed",
		Level:          "info",
		Payload:        map[string]any{"message": "run resumed"},
	})
	if err != nil {
		t.Fatalf("AppendEvent returned error: %v", err)
	}
	items, err := runs.ListEventsAfter(ctx, "user-a", run.ID, first.Seq)
	if err != nil {
		t.Fatalf("ListEventsAfter returned error: %v", err)
	}
	if len(items) != 1 || items[0].ID != second.ID {
		t.Fatalf("expected only resumed event after seq, got %#v", items)
	}
}

func TestListEventsFilteredLimitsTypesWithoutReplayingWholeRun(t *testing.T) {
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
		UserID:          "user-a",
		ConversationID:  "conv-a",
		UserMessageID:   "msg-a",
		ModelProviderID: "mp-a",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	for i := 0; i < 20; i++ {
		if _, err := runs.AppendEvent(ctx, AgentEventInput{
			RunID:          run.ID,
			UserID:         "user-a",
			ConversationID: "conv-a",
			Type:           "assistant_message_delta",
			Level:          "info",
			Payload:        map[string]any{"delta": "x"},
		}); err != nil {
			t.Fatalf("AppendEvent delta returned error: %v", err)
		}
	}
	received, err := runs.AppendEvent(ctx, AgentEventInput{
		RunID:          run.ID,
		UserID:         "user-a",
		ConversationID: "conv-a",
		Type:           "agent_steer_received",
		Level:          "info",
		Payload:        map[string]any{"content": "慢一点"},
	})
	if err != nil {
		t.Fatalf("AppendEvent steer received returned error: %v", err)
	}
	applied, err := runs.AppendEvent(ctx, AgentEventInput{
		RunID:          run.ID,
		UserID:         "user-a",
		ConversationID: "conv-a",
		Type:           "agent_steer_applied",
		Level:          "info",
		Payload:        map[string]any{"content": "慢一点"},
	})
	if err != nil {
		t.Fatalf("AppendEvent steer applied returned error: %v", err)
	}

	items, err := runs.ListEventsFiltered(ctx, "user-a", run.ID, AgentEventFilter{
		Types: []string{"agent_steer_received", "agent_steer_applied"},
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListEventsFiltered returned error: %v", err)
	}
	if len(items) != 2 || items[0].ID != received.ID || items[1].ID != applied.ID {
		t.Fatalf("expected only steer events in seq order, got %#v", items)
	}
}

func TestLatestConversationEventByTypeIsUserScoped(t *testing.T) {
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
	runA, err := runs.CreateRun(ctx, CreateAgentRunInput{
		UserID:          "user-a",
		ConversationID:  "conv-a",
		UserMessageID:   "msg-a",
		ModelProviderID: "mp-a",
	})
	if err != nil {
		t.Fatalf("CreateRun user-a returned error: %v", err)
	}
	runB, err := runs.CreateRun(ctx, CreateAgentRunInput{
		UserID:          "user-b",
		ConversationID:  "conv-a",
		UserMessageID:   "msg-b",
		ModelProviderID: "mp-b",
	})
	if err != nil {
		t.Fatalf("CreateRun user-b returned error: %v", err)
	}
	if _, err := runs.AppendEvent(ctx, AgentEventInput{
		RunID:          runA.ID,
		UserID:         "user-a",
		ConversationID: "conv-a",
		Type:           "model_request_summary",
		Level:          "info",
		Payload:        map[string]any{"estimated_tokens": 100},
	}); err != nil {
		t.Fatalf("AppendEvent first summary returned error: %v", err)
	}
	time.Sleep(time.Millisecond)
	latest, err := runs.AppendEvent(ctx, AgentEventInput{
		RunID:          runA.ID,
		UserID:         "user-a",
		ConversationID: "conv-a",
		Type:           "model_request_summary",
		Level:          "info",
		Payload:        map[string]any{"estimated_tokens": 200},
	})
	if err != nil {
		t.Fatalf("AppendEvent latest summary returned error: %v", err)
	}
	time.Sleep(time.Millisecond)
	if _, err := runs.AppendEvent(ctx, AgentEventInput{
		RunID:          runB.ID,
		UserID:         "user-b",
		ConversationID: "conv-a",
		Type:           "model_request_summary",
		Level:          "info",
		Payload:        map[string]any{"estimated_tokens": 999},
	}); err != nil {
		t.Fatalf("AppendEvent other user summary returned error: %v", err)
	}

	got, err := runs.LatestConversationEventByType(ctx, "user-a", "conv-a", "model_request_summary")
	if err != nil {
		t.Fatalf("LatestConversationEventByType returned error: %v", err)
	}
	if got.ID != latest.ID {
		t.Fatalf("expected latest user-a event %s, got %#v", latest.ID, got)
	}
}

func TestLatestConversationRunIsUserScoped(t *testing.T) {
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
	first, err := runs.CreateRun(ctx, CreateAgentRunInput{
		UserID:          "user-a",
		ConversationID:  "conv-a",
		UserMessageID:   "msg-a",
		ModelProviderID: "mp-a",
	})
	if err != nil {
		t.Fatalf("CreateRun first returned error: %v", err)
	}
	time.Sleep(time.Millisecond)
	latest, err := runs.CreateRun(ctx, CreateAgentRunInput{
		UserID:          "user-a",
		ConversationID:  "conv-a",
		UserMessageID:   "msg-b",
		ModelProviderID: "mp-b",
	})
	if err != nil {
		t.Fatalf("CreateRun latest returned error: %v", err)
	}
	time.Sleep(time.Millisecond)
	if _, err := runs.CreateRun(ctx, CreateAgentRunInput{
		UserID:          "user-b",
		ConversationID:  "conv-a",
		UserMessageID:   "msg-other",
		ModelProviderID: "mp-other",
	}); err != nil {
		t.Fatalf("CreateRun other user returned error: %v", err)
	}

	got, err := runs.LatestConversationRun(ctx, "user-a", "conv-a")
	if err != nil {
		t.Fatalf("LatestConversationRun returned error: %v", err)
	}
	if got.ID != latest.ID || got.ID == first.ID {
		t.Fatalf("expected latest user-a run %s, got %#v", latest.ID, got)
	}
}

func TestAgentRunCheckpointLifecycle(t *testing.T) {
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
		UserID:          "user-a",
		ConversationID:  "conv-a",
		UserMessageID:   "msg-a",
		ModelProviderID: "mp-a",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	first, err := runs.SaveCheckpoint(ctx, "user-a", run.ID, `{"round":1}`)
	if err != nil {
		t.Fatalf("SaveCheckpoint first returned error: %v", err)
	}
	second, err := runs.SaveCheckpoint(ctx, "user-a", run.ID, `{"round":2}`)
	if err != nil {
		t.Fatalf("SaveCheckpoint second returned error: %v", err)
	}
	if second.Seq <= first.Seq {
		t.Fatalf("expected checkpoint seq to increase, first=%d second=%d", first.Seq, second.Seq)
	}
	latest, err := runs.LatestCheckpoint(ctx, "user-a", run.ID)
	if err != nil {
		t.Fatalf("LatestCheckpoint returned error: %v", err)
	}
	if latest.ID != second.ID || latest.StateJSON != `{"round":2}` {
		t.Fatalf("unexpected latest checkpoint: %#v", latest)
	}
	if err := runs.DeleteCheckpoints(ctx, "user-a", run.ID); err != nil {
		t.Fatalf("DeleteCheckpoints returned error: %v", err)
	}
	if _, err := runs.LatestCheckpoint(ctx, "user-a", run.ID); err == nil {
		t.Fatal("expected LatestCheckpoint to fail after delete")
	}
}
