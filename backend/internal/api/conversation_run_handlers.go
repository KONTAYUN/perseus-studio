package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"webide-backend/internal/ai"
	"webide-backend/internal/store"
	"webide-backend/internal/tasks"
)

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
