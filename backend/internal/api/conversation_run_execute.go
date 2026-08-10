package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"webide-backend/internal/ai"
	"webide-backend/internal/store"
	"webide-backend/internal/tasks"
)

func assistantPresentationEventType(kind, phase string) string {
	if strings.TrimSpace(kind) == "assistant_commentary" {
		return "assistant_commentary_" + phase
	}
	return "assistant_message_" + phase
}

func (s *Server) executeConversationRun(userID, userRole, conversationID, userMessageID, modelProviderID, content, attachmentsJSON string, attachmentPayloads []ai.AttachmentPayload, thinkingMode, runID string) {
	ctx, cancel := context.WithCancel(context.Background())
	s.registerRunCancel(runID, cancel)
	defer func() {
		cancel()
		s.unregisterRunCancel(runID)
		time.AfterFunc(10*time.Minute, func() {
			if err := s.runs.DeleteRunEditPreviews(context.Background(), userID, runID); err != nil {
				log.Printf("[agent_run] failed to clear expired edit previews run=%s error=%v", runID, err)
			}
			s.runBroker.DeleteLatestTopic(runID)
		})
	}()
	running, err := s.publishRunStrict(ctx, userID, runID, store.AgentRunRunning, "calling_model", store.UpdateAgentRunInput{SetStartedAt: true})
	if err != nil {
		s.failConversationRunStartedAt(context.Background(), userID, conversationID, userMessageID, runID, err, time.Now())
		return
	}
	if running.Status != store.AgentRunRunning {
		return
	}
	eventCtx := ctx
	if ctx.Err() != nil {
		eventCtx = context.Background()
	}
	queueWait := time.Duration(0)
	if running.QueuedAt.Valid {
		queueWait = time.Since(running.QueuedAt.Time)
		if queueWait < 0 {
			queueWait = 0
		}
	}
	if _, err := s.appendRunEventStrict(eventCtx, store.AgentEventInput{
		RunID:          runID,
		UserID:         userID,
		ConversationID: conversationID,
		MessageID:      userMessageID,
		Type:           "run_started",
		Level:          "info",
		Payload: map[string]any{
			"message":       "run started",
			"queue_wait_ms": queueWait.Milliseconds(),
		},
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
	var activeAssistantKind string
	var activeAssistantContent strings.Builder
	var latestAssistantMessage store.Message
	var lastFinalizedAssistantContent string

	startActiveAssistantMessage := func(kind string) {
		if getPersistenceErr() != nil {
			return
		}
		if activeAssistantMessageID != "" {
			return
		}
		if kind == "" {
			kind = "assistant_segment"
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
			MetaJSON:        messageMetaJSON(kind, activeAssistantSegment, runID),
		})
		if createErr != nil {
			log.Printf("[ai] assistant message started failed conversation=%s run=%s error=%v", conversationID, runID, createErr)
			recordPersistenceErr("assistant_message_started_failed", createErr)
			return
		}
		activeAssistantMessageID = item.ID
		activeAssistantKind = kind
		activeAssistantContent.Reset()
		log.Printf("[ai] assistant message started conversation=%s run=%s message=%s segment=%d", conversationID, runID, item.ID, activeAssistantSegment)
		safeAppendEvent("append_assistant_message_started_event", ctx, store.AgentEventInput{
			RunID:          runID,
			UserID:         userID,
			ConversationID: conversationID,
			MessageID:      item.ID,
			Type:           assistantPresentationEventType(kind, "started"),
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
		startActiveAssistantMessage(activeAssistantKind)
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
			Type:           assistantPresentationEventType(activeAssistantKind, "delta"),
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
		if err := s.deleteConversationMessage(ctx, userID, conversationID, messageID); err != nil {
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
		activeAssistantKind = ""
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
		kind := activeAssistantKind
		if kind == "" {
			kind = "assistant_segment"
		}
		item, finalizeErr := s.conversations.FinalizeMessage(ctx, userID, activeAssistantMessageID, finalContent, messageMetaJSON(kind, activeAssistantSegment, runID))
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
			Type:           assistantPresentationEventType(kind, "completed"),
			Level:          "info",
			Payload: map[string]any{
				"assistant_message": item,
				"segment_index":     activeAssistantSegment,
			},
		})
		activeAssistantMessageID = ""
		activeAssistantKind = ""
		activeAssistantContent.Reset()
	}
	interruptActiveAssistantMessage := func() {
		if activeAssistantMessageID == "" || activeAssistantKind != "assistant_commentary" {
			return
		}
		messageID := activeAssistantMessageID
		content := strings.TrimSpace(activeAssistantContent.String())
		if content == "" {
			return
		}
		eventCtx := context.Background()
		item, err := s.conversations.PatchMessageMeta(
			eventCtx,
			userID,
			messageID,
			messageMetaJSON("assistant_commentary", activeAssistantSegment, runID),
			"interrupted",
		)
		if err != nil {
			log.Printf("[ai] assistant commentary interrupt failed conversation=%s run=%s message=%s error=%v", conversationID, runID, messageID, err)
			return
		}
		_, _ = s.appendRunEventStrict(eventCtx, store.AgentEventInput{
			RunID:          runID,
			UserID:         userID,
			ConversationID: conversationID,
			MessageID:      messageID,
			Type:           "assistant_commentary_interrupted",
			Level:          "warn",
			Payload:        map[string]any{"assistant_message": item, "message_id": messageID, "segment_index": activeAssistantSegment},
		})
		activeAssistantMessageID = ""
		activeAssistantKind = ""
		activeAssistantContent.Reset()
	}

	runOptions := func() ai.RunOptions {
		return ai.RunOptions{
			UserID:       userID,
			UserRole:     userRole,
			RunID:        runID,
			ThinkingMode: thinkingMode,
			Log:          logEvent,
			Event: func(eventType, level string, payload map[string]any) {
				if eventType == "model_delta" {
					if eventPayloadString(payload, "message_kind") == "assistant_commentary" && activeAssistantMessageID == "" {
						activeAssistantKind = "assistant_commentary"
					}
					appendActiveAssistantDeltaChunks(eventPayloadString(payload, "text"))
				} else if eventType == "model_message_completed" {
					finalContent := eventPayloadString(payload, "content")
					if shouldFinalizeModelMessageCompleted(finalContent, lastFinalizedAssistantContent) {
						finalizeActiveAssistantMessage(finalContent)
					}
				}
				if getPersistenceErr() != nil {
					return
				}
				if eventType == "model_call_retrying" {
					safePublish("publish_run_retrying_model", store.AgentRunRunning, "retrying_model", store.UpdateAgentRunInput{})
				} else if strings.HasPrefix(eventType, "model_call_attempt") {
					safePublish("publish_run_calling_model", store.AgentRunRunning, "calling_model", store.UpdateAgentRunInput{})
				} else if eventType == "provider_rate_limited" || eventType == "provider_cooldown_updated" {
					safePublish("publish_run_rate_limited_waiting", store.AgentRunRunning, "rate_limited_waiting", store.UpdateAgentRunInput{})
				} else if eventType == "user_input_requested" || eventType == "user_choice_requested" || eventType == "user_question_requested" {
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
			TransientEvent: func(eventType, level string, payload map[string]any) {
				raw, err := json.Marshal(payload)
				if err != nil {
					log.Printf("[agent_run] transient event encode failed run=%s type=%s error=%v", runID, eventType, err)
					return
				}
				previewID := eventPayloadString(payload, "preview_id")
				revision := eventPayloadInt(payload, "revision")
				s.runBroker.PublishLatest(runID, "delta:"+previewID, tasks.Event{Type: "preview", Log: store.AgentEvent{
					ID:             fmt.Sprintf("transient_%s_%d", previewID, revision),
					RunID:          runID,
					UserID:         userID,
					ConversationID: conversationID,
					MessageID:      userMessageID,
					Type:           eventType,
					Level:          level,
					PayloadJSON:    string(raw),
					CreatedAt:      time.Now().UTC(),
				}})
			},
			EditPreviewLatest: func(eventType, level string, payload map[string]any) {
				raw, err := json.Marshal(payload)
				if err != nil {
					log.Printf("[agent_run] latest preview encode failed run=%s type=%s error=%v", runID, eventType, err)
					return
				}
				previewID := eventPayloadString(payload, "preview_id")
				revision := eventPayloadInt(payload, "revision")
				s.runBroker.PublishLatest(runID, "snapshot:"+previewID, tasks.Event{Type: "preview_snapshot", Log: store.AgentEvent{
					ID:             fmt.Sprintf("preview_snapshot_%s_%d", previewID, revision),
					RunID:          runID,
					UserID:         userID,
					ConversationID: conversationID,
					MessageID:      userMessageID,
					Type:           eventType,
					Level:          level,
					PayloadJSON:    string(raw),
					CreatedAt:      time.Now().UTC(),
				}})
			},
			EditPreviewCheckpoint: func(checkpointCtx context.Context, eventType string, payload map[string]any) error {
				previewID := eventPayloadString(payload, "preview_id")
				if previewID == "" {
					return fmt.Errorf("edit preview checkpoint is missing preview_id")
				}
				if eventType == "workspace_edit_preview_completed" || eventType == "workspace_edit_preview_reverted" {
					return s.runs.DeleteEditPreview(checkpointCtx, userID, previewID)
				}
				raw, err := json.Marshal(payload)
				if err != nil {
					return err
				}
				return s.runs.UpsertEditPreview(checkpointCtx, store.AgentEditPreviewInput{
					PreviewID: previewID, RunID: runID, UserID: userID,
					ToolCallID: eventPayloadString(payload, "tool_call_id"),
					Path:       eventPayloadString(payload, "path"), BaseHash: eventPayloadString(payload, "base_hash"),
					Revision: eventPayloadInt(payload, "revision"), SnapshotJSON: string(raw),
				})
			},
			OnAssistantToolBoundary: func() {
				finalizeActiveAssistantMessage("")
			},
			OnAssistantCurrentMessageDiscarded: func(reason string) {
				discardActiveAssistantMessage(reason)
			},
			OnAssistantMessageDiscarded: func(messageID string, reason string, segmentIndex int) {
				if activeAssistantMessageID != "" && activeAssistantMessageID == messageID {
					discardActiveAssistantMessage(reason)
				}
			},
		}
	}

	var reply ai.Reply
	for {
		items, err := s.conversations.ListMessages(ctx, userID, conversationID)
		if err != nil {
			interruptActiveAssistantMessage()
			s.failConversationRunStartedAt(ctx, userID, conversationID, userMessageID, runID, err, runStartedAt)
			return
		}
		reply, err = s.ai.GenerateReply(ctx, conversationID, userMessageID, modelProviderID, items, attachmentPayloads, runOptions())
		if err == nil {
			break
		}
		var limited *ai.ProviderRateLimitError
		if !errors.As(err, &limited) || limited == nil {
			interruptActiveAssistantMessage()
			s.failConversationRunStartedAt(ctx, userID, conversationID, userMessageID, runID, err, runStartedAt)
			return
		}

		now := time.Now().UTC()
		wait := limited.State.Remaining(now)
		if wait <= 0 {
			wait = 20 * time.Second
		}
		retryAfterMS := wait.Milliseconds()
		payload := map[string]any{
			"failure_class":       ai.FailureMetadataFromError(err).FailureClass,
			"kind":                limited.State.Kind,
			"provider_id":         limited.State.ProviderID,
			"provider_name":       limited.State.ProviderName,
			"model_id":            limited.State.ModelID,
			"status_code":         limited.State.StatusCode,
			"retry_after_ms":      retryAfterMS,
			"reset_at":            now.Add(wait),
			"reset_time_inferred": true,
			"request_id":          limited.State.RequestID,
			"can_resume_now":      false,
			"recommended_action":  "auto_wait",
			"message":             "provider cooldown scheduled",
		}
		safePublish("publish_run_rate_limited_waiting", store.AgentRunRunning, "rate_limited_waiting", store.UpdateAgentRunInput{})
		safeAppendEvent("append_provider_auto_resume_scheduled", ctx, store.AgentEventInput{
			RunID:          runID,
			UserID:         userID,
			ConversationID: conversationID,
			MessageID:      userMessageID,
			Type:           "provider_auto_resume_scheduled",
			Level:          "warn",
			Payload:        payload,
		})
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			interruptActiveAssistantMessage()
			s.failConversationRunStartedAt(context.Background(), userID, conversationID, userMessageID, runID, ctx.Err(), runStartedAt)
			return
		case <-timer.C:
		}
		if err := getPersistenceErr(); err != nil {
			interruptActiveAssistantMessage()
			s.failConversationRunStartedAt(context.Background(), userID, conversationID, userMessageID, runID, err, runStartedAt)
			return
		}
		safeAppendEvent("append_provider_auto_resume_started", ctx, store.AgentEventInput{
			RunID:          runID,
			UserID:         userID,
			ConversationID: conversationID,
			MessageID:      userMessageID,
			Type:           "provider_auto_resume_started",
			Level:          "info",
			Payload:        map[string]any{"message": "provider cooldown elapsed"},
		})
		safePublish("publish_run_calling_model_after_rate_limit", store.AgentRunRunning, "calling_model", store.UpdateAgentRunInput{})
	}
	if err := getPersistenceErr(); err != nil {
		interruptActiveAssistantMessage()
		s.failConversationRunStartedAt(context.Background(), userID, conversationID, userMessageID, runID, err, runStartedAt)
		return
	}
	finalizeActiveAssistantMessage("")
	if err := getPersistenceErr(); err != nil {
		interruptActiveAssistantMessage()
		s.failConversationRunStartedAt(context.Background(), userID, conversationID, userMessageID, runID, err, runStartedAt)
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
