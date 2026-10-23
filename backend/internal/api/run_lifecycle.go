package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"webide-backend/internal/ai"
	"webide-backend/internal/runqueue"
	"webide-backend/internal/store"
)

func (s *Server) scheduleConversationRun(userID, role, conversationID, messageID, providerID, content, attachmentsJSON string, attachments []ai.AttachmentPayload, thinkingMode, runID string) error {
	return s.scheduleConversationRunPrepared(userID, role, conversationID, messageID, providerID, content, attachmentsJSON, attachments, thinkingMode, runID, nil)
}

func (s *Server) scheduleConversationRegeneration(userID, role, conversationID string, message store.Message, providerID, thinkingMode, selectedID, runID string) error {
	return s.scheduleConversationRunPrepared(userID, role, conversationID, message.ID, providerID, message.Content, message.AttachmentsJSON, payloadsFromStoredMessageAttachments(message), thinkingMode, runID, func(ctx context.Context) error {
		return s.runs.PrepareRegenerationHistory(ctx, userID, conversationID, selectedID, message.ID, runID)
	})
}

func (s *Server) scheduleConversationRunPrepared(userID, role, conversationID, messageID, providerID, content, attachmentsJSON string, attachments []ai.AttachmentPayload, thinkingMode, runID string, prepare func(context.Context) error) error {
	if !s.registerRunCancel(runID, func() { s.runQueue.Cancel(runID) }) {
		return nil // Another replay already owns this run.
	}
	err := s.runQueue.Submit(runID, userID, func(ctx context.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.failConversationRunStartedAt(context.Background(), userID, conversationID, messageID, runID, fmt.Errorf("agent run panic: %v", recovered), time.Now())
			}
		}()
		if prepare != nil {
			if err := prepare(ctx); err != nil {
				s.failConversationRunStartedAt(context.Background(), userID, conversationID, messageID, runID, err, time.Now())
				return
			}
		}
		s.executeConversationRun(ctx, userID, role, conversationID, messageID, providerID, content, attachmentsJSON, attachments, thinkingMode, runID)
	}, func() { s.unregisterRunCancel(runID) })
	if err != nil {
		if errors.Is(err, runqueue.ErrDuplicate) {
			return err
		}
		s.unregisterRunCancel(runID)
		errorText := err.Error()
		_, persistErr := s.publishRunStrict(context.Background(), userID, runID, store.AgentRunFailed, "admission_failed", store.UpdateAgentRunInput{ErrorMessage: &errorText, SetCompletedAt: true})
		if persistErr != nil {
			return fmt.Errorf("%w; persist admission failure: %v", err, persistErr)
		}
	}
	return err
}

func writeRunPreparationError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, store.ErrConversationBusy) || errors.Is(err, store.ErrRequestConflict) {
		status = http.StatusConflict
	}
	writeError(w, status, err.Error())
}

// ShutdownRuns owns the detached run lifecycle independently of HTTP clients.
func (s *Server) ShutdownRuns(ctx context.Context) error {
	runErr := s.runQueue.Shutdown(ctx)
	var titleErr error
	if s.titleQueue != nil {
		titleErr = s.titleQueue.Shutdown(ctx)
	}
	if err := errors.Join(runErr, titleErr); err != nil {
		return err
	}
	_, err := s.runs.FailStaleRunningRuns(ctx, 0, "服务已停止，本轮已暂停，可核验工作区后继续。")
	return err
}
