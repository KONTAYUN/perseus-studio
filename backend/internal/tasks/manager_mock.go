package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"webide-backend/internal/store"
)

func (m *Manager) executeTask(ctx context.Context, task store.Task, workerID string) {
	if task.Type == "ai_agent" {
		m.executeAIAgentTask(ctx, task, workerID)
		return
	}

	payload := MockPayload{
		Steps:   5,
		DelayMS: 1000,
		Message: "mock task running",
	}
	if task.PayloadJSON != "" {
		_ = json.Unmarshal([]byte(task.PayloadJSON), &payload)
	}
	if payload.Steps <= 0 {
		payload.Steps = 5
	}
	if payload.DelayMS <= 0 {
		payload.DelayMS = 1000
	}
	if payload.Message == "" {
		payload.Message = "mock task running"
	}

	m.logAndPublish(ctx, task.UserID, task.ID, "info", fmt.Sprintf("%s on %s", payload.Message, workerID))
	updated, err := m.store.UpdateTaskState(ctx, task.ID, store.UpdateTaskStateInput{
		Status:          store.TaskRunning,
		ProgressStage:   "running",
		ProgressMessage: "task execution started",
		WorkerID:        &workerID,
	})
	if err == nil {
		m.broker.Publish(task.ID, Event{Type: "task", Task: updated})
	}

	for step := 1; step <= payload.Steps; step++ {
		canceled, err := m.store.IsCancelRequested(ctx, task.ID)
		if err == nil && canceled {
			m.logAndPublish(ctx, task.UserID, task.ID, "warn", "task canceled by request")
			m.finishTask(ctx, task.ID, store.TaskCanceled, "canceled", "task canceled", nil, nil)
			return
		}

		stage := fmt.Sprintf("step_%d_of_%d", step, payload.Steps)
		message := fmt.Sprintf("executing step %d/%d", step, payload.Steps)
		current, err := m.store.UpdateTaskState(ctx, task.ID, store.UpdateTaskStateInput{
			Status:          store.TaskRunning,
			ProgressStage:   stage,
			ProgressMessage: message,
		})
		if err == nil {
			m.broker.Publish(task.ID, Event{Type: "task", Task: current})
		}
		m.logAndPublish(ctx, task.UserID, task.ID, "info", message)

		if payload.FailAtStep > 0 && step == payload.FailAtStep {
			errText := fmt.Sprintf("mock failure at step %d", step)
			m.logAndPublish(ctx, task.UserID, task.ID, "error", errText)
			m.finishTask(ctx, task.ID, store.TaskFailed, "failed", errText, nil, &errText)
			return
		}

		select {
		case <-ctx.Done():
			errText := "server shutdown interrupted task"
			m.logAndPublish(context.Background(), task.UserID, task.ID, "warn", errText)
			return
		case <-time.After(time.Duration(payload.DelayMS) * time.Millisecond):
		}
	}

	result := map[string]any{
		"ok":      true,
		"message": "mock task completed",
		"steps":   payload.Steps,
	}
	m.logAndPublish(ctx, task.UserID, task.ID, "info", "task completed")
	m.finishTask(ctx, task.ID, store.TaskDone, "done", "task completed", m.store.MarshalResult(result), nil)
}
