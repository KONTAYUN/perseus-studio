package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestCreateRunWithQueueLimitsEnforcesUserAndGlobalCapacity(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "agent-queue.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()

	runs := NewAgentRunStore(db)
	if err := runs.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	create := func(id, userID string) (AgentRun, error) {
		return runs.CreateRunWithQueueLimits(ctx, CreateAgentRunInput{
			ID: id, UserID: userID, ConversationID: "conv-" + id, ModelProviderID: "model-a",
		}, 3, 2)
	}
	if _, err := create("run-a1", "user-a"); err != nil {
		t.Fatalf("create run-a1: %v", err)
	}
	if _, err := create("run-a2", "user-a"); err != nil {
		t.Fatalf("create run-a2: %v", err)
	}
	if _, err := create("run-a3", "user-a"); err == nil {
		t.Fatal("expected per-user queue limit")
	} else {
		var full *AgentQueueFullError
		if !errors.As(err, &full) || full.Scope != "user" || full.Limit != 2 {
			t.Fatalf("unexpected per-user error: %v", err)
		}
	}
	if _, err := create("run-b1", "user-b"); err != nil {
		t.Fatalf("create run-b1: %v", err)
	}
	if _, err := create("run-c1", "user-c"); err == nil {
		t.Fatal("expected global queue limit")
	} else {
		var full *AgentQueueFullError
		if !errors.As(err, &full) || full.Scope != "global" || full.Limit != 3 {
			t.Fatalf("unexpected global error: %v", err)
		}
	}

	global, user, err := runs.QueueCounts(ctx, "user-a")
	if err != nil {
		t.Fatalf("QueueCounts returned error: %v", err)
	}
	if global != 3 || user != 2 {
		t.Fatalf("unexpected queue counts global=%d user=%d", global, user)
	}
}

func TestClaimQueuedRunIsSingleWinnerAndPreservesQueueOrder(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "agent-claim.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()

	runs := NewAgentRunStore(db)
	if err := runs.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	first, err := runs.CreateRun(ctx, CreateAgentRunInput{ID: "run-first", UserID: "user-a", ConversationID: "conv-a", ModelProviderID: "model-a"})
	if err != nil {
		t.Fatalf("CreateRun first: %v", err)
	}
	second, err := runs.CreateRun(ctx, CreateAgentRunInput{ID: "run-second", UserID: "user-b", ConversationID: "conv-b", ModelProviderID: "model-a"})
	if err != nil {
		t.Fatalf("CreateRun second: %v", err)
	}
	position, err := runs.QueuePosition(ctx, second.UserID, second.ID)
	if err != nil {
		t.Fatalf("QueuePosition returned error: %v", err)
	}
	if position != 2 {
		t.Fatalf("expected second run at position 2, got %d", position)
	}

	claimed, ok, err := runs.ClaimQueuedRun(ctx, first.UserID, first.ID)
	if err != nil || !ok {
		t.Fatalf("first claim returned run=%#v ok=%v err=%v", claimed, ok, err)
	}
	if claimed.Status != AgentRunRunning {
		t.Fatalf("expected running status, got %s", claimed.Status)
	}
	if _, ok, err := runs.ClaimQueuedRun(ctx, first.UserID, first.ID); err != nil || ok {
		t.Fatalf("replayed claim should not win: ok=%v err=%v", ok, err)
	}
}
