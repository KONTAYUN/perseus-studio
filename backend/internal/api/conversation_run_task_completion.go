package api

import (
	"context"
	"encoding/json"
	"fmt"

	"webide-backend/internal/ai"
	"webide-backend/internal/store"
	"webide-backend/internal/tasks"
)

func (s *Server) completeConversationRunWithTask(ctx context.Context, userID, userRole, conversationID, userMessageID, modelProviderID, content, attachmentsJSON, runID string, taskRequest *ai.BackgroundTaskRequest) {
	payload, _ := json.Marshal(tasks.AIAgentPayload{
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
	if _, err := s.appendRunEventStrict(ctx, store.AgentEventInput{
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
	}); err != nil {
		s.failConversationRun(context.Background(), userID, conversationID, userMessageID, runID, err)
		return
	}
	if err := s.ai.ClearRunCheckpoints(ctx, ai.RunOptions{UserID: userID, RunID: runID}); err != nil {
		s.failConversationRun(context.Background(), userID, conversationID, userMessageID, runID, err)
		return
	}
	if _, err := s.appendRunEventStrict(ctx, store.AgentEventInput{
		RunID:          runID,
		UserID:         userID,
		ConversationID: conversationID,
		MessageID:      assistantMessage.ID,
		TaskID:         task.ID,
		Type:           "run_delegated",
		Level:          "info",
		Payload:        map[string]any{"message": "run delegated", "task_id": task.ID},
	}); err != nil {
		s.failConversationRun(context.Background(), userID, conversationID, userMessageID, runID, err)
		return
	}
	if _, err := s.publishRunStrict(ctx, userID, runID, store.AgentRunDelegated, "delegated", store.UpdateAgentRunInput{
		AssistantMessageID: assistantMessage.ID,
		SetCompletedAt:     true,
	}); err != nil {
		s.failConversationRun(context.Background(), userID, conversationID, userMessageID, runID, err)
		return
	}
	s.maybeGenerateConversationTitleAsync(userID, userRole, conversationID, userMessageID, assistantMessage.ID, modelProviderID, content, assistantMessage.Content, runID)
}
