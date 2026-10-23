package api

import (
	"encoding/json"
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
	conversation, err := s.conversations.GetConversation(r.Context(), userID, conversationID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if _, err := s.configs.GetModelProviderForUser(r.Context(), userID, user.Role, conversation.SelectedModelProviderID); err != nil {
		writeError(w, http.StatusBadRequest, "selected model is not available")
		return
	}
	prepared, err := s.runs.PrepareConversationRun(r.Context(), store.PrepareConversationRunInput{
		UserID: userID, ConversationID: conversationID, ModelProviderID: conversation.SelectedModelProviderID,
		ThinkingMode: req.ThinkingMode, RegenerateMessageID: req.MessageID, RequestID: r.Header.Get("Idempotency-Key"),
	})
	if err != nil {
		writeRunPreparationError(w, err)
		return
	}
	run, lastUser := prepared.Run, prepared.Message
	if run.Status == store.AgentRunQueued && !s.runQueue.Contains(run.ID) {
		if err := s.scheduleConversationRegeneration(userID, user.Role, conversationID, lastUser, conversation.SelectedModelProviderID, req.ThinkingMode, req.MessageID, run.ID); err != nil {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"conversation": conversation, "run": run})
}

func lastUserMessage(messages []store.Message) *store.Message {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == "user" {
			return &messages[index]
		}
	}
	return nil
}

func payloadsFromStoredMessageAttachments(message store.Message) []ai.AttachmentPayload {
	payloads := ai.StoredMessageAttachments(message)
	var metadata []chatAttachmentMetadata
	if err := json.Unmarshal([]byte(message.AttachmentsJSON), &metadata); err != nil {
		return payloads
	}
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
	return payloads
}
