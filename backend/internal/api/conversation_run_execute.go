package api

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"webide-backend/internal/ai"
	"webide-backend/internal/failure"
	"webide-backend/internal/store"
)

func (s *Server) executeConversationRun(ctx context.Context, userID, userRole, conversationID, userMessageID, modelProviderID, content, attachmentsJSON string, attachmentPayloads []ai.AttachmentPayload, thinkingMode, runID string) {
	if ctx.Err() != nil {
		return
	}
	ctx, stopExecution := context.WithCancelCause(ctx)
	defer stopExecution(nil)
	run, err := s.runs.GetRun(ctx, userID, runID)
	if err != nil || run.Status != store.AgentRunQueued {
		return
	}
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
		wrapped := failure.New("local_run_persistence", fmt.Sprintf("%s: %v", stage, err), err)
		persistenceMu.Lock()
		first := persistenceErr == nil
		if first {
			persistenceErr = wrapped
		}
		persistenceMu.Unlock()
		stopExecution(wrapped)
		if !first {
			return
		}
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
		persistenceErr = wrapped
		persistenceMu.Unlock()
		stopExecution(wrapped)
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
	var lastFinalizedAssistantContent string
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
			MetaJSON:        messageMetaJSON("assistant_segment", activeAssistantSegment, runID),
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

	discardActiveAssistantMessage := func(reason string) {
		if getPersistenceErr() != nil || activeAssistantMessageID == "" {
			return
		}
		messageID := activeAssistantMessageID
		segmentIndex := activeAssistantSegment
		if err := s.conversations.DeleteMessage(ctx, userID, conversationID, messageID); err != nil {
			log.Printf("[ai] assistant message discard failed conversation=%s run=%s message=%s error=%v", conversationID, runID, messageID, err)
			recordPersistenceErr("assistant_message_discard_failed", err)
			return
		}
		log.Printf("[ai] assistant message discarded conversation=%s run=%s message=%s reason=%s", conversationID, runID, messageID, reason)
		safeAppendEvent("append_assistant_message_discarded_event", ctx, store.AgentEventInput{
			RunID:          runID,
			UserID:         userID,
			ConversationID: conversationID,
			MessageID:      messageID,
			Type:           "assistant_message_discarded",
			Level:          "info",
			Payload: map[string]any{
				"message_id":    messageID,
				"segment_index": segmentIndex,
				"reason":        reason,
			},
		})
		activeAssistantMessageID = ""
		activeAssistantContent.Reset()
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
		if finalContent == "" {
			discardActiveAssistantMessage("empty_assistant_segment")
			return
		}
		item, finalizeErr := s.conversations.FinalizeMessage(ctx, userID, activeAssistantMessageID, finalContent, messageMetaJSON("assistant_segment", activeAssistantSegment, runID))
		if finalizeErr != nil {
			log.Printf("[ai] assistant message completed failed conversation=%s run=%s message=%s error=%v", conversationID, runID, activeAssistantMessageID, finalizeErr)
			recordPersistenceErr("assistant_message_finalize_failed", finalizeErr)
			return
		}
		latestAssistantMessage = item
		lastFinalizedAssistantContent = finalContent
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
		if completionPreviewMessageID == "" {
			if activeAssistantMessageID != "" {
				discardActiveAssistantMessage("completion_result_supersedes_preamble")
			}
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
	action, err := s.documentActionForRun(ctx, userID, userMessageID)
	if err != nil {
		s.failConversationRunStartedAt(ctx, userID, conversationID, userMessageID, runID, err, runStartedAt)
		return
	}
	items, attachmentPayloads = documentRunContext(action, items, attachmentPayloads, userMessageID)
	reply, err := s.ai.GenerateReply(ctx, conversationID, userMessageID, modelProviderID, items, attachmentPayloads, ai.RunOptions{
		DocumentAction:        action,
		AllowedWorkspacePaths: documentActionPaths(action),
		UserID:                userID,
		UserRole:              userRole,
		RunID:                 runID,
		ThinkingMode:          thinkingMode,
		Log:                   logEvent,
		Event: func(eventType, level string, payload map[string]any) {
			streamState := assistantStreamState{
				ActiveMessageID:      activeAssistantMessageID,
				CompletionPreviewID:  completionPreviewMessageID,
				LastFinalizedContent: lastFinalizedAssistantContent,
			}
			if eventType == "model_delta" {
				if shouldAppendModelDelta(streamState) {
					appendActiveAssistantDeltaChunks(eventPayloadString(payload, "text"))
				}
			} else if eventType == "model_message_completed" {
				finalContent := eventPayloadString(payload, "content")
				if shouldFinalizeModelMessageCompleted(finalContent, streamState.LastFinalizedContent) {
					finalizeActiveAssistantMessage(finalContent)
				}
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
		OnAssistantMessageDiscarded: func(messageID string, reason string, segmentIndex int) {
			if activeAssistantMessageID != "" && activeAssistantMessageID == messageID {
				discardActiveAssistantMessage(reason)
			}
			if completionPreviewMessageID == messageID {
				completionPreviewMessageID = ""
				completionPreviewSegment = 0
			}
		},
		OnCompletionResultDelta:   appendCompletionResultDelta,
		OnCompletionResultDiscard: discardCompletionResultPreview,
	})
	if persistErr := getPersistenceErr(); persistErr != nil {
		s.failConversationRunStartedAt(context.Background(), userID, conversationID, userMessageID, runID, persistErr, runStartedAt)
		return
	}
	if err != nil {
		s.failConversationRunStartedAt(ctx, userID, conversationID, userMessageID, runID, err, runStartedAt)
		return
	}

	if reply.BackgroundTask != nil {
		s.completeConversationRunWithTask(ctx, userID, userRole, conversationID, userMessageID, modelProviderID, content, attachmentsJSON, runID, reply.BackgroundTask)
		return
	}

	s.completeConversationRun(ctx, conversationRunCompletionInput{
		UserID:                 userID,
		UserRole:               userRole,
		ConversationID:         conversationID,
		UserMessageID:          userMessageID,
		ModelProviderID:        modelProviderID,
		Content:                content,
		RunID:                  runID,
		RunStartedAt:           runStartedAt,
		Reply:                  reply,
		LatestAssistantMessage: latestAssistantMessage,
	})
}
