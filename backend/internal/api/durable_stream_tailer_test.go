package api

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDurableStreamTailerReplaysThroughFinalSequence(t *testing.T) {
	recorder := httptest.NewRecorder()
	tailer := durableStreamTailer{
		ctx:             context.Background(),
		w:               recorder,
		flusher:         recorder,
		wakeups:         make(chan struct{}),
		statePayloadKey: "run",
		stateEventName:  "run_state",
		loadState: func(context.Context) (durableStreamState, error) {
			return durableStreamState{Payload: map[string]any{"status": "succeeded"}, EventHeadSeq: 2, FinalSeq: 2, Terminal: true}, nil
		},
		listAfter: func(_ context.Context, after int64) ([]durableStreamRecord, error) {
			if after != 0 {
				t.Fatalf("unexpected cursor: %d", after)
			}
			return []durableStreamRecord{
				{Seq: 1, EventName: "item_started", Payload: map[string]any{"id": "one"}},
				{Seq: 2, EventName: "turn_completed", Payload: map[string]any{"id": "two"}},
			}, nil
		},
	}

	if err := tailer.serve(durableStreamState{Payload: map[string]any{"status": "succeeded"}, EventHeadSeq: 2, FinalSeq: 2, Terminal: true}); err != nil {
		t.Fatalf("serve: %v", err)
	}
	body := recorder.Body.String()
	for _, expected := range []string{"event: stream_opened", "id: 1", "id: 2", "event: replay_complete", `"through_seq":2`, `"final_seq":2`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("missing %q in stream:\n%s", expected, body)
		}
	}
}

func TestDurableStreamTailerRejectsSequenceGap(t *testing.T) {
	recorder := httptest.NewRecorder()
	tailer := durableStreamTailer{
		ctx:             context.Background(),
		w:               recorder,
		flusher:         recorder,
		wakeups:         make(chan struct{}),
		statePayloadKey: "run",
		stateEventName:  "run_state",
		loadState: func(context.Context) (durableStreamState, error) {
			return durableStreamState{}, nil
		},
		listAfter: func(context.Context, int64) ([]durableStreamRecord, error) {
			return []durableStreamRecord{{Seq: 2, EventName: "event", Payload: map[string]any{}}}, nil
		},
	}

	err := tailer.serve(durableStreamState{EventHeadSeq: 2})
	if err == nil || !strings.Contains(err.Error(), "durable event gap") {
		t.Fatalf("expected sequence gap error, got %v", err)
	}
}
