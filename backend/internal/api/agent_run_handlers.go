package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"webide-backend/internal/idgen"
	"webide-backend/internal/store"
	"webide-backend/internal/tasks"
)

type steerAgentRunRequest struct {
	Content       string `json:"content"`
	ExpectedRunID string `json:"expected_run_id"`
	ClientSteerID string `json:"client_steer_id"`
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
		run, err := s.cancelAgentRun(r.Context(), user.UserID, runID)
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
			writeError(w, http.StatusBadRequest, err.Error())
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
	case "events":
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		events, err := s.runs.ListEvents(r.Context(), user.UserID, runID)
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

func (s *Server) steerAgentRun(ctx context.Context, userID, runID string, body io.Reader) (store.AgentEvent, error) {
	var req steerAgentRunRequest
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		return store.AgentEvent{}, fmt.Errorf("invalid steer request")
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return store.AgentEvent{}, fmt.Errorf("content is required")
	}
	if strings.TrimSpace(req.ExpectedRunID) != runID {
		return store.AgentEvent{}, fmt.Errorf("expected_run_id does not match active run")
	}
	run, err := s.runs.GetRun(ctx, userID, runID)
	if err != nil {
		return store.AgentEvent{}, err
	}
	if run.Status != store.AgentRunRunning && run.Status != store.AgentRunQueued {
		return store.AgentEvent{}, fmt.Errorf("run is not accepting guidance")
	}
	steerID := idgen.New("steer")
	clientSteerID := strings.TrimSpace(req.ClientSteerID)
	preview := contentPreview(content, 120)
	return s.appendRunEventStrict(ctx, store.AgentEventInput{
		RunID:          runID,
		UserID:         userID,
		ConversationID: run.ConversationID,
		MessageID:      nullableStringValue(run.UserMessageID),
		Type:           "agent_steer_received",
		Level:          "info",
		Payload: map[string]any{
			"steer_id":        steerID,
			"client_steer_id": clientSteerID,
			"content":         content,
			"content_preview": preview,
			"created_at":      time.Now().UTC().Format(time.RFC3339Nano),
			"message":         "已收到引导，会在下一次安全边界应用",
		},
	})
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
	go s.executeConversationRun(userID, run.ConversationID, userMessage.ID, run.ModelProviderID, userMessage.Content, userMessage.AttachmentsJSON, attachmentPayloads, "", runID)
	return resumed, event, nil
}

func (s *Server) cancelAgentRun(ctx context.Context, userID, runID string) (store.AgentRun, error) {
	run, err := s.runs.GetRun(ctx, userID, runID)
	if err != nil {
		return store.AgentRun{}, err
	}
	if run.Status != store.AgentRunRunning && run.Status != store.AgentRunQueued {
		return run, nil
	}
	s.runCancelMu.Lock()
	cancel := s.runCancels[runID]
	s.runCancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if _, err := s.appendRunEventStrict(ctx, store.AgentEventInput{
		RunID:          runID,
		UserID:         userID,
		ConversationID: run.ConversationID,
		Type:           "run_cancel_requested",
		Level:          "warn",
		Payload:        map[string]any{"message": "cancel requested"},
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
	s.runCancels[runID] = cancel
}

func (s *Server) unregisterRunCancel(runID string) {
	s.runCancelMu.Lock()
	defer s.runCancelMu.Unlock()
	delete(s.runCancels, runID)
}
