package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
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
	completed, terminalEvent, err := runs.FinalizeRunWithEvent(ctx, FinalizeAgentRunInput{
		RunID: run.ID, UserID: "user-a", Status: AgentRunSucceeded, Stage: "completed",
		Update: UpdateAgentRunInput{SetCompletedAt: true},
		Event:  AgentEventInput{ID: "terminal-event", RunID: run.ID, UserID: "user-a", ConversationID: "conv-a", Type: "run_completed", Payload: map[string]any{"ok": true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if completed.EventHeadSeq != 101 || completed.FinalSeq != 101 || terminalEvent.Seq != 101 {
		t.Fatalf("unexpected terminal barrier: head=%d final=%d", completed.EventHeadSeq, completed.FinalSeq)
	}
	replayedRun, replayedEvent, err := runs.FinalizeRunWithEvent(ctx, FinalizeAgentRunInput{
		RunID: run.ID, UserID: "user-a", Status: AgentRunSucceeded, Stage: "completed",
		Update: UpdateAgentRunInput{SetCompletedAt: true},
		Event:  AgentEventInput{ID: "terminal-event", RunID: run.ID, UserID: "user-a", ConversationID: "conv-a", Type: "run_completed", Payload: map[string]any{"ok": true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if replayedRun.FinalSeq != 101 || replayedEvent.Seq != 101 {
		t.Fatalf("terminal replay changed barrier: run=%#v event=%#v", replayedRun, replayedEvent)
	}
	if _, err := runs.AppendEvent(ctx, AgentEventInput{
		RunID: run.ID, UserID: "user-a", ConversationID: "conv-a", Type: "late_event",
	}); !errors.Is(err, ErrAgentRunFinalized) {
		t.Fatalf("event appended after final barrier: %v", err)
	}
	sealed, err := runs.GetRun(ctx, "user-a", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sealed.EventHeadSeq != 101 || sealed.FinalSeq != 101 {
		t.Fatalf("late append changed terminal barrier: head=%d final=%d", sealed.EventHeadSeq, sealed.FinalSeq)
	}
}

func TestFinalizeRunWithEventRollsBackEventWhenRunUpdateFails(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "terminal-rollback.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	runs := NewAgentRunStore(db)
	if err := runs.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	run, err := runs.CreateRun(ctx, CreateAgentRunInput{UserID: "user-a", ConversationID: "conv-a", ModelProviderID: "model-a"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = runs.FinalizeRunWithEvent(ctx, FinalizeAgentRunInput{
		RunID: run.ID, UserID: "user-a", Status: AgentRunRunning, Stage: "running",
		Event: AgentEventInput{ID: "not-terminal", Type: "run_started"},
	})
	if err == nil {
		t.Fatal("expected non-terminal finalization to fail")
	}
	events, err := runs.ListEvents(ctx, "user-a", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("failed finalization persisted events: %#v", events)
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
	if _, err := runs.AppendEvent(ctx, input); !errors.Is(err, ErrAgentEventIntegrity) {
		t.Fatalf("expected event integrity error for different content, got %v", err)
	}
}

func TestResumeRunWithEventRollsBackWhenEventInsertConflicts(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "resume-rollback.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	runs := NewAgentRunStore(db)
	if err := runs.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	run, err := runs.CreateRun(ctx, CreateAgentRunInput{UserID: "user-a", ConversationID: "conv-a", ModelProviderID: "model-a"})
	if err != nil {
		t.Fatal(err)
	}
	failed, _, err := runs.FinalizeRunWithEvent(ctx, FinalizeAgentRunInput{
		RunID: run.ID, UserID: "user-a", Status: AgentRunFailed, Stage: "failed",
		Update: UpdateAgentRunInput{SetCompletedAt: true},
		Event:  AgentEventInput{ID: "shared-event-id", Type: "run_failed", Payload: map[string]any{"failed": true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = runs.ResumeRunWithEvent(ctx, ResumeAgentRunInput{
		RunID: run.ID, UserID: "user-a",
		Event: AgentEventInput{ID: "shared-event-id", Type: "run_resumed", Payload: map[string]any{"resumed": true}},
	})
	if !errors.Is(err, ErrAgentEventIntegrity) {
		t.Fatalf("expected event integrity conflict, got %v", err)
	}
	after, err := runs.GetRun(ctx, "user-a", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != AgentRunFailed || after.FinalSeq != failed.FinalSeq || after.EventHeadSeq != failed.EventHeadSeq {
		t.Fatalf("failed resume changed the terminal run: before=%#v after=%#v", failed, after)
	}
}

func TestConcurrentTerminalFinalizersProduceOneBarrier(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "terminal-concurrency.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	runs := NewAgentRunStore(db)
	if err := runs.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	run, err := runs.CreateRun(ctx, CreateAgentRunInput{UserID: "user-a", ConversationID: "conv-a", ModelProviderID: "model-a"})
	if err != nil {
		t.Fatal(err)
	}

	var wait sync.WaitGroup
	results := make(chan error, 16)
	for index := 0; index < 16; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, _, finalizeErr := runs.FinalizeRunWithEvent(ctx, FinalizeAgentRunInput{
				RunID: run.ID, UserID: "user-a", Status: AgentRunSucceeded, Stage: "completed",
				Update: UpdateAgentRunInput{SetCompletedAt: true},
				Event:  AgentEventInput{ID: fmt.Sprintf("terminal-%02d", index), RunID: run.ID, UserID: "user-a", ConversationID: "conv-a", Type: "run_completed"},
			})
			results <- finalizeErr
		}(index)
	}
	wait.Wait()
	close(results)
	successes := 0
	for finalizeErr := range results {
		if finalizeErr == nil {
			successes++
			continue
		}
		if !errors.Is(finalizeErr, ErrAgentRunFinalized) {
			t.Fatalf("unexpected concurrent finalization error: %v", finalizeErr)
		}
	}
	if successes != 1 {
		t.Fatalf("terminal finalization successes = %d, want 1", successes)
	}
	sealed, err := runs.GetRun(ctx, "user-a", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	events, err := runs.ListEvents(ctx, "user-a", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || sealed.EventHeadSeq != 1 || sealed.FinalSeq != 1 {
		t.Fatalf("concurrent finalization created an invalid log: events=%d head=%d final=%d", len(events), sealed.EventHeadSeq, sealed.FinalSeq)
	}
}

func TestRunningRunRecoveryPreservesTerminalEventBarrier(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "running-recovery.db"))
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
	claimed, ok, err := runs.ClaimQueuedRun(ctx, "user-a", run.ID)
	if err != nil || !ok {
		t.Fatalf("claim queued run: ok=%v err=%v", ok, err)
	}
	running, err := runs.ListRunningRuns(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(running) != 1 || running[0].ID != claimed.ID {
		t.Fatalf("running recovery candidates = %#v", running)
	}
	message := "agent run interrupted by service restart"
	failed, terminal, err := runs.FinalizeRunWithEvent(ctx, FinalizeAgentRunInput{
		RunID: run.ID, UserID: "user-a", Status: AgentRunFailed, Stage: "failed",
		Update: UpdateAgentRunInput{ErrorMessage: &message, SetCompletedAt: true},
		Event: AgentEventInput{
			ID: "restart-terminal", RunID: run.ID, UserID: "user-a",
			ConversationID: "conv-a", Type: "run_failed",
			Payload: map[string]any{"failure_class": "service_restart_interrupted"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != AgentRunFailed || failed.EventHeadSeq != terminal.Seq || failed.FinalSeq != terminal.Seq {
		t.Fatalf("invalid recovered terminal barrier: run=%#v event=%#v", failed, terminal)
	}
	remaining, err := runs.ListRunningRuns(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("recovered run remained active: %#v", remaining)
	}
}
