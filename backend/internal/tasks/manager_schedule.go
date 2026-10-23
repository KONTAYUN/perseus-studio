package tasks

import (
	"context"
	"fmt"
	"time"

	"webide-backend/internal/store"
)

func (m *Manager) executeScheduledTask(ctx context.Context, task store.Task, workerID string) {
	if m.scheduler == nil {
		m.executeTask(ctx, task, workerID)
		return
	}
	done := make(chan struct{})
	id := "task:" + task.ID
	err := m.scheduler.Submit(id, task.UserID, func(runCtx context.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				message := fmt.Sprintf("task execution panic: %v", recovered)
				m.finishTask(context.Background(), task.ID, store.TaskFailed, "failed", message, nil, &message)
			}
		}()
		m.executeTask(runCtx, task, workerID)
	}, func() { close(done) })
	if err != nil {
		message := err.Error()
		m.finishTask(context.Background(), task.ID, store.TaskFailed, "admission_failed", message, nil, &message)
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			m.scheduler.Cancel(id)
			<-done
			return
		case <-ticker.C:
			if canceled, err := m.store.IsCancelRequested(ctx, task.ID); err == nil && canceled {
				m.scheduler.Cancel(id)
				<-done
				m.finishTask(context.Background(), task.ID, store.TaskCanceled, "canceled", "task canceled", nil, nil)
				return
			}
		}
	}
}

func (m *Manager) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
