package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"webide-backend/internal/ai"
	"webide-backend/internal/store"
)

func (s *Server) executeConversationRun(userID, userRole, conversationID, userMessageID, modelProviderID, content, attachmentsJSON string, attachmentPayloads []ai.AttachmentPayload, thinkingMode, runID string) {
	ctx, cancel := context.WithCancel(context.Background())
	s.registerRunCancel(runID, cancel)
	defer func() {
		cancel()
		s.unregisterRunCancel(runID)
	}()
	if _, err := s.publishRunStrict(ctx, userID, runID, store.AgentRunRunning, "calling_model", store.UpdateAgentRunInput{SetStartedAt: true}); err != nil {
		s.failConversationRunStartedAt(context.Background(), userID, conversationID, userMessageID, runID, err, time.Now())
		return
	}
	eventCtx := ctx
	if ctx.Err() != nil {
		eventCtx = context.Background()
	}
	if _, err := s.appendRunEventStrict(eventCtx, store.AgentEventInput{
		RunID:          runID,
		UserID:         userID,
		ConversationID: conversationID,
		MessageID:      userMessageID,
		Type:           "run_started",
		Level:          "info",
		Payload:        map[string]any{"message": "run started"},
	}); err != nil {
		s.failConversationRunStartedAt(context.Background(), userID, conversationID, userMessageID, runID, err, time.Now())
		return
	}

	var persistenceMu sync.Mutex
	var persistenceErr error
	runStartedAt := time.Now()
	recordPersistenceErr := func(stage string, err error) {
		if err == nil {
			return
		}
		if isExpectedRunCancellation(err) {
			log.Printf("[agent_run] persistence canceled conversation=%s run=%s stage=%s error=%v", conversationID, runID, stage, err)
			return
		}
		wrapped := fmt.Errorf("%s: %w", stage, err)
		log.Printf("[agent_run] persistence failed conversation=%s run=%s stage=%s error=%v", conversationID, runID, stage, err)
		if s.diagnostics != nil {
			if item, diagErr := s.diagnostics.Append(context.Background(), store.DiagnosticLogInput{
				UserID:         userID,
				ConversationID: conversationID,
				MessageID:      userMessageID,
				Stage:          stage,
				Level:          "error",
				ErrorMessage:   err.Error(),
				RequestSummary: s.runPersistenceDiagnosticSummary(userID, runID, stage, runStartedAt),
			}); diagErr == nil {
				wrapped = ai.WithDiagnosticID(wrapped, item.ID)
			}
		}
		persistenceMu.Lock()
		if persistenceErr == nil {
			persistenceErr = wrapped
		}
		persistenceMu.Unlock()
	}
	getPersistenceErr := func() error {
		persistenceMu.Lock()
		defer persistenceMu.Unlock()
		return persistenceErr
	}
	safePublish := func(stage string, status store.AgentRunStatus, runStage string, input store.UpdateAgentRunInput) {
		if getPersistenceErr() != nil {
			return
		}
		if _, err := s.publishRunStrict(ctx, userID, runID, status, runStage, input); err != nil {
			recordPersistenceErr(stage, err)
		}
	}
	safeAppendEvent := func(stage string, eventCtx context.Context, input store.AgentEventInput) {
		if getPersistenceErr() != nil {
			return
		}
		if _, err := s.appendRunEventStrict(eventCtx, input); err != nil {
			recordPersistenceErr(stage, err)
		}
	}

	logEvent := func(level, message string) {
		if getPersistenceErr() != nil {
			return
		}
		safeAppendEvent("append_run_log_event", ctx, store.AgentEventInput{
			RunID:          runID,
			UserID:         userID,
			ConversationID: conversationID,
			MessageID:      userMessageID,
			Type:           "run_log",
			Level:          level,
			Payload:        map[string]any{"message": message},
		})
	}

	var activeAssistantMessageID string
	var activeAssistantSegment int
	var activeAssistantContent strings.Builder
	var latestAssistantMessage store.Message
	var completionPreviewMessageID string
	var completionPreviewSegment int

	startActiveAssistantMessage := func() {
		if getPersistenceErr() != nil {
			return
		}
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
			recordPersistenceErr("assistant_message_started_failed", createErr)
			return
		}
		activeAssistantMessageID = item.ID
		activeAssistantContent.Reset()
		log.Printf("[ai] assistant message started conversation=%s run=%s message=%s segment=%d", conversationID, runID, item.ID, activeAssistantSegment)
		safeAppendEvent("append_assistant_message_started_event", ctx, store.AgentEventInput{
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
		if getPersistenceErr() != nil {
			return
		}
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
			recordPersistenceErr("assistant_message_delta_failed", appendErr)
			return
		}
		log.Printf("[ai] assistant message delta conversation=%s run=%s message=%s delta_chars=%d", conversationID, runID, item.ID, len([]rune(delta)))
		safeAppendEvent("append_assistant_message_delta_event", ctx, store.AgentEventInput{
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

	appendActiveAssistantDeltaChunks := func(delta string) {
		replayAssistantDeltaChunks(ctx, delta, appendActiveAssistantDelta)
	}

	streamMissingAssistantFinalContent := func(finalContent string) {
		finalContent = strings.TrimSpace(finalContent)
		if finalContent == "" {
			return
		}
		appendActiveAssistantDeltaChunks(missingAssistantFinalDelta(activeAssistantContent.String(), finalContent))
	}

	finalizeActiveAssistantMessage := func(finalContent string) {
		if getPersistenceErr() != nil {
			return
		}
		streamMissingAssistantFinalContent(finalContent)
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
			recordPersistenceErr("assistant_message_finalize_failed", finalizeErr)
			return
		}
		latestAssistantMessage = item
		log.Printf("[ai] assistant message completed conversation=%s run=%s message=%s segment=%d chars=%d", conversationID, runID, item.ID, activeAssistantSegment, len([]rune(finalContent)))
		safeAppendEvent("append_assistant_message_completed_event", ctx, store.AgentEventInput{
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

	appendCompletionResultDelta := func(delta string) {
		if getPersistenceErr() != nil {
			return
		}
		if delta == "" {
			return
		}
		if activeAssistantMessageID == "" {
			startActiveAssistantMessage()
			completionPreviewMessageID = activeAssistantMessageID
			completionPreviewSegment = activeAssistantSegment
		}
		appendActiveAssistantDeltaChunks(delta)
	}

	discardCompletionResultPreview := func() {
		if getPersistenceErr() != nil || completionPreviewMessageID == "" {
			return
		}
		messageID := completionPreviewMessageID
		segmentIndex := completionPreviewSegment
		completionPreviewMessageID = ""
		completionPreviewSegment = 0
		if activeAssistantMessageID == messageID {
			activeAssistantMessageID = ""
			activeAssistantContent.Reset()
		}
		if err := s.conversations.DeleteMessage(ctx, userID, conversationID, messageID); err != nil {
			log.Printf("[ai] completion preview discard failed conversation=%s run=%s message=%s error=%v", conversationID, runID, messageID, err)
			recordPersistenceErr("completion_preview_discard_failed", err)
			return
		}
		log.Printf("[ai] completion preview discarded conversation=%s run=%s message=%s", conversationID, runID, messageID)
		safeAppendEvent("append_completion_preview_discarded_event", ctx, store.AgentEventInput{
			RunID:          runID,
			UserID:         userID,
			ConversationID: conversationID,
			MessageID:      messageID,
			Type:           "assistant_message_discarded",
			Level:          "info",
			Payload: map[string]any{
				"message_id":    messageID,
				"segment_index": segmentIndex,
				"reason":        "completion_rejected",
			},
		})
	}

	items, err := s.conversations.ListMessages(ctx, userID, conversationID)
	if err != nil {
		s.failConversationRunStartedAt(ctx, userID, conversationID, userMessageID, runID, err, runStartedAt)
		return
	}
	reply, err := s.ai.GenerateReply(ctx, conversationID, userMessageID, modelProviderID, items, attachmentPayloads, ai.RunOptions{
		UserID:       userID,
		UserRole:     userRole,
		RunID:        runID,
		ThinkingMode: thinkingMode,
		Log:          logEvent,
		Event: func(eventType, level string, payload map[string]any) {
			if eventType == "model_delta" {
				appendActiveAssistantDeltaChunks(eventPayloadString(payload, "text"))
			} else if eventType == "model_message_completed" {
				finalizeActiveAssistantMessage(eventPayloadString(payload, "content"))
				completionPreviewMessageID = ""
				completionPreviewSegment = 0
			}
			if getPersistenceErr() != nil {
				return
			}
			if eventType == "model_call_retrying" {
				safePublish("publish_run_retrying_model", store.AgentRunRunning, "retrying_model", store.UpdateAgentRunInput{})
			} else if strings.HasPrefix(eventType, "model_call_attempt") {
				safePublish("publish_run_calling_model", store.AgentRunRunning, "calling_model", store.UpdateAgentRunInput{})
			} else if eventType == "user_question_requested" {
				safePublish("publish_run_saving_message", store.AgentRunRunning, "saving_message", store.UpdateAgentRunInput{})
			} else if eventType == "tool_call_started" {
				safePublish("publish_run_executing_tool", store.AgentRunRunning, "executing_tool", store.UpdateAgentRunInput{})
			} else if eventType == "tool_call_completed" || eventType == "tool_call_failed" {
				safePublish("publish_run_waiting_model_after_tool", store.AgentRunRunning, "waiting_model_after_tool", store.UpdateAgentRunInput{})
			}
			if payload == nil {
				payload = map[string]any{}
			}
			safeAppendEvent("append_agent_event", ctx, store.AgentEventInput{
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
			if activeAssistantMessageID != "" && activeAssistantMessageID == completionPreviewMessageID {
				return
			}
			finalizeActiveAssistantMessage("")
		},
		OnCompletionResultDelta:   appendCompletionResultDelta,
		OnCompletionResultDiscard: discardCompletionResultPreview,
	})
	if err != nil {
		s.failConversationRunStartedAt(ctx, userID, conversationID, userMessageID, runID, err, runStartedAt)
		return
	}
	if err := getPersistenceErr(); err != nil {
		s.failConversationRunStartedAt(context.Background(), userID, conversationID, userMessageID, runID, err, runStartedAt)
		return
	}

	if reply.BackgroundTask != nil {
		s.completeConversationRunWithTask(ctx, userID, userRole, conversationID, userMessageID, modelProviderID, content, attachmentsJSON, runID, reply.BackgroundTask)
		return
	}

	var assistantMessage store.Message
	if latestAssistantMessage.ID != "" {
		if len(reply.ToolEvents) > 0 {
			rawToolEvents, _ := json.Marshal(reply.ToolEvents)
			updated, updateErr := s.conversations.UpdateMessageAttachments(ctx, userID, latestAssistantMessage.ID, string(rawToolEvents))
			if updateErr != nil {
				s.failConversationRunStartedAt(ctx, userID, conversationID, userMessageID, runID, updateErr, runStartedAt)
				return
			}
			latestAssistantMessage = updated
			if _, err := s.appendRunEventStrict(ctx, store.AgentEventInput{
				RunID:          runID,
				UserID:         userID,
				ConversationID: conversationID,
				MessageID:      latestAssistantMessage.ID,
				Type:           "assistant_message_completed",
				Level:          "info",
				Payload: map[string]any{
					"message":           "assistant message attachments updated",
					"assistant_message": latestAssistantMessage,
					"tool_events":       reply.ToolEvents,
				},
			}); err != nil {
				s.failConversationRunStartedAt(context.Background(), userID, conversationID, userMessageID, runID, err, runStartedAt)
				return
			}
		}
		assistantMessage = latestAssistantMessage
	} else {
		assistantAttachments := "[]"
		if len(reply.ToolEvents) > 0 {
			rawToolEvents, _ := json.Marshal(reply.ToolEvents)
			assistantAttachments = string(rawToolEvents)
		}
		if _, err := s.publishRunStrict(ctx, userID, runID, store.AgentRunRunning, "saving_message", store.UpdateAgentRunInput{}); err != nil {
			s.failConversationRunStartedAt(context.Background(), userID, conversationID, userMessageID, runID, err, runStartedAt)
			return
		}
		assistantMessage, err = s.conversations.AppendMessage(ctx, userID, conversationID, "assistant", reply.Content, assistantAttachments)
		if err != nil {
			s.failConversationRunStartedAt(ctx, userID, conversationID, userMessageID, runID, err, runStartedAt)
			return
		}
		if _, err := s.appendRunEventStrict(ctx, store.AgentEventInput{
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
		}); err != nil {
			s.failConversationRunStartedAt(context.Background(), userID, conversationID, userMessageID, runID, err, runStartedAt)
			return
		}
	}
	if reply.IncompleteReason == ai.IncompleteReasonBudgetExhausted {
		if err := s.completeConversationRunWithBudgetExhaustion(ctx, userID, conversationID, userMessageID, runID, assistantMessage, reply); err != nil {
			s.failConversationRunStartedAt(context.Background(), userID, conversationID, userMessageID, runID, err, runStartedAt)
		}
		return
	}
	if err := s.ai.ClearRunCheckpoints(ctx, ai.RunOptions{UserID: userID, RunID: runID}); err != nil {
		s.failConversationRunStartedAt(context.Background(), userID, conversationID, userMessageID, runID, err, runStartedAt)
		return
	}
	if _, err := s.publishRunStrict(ctx, userID, runID, store.AgentRunSucceeded, "completed", store.UpdateAgentRunInput{
		AssistantMessageID: assistantMessage.ID,
		SetCompletedAt:     true,
	}); err != nil {
		s.failConversationRunStartedAt(context.Background(), userID, conversationID, userMessageID, runID, err, runStartedAt)
		return
	}
	if _, err := s.appendRunEventStrict(ctx, store.AgentEventInput{
		RunID:          runID,
		UserID:         userID,
		ConversationID: conversationID,
		MessageID:      assistantMessage.ID,
		Type:           "run_completed",
		Level:          "info",
		Payload:        map[string]any{"message": "run completed"},
	}); err != nil {
		s.failConversationRunStartedAt(context.Background(), userID, conversationID, userMessageID, runID, err, runStartedAt)
		return
	}
	s.maybeGenerateConversationTitleAsync(userID, userRole, conversationID, userMessageID, assistantMessage.ID, modelProviderID, content, assistantMessage.Content, runID)
}
