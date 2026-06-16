package ai

import (
	"context"
	"fmt"
	"strings"
)

func (r *agentRuntime) handleNoToolTurn(ctx context.Context, turn agentModelTurn) (Reply, bool, error) {
	content := strings.TrimSpace(turn.Content)
	actionBoundaryActive := r.actionBoundaryActive()
	contentPolicy := r.contentOnlyCompletionPolicy(turn)
	r.lastProviderEmptyResponse = content == ""
	if turn.FinishReason == "timeout_after_mutation" || turn.FinishReason == "recoverable_final_response_error" {
		if content != "" && contentPolicy.Accept {
			emitToolLifecycleEvent(r.opts, "model_message_completed", "warn", map[string]any{"content": content})
			return Reply{Content: content, ToolEvents: r.events}, true, nil
		}
		reply, err := r.fail("模型最终回复连接中断；已有工具结果已保存到断点，可继续本轮", nil)
		return reply, true, err
	}
	if !actionBoundaryActive || contentPolicy.Accept {
		if content == "" {
			err := fmt.Errorf("empty model response")
			diagID := r.service.recordProviderDiagnostic(ctx, r.opts, r.provider, r.conversationID, r.messageID, "provider_empty_response", "warn", 0, err)
			r.invalidTurns++
			if r.invalidTurns > agentMaxInvalidTurns {
				emitToolLifecycleEvent(r.opts, "run_failed", "error", map[string]any{
					"error":             err.Error(),
					"diagnostic_log_id": diagID,
					"provider":          r.provider.Name,
					"model":             r.provider.ModelID,
					"finish_reason":     r.lastProviderFinishReason,
					"stage":             "provider_empty_response",
				})
				return Reply{}, true, WithDiagnosticID(err, diagID)
			}
			correction := "你的上一轮响应为空。请直接回答用户；如果用户要求安装 Skill、操作文件、联网或运行命令，请调用可用工具，不要返回空内容。"
			r.transcript.AppendRuntimeObservation(correction, map[string]any{"reason": "empty_model_response", "observation_type": "action_rejected"})
			if err := r.service.saveRunCheckpoint(ctx, r.opts, r.checkpoint()); err != nil {
				return Reply{}, true, err
			}
			emitToolLifecycleEvent(r.opts, "completion_rejected", "warn", map[string]any{"reason": "empty model response", "diagnostic_log_id": diagID})
			return Reply{}, false, nil
		}
		r.finalizeActiveAssistantMessage(content)
		emitToolLifecycleEvent(r.opts, "model_message_completed", "info", map[string]any{
			"content":                           content,
			"content_only_policy":               contentPolicy.Reason,
			"accepted_content_only_after_tools": actionBoundaryActive,
		})
		return Reply{Content: content, ToolEvents: r.events}, true, nil
	}
	if content != "" {
		r.discardActiveAssistantMessage("action_required_plain_text_rejected")
	}
	r.invalidTurns++
	if content == "" {
		diagID := r.service.recordProviderDiagnostic(ctx, r.opts, r.provider, r.conversationID, r.messageID, "provider_empty_response", "warn", 0, fmt.Errorf("empty model response"))
		if r.invalidTurns > agentMaxInvalidTurns {
			reply, err := r.fail("模型返回空内容且没有工具调用，workspace 任务未执行完成", nil)
			err = WithDiagnosticID(err, diagID)
			return reply, true, err
		}
		if err := r.service.saveRunCheckpoint(ctx, r.opts, r.checkpoint()); err != nil {
			return Reply{}, true, err
		}
		emitToolLifecycleEvent(r.opts, "completion_rejected", "warn", map[string]any{"reason": "empty model response"})
		return Reply{}, false, nil
	}
	if r.invalidTurns > agentMaxInvalidTurns {
		errText := "模型没有返回可执行工具调用或 attempt_completion，workspace 任务未执行完成"
		if content == "" {
			errText = "模型返回空内容且没有工具调用，workspace 任务未执行完成"
		}
		reply, err := r.fail(errText, nil)
		return reply, true, err
	}
	correction := r.correctiveMessage(content)
	r.transcript.AppendRuntimeObservation(correction, map[string]any{"reason": "missing_tool_call", "observation_type": "action_rejected"})
	if err := r.service.saveRunCheckpoint(ctx, r.opts, r.checkpoint()); err != nil {
		return Reply{}, true, err
	}
	emitToolLifecycleEvent(r.opts, "completion_rejected", "warn", map[string]any{"reason": "model returned no tool calls", "message": correction})
	return Reply{}, false, nil
}
