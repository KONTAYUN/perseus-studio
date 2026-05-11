package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"webide-backend/internal/ai"
	"webide-backend/internal/store"
	"webide-backend/internal/tasks"
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
		s.appendRunEvent(r.Context(), store.AgentEventInput{
			RunID:          run.ID,
			UserID:         user.UserID,
			ConversationID: conversationID,
			MessageID:      userMessage.ID,
			Type:           "run_queued",
			Level:          "info",
			Payload: map[string]any{
				"message": "run queued",
			},
		})

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

func (s *Server) parseConversationMessageRequest(r *http.Request) (string, string, string, []byte, []ai.AttachmentPayload, error) {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return s.parseMultipartConversationMessage(r)
	}

	var req createConversationMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return "", "", "", nil, nil, fmt.Errorf("invalid json body")
	}
	attachmentsJSON := []byte("[]")
	if len(req.AttachmentsJSON) > 0 {
		attachmentsJSON = req.AttachmentsJSON
	}
	return req.Content, req.ModelProviderID, req.ThinkingMode, attachmentsJSON, nil, nil
}

func (s *Server) handleRegenerateLastMessage(w http.ResponseWriter, r *http.Request, userID, conversationID string) {
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
	attachmentPayloads := payloadsFromStoredMessageAttachments(lastUser)
	segments := &assistantSegmentBuffer{}

	reply, err := s.ai.GenerateReply(r.Context(), conversationID, lastUser.ID, conversation.SelectedModelProviderID, history, attachmentPayloads, ai.RunOptions{
		UserID:       userID,
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

func messageMetaJSON(kind string, segmentIndex int) string {
	raw, _ := json.Marshal(map[string]any{
		"kind":          kind,
		"segment_index": segmentIndex,
	})
	return string(raw)
}

func eventPayloadString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

type assistantSegmentBuffer struct {
	current strings.Builder
	Items   []string
}

func (b *assistantSegmentBuffer) AppendDelta(delta string) {
	if delta == "" {
		return
	}
	b.current.WriteString(delta)
}

func (b *assistantSegmentBuffer) Flush(finalContent string) {
	content := finalContent
	if strings.TrimSpace(content) == "" {
		content = b.current.String()
	}
	content = strings.TrimSpace(content)
	if content != "" {
		b.Items = append(b.Items, content)
	}
	b.current.Reset()
}

func (s *Server) executeConversationRun(userID, conversationID, userMessageID, modelProviderID, content, attachmentsJSON string, attachmentPayloads []ai.AttachmentPayload, thinkingMode, runID string) {
	ctx, cancel := context.WithCancel(context.Background())
	s.registerRunCancel(runID, cancel)
	defer func() {
		cancel()
		s.unregisterRunCancel(runID)
	}()
	s.publishRun(ctx, userID, runID, store.AgentRunRunning, "calling_model", store.UpdateAgentRunInput{SetStartedAt: true})
	eventCtx := ctx
	if ctx.Err() != nil {
		eventCtx = context.Background()
	}
	s.appendRunEvent(eventCtx, store.AgentEventInput{
		RunID:          runID,
		UserID:         userID,
		ConversationID: conversationID,
		MessageID:      userMessageID,
		Type:           "run_started",
		Level:          "info",
		Payload:        map[string]any{"message": "run started"},
	})

	logEvent := func(level, message string) {
		eventType := "run_log"
		payload := map[string]any{"message": message}
		if strings.Contains(message, "tool call started") {
			eventType = "tool_call_log"
			s.publishRun(ctx, userID, runID, store.AgentRunRunning, "executing_tool", store.UpdateAgentRunInput{})
		} else if strings.Contains(message, "tool call done") {
			eventType = "tool_call_log"
			s.publishRun(ctx, userID, runID, store.AgentRunRunning, "waiting_model_after_tool", store.UpdateAgentRunInput{})
		} else if strings.Contains(message, "tool call failed") {
			eventType = "tool_call_log"
			s.publishRun(ctx, userID, runID, store.AgentRunRunning, "waiting_model_after_tool", store.UpdateAgentRunInput{})
		} else if strings.Contains(message, "calling model round") {
			eventType = "model_call_started"
			s.publishRun(ctx, userID, runID, store.AgentRunRunning, "calling_model", store.UpdateAgentRunInput{})
		}
		s.appendRunEvent(ctx, store.AgentEventInput{
			RunID:          runID,
			UserID:         userID,
			ConversationID: conversationID,
			MessageID:      userMessageID,
			Type:           eventType,
			Level:          level,
			Payload:        payload,
		})
	}

	var activeAssistantMessageID string
	var activeAssistantSegment int
	var activeAssistantContent strings.Builder
	var latestAssistantMessage store.Message

	startActiveAssistantMessage := func() {
		if activeAssistantMessageID != "" {
			return
		}
		activeAssistantSegment++
		item, createErr := s.conversations.CreateMessageDraft(ctx, store.MessageDraftInput{
			UserID:          userID,
			ConversationID:  conversationID,
			Role:            "assistant",
			Content:         "",
			AttachmentsJSON: "[]",
			Status:          "streaming",
			SegmentIndex:    activeAssistantSegment,
			MetaJSON:        messageMetaJSON("assistant_segment", activeAssistantSegment),
		})
		if createErr != nil {
			log.Printf("[ai] assistant message started failed conversation=%s run=%s error=%v", conversationID, runID, createErr)
			return
		}
		activeAssistantMessageID = item.ID
		activeAssistantContent.Reset()
		log.Printf("[ai] assistant message started conversation=%s run=%s message=%s segment=%d", conversationID, runID, item.ID, activeAssistantSegment)
		s.appendRunEvent(ctx, store.AgentEventInput{
			RunID:          runID,
			UserID:         userID,
			ConversationID: conversationID,
			MessageID:      item.ID,
			Type:           "assistant_message_started",
			Level:          "info",
			Payload: map[string]any{
				"assistant_message": item,
				"segment_index":     activeAssistantSegment,
			},
		})
	}

	appendActiveAssistantDelta := func(delta string) {
		if delta == "" {
			return
		}
		startActiveAssistantMessage()
		if activeAssistantMessageID == "" {
			return
		}
		activeAssistantContent.WriteString(delta)
		item, appendErr := s.conversations.AppendMessageContent(ctx, userID, activeAssistantMessageID, delta)
		if appendErr != nil {
			log.Printf("[ai] assistant message delta failed conversation=%s run=%s message=%s error=%v", conversationID, runID, activeAssistantMessageID, appendErr)
			return
		}
		log.Printf("[ai] assistant message delta conversation=%s run=%s message=%s delta_chars=%d", conversationID, runID, item.ID, len([]rune(delta)))
		s.appendRunEvent(ctx, store.AgentEventInput{
			RunID:          runID,
			UserID:         userID,
			ConversationID: conversationID,
			MessageID:      item.ID,
			Type:           "assistant_message_delta",
			Level:          "info",
			Payload: map[string]any{
				"message_id": item.ID,
				"delta":      delta,
			},
		})
	}

	finalizeActiveAssistantMessage := func(finalContent string) {
		if activeAssistantMessageID == "" && strings.TrimSpace(finalContent) != "" {
			appendActiveAssistantDelta(finalContent)
		}
		if activeAssistantMessageID == "" {
			return
		}
		finalContent = strings.TrimSpace(finalContent)
		if finalContent == "" {
			finalContent = strings.TrimSpace(activeAssistantContent.String())
		}
		item, finalizeErr := s.conversations.FinalizeMessage(ctx, userID, activeAssistantMessageID, finalContent, messageMetaJSON("assistant_segment", activeAssistantSegment))
		if finalizeErr != nil {
			log.Printf("[ai] assistant message completed failed conversation=%s run=%s message=%s error=%v", conversationID, runID, activeAssistantMessageID, finalizeErr)
			return
		}
		latestAssistantMessage = item
		log.Printf("[ai] assistant message completed conversation=%s run=%s message=%s segment=%d chars=%d", conversationID, runID, item.ID, activeAssistantSegment, len([]rune(finalContent)))
		s.appendRunEvent(ctx, store.AgentEventInput{
			RunID:          runID,
			UserID:         userID,
			ConversationID: conversationID,
			MessageID:      item.ID,
			Type:           "assistant_message_completed",
			Level:          "info",
			Payload: map[string]any{
				"assistant_message": item,
				"segment_index":     activeAssistantSegment,
			},
		})
		activeAssistantMessageID = ""
		activeAssistantContent.Reset()
	}

	items, err := s.conversations.ListMessages(ctx, userID, conversationID)
	if err != nil {
		s.failConversationRun(ctx, userID, conversationID, userMessageID, runID, err)
		return
	}
	reply, err := s.ai.GenerateReply(ctx, conversationID, userMessageID, modelProviderID, items, attachmentPayloads, ai.RunOptions{
		UserID:       userID,
		RunID:        runID,
		ThinkingMode: thinkingMode,
		Log:          logEvent,
		Event: func(eventType, level string, payload map[string]any) {
			if eventType == "model_delta" {
				appendActiveAssistantDelta(eventPayloadString(payload, "text"))
			} else if eventType == "model_message_completed" {
				finalizeActiveAssistantMessage(eventPayloadString(payload, "content"))
			}
			if eventType == "model_call_retrying" {
				s.publishRun(ctx, userID, runID, store.AgentRunRunning, "retrying_model", store.UpdateAgentRunInput{})
			} else if strings.HasPrefix(eventType, "model_call_attempt") {
				s.publishRun(ctx, userID, runID, store.AgentRunRunning, "calling_model", store.UpdateAgentRunInput{})
			} else if eventType == "tool_call_started" {
				s.publishRun(ctx, userID, runID, store.AgentRunRunning, "executing_tool", store.UpdateAgentRunInput{})
			} else if eventType == "tool_call_completed" || eventType == "tool_call_failed" {
				s.publishRun(ctx, userID, runID, store.AgentRunRunning, "waiting_model_after_tool", store.UpdateAgentRunInput{})
			}
			if payload == nil {
				payload = map[string]any{}
			}
			s.appendRunEvent(ctx, store.AgentEventInput{
				RunID:          runID,
				UserID:         userID,
				ConversationID: conversationID,
				MessageID:      userMessageID,
				Type:           eventType,
				Level:          level,
				Payload:        payload,
			})
		},
		OnAssistantToolBoundary: func() {
			finalizeActiveAssistantMessage("")
		},
	})
	if err != nil {
		s.failConversationRun(ctx, userID, conversationID, userMessageID, runID, err)
		return
	}

	if reply.BackgroundTask != nil {
		s.completeConversationRunWithTask(ctx, userID, conversationID, userMessageID, modelProviderID, content, attachmentsJSON, runID, reply.BackgroundTask)
		return
	}

	var assistantMessage store.Message
	if latestAssistantMessage.ID != "" {
		if len(reply.ToolEvents) > 0 {
			rawToolEvents, _ := json.Marshal(reply.ToolEvents)
			updated, updateErr := s.conversations.UpdateMessageAttachments(ctx, userID, latestAssistantMessage.ID, string(rawToolEvents))
			if updateErr != nil {
				s.failConversationRun(ctx, userID, conversationID, userMessageID, runID, updateErr)
				return
			}
			latestAssistantMessage = updated
		}
		assistantMessage = latestAssistantMessage
	} else {
		assistantAttachments := "[]"
		if len(reply.ToolEvents) > 0 {
			rawToolEvents, _ := json.Marshal(reply.ToolEvents)
			assistantAttachments = string(rawToolEvents)
		}
		s.publishRun(ctx, userID, runID, store.AgentRunRunning, "saving_message", store.UpdateAgentRunInput{})
		assistantMessage, err = s.conversations.AppendMessage(ctx, userID, conversationID, "assistant", reply.Content, assistantAttachments)
		if err != nil {
			s.failConversationRun(ctx, userID, conversationID, userMessageID, runID, err)
			return
		}
		s.appendRunEvent(ctx, store.AgentEventInput{
			RunID:          runID,
			UserID:         userID,
			ConversationID: conversationID,
			MessageID:      assistantMessage.ID,
			Type:           "assistant_message_completed",
			Level:          "info",
			Payload: map[string]any{
				"message":           "assistant message completed",
				"assistant_message": assistantMessage,
				"tool_events":       reply.ToolEvents,
			},
		})
	}
	s.ai.ClearRunCheckpoints(ctx, ai.RunOptions{UserID: userID, RunID: runID})
	s.publishRun(ctx, userID, runID, store.AgentRunSucceeded, "completed", store.UpdateAgentRunInput{
		AssistantMessageID: assistantMessage.ID,
		SetCompletedAt:     true,
	})
	s.appendRunEvent(ctx, store.AgentEventInput{
		RunID:          runID,
		UserID:         userID,
		ConversationID: conversationID,
		MessageID:      assistantMessage.ID,
		Type:           "run_completed",
		Level:          "info",
		Payload:        map[string]any{"message": "run completed"},
	})
}

func (s *Server) completeConversationRunWithTask(ctx context.Context, userID, conversationID, userMessageID, modelProviderID, content, attachmentsJSON, runID string, taskRequest *ai.BackgroundTaskRequest) {
	payload, _ := json.Marshal(tasks.AIAgentPayload{
		ConversationID:  conversationID,
		UserMessageID:   userMessageID,
		UserMessage:     content,
		ModelProviderID: modelProviderID,
		Attachments:     json.RawMessage(attachmentsJSON),
		CreatedFromChat: true,
		Goal:            taskRequest.Goal,
		ExpectedOutputs: taskRequest.ExpectedOutputs,
		SuggestedSteps:  taskRequest.SuggestedSteps,
		DecisionReason:  "model called start_background_task",
	})
	task, err := s.store.CreateTask(ctx, store.CreateTaskInput{
		UserID:      userID,
		ID:          newTaskID(),
		Type:        "ai_agent",
		Title:       taskRequest.Title,
		PayloadJSON: string(payload),
	})
	if err != nil {
		s.failConversationRun(ctx, userID, conversationID, userMessageID, runID, err)
		return
	}
	if logItem, err := s.store.AppendLog(ctx, userID, task.ID, "info", "decision result: start_background_task"); err == nil {
		s.broker.Publish(task.ID, tasks.Event{Type: "log", Log: logItem})
	}
	if logItem, err := s.store.AppendLog(ctx, userID, task.ID, "info", "task created and waiting for worker"); err == nil {
		s.broker.Publish(task.ID, tasks.Event{Type: "log", Log: logItem})
	}
	s.broker.Publish(task.ID, tasks.Event{Type: "task", Task: task})

	cardAttachments, _ := json.Marshal([]map[string]any{{
		"type":    "task",
		"task_id": task.ID,
		"title":   task.Title,
		"status":  task.Status,
	}})
	assistantMessage, err := s.conversations.AppendMessage(ctx, userID, conversationID, "assistant", fmt.Sprintf("已创建后台任务：%s\n状态：%s\n可以在右上角任务面板查看详情。", task.Title, task.Status), string(cardAttachments))
	if err != nil {
		s.failConversationRun(ctx, userID, conversationID, userMessageID, runID, err)
		return
	}
	s.appendRunEvent(ctx, store.AgentEventInput{
		RunID:          runID,
		UserID:         userID,
		ConversationID: conversationID,
		MessageID:      assistantMessage.ID,
		TaskID:         task.ID,
		Type:           "assistant_message_completed",
		Level:          "info",
		Payload: map[string]any{
			"message":           "background task created",
			"assistant_message": assistantMessage,
			"task":              task,
		},
	})
	s.ai.ClearRunCheckpoints(ctx, ai.RunOptions{UserID: userID, RunID: runID})
	s.publishRun(ctx, userID, runID, store.AgentRunSucceeded, "completed", store.UpdateAgentRunInput{
		AssistantMessageID: assistantMessage.ID,
		SetCompletedAt:     true,
	})
}

func (s *Server) failConversationRun(ctx context.Context, userID, conversationID, messageID, runID string, err error) {
	message := ai.UserFacingErrorMessage(err)
	if message == "" {
		message = "执行失败"
	}
	diagnosticID := ai.DiagnosticIDFromError(err)
	status := store.AgentRunFailed
	stage := "failed"
	eventType := "run_failed"
	level := "error"
	if ctx.Err() != nil || strings.Contains(strings.ToLower(message), "context canceled") {
		status = store.AgentRunCanceled
		stage = "canceled"
		eventType = "run_canceled"
		level = "warn"
		message = "执行已取消"
	}
	eventCtx := ctx
	if ctx.Err() != nil {
		eventCtx = context.Background()
	}
	if diagnosticID != "" {
		s.appendRunEvent(eventCtx, store.AgentEventInput{
			RunID:          runID,
			UserID:         userID,
			ConversationID: conversationID,
			MessageID:      messageID,
			Type:           "diagnostic_created",
			Level:          "error",
			Payload: map[string]any{
				"diagnostic_log_id": diagnosticID,
				"message":           "diagnostic log created",
			},
		})
	}
	s.appendRunEvent(eventCtx, store.AgentEventInput{
		RunID:          runID,
		UserID:         userID,
		ConversationID: conversationID,
		MessageID:      messageID,
		Type:           eventType,
		Level:          level,
		Payload: map[string]any{
			"error":             message,
			"diagnostic_log_id": diagnosticID,
		},
	})
	s.publishRun(context.Background(), userID, runID, status, stage, store.UpdateAgentRunInput{
		ErrorMessage:    &message,
		DiagnosticLogID: diagnosticID,
		SetCompletedAt:  true,
	})
}

func (s *Server) parseMultipartConversationMessage(r *http.Request) (string, string, string, []byte, []ai.AttachmentPayload, error) {
	if err := r.ParseMultipartForm(80 << 20); err != nil {
		return "", "", "", nil, nil, fmt.Errorf("invalid multipart body")
	}

	var metadata []chatAttachmentMetadata
	var payloads []ai.AttachmentPayload
	totalTextChars := 0

	attachmentIDs := append([]string{}, r.MultipartForm.Value["attachment_ids[]"]...)
	attachmentIDs = append(attachmentIDs, r.MultipartForm.Value["attachment_ids"]...)
	fileIndex := 0
	for _, fieldName := range []string{"files[]", "files"} {
		for _, header := range r.MultipartForm.File[fieldName] {
			file, err := header.Open()
			if err != nil {
				return "", "", "", nil, nil, fmt.Errorf("failed to open attachment")
			}
			raw, readErr := io.ReadAll(io.LimitReader(file, 64<<20))
			closeErr := file.Close()
			if readErr != nil {
				return "", "", "", nil, nil, fmt.Errorf("failed to read attachment")
			}
			if closeErr != nil {
				return "", "", "", nil, nil, fmt.Errorf("failed to close attachment")
			}

			attachmentID := ""
			if fileIndex < len(attachmentIDs) {
				attachmentID = attachmentIDs[fileIndex]
			}
			fileIndex++
			payload, item := buildChatAttachment(attachmentID, header.Filename, header.Size, raw, &totalTextChars)
			payloads = append(payloads, payload)
			metadata = append(metadata, item)
		}
	}

	referencePayloads, referenceMetadata, err := s.parseChatReferences(r)
	if err != nil {
		return "", "", "", nil, nil, err
	}
	payloads = append(payloads, referencePayloads...)
	metadata = append(metadata, referenceMetadata...)

	rawMetadata, err := json.Marshal(metadata)
	if err != nil {
		return "", "", "", nil, nil, fmt.Errorf("failed to encode attachment metadata")
	}
	return r.FormValue("content"), r.FormValue("model_provider_id"), r.FormValue("thinking_mode"), rawMetadata, payloads, nil
}

func (s *Server) parseChatReferences(r *http.Request) ([]ai.AttachmentPayload, []chatAttachmentMetadata, error) {
	var payloads []ai.AttachmentPayload
	var metadata []chatAttachmentMetadata
	files, workspaceErr := s.factory.Workspace(MustUserFromContext(r.Context()).UserID)
	if workspaceErr != nil {
		files = nil
	}

	for _, ref := range parseWorkspaceFileRefs(r.FormValue("workspace_refs_json")) {
		cleanPath, err := cleanWorkspaceReferencePath(ref.Path)
		if err != nil {
			return nil, nil, err
		}
		name := strings.TrimSpace(ref.Name)
		if name == "" {
			name = filepath.Base(cleanPath)
		}
		fileType := strings.TrimSpace(ref.FileType)
		if fileType == "" {
			fileType = "file"
		}
		payload := ai.AttachmentPayload{
			ID:       ref.ID,
			Name:     name,
			Kind:     "workspace_file",
			Path:     cleanPath,
			FileType: fileType,
			Note:     fmt.Sprintf("workspace_file reference path=%s", cleanPath),
		}
		if files != nil && isTextWorkspaceReference(cleanPath) {
			if result, readErr := files.ReadTextFile(cleanPath, 6000); readErr == nil {
				payload.Kind = "text"
				payload.Text = result.Content
				payload.IncludedInContext = true
				payload.Truncated = result.Truncated
				payload.Note = fmt.Sprintf("workspace_file text content path=%s", cleanPath)
			}
		}
		item := chatAttachmentMetadata{
			ID:                ref.ID,
			Name:              name,
			Kind:              "workspace_file",
			Type:              "workspace_file",
			Path:              cleanPath,
			FileType:          fileType,
			IncludedInContext: payload.IncludedInContext,
			Truncated:         payload.Truncated,
		}
		payloads = append(payloads, payload)
		metadata = append(metadata, item)
	}

	if panelsContext := parseEditorPanelsContext(r.FormValue("current_file_context_json")); panelsContext.Type == "editor_panels_context" && len(panelsContext.Panels) > 0 {
		cleanPanels := make([]ai.EditorPanelContext, 0, len(panelsContext.Panels))
		for _, panel := range panelsContext.Panels {
			if panel.ActiveFilePath != "" {
				cleanPath, err := cleanWorkspaceReferencePath(panel.ActiveFilePath)
				if err != nil {
					return nil, nil, err
				}
				panel.ActiveFilePath = cleanPath
				if panel.ActiveFileName == "" {
					panel.ActiveFileName = filepath.Base(cleanPath)
				}
			}
			var cleanTabs []string
			for _, tab := range panel.OpenTabs {
				cleanPath, err := cleanWorkspaceReferencePath(tab)
				if err != nil {
					return nil, nil, err
				}
				cleanTabs = append(cleanTabs, cleanPath)
			}
			panel.OpenTabs = cleanTabs
			cleanPanels = append(cleanPanels, panel)
		}
		payloads = append(payloads, ai.AttachmentPayload{
			ID:           "editor_panels_context",
			Name:         "editor_panels_context",
			Kind:         "editor_panels_context",
			EditorPanels: cleanPanels,
		})
		metadata = append(metadata, chatAttachmentMetadata{
			ID:   "editor_panels_context",
			Name: "editor_panels_context",
			Kind: "editor_panels_context",
			Type: "editor_panels_context",
		})
	} else if current := parseCurrentFileContext(r.FormValue("current_file_context_json")); current.Path != "" {
		cleanPath, err := cleanWorkspaceReferencePath(current.Path)
		if err != nil {
			return nil, nil, err
		}
		name := strings.TrimSpace(current.Name)
		if name == "" {
			name = filepath.Base(cleanPath)
		}
		payloads = append(payloads, ai.AttachmentPayload{
			ID:       "current_file",
			Name:     name,
			Kind:     "current_file",
			Path:     cleanPath,
			FileType: current.FileType,
		})
		if files != nil && isTextWorkspaceReference(cleanPath) {
			if result, readErr := files.ReadTextFile(cleanPath, 6000); readErr == nil {
				payloads[len(payloads)-1].Text = result.Content
				payloads[len(payloads)-1].IncludedInContext = true
				payloads[len(payloads)-1].Truncated = result.Truncated
			}
		}
		metadata = append(metadata, chatAttachmentMetadata{
			ID:                "current_file",
			Name:              name,
			Kind:              "current_file",
			Type:              "current_file",
			Path:              cleanPath,
			FileType:          current.FileType,
			IncludedInContext: payloads[len(payloads)-1].IncludedInContext,
			Truncated:         payloads[len(payloads)-1].Truncated,
		})
	}

	for _, skill := range parseSelectedSkillRefs(r.FormValue("selected_skills_json")) {
		skillID := skill.SkillID
		if skillID == "" {
			skillID = skill.ID
		}
		if strings.TrimSpace(skillID) == "" {
			continue
		}
		name := strings.TrimSpace(skill.Name)
		if name == "" {
			name = skillID
		}
		payloads = append(payloads, ai.AttachmentPayload{
			ID:          "skill_" + skillID,
			Name:        name,
			Kind:        "selected_skill",
			SkillID:     skillID,
			Description: skill.Description,
		})
		metadata = append(metadata, chatAttachmentMetadata{
			ID:          "skill_" + skillID,
			Name:        name,
			Kind:        "selected_skill",
			Type:        "selected_skill",
			SkillID:     skillID,
			Description: skill.Description,
		})
	}
	return payloads, metadata, nil
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
