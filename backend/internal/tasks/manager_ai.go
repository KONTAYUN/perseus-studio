package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"webide-backend/internal/ai"
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
	model := ai.ModelIdentity{ProviderID: payload.ModelProviderID}
	reply, err := m.runAIAgentOnce(taskCtx, userID, task.ID, payload, m.executionPrompt(payload), "", 0, &model)
	if err != nil {
		if taskCtx.Err() != nil {
			m.logAndPublish(context.Background(), userID, task.ID, "warn", "task canceled by request")
			m.finishTask(context.Background(), task.ID, store.TaskCanceled, "canceled", "task canceled", nil, nil)
			return
		}
		m.failAIAgentTask(taskCtx, userID, task.ID, payload, model, "AI task failed: "+err.Error())
		return
	}

	var validation validationResult
	var repairs int
	for repairRound := 0; repairRound <= 3; repairRound++ {
		validation = m.validateOutputs(taskCtx, payload, reply, files)
		if validation.OK {
			m.logAndPublish(taskCtx, userID, task.ID, "info", "validation passed")
			break
		}
		// An honest partial/blocked result is a completed execution, not a
		// request to force the model through the same unsuccessful work again.
		if reply.Outcome == "partial" || reply.Outcome == "blocked" || reply.IncompleteReason != "" {
			break
		}
		m.logAndPublish(taskCtx, userID, task.ID, "warn", "validation failed: "+strings.Join(validation.Missing, "; "))
		if repairRound == 3 {
			m.failAIAgentTask(taskCtx, userID, task.ID, payload, model, "validation failed after repair: "+strings.Join(validation.Missing, "; "))
			return
		}
		m.updateAndPublish(taskCtx, task.ID, store.TaskRunning, "repair", fmt.Sprintf("repairing missing outputs %d/3", repairRound+1))
		repairs = repairRound + 1
		reply, err = m.runAIAgentOnce(taskCtx, userID, task.ID, payload, m.repairPrompt(payload, validation.Missing, repairs), repairForceTool(validation), repairs, &model)
		if err != nil {
			if taskCtx.Err() != nil {
				m.logAndPublish(context.Background(), userID, task.ID, "warn", "task canceled by request")
				m.finishTask(context.Background(), task.ID, store.TaskCanceled, "canceled", "task canceled", nil, nil)
				return
			}
			m.failAIAgentTask(taskCtx, userID, task.ID, payload, model, "AI repair failed: "+err.Error())
			return
		}
	}

	m.logAndPublish(taskCtx, userID, task.ID, "info", "writing assistant message")
	assistantAttachments := "[]"
	if len(reply.ToolEvents) > 0 {
		rawToolEvents, _ := json.Marshal(reply.ToolEvents)
		assistantAttachments = string(rawToolEvents)
	}
	meta, _ := json.Marshal(map[string]any{"kind": "background_task_result", "task_id": task.ID,
		"source_run_id": payload.SourceRunID, "outcome": reply.Outcome, "incomplete_reason": reply.IncompleteReason})
	assistant, err := m.conversations.AppendMessageWithMeta(taskCtx, store.MessageInput{
		UserID: userID, ConversationID: payload.ConversationID, Role: "assistant", Content: reply.Content,
		AttachmentsJSON: assistantAttachments, MetaJSON: string(meta), Status: "final",
	})
	if err != nil {
		m.failAIAgentTask(taskCtx, userID, task.ID, payload, model, "failed to write assistant message: "+err.Error())
		return
	}

	result := map[string]any{
		"ok":                   validation.OK && reply.Outcome == "success" && reply.IncompleteReason == "",
		"conversation_id":      payload.ConversationID,
		"message":              "AI task completed",
		"assistant_message_id": assistant.ID,
		"task_id":              task.ID,
		"source_run_id":        payload.SourceRunID,
		"outcome":              reply.Outcome,
		"incomplete_reason":    reply.IncompleteReason,
		"recoverable":          reply.Recoverable,
		"budget":               reply.Budget,
		"repair_rounds":        repairs,
		"validation":           validation,
	}
	stage, message := "done", "task completed"
	if reply.Outcome != "success" || !validation.OK || reply.IncompleteReason != "" {
		stage, message = "completed_with_issues", "执行已结束，存在未完成事项，请查看最终回复。"
	}
	m.logAndPublish(taskCtx, userID, task.ID, "info", message)
	if err := m.finishTask(taskCtx, task.ID, store.TaskDone, stage, message, m.store.MarshalResult(result), nil); err != nil {
		m.failAIAgentTask(context.Background(), userID, task.ID, payload, model, "task completion persistence failed: "+err.Error())
	}
}
