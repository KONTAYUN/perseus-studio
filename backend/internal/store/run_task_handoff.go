package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type TaskHandoffResult struct {
	Task    Task
	Message Message
	Run     AgentRun
	Events  []AgentEvent
}

// The worker cannot see a pending task until its receipt and foreground terminal
// event are durable. Retrying a committed handoff reuses its identity.
func (s *AgentRunStore) CompleteTaskHandoff(ctx context.Context, userID, runID string, input CreateTaskInput) (out TaskHandoffResult, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	run, err := scanAgentRun(tx.QueryRowContext(ctx, `SELECT id,user_id,conversation_id,user_message_id,assistant_message_id,model_provider_id,status,stage,error_message,diagnostic_log_id,created_at,started_at,completed_at,updated_at FROM agent_runs WHERE id=? AND user_id=?`, runID, userID))
	if err != nil {
		return out, err
	}
	if run.Status != AgentRunQueued && run.Status != AgentRunRunning && run.Status != AgentRunSucceeded {
		return out, fmt.Errorf("cannot hand off run in %s state", run.Status)
	}
	// Also recover tasks made by older versions before their receipt failed.
	task, err := scanTask(tx.QueryRowContext(ctx, `SELECT id,user_id,type,status,title,payload_json,result_json,error_message,worker_id,retry_count,progress_stage,progress_message,created_at,started_at,finished_at,cancel_requested_at FROM tasks WHERE user_id=? AND type='ai_agent' AND CASE WHEN json_valid(payload_json) THEN json_extract(payload_json,'$.source_run_id') END=? ORDER BY created_at,id LIMIT 1`, userID, runID))
	if errors.Is(err, sql.ErrNoRows) && run.Status != AgentRunSucceeded {
		var payload map[string]any
		if err = json.Unmarshal([]byte(input.PayloadJSON), &payload); err != nil {
			return out, err
		}
		if payload["source_run_id"] != runID || payload["conversation_id"] != run.ConversationID {
			return out, fmt.Errorf("task handoff identity mismatch")
		}
		task = Task{ID: nextStoreID("task", time.Now().UTC()), UserID: userID, Type: "ai_agent", Status: TaskPending, Title: input.Title, PayloadJSON: input.PayloadJSON, CreatedAt: time.Now().UTC()}
		_, err = tx.ExecContext(ctx, `INSERT INTO tasks(id,user_id,type,status,title,payload_json,created_at) VALUES(?,?,?,?,?,?,?)`, task.ID, userID, task.Type, task.Status, task.Title, task.PayloadJSON, task.CreatedAt)
	}
	if err != nil {
		return out, err
	}
	message, err := scanMessage(tx.QueryRowContext(ctx, `SELECT id,user_id,conversation_id,role,content,attachments_json,status,segment_index,meta_json,created_at,updated_at,'' FROM messages WHERE user_id=? AND conversation_id=? AND CASE WHEN json_valid(meta_json) THEN json_extract(meta_json,'$.kind') END='background_task_handoff' AND CASE WHEN json_valid(meta_json) THEN json_extract(meta_json,'$.run_id') END=? ORDER BY created_at LIMIT 1`, userID, run.ConversationID, runID))
	if errors.Is(err, sql.ErrNoRows) && run.Status != AgentRunSucceeded {
		attachments, _ := json.Marshal([]map[string]any{{"type": "task", "task_id": task.ID, "title": task.Title, "status": task.Status}})
		meta, _ := json.Marshal(map[string]any{"kind": "background_task_handoff", "run_id": runID, "task_id": task.ID, "outcome": "success"})
		now := time.Now().UTC()
		message = Message{ID: nextStoreID("msg", now), UserID: userID, ConversationID: run.ConversationID, Role: "assistant", Status: "final", Content: fmt.Sprintf("已创建后台任务：%s\n任务编号：%s\n可以在右上角任务面板查看当前状态与详情。", task.Title, task.ID), AttachmentsJSON: string(attachments), Attachments: attachments, MetaJSON: string(meta), Meta: meta, CreatedAt: now}
		_, err = tx.ExecContext(ctx, `INSERT INTO messages(id,user_id,conversation_id,role,content,attachments_json,status,segment_index,meta_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,0,?,?,?)`, message.ID, userID, run.ConversationID, message.Role, message.Content, message.AttachmentsJSON, message.Status, message.MetaJSON, now, now)
	}
	if err != nil {
		return out, err
	}
	if run.Status == AgentRunSucceeded {
		return TaskHandoffResult{Task: task, Message: message, Run: run}, nil
	}
	for _, event := range []AgentEventInput{
		{Type: "task_handoff_completed", MessageID: run.UserMessageID.String, Payload: map[string]any{"task_id": task.ID, "tool_name": "start_background_task", "status": task.Status}},
		{Type: "assistant_message_completed", MessageID: message.ID, Payload: map[string]any{"assistant_message": message, "task": task}},
	} {
		event.RunID, event.UserID, event.ConversationID, event.TaskID = runID, userID, run.ConversationID, task.ID
		item, eventErr := appendAgentEvent(ctx, tx, event)
		if eventErr != nil {
			return out, eventErr
		}
		out.Events = append(out.Events, item)
	}
	run, terminal, err := completeAgentRun(ctx, tx, userID, runID, CompleteAgentRunInput{AssistantMessageID: message.ID, TaskID: task.ID, Outcome: "success"})
	if err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE conversations SET updated_at=? WHERE id=? AND user_id=?`, time.Now().UTC(), run.ConversationID, userID); err != nil {
		return out, err
	}
	if err = tx.Commit(); err != nil {
		return out, err
	}
	out.Task, out.Message, out.Run = task, message, run
	out.Events = append(out.Events, terminal)
	return out, nil
}
