package api

import (
	"context"
	"log"
	"strings"
	"time"

	"errors"
	"webide-backend/internal/runqueue"
	"webide-backend/internal/store"
)

func firstNonEmptyAPIString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (s *Server) maybeGenerateConversationTitle(ctx context.Context, userID, userRole, conversationID, userMessageID, assistantMessageID, modelProviderID, userContent, assistantContent, runID string) {
	conversation, err := s.conversations.GetConversation(ctx, userID, conversationID)
	if err != nil {
		log.Printf("[conversation_title] load conversation failed conversation=%s error=%v", conversationID, err)
		return
	}
	if !shouldAutoGenerateConversationTitle(conversation) {
		return
	}
	if strings.TrimSpace(userContent) == "" && strings.TrimSpace(assistantContent) == "" {
		return
	}
	titleCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	result, err := s.ai.GenerateConversationTitle(titleCtx, userID, userRole, conversationID, userMessageID, modelProviderID, userContent, assistantContent)
	if err != nil {
		log.Printf("[conversation_title] generation skipped conversation=%s model_provider=%s error=%v", conversationID, modelProviderID, err)
		if _, appendErr := s.appendRunEventStrict(context.Background(), store.AgentEventInput{
			RunID:          runID,
			UserID:         userID,
			ConversationID: conversationID,
			MessageID:      firstNonEmptyAPIString(assistantMessageID, userMessageID),
			Type:           "conversation_title_failed",
			Level:          "warn",
			Payload: map[string]any{
				"message": "conversation title generation failed",
				"error":   err.Error(),
			},
		}); appendErr != nil {
			log.Printf("[conversation_title] failed event append failed conversation=%s error=%v", conversationID, appendErr)
		}
		return
	}
	updated, ok, err := s.conversations.UpdateConversationAutoTitle(context.Background(), userID, conversationID, result.Title)
	if err != nil {
		log.Printf("[conversation_title] update failed conversation=%s error=%v", conversationID, err)
		return
	}
	if !ok {
		return
	}
	if _, err := s.appendRunEventStrict(context.Background(), store.AgentEventInput{
		RunID:          runID,
		UserID:         userID,
		ConversationID: conversationID,
		MessageID:      firstNonEmptyAPIString(assistantMessageID, userMessageID),
		Type:           "conversation_title_updated",
		Level:          "info",
		Payload: map[string]any{
			"message":      "conversation title updated",
			"conversation": updated,
			"title":        updated.Title,
		},
	}); err != nil {
		log.Printf("[conversation_title] updated event append failed conversation=%s error=%v", conversationID, err)
	}
}

func (s *Server) maybeGenerateConversationTitleAsync(userID, userRole, conversationID, userMessageID, assistantMessageID, modelProviderID, userContent, assistantContent, runID string) {
	if s.titleQueue == nil {
		return
	}
	err := s.titleQueue.Submit("title:"+conversationID, userID, func(ctx context.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("[conversation_title] panic conversation=%s error=%v", conversationID, recovered)
			}
		}()
		s.maybeGenerateConversationTitle(ctx, userID, userRole, conversationID, userMessageID, assistantMessageID, modelProviderID, userContent, assistantContent, runID)
	})
	if err != nil && !errors.Is(err, runqueue.ErrDuplicate) {
		log.Printf("[conversation_title] optional job skipped conversation=%s error=%v", conversationID, err)
	}
}

func shouldAutoGenerateConversationTitle(conversation store.Conversation) bool {
	source := strings.TrimSpace(conversation.TitleSource)
	if source == "" {
		source = "default"
	}
	if source != "default" && source != "auto" {
		return false
	}
	title := strings.TrimSpace(conversation.Title)
	return title == "" || title == "新对话"
}
