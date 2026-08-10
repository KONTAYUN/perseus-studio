package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"webide-backend/internal/ai"
	"webide-backend/internal/store"
)

func (s *Server) handleRegenerateLastMessage(w http.ResponseWriter, r *http.Request, userID, conversationID string) {
	user := MustUserFromContext(r.Context())
	if user.UserID == "" {
		user.UserID = userID
	}
	var req regenerateConversationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	messages, err := s.conversations.ListMessages(r.Context(), userID, conversationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(messages) == 0 {
		writeError(w, http.StatusBadRequest, "conversation has no messages")
		return
	}
	selectedIndex := -1
	lastUserIndex := -1
	for index, item := range messages {
		if item.ID == req.MessageID {
			selectedIndex = index
		}
		if item.Role == "user" {
			lastUserIndex = index
		}
	}
	if selectedIndex < 0 || messages[selectedIndex].Role != "assistant" {
		writeError(w, http.StatusBadRequest, "only an assistant message can be regenerated")
		return
	}
	if lastUserIndex < 0 || selectedIndex <= lastUserIndex {
		writeError(w, http.StatusBadRequest, "no user message to regenerate from")
		return
	}
	for index := selectedIndex + 1; index < len(messages); index++ {
		if messages[index].Role == "user" {
			writeError(w, http.StatusBadRequest, "only the latest assistant tail can be regenerated")
			return
		}
	}
	lastUser := messages[lastUserIndex]
	conversation, err := s.conversations.GetConversation(r.Context(), userID, conversationID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if conversation.SelectedModelProviderID == "" {
		writeError(w, http.StatusBadRequest, "model_provider_id is required")
		return
	}
	if _, err := s.configs.GetModelProviderForUser(r.Context(), userID, user.Role, conversation.SelectedModelProviderID); err != nil {
		writeError(w, http.StatusBadRequest, "selected model is not available")
		return
	}
	if err := s.ensureAgentQueueCapacity(r.Context(), userID); err != nil {
		var queueFull *store.AgentQueueFullError
		if errors.As(err, &queueFull) {
			writeCodedError(w, http.StatusTooManyRequests, "agent_queue_full", queueFull.Error())
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	if err := s.deleteConversationMessagesAfter(r.Context(), userID, conversationID, lastUser.ID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	run, err := s.runs.CreateRunWithQueueLimits(r.Context(), store.CreateAgentRunInput{
		UserID: userID, ConversationID: conversationID, UserMessageID: lastUser.ID,
		ModelProviderID: conversation.SelectedModelProviderID, ThinkingMode: req.ThinkingMode,
	}, s.cfg.AgentMaxQueuedGlobal, s.cfg.AgentMaxQueuedPerUser)
	if err != nil {
		var queueFull *store.AgentQueueFullError
		if errors.As(err, &queueFull) {
			writeCodedError(w, http.StatusTooManyRequests, "agent_queue_full", queueFull.Error())
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	if _, err := s.appendRunEventStrict(r.Context(), store.AgentEventInput{
		RunID: run.ID, UserID: userID, ConversationID: conversationID, MessageID: lastUser.ID,
		Type: "regeneration_requested", Level: "info", Payload: map[string]any{"message_id": lastUser.ID},
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := s.appendRunEventStrict(r.Context(), store.AgentEventInput{
		RunID: run.ID, UserID: userID, ConversationID: conversationID, MessageID: lastUser.ID,
		Type: "run_queued", Level: "info", Payload: map[string]any{"message": "run queued", "reason": "regenerate"},
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if position, positionErr := s.runs.QueuePosition(r.Context(), userID, run.ID); positionErr == nil {
		run.QueuePosition = position
	}
	s.wakeAgentScheduler()
	updatedConversation, _ := s.conversations.GetConversation(r.Context(), userID, conversationID)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"conversation": updatedConversation,
		"user_message": lastUser,
		"run":          run,
	})
}

func lastUserMessage(messages []store.Message) *store.Message {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == "user" {
			return &messages[index]
		}
	}
	return nil
}

func (s *Server) payloadsFromStoredMessageAttachments(ctx context.Context, userID string, message store.Message) []ai.AttachmentPayload {
	var metadata []chatAttachmentMetadata
	if err := json.Unmarshal([]byte(message.AttachmentsJSON), &metadata); err != nil {
		return nil
	}
	var payloads []ai.AttachmentPayload
	for _, item := range metadata {
		switch firstNonEmpty(item.Kind, item.Type) {
		case "workspace_file":
			payloads = append(payloads, ai.AttachmentPayload{
				ID:       item.ID,
				Name:     item.Name,
				Kind:     "workspace_file",
				Path:     item.Path,
				FileType: item.FileType,
				Note:     fmt.Sprintf("workspace_file reference path=%s", item.Path),
			})
		case "current_file":
			payloads = append(payloads, ai.AttachmentPayload{
				ID:       "current_file",
				Name:     item.Name,
				Kind:     "current_file",
				Path:     item.Path,
				FileType: item.FileType,
			})
		case "selected_skill":
			skillID := item.SkillID
			if skillID == "" {
				skillID = strings.TrimPrefix(item.ID, "skill_")
			}
			payloads = append(payloads, ai.AttachmentPayload{
				ID:          item.ID,
				Name:        item.Name,
				Kind:        "selected_skill",
				SkillID:     skillID,
				Description: item.Description,
			})
		}
	}
	if stored, err := s.conversations.ListMessageChatAttachments(ctx, userID, message.ID); err == nil {
		for _, item := range stored {
			if payload, loadErr := s.chatAttachmentPayload(item, message.ID); loadErr == nil {
				payloads = append(payloads, payload)
			}
		}
	}
	return payloads
}
