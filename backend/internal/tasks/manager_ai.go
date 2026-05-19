package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"webide-backend/internal/store"
)

func (m *Manager) executeAIAgentTask(ctx context.Context, task store.Task, workerID string) {
	taskCtx, cancelTaskCtx := context.WithCancel(ctx)
	defer cancelTaskCtx()
	cancelWatcherDone := make(chan struct{})
	go func() {
		defer close(cancelWatcherDone)
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-taskCtx.Done():
				return
			case <-ticker.C:
				canceled, err := m.store.IsCancelRequested(context.Background(), task.ID)
				if err == nil && canceled {
					cancelTaskCtx()
					return
				}
			}
		}
	}()
	defer func() {
		cancelTaskCtx()
		<-cancelWatcherDone
	}()

	userID := task.UserID

	files, err := m.factory.Workspace(userID)
	if err != nil {
		errText := "failed to resolve user workspace: " + err.Error()
		m.logAndPublish(taskCtx, userID, task.ID, "error", errText)
		m.finishTask(taskCtx, task.ID, store.TaskFailed, "failed", errText, nil, &errText)
		return
	}

	var payload AIAgentPayload
	if err := json.Unmarshal([]byte(task.PayloadJSON), &payload); err != nil {
		errText := "invalid ai_agent payload: " + err.Error()
		m.logAndPublish(taskCtx, userID, task.ID, "error", errText)
		m.finishTask(taskCtx, task.ID, store.TaskFailed, "failed", errText, nil, &errText)
		return
	}
	m.logAndPublish(taskCtx, userID, task.ID, "info", "AI task started")
	m.logAndPublish(taskCtx, userID, task.ID, "info", "decision result: background_task; reason: "+payload.DecisionReason)
	updated, err := m.store.UpdateTaskState(taskCtx, task.ID, store.UpdateTaskStateInput{
		Status:          store.TaskRunning,
		ProgressStage:   "ai_started",
		ProgressMessage: "AI task started",
		WorkerID:        &workerID,
	})
	if err == nil {
		m.broker.Publish(task.ID, Event{Type: "task", Task: updated})
	}

	if canceled, err := m.store.IsCancelRequested(taskCtx, task.ID); err == nil && canceled {
		m.logAndPublish(taskCtx, userID, task.ID, "warn", "task canceled by request")
		m.finishTask(taskCtx, task.ID, store.TaskCanceled, "canceled", "task canceled", nil, nil)
		return
	}

	m.updateAndPublish(taskCtx, task.ID, store.TaskRunning, "calling_model", "calling model")
	m.logPlan(taskCtx, userID, task.ID, payload)
	reply, err := m.runAIAgentOnce(taskCtx, userID, task.ID, payload, m.executionPrompt(payload), "")
	if err != nil {
		if taskCtx.Err() != nil {
			m.logAndPublish(context.Background(), userID, task.ID, "warn", "task canceled by request")
			m.finishTask(context.Background(), task.ID, store.TaskCanceled, "canceled", "task canceled", nil, nil)
			return
		}
		m.failAIAgentTask(taskCtx, userID, task.ID, payload.ConversationID, "AI task failed: "+err.Error())
		return
	}

	var validation validationResult
	for repairRound := 0; repairRound <= 3; repairRound++ {
		validation = m.validateOutputs(taskCtx, payload, reply, files)
		if validation.OK {
			m.logAndPublish(taskCtx, userID, task.ID, "info", "validation passed")
			break
		}
		m.logAndPublish(taskCtx, userID, task.ID, "warn", "validation failed: "+strings.Join(validation.Missing, "; "))
		if repairRound == 3 {
			m.failAIAgentTask(taskCtx, userID, task.ID, payload.ConversationID, "validation failed after repair: "+strings.Join(validation.Missing, "; "))
			return
		}
		m.updateAndPublish(taskCtx, task.ID, store.TaskRunning, "repair", fmt.Sprintf("repairing missing outputs %d/3", repairRound+1))
		reply, err = m.runAIAgentOnce(taskCtx, userID, task.ID, payload, m.repairPrompt(payload, validation.Missing, repairRound+1), repairForceTool(validation.Missing))
		if err != nil {
			if taskCtx.Err() != nil {
				m.logAndPublish(context.Background(), userID, task.ID, "warn", "task canceled by request")
				m.finishTask(context.Background(), task.ID, store.TaskCanceled, "canceled", "task canceled", nil, nil)
				return
			}
			m.failAIAgentTask(taskCtx, userID, task.ID, payload.ConversationID, "AI repair failed: "+err.Error())
			return
		}
	}

	m.logAndPublish(taskCtx, userID, task.ID, "info", "writing assistant message")
	assistantAttachments := "[]"
	if len(reply.ToolEvents) > 0 {
		rawToolEvents, _ := json.Marshal(reply.ToolEvents)
		assistantAttachments = string(rawToolEvents)
	}
	if _, err := m.conversations.AppendMessage(taskCtx, userID, payload.ConversationID, "assistant", reply.Content, assistantAttachments); err != nil {
		m.failAIAgentTask(taskCtx, userID, task.ID, payload.ConversationID, "failed to write assistant message: "+err.Error())
		return
	}

	result := map[string]any{
		"ok":              true,
		"conversation_id": payload.ConversationID,
		"message":         "AI task completed",
	}
	m.logAndPublish(taskCtx, userID, task.ID, "info", "task done")
	m.finishTask(taskCtx, task.ID, store.TaskDone, "done", "task completed", m.store.MarshalResult(result), nil)
}
