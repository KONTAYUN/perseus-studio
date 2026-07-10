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
			if errors.As(err, &limited) {
				writeAIError(w, http.StatusTooManyRequests, err)
			} else {
				writeAIError(w, http.StatusBadRequest, err)
			}
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"run": run, "resume_event": event})
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
		events, err := s.runs.ListEventsFiltered(r.Context(), user.UserID, runID, store.AgentEventFilter{
			Types: strings.Split(r.URL.Query().Get("types"), ","),
			Limit: queryInt(r, "limit", 200),
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": events})
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
	attachmentPayloads := payloadsFromStoredMessageAttachments(userMessage)
	resumed, err := s.runs.ResumeRun(ctx, userID, runID)
	if err != nil {
		return store.AgentRun{}, store.AgentEvent{}, err
	}
	event, err := s.appendRunEventStrict(ctx, store.AgentEventInput{
		RunID:          runID,
		UserID:         userID,
		ConversationID: run.ConversationID,
		MessageID:      userMessage.ID,
		Type:           "run_resumed",
		Level:          "info",
		Payload:        map[string]any{"message": "run resumed"},
	})
	if err != nil {
		return store.AgentRun{}, store.AgentEvent{}, err
	}
	go s.executeConversationRun(userID, user.Role, run.ConversationID, userMessage.ID, run.ModelProviderID, userMessage.Content, userMessage.AttachmentsJSON, attachmentPayloads, "", runID)
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
	if _, err := s.appendRunEventStrict(ctx, store.AgentEventInput{
		RunID:          runID,
		UserID:         userID,
		ConversationID: run.ConversationID,
		Type:           "run_cancel_requested",
		Level:          "warn",
		Payload: map[string]any{
			"message": "cancel requested",
			"source":  source,
			"reason":  reason,
		},
	}); err != nil {
		return store.AgentRun{}, err
	}
	return s.publishRunStrict(ctx, userID, runID, store.AgentRunCanceled, "canceled", store.UpdateAgentRunInput{SetCompletedAt: true})
}

func (s *Server) handleAgentRunStream(w http.ResponseWriter, r *http.Request, runID string) {
	user := MustUserFromContext(r.Context())
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	run, err := s.runs.GetRun(r.Context(), user.UserID, runID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	afterSeq := int64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("after_seq")); raw != "" {
		if parsed, parseErr := strconv.ParseInt(raw, 10, 64); parseErr == nil && parsed > 0 {
			afterSeq = parsed
		}
	}
	events, err := s.runs.ListEventsAfter(r.Context(), user.UserID, runID, afterSeq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	writeSSE(w, "run", run)
	for _, item := range events {
		writeSSE(w, "event", item)
	}
	flusher.Flush()

	eventsCh, unsubscribe := s.runBroker.Subscribe(runID)
	defer unsubscribe()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-eventsCh:
			switch event.Type {
			case "run":
				writeSSE(w, "run", event.Task)
			case "event":
				writeSSE(w, "event", event.Log)
			}
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprint(w, ": heartbeat\n\n")
			flusher.Flush()
		}
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
	return event, err
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
