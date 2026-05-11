package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"webide-backend/internal/ai"
	"webide-backend/internal/store"
	"webide-backend/internal/usersvc"
	"webide-backend/internal/workspace"
)

type Config struct {
	WorkerCount  int
	PollInterval time.Duration
}

type Manager struct {
	store         *store.TaskStore
	conversations *store.ConversationStore
	diagnostics   *store.DiagnosticStore
	ai            *ai.Service
	factory       *usersvc.Factory
	broker        *Broker
	cfg           Config
	wg            sync.WaitGroup
}

type MockPayload struct {
	Steps      int    `json:"steps"`
	DelayMS    int    `json:"delay_ms"`
	FailAtStep int    `json:"fail_at_step"`
	Message    string `json:"message"`
}

type AIAgentPayload struct {
	ConversationID  string              `json:"conversation_id"`
	UserMessageID   string              `json:"user_message_id"`
	UserMessage     string              `json:"user_message"`
	ModelProviderID string              `json:"model_provider_id"`
	Attachments     json.RawMessage     `json:"attachments"`
	CreatedFromChat bool                `json:"created_from_chat"`
	DecisionReason  string              `json:"decision_reason"`
	Goal            string              `json:"goal"`
	ExpectedOutputs []ai.ExpectedOutput `json:"expected_outputs"`
	SuggestedSteps  []string            `json:"suggested_steps"`
}

func NewManager(taskStore *store.TaskStore, conversationStore *store.ConversationStore, diagnosticStore *store.DiagnosticStore, aiService *ai.Service, factory *usersvc.Factory, broker *Broker, cfg Config) *Manager {
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = 2
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 1500 * time.Millisecond
	}
	return &Manager{
		store:         taskStore,
		conversations: conversationStore,
		diagnostics:   diagnosticStore,
		ai:            aiService,
		factory:       factory,
		broker:        broker,
		cfg:           cfg,
	}
}

func (m *Manager) Start(ctx context.Context) {
	for index := 0; index < m.cfg.WorkerCount; index++ {
		workerID := fmt.Sprintf("worker-%d", index+1)
		m.wg.Add(1)
		go func(id string) {
			defer m.wg.Done()
			m.runWorker(ctx, id)
		}(workerID)
	}
}

func (m *Manager) runWorker(ctx context.Context, workerID string) {
	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		task, err := m.store.ClaimNextTask(ctx, workerID)
		if err != nil {
			time.Sleep(m.cfg.PollInterval)
			continue
		}
		if task == nil {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				continue
			}
		}

		m.publishTask(ctx, task.ID)
		m.executeTask(ctx, *task, workerID)
	}
}

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

func (m *Manager) failAIAgentTask(ctx context.Context, userID, taskID, conversationID, errText string) {
	m.logAndPublish(ctx, userID, taskID, "error", errText)
	if m.diagnostics != nil {
		_, _ = m.diagnostics.Append(ctx, store.DiagnosticLogInput{
			UserID:         userID,
			ConversationID: conversationID,
			TaskID:         taskID,
			Stage:          "background_task_failed",
			Level:          "error",
			ErrorMessage:   errText,
		})
	}
	if conversationID != "" {
		_, _ = m.conversations.AppendMessage(ctx, userID, conversationID, "assistant", "后台任务失败："+errText, "[]")
	}
	m.finishTask(ctx, taskID, store.TaskFailed, "failed", errText, nil, &errText)
}

type validationResult struct {
	OK      bool
	Missing []string
}

func (m *Manager) logPlan(ctx context.Context, userID, taskID string, payload AIAgentPayload) {
	goal := strings.TrimSpace(payload.Goal)
	if goal == "" {
		goal = strings.TrimSpace(payload.UserMessage)
	}
	m.logAndPublish(ctx, userID, taskID, "info", "plan goal: "+goal)
	for index, step := range payload.SuggestedSteps {
		if strings.TrimSpace(step) != "" {
			m.logAndPublish(ctx, userID, taskID, "info", fmt.Sprintf("plan step %d: %s", index+1, step))
		}
	}
	for index, output := range payload.ExpectedOutputs {
		m.logAndPublish(ctx, userID, taskID, "info", fmt.Sprintf("expected output %d: %s %s %s", index+1, output.Type, output.Path, output.Description))
	}
}

func (m *Manager) runAIAgentOnce(ctx context.Context, userID, taskID string, payload AIAgentPayload, prompt string, forceTool string) (ai.Reply, error) {
	messages := []store.Message{{
		ID:             payload.UserMessageID,
		ConversationID: payload.ConversationID,
		Role:           "user",
		Content:        prompt,
	}}
	return m.ai.GenerateTaskReply(ctx, payload.ConversationID, payload.UserMessageID, payload.ModelProviderID, messages, nil, ai.RunOptions{
		UserID:    userID,
		TaskID:    taskID,
		ForceTool: forceTool,
		Log: func(level, message string) {
			m.logAndPublish(ctx, userID, taskID, level, message)
			if strings.HasPrefix(message, "tool call started") {
				m.updateAndPublish(ctx, taskID, store.TaskRunning, "tool_call", message)
			}
		},
		Event: func(eventType, level string, payload map[string]any) {
			message := taskAgentEventMessage(eventType, payload)
			if message == "" {
				return
			}
			m.logAndPublish(ctx, userID, taskID, level, message)
			switch eventType {
			case "tool_call_started":
				m.updateAndPublish(ctx, taskID, store.TaskRunning, "tool_call", message)
			case "completion_rejected":
				m.updateAndPublish(ctx, taskID, store.TaskRunning, "repair", message)
			case "model_message_started":
				m.updateAndPublish(ctx, taskID, store.TaskRunning, "calling_model", message)
			}
		},
	})
}

