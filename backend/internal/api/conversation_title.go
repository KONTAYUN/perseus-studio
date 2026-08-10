package api

import (
	"context"
	"log"
	"strings"
	"time"

	"webide-backend/internal/store"
)

func (s *Server) maybeGenerateConversationTitle(ctx context.Context, userID, userRole, conversationID, userMessageID, modelProviderID, userContent, assistantContent string) {
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
	titleCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	result, err := s.ai.GenerateConversationTitle(titleCtx, userID, userRole, conversationID, userMessageID, modelProviderID, userContent, assistantContent)
	if err != nil {
		log.Printf("[conversation_title] generation skipped conversation=%s model_provider=%s error=%v", conversationID, modelProviderID, err)
		return
	}
	updated, ok, err := s.conversations.UpdateConversationAutoTitle(context.Background(), userID, conversationID, result.Title)
	if err != nil {
		log.Printf("[conversation_title] update failed conversation=%s error=%v", conversationID, err)
		return
	}
	if ok {
		log.Printf("[conversation_title] updated conversation=%s title_source=%s", conversationID, updated.TitleSource)
	}
}

func (s *Server) maybeGenerateConversationTitleAsync(userID, userRole, conversationID, userMessageID, modelProviderID, userContent, assistantContent string) {
	go s.maybeGenerateConversationTitle(context.Background(), userID, userRole, conversationID, userMessageID, modelProviderID, userContent, assistantContent)
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
