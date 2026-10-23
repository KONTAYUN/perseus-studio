package api

import (
	"context"
	"testing"
	"time"

	"webide-backend/internal/runqueue"
)

func TestOptionalTitleSaturationDoesNotStartUntrackedWork(t *testing.T) {
	queue := runqueue.New(1, 1)
	defer queue.Shutdown(t.Context())
	started := make(chan struct{})
	if err := queue.Submit("title:conv", "user", func(ctx context.Context) { close(started); <-ctx.Done() }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("queue did not start")
	}
	s := &Server{titleQueue: queue}
	// With no stores/AI configured these calls would panic if they spawned work
	// outside the bounded queue. Duplicate and saturated work must be optional.
	s.maybeGenerateConversationTitleAsync("user", "admin", "conv", "", "", "", "", "", "run")
	s.maybeGenerateConversationTitleAsync("user", "admin", "other", "", "", "", "", "", "run")
	if state := queue.Snapshot(); state.Running != 1 || state.Queued != 0 {
		t.Fatalf("title admission bypassed: %+v", state)
	}
}
