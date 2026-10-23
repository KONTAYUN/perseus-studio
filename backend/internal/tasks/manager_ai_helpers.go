package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"webide-backend/internal/ai"
	"webide-backend/internal/store"
	"webide-backend/internal/workspace"
)

func (m *Manager) failAIAgentTask(ctx context.Context, userID, taskID string, payload AIAgentPayload, model ai.ModelIdentity, errText string) {
	conversationID := payload.ConversationID
	m.logAndPublish(ctx, userID, taskID, "error", errText)
	if m.diagnostics != nil {
		_, _ = m.diagnostics.Append(ctx, store.DiagnosticLogInput{
			UserID:          userID,
			ConversationID:  conversationID,
			TaskID:          taskID,
			MessageID:       payload.UserMessageID,
			ModelProviderID: model.ProviderID,
			ModelName:       model.ProviderName,
			ProviderName:    model.ProviderName,
			ModelID:         model.ModelID,
			Protocol:        model.Protocol,
			Stage:           "background_task_failed",
			Level:           "error",
			ErrorMessage:    errText,
		})
	}
	if conversationID != "" {
		meta, _ := json.Marshal(map[string]any{"kind": "background_task_result", "task_id": taskID, "execution_status": "failed"})
		_, _ = m.conversations.AppendMessageWithMeta(ctx, store.MessageInput{UserID: userID, ConversationID: conversationID,
			Role: "assistant", Content: "后台任务失败：" + errText, AttachmentsJSON: "[]", Status: "final", MetaJSON: string(meta)})
	}
	m.finishTask(ctx, taskID, store.TaskFailed, "failed", errText, nil, &errText)
}

type validationResult struct {
	OK           bool     `json:"ok"`
	Missing      []string `json:"missing,omitempty"`
	MissingKinds []string `json:"missing_kinds,omitempty"`
}

const (
	missingOutputFile    = "file"
	missingOutputMessage = "message"
	missingOutputCommand = "command_result"
)

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
- 文件内容变更须调用编辑工具：%s
- 命令只在构建、测试、检查、批处理或脚本执行需要时使用；命令失败要说明关键错误。
- 使用联网资料时，最终内容里保留来源 URL。
- 完成前回读预期文件，确保真实存在且非空；完成后调用 attempt_completion，用中文简短总结实际产物。`, goal, payload.UserMessage, string(rawOutputs), string(rawSteps), ai.WorkspacePatchExecutionRule)
}

func (m *Manager) repairPrompt(payload AIAgentPayload, missing []string, round int) string {
	rawOutputs, _ := json.MarshalIndent(payload.ExpectedOutputs, "", "  ")
	return fmt.Sprintf(`后台任务产物校验失败，需要补做第 %d 轮。

缺失或不合格的产物：
%s

预期产物 JSON：
%s

请只补做缺失项。若缺少文件，必须实际创建或写入 workspace 文件。%s
写入后回读核验。如果写入失败，要基于工具错误继续修复或明确失败，不要只回复说明。`, round, strings.Join(missing, "\n"), string(rawOutputs), ai.WorkspacePatchExecutionRule)
}

func (m *Manager) validateOutputs(ctx context.Context, payload AIAgentPayload, reply ai.Reply, files *workspace.Service) validationResult {
	_ = ctx
	result := validationResult{}
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
					result.addMissing(missingOutputFile, "file output has no path and no created/modified file was recorded")
					continue
				}
			}
			file, err := files.ReadFile(path)
			if err != nil {
				result.addMissing(missingOutputFile, "missing file: "+path)
				continue
			}
			if file.Size <= 0 || strings.TrimSpace(file.Content) == "" {
				result.addMissing(missingOutputFile, "empty file: "+path)
			}
		case "message":
			if strings.TrimSpace(reply.Content) == "" {
				result.addMissing(missingOutputMessage, "missing assistant message: "+output.Description)
			}
		case "command_result":
			if !hasCommandResult(reply.ToolEvents) {
				result.addMissing(missingOutputCommand, "missing command result: "+output.Description)
			}
		}
	}
	result.OK = len(result.Missing) == 0
	return result
}

func (r *validationResult) addMissing(kind, message string) {
	if r == nil {
		return
	}
	r.Missing = append(r.Missing, message)
	r.MissingKinds = append(r.MissingKinds, kind)
}

func firstChangedFile(events []ai.ToolEvent) string {
	for _, event := range events {
		if event.Status != "success" {
			continue
		}
		if isWorkspaceFileOutputTool(event.ToolName) {
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

func isWorkspaceFileOutputTool(toolName string) bool {
	return ai.IsWorkspaceMutationTool(toolName)
}

func hasCommandResult(events []ai.ToolEvent) bool {
	for _, event := range events {
		if event.ToolName == "run_workspace_command" && event.ExitCode != nil {
			return true
		}
	}
	return false
}

func repairForceTool(validation validationResult) string {
	return ""
}
