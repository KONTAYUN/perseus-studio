package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"webide-backend/internal/store"
)

func (s *Server) handleConversations(w http.ResponseWriter, r *http.Request) {
	user := MustUserFromContext(r.Context())
	switch r.Method {
	case http.MethodGet:
		items, err := s.conversations.ListConversations(r.Context(), user.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		var req createConversationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		if req.SelectedModelProviderID == "" {
			writeError(w, http.StatusBadRequest, "selected_model_provider_id is required")
			return
		}
		item, err := s.conversations.CreateConversation(r.Context(), store.CreateConversationInput{
			UserID:                  user.UserID,
			Title:                   req.Title,
			SelectedModelProviderID: req.SelectedModelProviderID,
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, item)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleConversationRoutes(w http.ResponseWriter, r *http.Request) {
	user := MustUserFromContext(r.Context())
	rest := strings.TrimPrefix(r.URL.Path, "/api/conversations/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	conversationID := parts[0]

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodPatch:
			var req updateConversationRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "invalid json body")
				return
			}
			title := strings.TrimSpace(req.Title)
			if title == "" {
				writeError(w, http.StatusBadRequest, "title is required")
				return
			}
			if err := s.conversations.UpdateConversationTitle(r.Context(), user.UserID, conversationID, title); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			item, err := s.conversations.GetConversation(r.Context(), user.UserID, conversationID)
			if err != nil {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, item)
			return
		case http.MethodDelete:
			if err := s.conversations.DeleteConversation(r.Context(), user.UserID, conversationID); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"id": conversationID})
			return
		}
		http.NotFound(w, r)
		return
	}

	if len(parts) == 2 && parts[1] == "branch" {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var req branchConversationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		if strings.TrimSpace(req.MessageID) == "" {
			writeError(w, http.StatusBadRequest, "message_id is required")
			return
		}
		conversation, messages, err := s.conversations.BranchConversation(r.Context(), user.UserID, conversationID, req.MessageID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"conversation": conversation,
			"messages":     messages,
		})
		return
	}

	if len(parts) == 2 && parts[1] == "regenerate-last" {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		s.handleRegenerateLastMessage(w, r, user.UserID, conversationID)
		return
	}

	if len(parts) == 2 && parts[1] == "compact" {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var req compactConversationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		conversation, err := s.conversations.GetConversation(r.Context(), user.UserID, conversationID)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if conversation.SelectedModelProviderID == "" {
			writeError(w, http.StatusBadRequest, "model_provider_id is required")
			return
		}
		result, err := s.ai.CompactConversation(r.Context(), user.UserID, conversationID, conversation.SelectedModelProviderID, "manual", req.Instructions)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}

	if len(parts) == 2 && parts[1] == "context-usage" {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if _, err := s.conversations.GetConversation(r.Context(), user.UserID, conversationID); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		event, err := s.runs.LatestConversationEventByType(r.Context(), user.UserID, conversationID, "model_request_summary")
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusOK, map[string]any{"usage": nil})
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		var payload any
		if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
			payload = map[string]any{}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"usage": map[string]any{
				"event_id":   event.ID,
				"run_id":     event.RunID,
				"created_at": event.CreatedAt,
				"payload":    payload,
			},
		})
		return
	}

	if len(parts) != 2 || parts[1] != "messages" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		items, err := s.conversations.ListMessages(r.Context(), user.UserID, conversationID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		content, modelProviderID, thinkingMode, attachmentsJSON, attachmentPayloads, err := s.parseConversationMessageRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if strings.TrimSpace(content) == "" && len(attachmentPayloads) == 0 {
			writeError(w, http.StatusBadRequest, "content or attachments is required")
			return
		}

		conversation, err := s.conversations.GetConversation(r.Context(), user.UserID, conversationID)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}

		if modelProviderID == "" {
			modelProviderID = conversation.SelectedModelProviderID
		}
		if modelProviderID == "" {
			writeError(w, http.StatusBadRequest, "model_provider_id is required")
			return
		}
		if modelProviderID != conversation.SelectedModelProviderID {
			if err := s.conversations.UpdateConversationModel(r.Context(), user.UserID, conversationID, modelProviderID); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}

		if conversation.Title == "" || conversation.Title == "新对话" {
			title := buildConversationTitle(content)
			if title != "" {
				_ = s.conversations.UpdateConversationTitle(r.Context(), user.UserID, conversationID, title)
			}
		}

		userMessage, err := s.conversations.AppendMessage(r.Context(), user.UserID, conversationID, "user", strings.TrimSpace(content), string(attachmentsJSON))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		run, err := s.runs.CreateRun(r.Context(), store.CreateAgentRunInput{
			UserID:          user.UserID,
			ConversationID:  conversationID,
			UserMessageID:   userMessage.ID,
			ModelProviderID: modelProviderID,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if _, err := s.appendRunEventStrict(r.Context(), store.AgentEventInput{
			RunID:          run.ID,
			UserID:         user.UserID,
			ConversationID: conversationID,
			MessageID:      userMessage.ID,
			Type:           "user_message_committed",
			Level:          "info",
			Payload: map[string]any{
				"message":      "user message committed",
				"user_message": userMessage,
			},
		}); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if _, err := s.appendRunEventStrict(r.Context(), store.AgentEventInput{
			RunID:          run.ID,
			UserID:         user.UserID,
			ConversationID: conversationID,
			MessageID:      userMessage.ID,
			Type:           "run_queued",
			Level:          "info",
			Payload: map[string]any{
				"message": "run queued",
			},
		}); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		updatedConversation, _ := s.conversations.GetConversation(r.Context(), user.UserID, conversationID)
		go s.executeConversationRun(user.UserID, conversationID, userMessage.ID, modelProviderID, strings.TrimSpace(content), string(attachmentsJSON), attachmentPayloads, thinkingMode, run.ID)
		writeJSON(w, http.StatusAccepted, map[string]any{
			"conversation": updatedConversation,
			"user_message": userMessage,
			"run":          run,
		})
	default:
		http.NotFound(w, r)
	}
}

func buildConversationTitle(content string) string {
	text := strings.TrimSpace(content)
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) > 20 {
		return string(runes[:20])
	}
	return text
}
