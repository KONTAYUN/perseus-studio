package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"webide-backend/internal/ai"
	"webide-backend/internal/store"
	"webide-backend/internal/tasks"
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
	history := append([]store.Message(nil), messages[:lastUserIndex+1]...)
	if len(history) == 0 {
		writeError(w, http.StatusBadRequest, "no user message to regenerate from")
		return
	}
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
	attachmentPayloads := payloadsFromStoredMessageAttachments(lastUser)
	segments := &assistantSegmentBuffer{}

	reply, err := s.ai.GenerateReply(r.Context(), conversationID, lastUser.ID, conversation.SelectedModelProviderID, history, attachmentPayloads, ai.RunOptions{
		UserID:       userID,
		UserRole:     user.Role,
		ThinkingMode: req.ThinkingMode,
		Event: func(eventType, level string, payload map[string]any) {
			switch eventType {
			case "model_delta":
				segments.AppendDelta(eventPayloadString(payload, "text"))
			case "model_message_completed":
				segments.Flush(eventPayloadString(payload, "content"))
			}
		},
		OnAssistantToolBoundary: func() {
			segments.Flush("")
		},
	})
	if err != nil {
		writeAIError(w, http.StatusBadGateway, err)
		return
	}
	segments.Flush("")
	if len(segments.Items) == 0 && strings.TrimSpace(reply.Content) != "" {
		segments.Items = append(segments.Items, strings.TrimSpace(reply.Content))
	}
	if err := s.conversations.DeleteMessagesAfter(r.Context(), userID, conversationID, lastUser.ID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	for index, content := range segments.Items {
		if reply.BackgroundTask == nil && index == len(segments.Items)-1 {
			break
		}
		if _, err := s.conversations.AppendMessage(r.Context(), userID, conversationID, "assistant", content, "[]"); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if reply.BackgroundTask != nil {
		taskRequest := reply.BackgroundTask
		payload, _ := json.Marshal(tasks.AIAgentPayload{
			ConversationID:  conversationID,
			UserMessageID:   lastUser.ID,
			UserMessage:     strings.TrimSpace(lastUser.Content),
			ModelProviderID: conversation.SelectedModelProviderID,
			UserRole:        user.Role,
			Attachments:     json.RawMessage(lastUser.AttachmentsJSON),
			CreatedFromChat: true,
			Goal:            taskRequest.Goal,
			ExpectedOutputs: taskRequest.ExpectedOutputs,
			SuggestedSteps:  taskRequest.SuggestedSteps,
			DecisionReason:  "model called start_background_task",
		})
		task, err := s.store.CreateTask(r.Context(), store.CreateTaskInput{
			UserID:      userID,
			ID:          newTaskID(),
			Type:        "ai_agent",
			Title:       taskRequest.Title,
			PayloadJSON: string(payload),
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.broker.Publish(task.ID, tasks.Event{Type: "task", Task: task})
		cardAttachments, _ := json.Marshal([]map[string]any{{
			"type":    "task",
			"task_id": task.ID,
			"title":   task.Title,
			"status":  task.Status,
		}})
		assistantMessage, err := s.conversations.AppendMessage(r.Context(), userID, conversationID, "assistant", fmt.Sprintf("已创建后台任务：%s\n状态：%s\n可以在右上角任务面板查看详情。", task.Title, task.Status), string(cardAttachments))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		updatedConversation, _ := s.conversations.GetConversation(r.Context(), userID, conversationID)
		writeJSON(w, http.StatusAccepted, map[string]any{
			"conversation":      updatedConversation,
			"assistant_message": assistantMessage,
			"task":              task,
			"execution_mode":    "background_task",
		})
		return
	}

	assistantAttachments := "[]"
	if len(reply.ToolEvents) > 0 {
		rawToolEvents, _ := json.Marshal(reply.ToolEvents)
		assistantAttachments = string(rawToolEvents)
	}
	assistantContent := strings.TrimSpace(reply.Content)
	if len(segments.Items) > 0 {
		assistantContent = segments.Items[len(segments.Items)-1]
	}
	assistantMessage, err := s.conversations.AppendMessage(r.Context(), userID, conversationID, "assistant", assistantContent, assistantAttachments)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	updatedConversation, _ := s.conversations.GetConversation(r.Context(), userID, conversationID)
	writeJSON(w, http.StatusCreated, map[string]any{
		"conversation":      updatedConversation,
		"assistant_message": assistantMessage,
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

func payloadsFromStoredMessageAttachments(message store.Message) []ai.AttachmentPayload {
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
	return payloads
}
