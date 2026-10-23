package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) handleAgentRunStream(w http.ResponseWriter, r *http.Request, runID string) {
	user := MustUserFromContext(r.Context())
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	// Notifications may be dropped or arrive out of order. SQLite is the
	// authoritative stream; subscribe before loading it to close the setup gap.
	eventsCh, unsubscribe := s.runBroker.Subscribe(runID)
	defer unsubscribe()
	run, err := s.runs.GetRun(r.Context(), user.UserID, runID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	cursor, _ := strconv.ParseInt(r.URL.Query().Get("after_seq"), 10, 64)
	if cursor < 0 {
		cursor = 0
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	if err := s.writeAgentRunEvents(r.Context(), w, user.UserID, runID, &cursor); err != nil {
		return
	}
	writeSSE(w, "run", run)
	flusher.Flush()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		refreshRun := false
		select {
		case <-r.Context().Done():
			return
		case event, open := <-eventsCh:
			if !open {
				return
			}
			refreshRun = event.Type == "run"
		case <-heartbeat.C:
			refreshRun = true
			fmt.Fprint(w, ": heartbeat\n\n")
		}
		if err := s.writeAgentRunEvents(r.Context(), w, user.UserID, runID, &cursor); err != nil {
			return
		}
		if refreshRun {
			latest, err := s.runs.GetRun(r.Context(), user.UserID, runID)
			if err != nil {
				return
			}
			writeSSE(w, "run", latest)
		}
		flusher.Flush()
	}
}

func (s *Server) writeAgentRunEvents(ctx context.Context, w http.ResponseWriter, userID, runID string, cursor *int64) error {
	events, err := s.runs.ListEventsAfter(ctx, userID, runID, *cursor)
	if err != nil {
		return err
	}
	for _, item := range events {
		writeSSE(w, "event", item)
		*cursor = item.Seq
	}
	return nil
}
