package api

import (
	"context"
	"encoding/json"

	"webide-backend/internal/ai"
	"webide-backend/internal/store"
	"webide-backend/internal/tasks"
)

func (s *Server) completeConversationRunWithTask(ctx context.Context, userID, userRole, conversationID, userMessageID, modelProviderID, content, attachmentsJSON, runID string, taskRequest *ai.BackgroundTaskRequest) {
	payload, _ := json.Marshal(tasks.AIAgentPayload{
		SourceRunID:     runID,
		ConversationID:  conversationID,
		UserMessageID:   userMessageID,
		UserMessage:     content,
		ModelProviderID: modelProviderID,
		UserRole:        userRole,
		Attachments:     json.RawMessage(attachmentsJSON),
		CreatedFromChat: true,
		Goal:            taskRequest.Goal,
		ExpectedOutputs: taskRequest.ExpectedOutputs,
		SuggestedSteps:  taskRequest.SuggestedSteps,
		DecisionReason:  "model called start_background_task",
	})
	result, err := s.runs.CompleteTaskHandoff(ctx, userID, runID, store.CreateTaskInput{
		Title: taskRequest.Title, PayloadJSON: string(payload),
	})
	if err != nil {
		s.failConversationRun(context.Background(), userID, conversationID, userMessageID, runID, err)
		return
	}
	for _, event := range result.Events {
		s.runBroker.Publish(runID, tasks.Event{Type: "event", Log: event})
	}
	s.runBroker.Publish(runID, tasks.Event{Type: "run", Task: result.Run})
	s.broker.Publish(result.Task.ID, tasks.Event{Type: "task", Task: result.Task})
	if len(result.Events) == 0 {
		return
	}
	assistantMessage := result.Message
	s.maybeGenerateConversationTitleAsync(userID, userRole, conversationID, userMessageID, assistantMessage.ID, modelProviderID, content, assistantMessage.Content, runID)
}
