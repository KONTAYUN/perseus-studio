package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

type durableStreamRecord struct {
	Seq       int64
	EventName string
	Payload   any
}

type durableStreamState struct {
	Payload      any
	EventHeadSeq int64
	FinalSeq     int64
	Terminal     bool
}

type durableStreamTailer struct {
	ctx             context.Context
	w               http.ResponseWriter
	flusher         http.Flusher
	wakeups         <-chan struct{}
	cursor          int64
	statePayloadKey string
	stateEventName  string
	heartbeat       time.Duration
	loadState       func(context.Context) (durableStreamState, error)
	listAfter       func(context.Context, int64) ([]durableStreamRecord, error)
	sendTransient   func() error
}

func (t *durableStreamTailer) serve(initial durableStreamState) error {
	t.w.Header().Set("Content-Type", "text/event-stream")
	t.w.Header().Set("Cache-Control", "no-cache")
	t.w.Header().Set("Connection", "keep-alive")
	t.w.Header().Set("X-Accel-Buffering", "no")

	if t.heartbeat <= 0 {
		t.heartbeat = 15 * time.Second
	}
	if t.cursor > initial.EventHeadSeq {
		t.cursor = 0
	}
	if err := writeSSE(t.w, "stream_opened", map[string]any{
		t.statePayloadKey: initial.Payload,
		"event_head_seq":  initial.EventHeadSeq,
		"final_seq":       initial.FinalSeq,
	}); err != nil {
		return err
	}

	state, err := t.drainAndSnapshot()
	if err != nil {
		return err
	}
	if err := t.writeState(state, true); err != nil {
		return err
	}
	if state.Terminal && t.cursor >= state.FinalSeq {
		return nil
	}

	heartbeat := time.NewTicker(t.heartbeat)
	defer heartbeat.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return t.ctx.Err()
		case <-t.wakeups:
			state, err = t.drainAndSnapshot()
			if err != nil {
				return err
			}
			if err := t.writeState(state, true); err != nil {
				return err
			}
			if state.Terminal && t.cursor >= state.FinalSeq {
				return nil
			}
		case <-heartbeat.C:
			state, err = t.drainAndSnapshot()
			if err != nil {
				return err
			}
			if err := writeSSE(t.w, t.stateEventName, state.Payload); err != nil {
				return err
			}
			if err := writeSSEHeartbeat(t.w); err != nil {
				return err
			}
			t.flusher.Flush()
			if state.Terminal && t.cursor >= state.FinalSeq {
				if err := writeSSE(t.w, "replay_complete", map[string]any{
					"through_seq": t.cursor,
					"final_seq":   state.FinalSeq,
				}); err != nil {
					return err
				}
				t.flusher.Flush()
				return nil
			}
		}
	}
}

func (t *durableStreamTailer) drainAndSnapshot() (durableStreamState, error) {
	records, err := t.listAfter(t.ctx, t.cursor)
	if err != nil {
		return durableStreamState{}, err
	}
	for _, record := range records {
		if record.Seq != t.cursor+1 {
			return durableStreamState{}, fmt.Errorf("durable event gap: expected %d, got %d", t.cursor+1, record.Seq)
		}
		if err := writeSSEWithID(t.w, record.EventName, strconv.FormatInt(record.Seq, 10), record.Payload); err != nil {
			return durableStreamState{}, err
		}
		t.cursor = record.Seq
	}
	if t.sendTransient != nil {
		if err := t.sendTransient(); err != nil {
			return durableStreamState{}, err
		}
	}
	return t.loadState(t.ctx)
}

func (t *durableStreamTailer) writeState(state durableStreamState, replayComplete bool) error {
	if err := writeSSE(t.w, t.stateEventName, state.Payload); err != nil {
		return err
	}
	if replayComplete {
		if err := writeSSE(t.w, "replay_complete", map[string]any{
			"through_seq": t.cursor,
			"final_seq":   state.FinalSeq,
		}); err != nil {
			return err
		}
	}
	t.flusher.Flush()
	return nil
}
