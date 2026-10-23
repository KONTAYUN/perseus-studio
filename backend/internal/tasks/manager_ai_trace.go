package tasks

import (
	"context"
	"fmt"
	"sync"
	"time"

	"webide-backend/internal/ai"
	"webide-backend/internal/failure"
	"webide-backend/internal/idgen"
	"webide-backend/internal/store"
)

// Task-owned traces stay separate from the foreground run that handed work off.
// A fresh execution identity separates validation repairs within the same task.
type taskExecutionTrace struct {
	manager     *Manager
	ctx         context.Context
	cancel      context.CancelCauseFunc
	userID      string
	taskID      string
	executionID string
	mu          sync.Mutex
	err         error
}

func (t *taskExecutionTrace) append(kind, level string, payload any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.err != nil {
		return
	}
	ctx := t.ctx
	// Cancellation must still leave an auditable last attempt/termination.
	if ctx.Err() != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
	}
	event, err := t.manager.store.AppendEvent(ctx, store.TaskEventInput{UserID: t.userID,
		TaskID: t.taskID, ExecutionID: t.executionID, Type: kind, Level: level, Payload: payload})
	if err != nil {
		t.err = failure.New("local_task_persistence", fmt.Sprintf("save task event %s: %v", kind, err), err)
		t.cancel(t.err)
		return
	}
	t.manager.broker.Publish(t.taskID, Event{Type: "event", Log: event})
}

func (t *taskExecutionTrace) failure() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.err
}

func (m *Manager) runAIAgentOnce(ctx context.Context, userID, taskID string, payload AIAgentPayload, prompt string, forceTool string, repairRound int, model *ai.ModelIdentity) (ai.Reply, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	trace := &taskExecutionTrace{manager: m, ctx: ctx, cancel: cancel, userID: userID,
		taskID: taskID, executionID: idgen.New("task_execution")}
	trace.append("task_execution_started", "info", map[string]any{"conversation_id": payload.ConversationID,
		"user_message_id": payload.UserMessageID, "source_run_id": payload.SourceRunID, "repair_round": repairRound})
	if err := trace.failure(); err != nil {
		return ai.Reply{}, err
	}
	opts := ai.RunOptions{UserID: userID, UserRole: payload.UserRole, TaskID: taskID, ForceTool: forceTool,
		OnModelResolved: func(identity ai.ModelIdentity) { *model = identity },
		Log: func(level, message string) {
			trace.append("task_log", level, map[string]any{"message": message})
			if trace.failure() == nil {
				m.logAndPublish(ctx, userID, taskID, level, message)
			}
		},
		Event: func(kind, level string, data map[string]any) {
			trace.append(kind, level, data)
			if trace.failure() != nil {
				return
			}
			if message := taskAgentEventMessage(kind, data); message != "" {
				m.logAndPublish(ctx, userID, taskID, level, message)
				switch kind {
				case "tool_call_started":
					m.updateAndPublish(ctx, taskID, store.TaskRunning, "tool_call", message)
				case "completion_rejected":
					m.updateAndPublish(ctx, taskID, store.TaskRunning, "repair", message)
				case "model_message_started":
					m.updateAndPublish(ctx, taskID, store.TaskRunning, "calling_model", message)
				}
			}
		},
		OnAssistantMessageStarted: func(id, content string, index int) {
			trace.append("assistant_message_started", "info", map[string]any{"message_id": id, "content": content, "segment_index": index})
		},
		OnAssistantMessageDelta: func(id, delta string) {
			trace.append("assistant_message_delta", "info", map[string]any{"message_id": id, "delta": delta})
		},
		OnAssistantMessageCompleted: func(id, content string, index int) {
			trace.append("assistant_message_completed", "info", map[string]any{"message_id": id, "content": content, "segment_index": index})
		},
		OnAssistantMessageDiscarded: func(id, reason string, index int) {
			trace.append("assistant_message_discarded", "info", map[string]any{"message_id": id, "reason": reason, "segment_index": index})
		},
		OnCompletionResultDelta: func(delta string) {
			trace.append("completion_result_delta", "info", map[string]any{"delta": delta})
		},
		OnCompletionResultDiscard: func() {
			trace.append("completion_result_discarded", "info", map[string]any{"reason": "completion_rejected"})
		},
	}
	messages, err := m.taskMessages(ctx, userID, payload, prompt)
	if err != nil {
		return ai.Reply{}, err
	}
	*model = ai.ModelIdentity{ProviderID: payload.ModelProviderID}
	reply, err := m.ai.GenerateTaskReply(ctx, payload.ConversationID, payload.UserMessageID, payload.ModelProviderID, messages, nil, opts)
	if persistErr := trace.failure(); persistErr != nil {
		return ai.Reply{}, persistErr
	}
	if err != nil {
		trace.append("task_execution_failed", "error", map[string]any{"error": err.Error(), "canceled": ctx.Err() != nil})
	} else {
		trace.append("task_execution_completed", "info", map[string]any{"outcome": reply.Outcome,
			"incomplete_reason": reply.IncompleteReason, "recoverable": reply.Recoverable, "budget": reply.Budget})
	}
	if persistErr := trace.failure(); persistErr != nil {
		return ai.Reply{}, persistErr
	}
	return reply, err
}
