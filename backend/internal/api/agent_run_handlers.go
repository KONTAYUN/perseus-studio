package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"webide-backend/internal/ai"
	"webide-backend/internal/store"
	"webide-backend/internal/tasks"
)

type cancelAgentRunRequest struct {
	Source string `json:"source"`
	Reason string `json:"reason"`
}

func (s *Server) handleAgentRunRoutes(w http.ResponseWriter, r *http.Request) {
	user := MustUserFromContext(r.Context())
	path := strings.TrimPrefix(r.URL.Path, "/api/agent-runs/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	runID := parts[0]

	if len(parts) == 1 && r.Method == http.MethodGet {
		run, err := s.runs.GetRun(r.Context(), user.UserID, runID)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if run.Status == store.AgentRunQueued {
			if position, positionErr := s.runs.QueuePosition(r.Context(), user.UserID, run.ID); positionErr == nil {
				run.QueuePosition = position
			}
		}
		writeJSON(w, http.StatusOK, run)
		return
	}
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}

	switch parts[1] {
	case "cancel":
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var req cancelAgentRunRequest
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}
		run, err := s.cancelAgentRun(r.Context(), user.UserID, runID, req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, run)
	case "resume":
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		run, event, err := s.resumeAgentRun(r.Context(), user.UserID, runID)
		if err != nil {
			var limited *ai.ProviderRateLimitError
			var queueFull *store.AgentQueueFullError
			if errors.As(err, &limited) {
				writeAIError(w, http.StatusTooManyRequests, err)
			} else if errors.As(err, &queueFull) {
				writeCodedError(w, http.StatusTooManyRequests, "agent_queue_full", queueFull.Error())
			} else {
				writeAIError(w, http.StatusBadRequest, err)
			}
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"run": run, "resume_event": event})
	case "followup-answer":
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		result, err := s.answerAgentFollowupQuestion(r.Context(), user.UserID, runID, r.Body)
		if err != nil {
			var conflict *store.AgentFollowupAnswerConflictError
			var queueFull *store.AgentQueueFullError
			if errors.As(err, &conflict) {
				writeError(w, http.StatusConflict, err.Error())
			} else if errors.As(err, &queueFull) {
				writeCodedError(w, http.StatusTooManyRequests, "agent_queue_full", queueFull.Error())
			} else {
				writeError(w, http.StatusBadRequest, err.Error())
			}
			return
		}
		writeJSON(w, http.StatusAccepted, result)
	case "steer":
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		event, err := s.steerAgentRun(r.Context(), user.UserID, runID, r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"event": event})
	case "steers":
		if r.Method != http.MethodDelete && r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		event, err := s.cancelAgentRunSteer(r.Context(), user.UserID, runID, r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"event": event})
	case "events":
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		afterSeq := streamCursor(r)
		var events []store.AgentEvent
		var err error
		if afterSeq > 0 || strings.TrimSpace(r.URL.Query().Get("after_seq")) != "" {
			events, err = s.runs.ListEventsAfter(r.Context(), user.UserID, runID, afterSeq)
		} else {
			events, err = s.runs.ListEventsFiltered(r.Context(), user.UserID, runID, store.AgentEventFilter{
				Types: strings.Split(r.URL.Query().Get("types"), ","),
				Limit: queryInt(r, "limit", 200),
			})
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		run, runErr := s.runs.GetRun(r.Context(), user.UserID, runID)
		if runErr != nil {
			writeError(w, http.StatusInternalServerError, runErr.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": events, "run": run})
	case "previews":
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		previews, err := s.runs.ListEditPreviews(r.Context(), user.UserID, runID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		requestedPreviewID := strings.TrimSpace(r.URL.Query().Get("preview_id"))
		items := make([]map[string]any, 0, len(previews))
		for _, preview := range previews {
			if requestedPreviewID != "" && preview.PreviewID != requestedPreviewID {
				continue
			}
			var payload map[string]any
			if json.Unmarshal([]byte(preview.SnapshotJSON), &payload) != nil {
				continue
			}
			payload["type"] = "workspace_edit_preview_updated"
			items = append(items, payload)
		}
		for _, event := range s.runBroker.Latest(runID) {
			if event.Type != "preview_snapshot" {
				continue
			}
			stored, ok := event.Log.(store.AgentEvent)
			if !ok {
				continue
			}
			var payload map[string]any
			if json.Unmarshal([]byte(stored.PayloadJSON), &payload) != nil {
				continue
			}
			payload["type"] = stored.Type
			previewID := eventPayloadString(payload, "preview_id")
			if requestedPreviewID != "" && previewID != requestedPreviewID {
				continue
			}
			revision := eventPayloadInt(payload, "revision")
			replaced := false
			for index := range items {
				if eventPayloadString(items[index], "preview_id") == previewID {
					if eventPayloadInt(items[index], "revision") < revision {
						items[index] = payload
					}
					replaced = true
					break
				}
			}
			if !replaced {
				items = append(items, payload)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case "stream":
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		s.handleAgentRunStream(w, r, runID)
	default:
		http.NotFound(w, r)
	}
}

func queryInt(r *http.Request, key string, fallback int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func nullableStringValue(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

func contentPreview(content string, maxRunes int) string {
	content = strings.TrimSpace(content)
	if maxRunes <= 0 {
		maxRunes = 120
	}
	runes := []rune(content)
	if len(runes) <= maxRunes {
		return content
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "..."
}

func (s *Server) resumeAgentRun(ctx context.Context, userID, runID string) (store.AgentRun, store.AgentEvent, error) {
	run, err := s.runs.GetRun(ctx, userID, runID)
	if err != nil {
		return store.AgentRun{}, store.AgentEvent{}, err
	}
	if run.Status != store.AgentRunFailed && run.Status != store.AgentRunCanceled {
		return store.AgentRun{}, store.AgentEvent{}, fmt.Errorf("only failed or canceled runs can be continued")
	}
	if !run.UserMessageID.Valid || strings.TrimSpace(run.UserMessageID.String) == "" {
		return store.AgentRun{}, store.AgentEvent{}, fmt.Errorf("run has no user message to continue")
	}
	user := MustUserFromContext(ctx)
	if user.UserID == "" {
		user.UserID = userID
	}
	if state, limited, limitErr := s.ai.ActiveProviderRateLimit(ctx, userID, user.Role, run.ModelProviderID); limitErr != nil {
		return store.AgentRun{}, store.AgentEvent{}, limitErr
	} else if limited {
		return store.AgentRun{}, store.AgentEvent{}, &ai.ProviderRateLimitError{State: state, Err: fmt.Errorf("provider cooldown is active")}
	}
	userMessage, err := s.conversations.GetMessage(ctx, userID, run.UserMessageID.String)
	if err != nil {
		return store.AgentRun{}, store.AgentEvent{}, err
	}
	if err := s.ensureAgentQueueCapacity(ctx, userID); err != nil {
		return store.AgentRun{}, store.AgentEvent{}, err
	}
	resumed, event, err := s.runs.ResumeRunWithEvent(ctx, store.ResumeAgentRunInput{
		RunID:  runID,
		UserID: userID,
		Event: store.AgentEventInput{
			ID:             fmt.Sprintf("evt_resume_%s_%d", runID, run.EventHeadSeq+1),
			ConversationID: run.ConversationID,
			MessageID:      userMessage.ID,
			Type:           "run_resumed",
			Level:          "info",
			Payload:        map[string]any{"message": "run resumed"},
		},
	})
	if err != nil {
		return store.AgentRun{}, store.AgentEvent{}, runLifecycleError(err)
	}
	s.runBroker.Publish(runID, tasks.Event{Type: "event", Log: event})
	s.runBroker.Publish(runID, tasks.Event{Type: "run", Task: resumed})
	s.wakeAgentScheduler()
	return resumed, event, nil
}

func (s *Server) cancelAgentRun(ctx context.Context, userID, runID string, req cancelAgentRunRequest) (store.AgentRun, error) {
	run, err := s.runs.GetRun(ctx, userID, runID)
	if err != nil {
		return store.AgentRun{}, err
	}
	if run.Status != store.AgentRunRunning && run.Status != store.AgentRunQueued {
		return run, nil
	}
	s.runCancelMu.Lock()
	entry := s.runCancels[runID]
	if entry != nil {
		entry.source = normalizeCancelSource(req.Source)
		entry.reason = normalizeCancelReason(req.Reason)
		entry.requestedAt = time.Now().UTC()
	}
	s.runCancelMu.Unlock()
	if entry != nil && entry.cancel != nil {
		entry.cancel()
	}
	source := normalizeCancelSource(req.Source)
	reason := normalizeCancelReason(req.Reason)
	finalRun, _, err := s.finalizeRunStrict(ctx, userID, runID, store.AgentRunCanceled, "canceled", store.UpdateAgentRunInput{SetCompletedAt: true}, store.AgentEventInput{
		RunID:          runID,
		UserID:         userID,
		ConversationID: run.ConversationID,
		Type:           "run_canceled",
		Level:          "warn",
		Payload: map[string]any{
			"message": "run canceled",
			"source":  source,
			"reason":  reason,
		},
	})
	if err != nil {
		return store.AgentRun{}, err
	}
	return finalRun, nil
}

func (s *Server) handleAgentRunStream(w http.ResponseWriter, r *http.Request, runID string) {
	user := MustUserFromContext(r.Context())
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	eventsCh, unsubscribe := s.runBroker.Subscribe(runID)
	defer unsubscribe()

	run, err := s.runs.GetRun(r.Context(), user.UserID, runID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	previewRevisions := map[string]int{}
	sendPreviewSnapshots := func() error {
		snapshots := s.runBroker.Latest(runID)
		sentFromMemory := false
		for _, snapshot := range snapshots {
			if snapshot.Type != "preview_snapshot" {
				continue
			}
			event, ok := snapshot.Log.(store.AgentEvent)
			if !ok {
				continue
			}
			var payload map[string]any
			if json.Unmarshal([]byte(event.PayloadJSON), &payload) != nil {
				continue
			}
			previewID := eventPayloadString(payload, "preview_id")
			revision := eventPayloadInt(payload, "revision")
			if previewID == "" || revision <= previewRevisions[previewID] {
				continue
			}
			if writeSSE(w, "preview", event) != nil {
				return fmt.Errorf("write preview snapshot")
			}
			previewRevisions[previewID] = revision
			sentFromMemory = true
		}
		if sentFromMemory {
			return nil
		}
		previews, listErr := s.runs.ListEditPreviews(r.Context(), user.UserID, runID)
		if listErr != nil {
			return listErr
		}
		for _, preview := range previews {
			if preview.Revision <= previewRevisions[preview.PreviewID] {
				continue
			}
			event := store.AgentEvent{
				ID:    fmt.Sprintf("preview_snapshot_%s_%d", preview.PreviewID, preview.Revision),
				RunID: preview.RunID, UserID: preview.UserID, ConversationID: run.ConversationID,
				MessageID: nullableStringValue(run.UserMessageID), Type: "workspace_edit_preview_updated",
				Level: "info", PayloadJSON: preview.SnapshotJSON, CreatedAt: preview.UpdatedAt,
			}
			if writeSSE(w, "preview", event) != nil {
				return fmt.Errorf("write persisted preview snapshot")
			}
			previewRevisions[preview.PreviewID] = preview.Revision
		}
		return nil
	}
	tailer := durableStreamTailer{
		ctx: r.Context(), w: w, flusher: flusher, wakeups: eventsCh, cursor: streamCursor(r),
		statePayloadKey: "run", stateEventName: "run", sendTransient: sendPreviewSnapshots,
		loadState: func(ctx context.Context) (durableStreamState, error) {
			current, loadErr := s.runs.GetRun(ctx, user.UserID, runID)
			if loadErr != nil {
				return durableStreamState{}, loadErr
			}
			return durableStreamState{Payload: current, EventHeadSeq: current.EventHeadSeq, FinalSeq: current.FinalSeq, Terminal: runIsTerminal(current.Status)}, nil
		},
		listAfter: func(ctx context.Context, after int64) ([]durableStreamRecord, error) {
			events, listErr := s.runs.ListEventsAfter(ctx, user.UserID, runID, after)
			if listErr != nil {
				return nil, listErr
			}
			records := make([]durableStreamRecord, 0, len(events))
			for _, item := range events {
				records = append(records, durableStreamRecord{Seq: item.Seq, EventName: "event", Payload: item})
			}
			return records, nil
		},
	}
	_ = tailer.serve(durableStreamState{Payload: run, EventHeadSeq: run.EventHeadSeq, FinalSeq: run.FinalSeq, Terminal: runIsTerminal(run.Status)})
}

func streamCursor(r *http.Request) int64 {
	raw := strings.TrimSpace(r.URL.Query().Get("after_seq"))
	if raw == "" {
		raw = strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}

func runIsTerminal(status store.AgentRunStatus) bool {
	switch status {
	case store.AgentRunSucceeded, store.AgentRunWaiting, store.AgentRunDelegated, store.AgentRunFailed, store.AgentRunCanceled:
		return true
	default:
		return false
	}
}

func (s *Server) publishRun(ctx context.Context, userID, runID string, status store.AgentRunStatus, stage string, input store.UpdateAgentRunInput) store.AgentRun {
	run, err := s.publishRunStrict(ctx, userID, runID, status, stage, input)
	if err != nil {
		log.Printf("[agent_run] publish failed run=%s status=%s stage=%s error=%v", runID, status, stage, err)
	}
	return run
}

func (s *Server) publishRunStrict(ctx context.Context, userID, runID string, status store.AgentRunStatus, stage string, input store.UpdateAgentRunInput) (store.AgentRun, error) {
	input.Status = status
	input.Stage = stage
	run, err := s.runs.UpdateRun(ctx, userID, runID, input)
	if err == nil {
		s.runBroker.Publish(runID, tasks.Event{Type: "run", Task: run})
	}
	return run, err
}

func (s *Server) appendRunEvent(ctx context.Context, input store.AgentEventInput) store.AgentEvent {
	event, err := s.appendRunEventStrict(ctx, input)
	if err != nil {
		log.Printf("[agent_run] append event failed run=%s type=%s error=%v", input.RunID, input.Type, err)
	}
	return event
}

func (s *Server) appendRunEventStrict(ctx context.Context, input store.AgentEventInput) (store.AgentEvent, error) {
	event, err := s.runs.AppendEvent(ctx, input)
	if err == nil {
		s.runBroker.Publish(input.RunID, tasks.Event{Type: "event", Log: event})
	}
	return event, runLifecycleError(err)
}

func (s *Server) finalizeRunStrict(ctx context.Context, userID, runID string, status store.AgentRunStatus, stage string, update store.UpdateAgentRunInput, event store.AgentEventInput) (store.AgentRun, store.AgentEvent, error) {
	if strings.TrimSpace(event.ID) == "" {
		current, err := s.runs.GetRun(ctx, userID, runID)
		if err != nil {
			return store.AgentRun{}, store.AgentEvent{}, err
		}
		event.ID = fmt.Sprintf("evt_terminal_%s_%d", runID, current.EventHeadSeq+1)
	}
	run, terminalEvent, err := s.runs.FinalizeRunWithEvent(ctx, store.FinalizeAgentRunInput{
		RunID: runID, UserID: userID, Status: status, Stage: stage, Update: update, Event: event,
	})
	if err == nil {
		s.runBroker.Publish(runID, tasks.Event{Type: "event", Log: terminalEvent})
		s.runBroker.Publish(runID, tasks.Event{Type: "run", Task: run})
	}
	return run, terminalEvent, runLifecycleError(err)
}

func runLifecycleError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, store.ErrAgentEventIntegrity) || errors.Is(err, store.ErrAgentRunFinalized) {
		return &ai.AgentRuntimeError{
			Code: ai.AgentRuntimeEventIntegrity, Category: "event_log", Retryable: false,
			Message: "agent event log integrity check failed", Cause: err,
		}
	}
	return err
}

func (s *Server) registerRunCancel(runID string, cancel context.CancelFunc) {
	s.runCancelMu.Lock()
	defer s.runCancelMu.Unlock()
	s.runCancels[runID] = &runCancelEntry{cancel: cancel}
}

func (s *Server) unregisterRunCancel(runID string) {
	s.runCancelMu.Lock()
	defer s.runCancelMu.Unlock()
	delete(s.runCancels, runID)
}

func normalizeCancelSource(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "stop_button", "client_abort", "server", "unknown":
		return normalized
	default:
		return "unknown"
	}
}

func normalizeCancelReason(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unspecified"
	}
	return contentPreview(value, 120)
}