func taskAgentEventMessage(eventType string, payload map[string]any) string {
	toolName, _ := payload["tool_name"].(string)
	switch eventType {
	case "model_message_started":
		return "model turn started"
	case "tool_call_started":
		if toolName != "" {
			return "tool call started: " + toolName
		}
		return "tool call started"
	case "tool_call_completed":
		if toolName != "" {
			return "tool call completed: " + toolName
		}
		return "tool call completed"
	case "tool_call_failed":
		if errText, _ := payload["error"].(string); errText != "" {
			return "tool call failed: " + errText
		}
		return "tool call failed"
	case "tool_result_returned":
		if isError, _ := payload["is_error"].(bool); isError {
			return "tool result returned with error"
		}
	case "completion_rejected":
		if reason, _ := payload["reason"].(string); reason != "" {
			return "completion rejected: " + reason
		}
		return "completion rejected"
	}
	return ""
}

func (m *Manager) executionPrompt(payload AIAgentPayload) string {
	goal := strings.TrimSpace(payload.Goal)
	if goal == "" {
		goal = strings.TrimSpace(payload.UserMessage)
	}
	rawOutputs, _ := json.MarshalIndent(payload.ExpectedOutputs, "", "  ")
	rawSteps, _ := json.MarshalIndent(payload.SuggestedSteps, "", "  ")
	return fmt.Sprintf(`你正在执行 Perseus Studio 后台任务。请按目标和计划完成真实操作，必要时使用 perseus_search_web、web_extract、workspace 文件工具、命令工具或 Skill 工具。

目标：
%s

原始用户请求：
%s

预期产物 JSON：
%s

建议步骤 JSON：
%s

要求：
- 如果预期产物包含 file，必须实际创建或写入该 workspace 相对路径；不要只口头说明。
- 如果没有明确文件路径，请为报告/Markdown 选择一个简洁中文或英文文件名，放在 workspace 根目录。
- 修改已有文件前优先 read_workspace_text；需要项目结构时先 inspect_workspace。
- 命令只在构建、测试、检查、批处理或脚本执行需要时使用；命令失败要说明关键错误。
- 使用联网资料时，最终内容里保留来源 URL。
- 完成前确保预期文件真实存在且非空；完成后调用 attempt_completion，用中文简短总结实际产物。`, goal, payload.UserMessage, string(rawOutputs), string(rawSteps))
}

func (m *Manager) repairPrompt(payload AIAgentPayload, missing []string, round int) string {
	rawOutputs, _ := json.MarshalIndent(payload.ExpectedOutputs, "", "  ")
	return fmt.Sprintf(`后台任务产物校验失败，需要补做第 %d 轮。

缺失或不合格的产物：
%s

预期产物 JSON：
%s

请只补做缺失项。若缺少文件，必须实际调用 save_workspace_file 写入 workspace；如果写入失败，要基于工具错误继续修复或明确失败，不要只回复说明。`, round, strings.Join(missing, "\n"), string(rawOutputs))
}

func (m *Manager) validateOutputs(ctx context.Context, payload AIAgentPayload, reply ai.Reply, files *workspace.Service) validationResult {
	_ = ctx
	var missing []string
	outputs := payload.ExpectedOutputs
	if len(outputs) == 0 {
		outputs = []ai.ExpectedOutput{{Type: "message", Description: "任务完成说明"}}
	}
	for _, output := range outputs {
		switch output.Type {
		case "file":
			path := strings.TrimSpace(output.Path)
			if path == "" {
				path = firstChangedFile(reply.ToolEvents)
				if path == "" {
					missing = append(missing, "file output has no path and no created/modified file was recorded")
					continue
				}
			}
			file, err := files.ReadFile(path)
			if err != nil {
				missing = append(missing, "missing file: "+path)
				continue
			}
			if file.Size <= 0 || strings.TrimSpace(file.Content) == "" {
				missing = append(missing, "empty file: "+path)
			}
		case "message":
			if strings.TrimSpace(reply.Content) == "" {
				missing = append(missing, "missing assistant message: "+output.Description)
			}
		case "command_result":
			if !hasCommandResult(reply.ToolEvents) {
				missing = append(missing, "missing command result: "+output.Description)
			}
		}
	}
	return validationResult{OK: len(missing) == 0, Missing: missing}
}

func firstChangedFile(events []ai.ToolEvent) string {
	for _, event := range events {
		if event.Status != "success" {
			continue
		}
		if event.ToolName == "save_workspace_file" {
			if event.Path != "" {
				return event.Path
			}
			if len(event.ChangedPaths) > 0 {
				return event.ChangedPaths[0]
			}
		}
	}
	return ""
}

func hasCommandResult(events []ai.ToolEvent) bool {
	for _, event := range events {
		if event.ToolName == "run_workspace_command" && event.ExitCode != nil {
			return true
		}
	}
	return false
}

func repairForceTool(missing []string) string {
	for _, item := range missing {
		if strings.Contains(item, "file") || strings.Contains(item, "文件") {
			return "save_workspace_file"
		}
	}
	return ""
}

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
