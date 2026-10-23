package runqueue

import (
	"context"
	"errors"
	"testing"
	"time"
)

func receive(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("job did not run")
		return ""
	}
}

func TestSerializesWorkspaceButAllowsOtherUsers(t *testing.T) {
	m := New(2, 4)
	defer m.Shutdown(context.Background())
	started := make(chan string, 4)
	release := make(chan struct{})
	if err := m.Submit("a", "user-a", func(ctx context.Context) {
		started <- "a"
		select {
		case <-release:
		case <-ctx.Done():
		}
	}); err != nil {
		t.Fatal(err)
	}
	if receive(t, started) != "a" {
		t.Fatal("wrong first job")
	}
	_ = m.Submit("b", "user-a", func(context.Context) { started <- "b" })
	_ = m.Submit("c", "user-c", func(context.Context) { started <- "c" })
	if receive(t, started) != "c" {
		t.Fatal("same workspace ran concurrently")
	}
	close(release)
	if receive(t, started) != "b" {
		t.Fatal("queued workspace did not resume")
	}
}

func TestPendingCancellationCapacityAndDrain(t *testing.T) {
	m := New(1, 2)
	started := make(chan string, 1)
	_ = m.Submit("active", "a", func(ctx context.Context) { started <- "active"; <-ctx.Done() })
	receive(t, started)
	cleaned := make(chan string, 1)
	_ = m.Submit("pending", "a", func(context.Context) { t.Error("canceled pending job executed") }, func() { cleaned <- "cleaned" })
	if !errors.Is(m.Submit("full", "b", func(context.Context) {}), ErrFull) {
		t.Fatal("unbounded admission")
	}
	if !m.Cancel("pending") || receive(t, cleaned) != "cleaned" {
		t.Fatal("pending cancellation failed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := m.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(m.Submit("late", "b", func(context.Context) {}), ErrClosed) {
		t.Fatal("admitted during shutdown")
	}
	if state := m.Snapshot(); state.Running != 0 || state.Queued != 0 || state.Accepting {
		t.Fatalf("not drained: %+v", state)
	}
}

func TestDuplicateDoesNotReplaceActiveJob(t *testing.T) {
	m := New(1, 2)
	defer m.Shutdown(context.Background())
	_ = m.Submit("same", "user", func(ctx context.Context) { <-ctx.Done() })
	if !errors.Is(m.Submit("same", "user", func(context.Context) { t.Error("duplicate executed") }), ErrDuplicate) {
		t.Fatal("duplicate accepted")
	}
}
