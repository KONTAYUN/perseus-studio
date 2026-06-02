package api

import (
	"context"
	"encoding/json"
	"time"

	"webide-backend/internal/ai"
	"webide-backend/internal/store"
)

type conversationRunCompletionInput struct {
	UserID                 string
	UserRole               string
	ConversationID         string
	UserMessageID          string
	ModelProviderID        string
	Content                string
	RunID                  string
	RunStartedAt           time.Time
	Reply                  ai.Reply
	LatestAssistantMessage store.Message
}

func (s *Server) completeConversationRun(ctx context.Context, input conversationRunCompletionInput) {
	reply := input.Reply
	assistantMessage := input.LatestAssistantMessage
	if assistantMessage.ID != "" {
		if len(reply.ToolEvents) > 0 {
			rawToolEvents, _ := json.Marshal(reply.ToolEvents)
			updated, err := s.conversations.UpdateMessageAttachments(ctx, input.UserID, assistantMessage.ID, string(rawToolEvents))
			if err != nil {
				s.failConversationRunStartedAt(ctx, input.UserID, input.ConversationID, input.UserMessageID, input.RunID, err, input.RunStartedAt)
				return
			}
			assistantMessage = updated
			if _, err := s.appendRunEventStrict(ctx, store.AgentEventInput{
				RunID:          input.RunID,
				UserID:         input.UserID,
				ConversationID: input.ConversationID,
				MessageID:      assistantMessage.ID,
				Type:           "assistant_message_completed",
				Level:          "info",
				Payload: map[string]any{
					"message":           "assistant message attachments updated",
					"assistant_message": assistantMessage,
					"tool_events":       reply.ToolEvents,
				},
			}); err != nil {
				s.failConversationRunStartedAt(context.Background(), input.UserID, input.ConversationID, input.UserMessageID, input.RunID, err, input.RunStartedAt)
				return
			}
		}
	} else {
		var err error
		assistantMessage, err = s.createFinalConversationRunMessage(ctx, input, reply)
		if err != nil {
			s.failConversationRunStartedAt(ctx, input.UserID, input.ConversationID, input.UserMessageID, input.RunID, err, input.RunStartedAt)
			return
		}
	}
	if reply.IncompleteReason == ai.IncompleteReasonBudgetExhausted {
		if err := s.completeConversationRunWithBudgetExhaustion(ctx, input.UserID, input.ConversationID, input.UserMessageID, input.RunID, assistantMessage, reply); err != nil {
			s.failConversationRunStartedAt(context.Background(), input.UserID, input.ConversationID, input.UserMessageID, input.RunID, err, input.RunStartedAt)
		}
		return
	}
	if err := s.ai.ClearRunCheckpoints(ctx, ai.RunOptions{UserID: input.UserID, RunID: input.RunID}); err != nil {
		s.failConversationRunStartedAt(context.Background(), input.UserID, input.ConversationID, input.UserMessageID, input.RunID, err, input.RunStartedAt)
		return
	}
	if _, err := s.publishRunStrict(ctx, input.UserID, input.RunID, store.AgentRunSucceeded, "completed", store.UpdateAgentRunInput{
		AssistantMessageID: assistantMessage.ID,
		SetCompletedAt:     true,
	}); err != nil {
		s.failConversationRunStartedAt(context.Background(), input.UserID, input.ConversationID, input.UserMessageID, input.RunID, err, input.RunStartedAt)
		return
	}
	if _, err := s.appendRunEventStrict(ctx, store.AgentEventInput{
		RunID:          input.RunID,
		UserID:         input.UserID,
		ConversationID: input.ConversationID,
		MessageID:      assistantMessage.ID,
		Type:           "run_completed",
		Level:          "info",
		Payload:        map[string]any{"message": "run completed"},
	}); err != nil {
		s.failConversationRunStartedAt(context.Background(), input.UserID, input.ConversationID, input.UserMessageID, input.RunID, err, input.RunStartedAt)
		return
	}
	s.maybeGenerateConversationTitleAsync(input.UserID, input.UserRole, input.ConversationID, input.UserMessageID, assistantMessage.ID, input.ModelProviderID, input.Content, assistantMessage.Content, input.RunID)
}

func (s *Server) createFinalConversationRunMessage(ctx context.Context, input conversationRunCompletionInput, reply ai.Reply) (store.Message, error) {
	assistantAttachments := "[]"
	if len(reply.ToolEvents) > 0 {
		rawToolEvents, _ := json.Marshal(reply.ToolEvents)
		assistantAttachments = string(rawToolEvents)
	}
	if _, err := s.publishRunStrict(ctx, input.UserID, input.RunID, store.AgentRunRunning, "saving_message", store.UpdateAgentRunInput{}); err != nil {
		return store.Message{}, err
	}
	assistantMessage, err := s.conversations.AppendMessageWithMeta(ctx, store.MessageInput{
		UserID:          input.UserID,
		ConversationID:  input.ConversationID,
		Role:            "assistant",
		Content:         reply.Content,
		AttachmentsJSON: assistantAttachments,
		Status:          "final",
		SegmentIndex:    1,
		MetaJSON:        messageMetaJSON("assistant_segment", 1, input.RunID),
	})
	if err != nil {
		return store.Message{}, err
	}
	if _, err := s.appendRunEventStrict(ctx, store.AgentEventInput{
		RunID:          input.RunID,
		UserID:         input.UserID,
		ConversationID: input.ConversationID,
		MessageID:      assistantMessage.ID,
		Type:           "assistant_message_completed",
		Level:          "info",
		Payload: map[string]any{
			"message":           "assistant message completed",
			"assistant_message": assistantMessage,
			"tool_events":       reply.ToolEvents,
		},
	}); err != nil {
		return store.Message{}, err
	}
	return assistantMessage, nil
}
