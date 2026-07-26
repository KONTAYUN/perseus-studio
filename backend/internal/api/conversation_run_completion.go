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
	presentationEvents := reply.PresentationEvents
	presentationKind := assistantPresentationKind(reply)
	if assistantMessage.ID != "" {
		rawPresentationEvents, _ := json.Marshal(presentationEvents)
		updated, err := s.conversations.UpdateMessagePresentation(
			ctx,
			input.UserID,
			assistantMessage.ID,
			string(rawPresentationEvents),
			messageMetaJSON(presentationKind, assistantMessage.SegmentIndex, input.RunID),
		)
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
			Type:           "assistant_message_presentation_updated",
			Level:          "info",
			Payload: map[string]any{
				"message":             "assistant message presentation updated",
				"assistant_message":   assistantMessage,
				"presentation_events": presentationEvents,
				"presentation_kind":   presentationKind,
			},
		}); err != nil {
			s.failConversationRunStartedAt(context.Background(), input.UserID, input.ConversationID, input.UserMessageID, input.RunID, err, input.RunStartedAt)
			return
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
	runStatus := store.AgentRunSucceeded
	runStage := "completed"
	setCompletedAt := true
	terminalState := string(reply.TerminalState)
	if terminalState == "" {
		terminalState = "completed"
	}
	if reply.TerminalState == ai.AgentTerminationWaitingForUser {
		runStatus = store.AgentRunWaiting
		runStage = "waiting_for_user"
		setCompletedAt = false
	} else if err := s.ai.ClearRunCheckpoints(ctx, ai.RunOptions{UserID: input.UserID, RunID: input.RunID}); err != nil {
		s.failConversationRunStartedAt(context.Background(), input.UserID, input.ConversationID, input.UserMessageID, input.RunID, err, input.RunStartedAt)
		return
	}
	if _, err := s.publishRunStrict(ctx, input.UserID, input.RunID, runStatus, runStage, store.UpdateAgentRunInput{
		AssistantMessageID: assistantMessage.ID,
		SetCompletedAt:     setCompletedAt,
	}); err != nil {
		s.failConversationRunStartedAt(context.Background(), input.UserID, input.ConversationID, input.UserMessageID, input.RunID, err, input.RunStartedAt)
		return
	}
	if _, err := s.appendRunEventStrict(ctx, store.AgentEventInput{
		RunID:          input.RunID,
		UserID:         input.UserID,
		ConversationID: input.ConversationID,
		MessageID:      assistantMessage.ID,
		Type:           map[bool]string{true: "run_waiting_for_user", false: "run_completed"}[reply.TerminalState == ai.AgentTerminationWaitingForUser],
		Level:          "info",
		Payload:        map[string]any{"message": "run completed", "terminal_state": terminalState},
	}); err != nil {
		s.failConversationRunStartedAt(context.Background(), input.UserID, input.ConversationID, input.UserMessageID, input.RunID, err, input.RunStartedAt)
		return
	}
	if reply.TerminalState != ai.AgentTerminationWaitingForUser {
		s.maybeGenerateConversationTitleAsync(input.UserID, input.UserRole, input.ConversationID, input.UserMessageID, assistantMessage.ID, input.ModelProviderID, input.Content, assistantMessage.Content, input.RunID)
	}
}

func (s *Server) createFinalConversationRunMessage(ctx context.Context, input conversationRunCompletionInput, reply ai.Reply) (store.Message, error) {
	assistantAttachments := "[]"
	if len(reply.PresentationEvents) > 0 {
		rawToolEvents, _ := json.Marshal(reply.PresentationEvents)
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
		MetaJSON:        messageMetaJSON(assistantPresentationKind(reply), 1, input.RunID),
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
			"message":             "assistant message completed",
			"assistant_message":   assistantMessage,
			"presentation_events": reply.PresentationEvents,
		},
	}); err != nil {
		return store.Message{}, err
	}
	return assistantMessage, nil
}

func assistantPresentationKind(reply ai.Reply) string {
	switch reply.TerminalState {
	case ai.AgentTerminationWaitingForUser:
		for index := len(reply.PresentationEvents) - 1; index >= 0; index-- {
			switch reply.PresentationEvents[index].ToolName {
			case "request_user_input":
				return "user_input_request"
			case "request_user_choice":
				return "user_choice_request"
			case "ask_followup_question":
				return "followup_question"
			}
		}
		return "user_input_request"
	case ai.AgentTerminationCompleted:
		if reply.IncompleteReason == "" {
			return "completion_result"
		}
	}
	return "assistant_segment"
}
