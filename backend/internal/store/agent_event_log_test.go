package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestAgentEventLogAllocatesContinuousSequencesAndFinalBarrier(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "event-log.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	runs := NewAgentRunStore(db)
	if err := runs.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	run, err := runs.CreateRun(ctx, CreateAgentRunInput{
		UserID: "user-a", ConversationID: "conv-a", ModelProviderID: "model-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 100; index++ {
		event, appendErr := runs.AppendEvent(ctx, AgentEventInput{
			RunID: run.ID, UserID: "user-a", ConversationID: "conv-a",
			Type: "test", Payload: map[string]any{"index": index},
		})
		if appendErr != nil {
			t.Fatal(appendErr)
		}
		if event.Seq != int64(index) {
			t.Fatalf("seq = %d, want %d", event.Seq, index)
		}
	}
	completed, err := runs.UpdateRun(ctx, "user-a", run.ID, UpdateAgentRunInput{
		Status: AgentRunSucceeded, Stage: "completed", SetCompletedAt: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if completed.EventHeadSeq != 100 || completed.FinalSeq != 100 {
		t.Fatalf("unexpected terminal barrier: head=%d final=%d", completed.EventHeadSeq, completed.FinalSeq)
	}
}

func TestAgentEventIDReplayIsIdempotentAndRejectsDifferentContent(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "event-id.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	runs := NewAgentRunStore(db)
	if err := runs.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	run, err := runs.CreateRun(ctx, CreateAgentRunInput{
		UserID: "user-a", ConversationID: "conv-a", ModelProviderID: "model-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	input := AgentEventInput{
		ID: "provider-event-1", RunID: run.ID, UserID: "user-a",
		ConversationID: "conv-a", Type: "test", Payload: map[string]any{"value": 1},
	}
	first, err := runs.AppendEvent(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := runs.AppendEvent(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ID != first.ID || replayed.Seq != first.Seq {
		t.Fatalf("replay changed event identity: first=%#v replayed=%#v", first, replayed)
	}
	input.Payload = map[string]any{"value": 2}
	if _, err := runs.AppendEvent(ctx, input); err == nil {
		t.Fatal("expected different content for the same event id to be rejected")
	}
}
