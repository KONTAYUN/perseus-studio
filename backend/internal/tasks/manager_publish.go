package tasks

import (
	"context"
	"database/sql"
	"errors"

	"webide-backend/internal/store"
)

func (m *Manager) updateAndPublish(ctx context.Context, taskID string, status store.TaskStatus, stage, message string) {
	task, err := m.store.UpdateTaskState(ctx, taskID, store.UpdateTaskStateInput{
		Status:          status,
		ProgressStage:   stage,
		ProgressMessage: message,
	})
	if err == nil {
		m.broker.Publish(taskID, Event{Type: "task", Task: task})
	}
}

func (m *Manager) finishTask(ctx context.Context, taskID string, status store.TaskStatus, stage, message string, resultJSON *string, errText *string) {
	task, err := m.store.UpdateTaskState(ctx, taskID, store.UpdateTaskStateInput{
		Status:          status,
		ProgressStage:   stage,
		ProgressMessage: message,
		ResultJSON:      resultJSON,
		ErrorMessage:    errText,
		SetFinishedAt:   true,
	})
	if err != nil {
		return
	}
	m.broker.Publish(taskID, Event{Type: "task", Task: task})
}

func (m *Manager) logAndPublish(ctx context.Context, userID, taskID, level, message string) {
	logItem, err := m.store.AppendLog(ctx, userID, taskID, level, message)
	if err != nil {
		return
	}
	m.broker.Publish(taskID, Event{Type: "log", Log: logItem})
}

func (m *Manager) publishTask(ctx context.Context, taskID string) {
	task, err := m.store.GetTaskByID(ctx, taskID)
	if err != nil {
		return
	}
	m.broker.Publish(taskID, Event{Type: "task", Task: task})
}

func IsNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
