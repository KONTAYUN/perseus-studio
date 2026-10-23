package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"webide-backend/internal/idgen"
	"webide-backend/internal/store"
)

type steerAgentRunRequest struct {
	PendingMessageID string `json:"pending_message_id"`
	Content          string `json:"content"`
	ExpectedRunID    string `json:"expected_run_id"`
	ClientSteerID    string `json:"client_steer_id"`
}

type cancelSteerRequest struct {
	ExpectedRunID string `json:"expected_run_id"`
	ClientSteerID string `json:"client_steer_id"`
	SteerID       string `json:"steer_id"`
	Reason        string `json:"reason"`
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
	if req.PendingMessageID != "" {
		if existing, err := s.runs.PendingSteerEvent(ctx, userID, req.PendingMessageID); err == nil {
			if existing.RunID != runID {
				return store.AgentEvent{}, store.ErrRequestConflict
			}
			return existing, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return store.AgentEvent{}, err
		}
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
	receivedAt := time.Now().UTC()
	userMessage, err := s.conversations.AppendMessageWithMeta(ctx, store.MessageInput{
		UserID:          userID,
		ConversationID:  run.ConversationID,
		Role:            "user",
		Content:         content,
		AttachmentsJSON: "[]",
		Status:          "final",
		MetaJSON:        store.SteerMessageMetaJSON(steerID, clientSteerID, runID, "received", 0, receivedAt, time.Time{}),
		CreatedAt:       receivedAt,
	})
	if err != nil {
		return store.AgentEvent{}, err
	}
	event, err := s.appendRunEventStrict(ctx, store.AgentEventInput{
		PendingMessageID: req.PendingMessageID,
		RunID:            runID,
		UserID:           userID,
		ConversationID:   run.ConversationID,
		MessageID:        userMessage.ID,
		Type:             "agent_steer_received",
		Level:            "info",
		Payload: map[string]any{
			"pending_message_id": req.PendingMessageID,
			"steer_id":           steerID,
			"client_steer_id":    clientSteerID,
			"content":            content,
			"content_preview":    preview,
			"created_at":         receivedAt.Format(time.RFC3339Nano),
			"message_id":         userMessage.ID,
			"user_message":       userMessage,
			"message":            "已收到引导，会在下一次安全边界应用",
		},
	})
	if err != nil {
		if deleteErr := s.conversations.DeleteMessage(context.Background(), userID, run.ConversationID, userMessage.ID); deleteErr != nil && deleteErr != sql.ErrNoRows {
			return store.AgentEvent{}, fmt.Errorf("%w; rollback steer message failed: %v", err, deleteErr)
		}
		return store.AgentEvent{}, err
	}
	if event.MessageID != userMessage.ID {
		if err := s.conversations.DeleteMessage(ctx, userID, run.ConversationID, userMessage.ID); err != nil {
			return store.AgentEvent{}, err
		}
	}
	return event, nil
}

func (s *Server) cancelAgentRunSteer(ctx context.Context, userID, runID string, body io.Reader) (store.AgentEvent, error) {
	var req cancelSteerRequest
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		return store.AgentEvent{}, fmt.Errorf("invalid steer cancel request")
	}
	if strings.TrimSpace(req.ExpectedRunID) != runID {
		return store.AgentEvent{}, fmt.Errorf("expected_run_id does not match active run")
	}
	clientSteerID := strings.TrimSpace(req.ClientSteerID)
	steerID := strings.TrimSpace(req.SteerID)
	if clientSteerID == "" && steerID == "" {
		return store.AgentEvent{}, fmt.Errorf("steer_id or client_steer_id is required")
	}
	run, err := s.runs.GetRun(ctx, userID, runID)
	if err != nil {
		return store.AgentEvent{}, err
	}
	if run.Status != store.AgentRunRunning && run.Status != store.AgentRunQueued {
		return store.AgentEvent{}, fmt.Errorf("run is not accepting guidance")
	}
	messageID := ""
	if foundMessageID, err := s.conversations.FindSteerMessageID(ctx, userID, run.ConversationID, steerID, clientSteerID); foundMessageID != "" {
		messageID = foundMessageID
		if deleteErr := s.conversations.DeleteMessage(ctx, userID, run.ConversationID, messageID); deleteErr != nil && deleteErr != sql.ErrNoRows {
			return store.AgentEvent{}, deleteErr
		}
	} else if err != nil {
		return store.AgentEvent{}, err
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "user_deleted_guidance"
	}
	return s.appendRunEventStrict(ctx, store.AgentEventInput{
		RunID:          runID,
		UserID:         userID,
		ConversationID: run.ConversationID,
		MessageID:      firstNonEmpty(messageID, nullableStringValue(run.UserMessageID)),
		Type:           "agent_steer_canceled",
		Level:          "info",
		Payload: map[string]any{
			"steer_id":        steerID,
			"client_steer_id": clientSteerID,
			"reason":          contentPreview(reason, 120),
			"created_at":      time.Now().UTC().Format(time.RFC3339Nano),
			"message_id":      messageID,
			"message_deleted": true,
			"message":         "引导已取消",
		},
	})
}
