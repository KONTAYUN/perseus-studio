package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"webide-backend/internal/store"
)

const (
	agentChatMaxRounds          = 24
	agentWorkspaceMaxRounds     = 48
	agentTaskMaxRounds          = 120
	agentMaxInvalidTurns        = 2
	agentChatMaxToolCalls       = 30
	agentWorkspaceMaxToolCalls  = 80
	agentTaskMaxToolCalls       = 120
	agentMaxConsecutiveToolErrs = 3
)

type agentProtocol string

const (
	agentProtocolOpenAI    agentProtocol = "openai"
	agentProtocolAnthropic agentProtocol = "anthropic"
)

type agentRuntime struct {
	service        *Service
	protocol       agentProtocol
	conversationID string
	messageID      string
	provider       store.ModelProvider
	messages       []store.Message
	attachments    []AttachmentPayload
	opts           RunOptions
	profile        providerRuntimeProfile
	allowTask      bool

	events                    []ToolEvent
	appliedMutations          map[string]ToolEvent
	performedMutation         bool
	lastToolFailed            bool
	invalidTurns              int
	toolCalls                 int
	consecutiveToolErrors     int
	loadedToolsets            map[string]bool
	requiresCompletion        bool
	toolLoopActive            bool
	lastProviderFinishReason  string
	lastProviderEmptyResponse bool
	completedContent          string
	currentRound              int
	transcript                *AgentTranscript
	contextManager            *agentContextManager
	lastContextSnapshot       ContextSnapshot
	activeAssistantMessageID  string
	activeAssistantSegment    int
}

type agentModelTurn struct {
	Content       string
	FinishReason  string
	ToolCalls     []agentToolCall
	OpenAIMessage openAIMessage
	AnthropicRaw  []map[string]any
	Streamed      bool
	Usage         ProviderUsage
}

type agentToolCall struct {
	ID             string
	Name           string
	ArgumentsJSON  string
	AnthropicInput map[string]any
}

type completionAttempt struct {
	Summary      string   `json:"summary"`
	ChangedPaths []string `json:"changed_paths"`
	FailedItems  []string `json:"failed_items"`
	Notes        string   `json:"notes"`
}

func (s *Service) runOpenAIAgent(ctx context.Context, conversationID, messageID string, provider store.ModelProvider, messages []store.Message, attachments []AttachmentPayload, opts RunOptions) (Reply, error) {
	profile := runtimeProfileForProvider(provider)
	messages = sanitizeConversationMessages(messages, profile)
	attachments = sanitizeAttachmentPayloads(attachments, profile)
	runtime := &agentRuntime{
		service:            s,
		protocol:           agentProtocolOpenAI,
		conversationID:     conversationID,
		messageID:          messageID,
		provider:           provider,
		messages:           messages,
		attachments:        attachments,
		opts:               opts,
		profile:            profile,
		allowTask:          opts.TaskID == "",
		loadedToolsets:     allAgentToolsets(opts.TaskID == ""),
		appliedMutations:   map[string]ToolEvent{},
		requiresCompletion: opts.ForceTool != "" || opts.TaskID != "",
		toolLoopActive:     opts.ForceTool != "" || opts.TaskID != "" || attachmentsRequireToolLoop(attachments),
		transcript:         newAgentTranscript(),
		contextManager:     newAgentContextManager(),
	}
	if checkpoint, ok := s.loadRunCheckpoint(ctx, opts); ok {
		runtime.restoreCheckpoint(checkpoint)
	}
	return runtime.run(ctx)
}

func (s *Service) runAnthropicAgent(ctx context.Context, conversationID, messageID string, provider store.ModelProvider, messages []store.Message, attachments []AttachmentPayload, opts RunOptions) (Reply, error) {
	profile := runtimeProfileForProvider(provider)
	messages = sanitizeConversationMessages(messages, profile)
	attachments = sanitizeAttachmentPayloads(attachments, profile)
	runtime := &agentRuntime{
		service:            s,
		protocol:           agentProtocolAnthropic,
		conversationID:     conversationID,
		messageID:          messageID,
		provider:           provider,
		messages:           messages,
		attachments:        attachments,
		opts:               opts,
		profile:            profile,
		allowTask:          opts.TaskID == "",
		loadedToolsets:     allAgentToolsets(opts.TaskID == ""),
		appliedMutations:   map[string]ToolEvent{},
		requiresCompletion: opts.ForceTool != "" || opts.TaskID != "",
		toolLoopActive:     opts.ForceTool != "" || opts.TaskID != "" || attachmentsRequireToolLoop(attachments),
		transcript:         newAgentTranscript(),
		contextManager:     newAgentContextManager(),
	}
	if checkpoint, ok := s.loadRunCheckpoint(ctx, opts); ok {
		runtime.restoreCheckpoint(checkpoint)
	}
	return runtime.run(ctx)
}

func allAgentToolsets(allowTask bool) map[string]bool {
	loaded := map[string]bool{
		"workspace": true,
		"command":   true,
		"search":    true,
		"skills":    true,
	}
	if allowTask {
		loaded["task"] = true
	}
	return loaded
}

func (r *agentRuntime) run(ctx context.Context) (Reply, error) {
	maxRounds, maxToolCalls := r.loopLimits()

	for round := r.currentRound; round < maxRounds; round++ {
		r.currentRound = round + 1
		if r.toolCalls >= maxToolCalls {
			return r.fail("工具调用次数超过上限，任务未完成", nil)
		}
		turn, err := r.callModel(ctx, round)
		if err != nil {
			if isTimeoutError(err) {
				if reply, ok := r.service.tryFallbackTextReplacement(ctx, r.conversationID, r.messageID, r.messages, r.events, r.opts); ok {
					return reply, nil
				}
			}
			return Reply{}, err
		}
		r.lastProviderFinishReason = turn.FinishReason
		if strings.TrimSpace(turn.Content) != "" && !turn.Streamed {
			emitToolLifecycleEvent(r.opts, "model_delta", "info", map[string]any{"text": turn.Content})
		}
		if strings.TrimSpace(turn.Content) != "" {
			r.ensureActiveAssistantMessage(turn.Content)
			r.appendActiveAssistantDelta(turn.Content)
		}
		if len(turn.ToolCalls) == 0 {
			if reply, ok, err := r.handleNoToolTurn(ctx, turn); ok || err != nil {
				return reply, err
			}
			continue
		}
		if r.opts.OnAssistantToolBoundary != nil {
			r.opts.OnAssistantToolBoundary()
		}
		if strings.TrimSpace(turn.Content) != "" {
			r.finalizeActiveAssistantMessage(turn.Content)
		}
		r.invalidTurns = 0
		r.lastProviderEmptyResponse = false
		r.appendAssistantTurn(turn)

		if request, ok, err := r.processToolCalls(ctx, turn.ToolCalls); ok || err != nil {
			if err != nil {
				return Reply{}, err
			}
			if request != nil {
				return Reply{BackgroundTask: request, ToolEvents: r.events}, nil
			}
			if strings.TrimSpace(r.completedContent) != "" {
				return Reply{Content: r.completedContent, ToolEvents: r.events}, nil
			}
		}
	}
	if reply, ok := r.finishWithVerifiedResults("已完成并检查结果。"); ok {
		return reply, nil
	}
	return r.fail("Agent Loop 达到最大轮次，任务未完成", nil)
}

func (r *agentRuntime) loopLimits() (int, int) {
	if r.opts.TaskID != "" {
		return agentTaskMaxRounds, agentTaskMaxToolCalls
	}
	if r.requiresCompletion || r.toolLoopActive || r.opts.ForceTool != "" {
		return agentWorkspaceMaxRounds, agentWorkspaceMaxToolCalls
	}
	return agentChatMaxRounds, agentChatMaxToolCalls
}

func (r *agentRuntime) callModel(ctx context.Context, round int) (agentModelTurn, error) {
	emitToolLifecycleEvent(r.opts, "model_message_started", "info", map[string]any{"round": round + 1})
	logTask(r.opts, "info", fmt.Sprintf("calling model round %d", round+1))
	r.lastContextSnapshot = r.contextManager.Build(r.protocol, r.messages, r.attachments, r.transcript, r.profile, round+1)
	emitProviderRequestSummary(r.opts, r.lastContextSnapshot.Summary)
	return r.service.providerV2(r.protocol).Turn(ctx, providerTurnRequest{
		ConversationID:                r.conversationID,
		MessageID:                     r.messageID,
		Provider:                      r.provider,
		Protocol:                      r.protocol,
		Tools:                         r.tools(),
		Snapshot:                      r.lastContextSnapshot,
		PerformedMutation:             r.performedMutation,
		HasToolEvents:                 len(r.events) > 0,
		HasVerifiedCompletionEvidence: r.hasVerifiedCompletionEvidence(),
		Opts:                          r.opts,
	})
}

func (r *agentRuntime) tools() []map[string]any {
	switch r.protocol {
	case agentProtocolAnthropic:
		return anthropicToolsForLoadedToolsets(r.loadedToolsets, false)
	default:
		return openAIToolsForLoadedToolsets(r.loadedToolsets, false)
	}
}

func (r *agentRuntime) appendAssistantTurn(turn agentModelTurn) {
	r.transcript.AppendAssistantTurn(turn)
}

func (r *agentRuntime) appendToolResults(results []TranscriptToolResult) {
	r.transcript.AppendToolResults(results)
}

func (r *agentRuntime) ensureActiveAssistantMessage(content string) {
	content = strings.TrimSpace(content)
	if content == "" || r.activeAssistantMessageID != "" || r.opts.OnAssistantMessageStarted == nil {
		return
	}
	r.activeAssistantSegment++
	r.activeAssistantMessageID = fmt.Sprintf("active_msg_%d", time.Now().UTC().UnixNano())
	r.opts.OnAssistantMessageStarted(r.activeAssistantMessageID, content, r.activeAssistantSegment)
}

func (r *agentRuntime) appendActiveAssistantDelta(delta string) {
	delta = strings.TrimSpace(delta)
	if delta == "" || r.activeAssistantMessageID == "" || r.opts.OnAssistantMessageDelta == nil {
		return
	}
	r.opts.OnAssistantMessageDelta(r.activeAssistantMessageID, delta)
}

func (r *agentRuntime) finalizeActiveAssistantMessage(content string) {
	content = strings.TrimSpace(content)
	if r.activeAssistantMessageID == "" || r.opts.OnAssistantMessageCompleted == nil {
		return
	}
	r.opts.OnAssistantMessageCompleted(r.activeAssistantMessageID, content, r.activeAssistantSegment)
	r.activeAssistantMessageID = ""
}

func (r *agentRuntime) processToolCalls(ctx context.Context, calls []agentToolCall) (*BackgroundTaskRequest, bool, error) {
	results := make([]TranscriptToolResult, 0, len(calls))
	for _, call := range calls {
		r.toolCalls++
		_, maxToolCalls := r.loopLimits()
		if r.toolCalls > maxToolCalls {
			return nil, false, nil
		}
		eventCountBefore := len(r.events)
		resultContent, isError, request, completed, err := r.processOneToolCall(ctx, call)
		if err != nil || request != nil || completed {
			return request, completed, err
		}
		event := ToolEvent{}
		if len(r.events) > eventCountBefore {
			event = r.events[len(r.events)-1]
		}
		results = append(results, TranscriptToolResult{
			ToolCallID: call.ID,
			ToolName:   call.Name,
			Content:    resultContent,
			IsError:    isError,
			Event:      event,
		})
	}
	r.appendToolResults(results)
	r.service.saveRunCheckpoint(ctx, r.opts, r.checkpoint())
	return nil, false, nil
}

func (r *agentRuntime) processOneToolCall(ctx context.Context, call agentToolCall) (string, bool, *BackgroundTaskRequest, bool, error) {
	if call.ID == "" {
		call.ID = fmt.Sprintf("tool_%d", r.toolCalls)
	}
	if errText := r.validateToolCall(call); errText != "" {
		r.recordToolError()
		return r.toolErrorJSON(call.Name, errText), true, nil, false, nil
	}
	if r.allowTask && call.Name == "start_background_task" {
		r.requiresCompletion = true
		request, err := parseBackgroundTaskRequest(call.ArgumentsJSON, lastUserMessageContent(r.messages))
		if err != nil {
			r.recordToolError()
			return r.toolErrorJSON(call.Name, err.Error()), true, nil, false, nil
		}
		return "", false, &request, false, nil
	}
	if call.Name == "attempt_completion" {
		content, isError, completed := r.handleCompletionAttempt(call.ArgumentsJSON)
		return content, isError, nil, completed, nil
	}
	r.toolLoopActive = true
	if toolRequiresCompletionAfterUse(call.Name) {
		r.requiresCompletion = true
	}
	operationHash := ""
	if isWorkspaceMutationTool(call.Name) {
		operationHash = mutationOperationHash(call.Name, call.ArgumentsJSON)
		if previous, ok := r.appliedMutations[operationHash]; ok {
			result, event := r.alreadyAppliedToolResult(call, previous)
			r.events = append(r.events, event)
			emitToolLifecycleEvent(r.opts, "tool_call_completed", "info", map[string]any{
				"tool_call_id":    call.ID,
				"tool_name":       call.Name,
				"round":           r.currentRound,
				"status":          "success",
				"label":           event.Label,
				"path":            event.Path,
				"old_path":        event.OldPath,
				"new_path":        event.NewPath,
				"changed_paths":   event.ChangedPaths,
				"operation_hash":  operationHash,
				"already_applied": true,
				"result_summary":  "workspace mutation already applied in this run",
				"verification":    ToolVerification{Status: "verified", Message: "already applied earlier in this run", Path: event.Path},
			})
			emitToolLifecycleEvent(r.opts, "tool_result_returned", "info", map[string]any{
				"tool_call_id":    call.ID,
				"tool_name":       call.Name,
				"round":           r.currentRound,
				"status":          "success",
				"is_error":        false,
				"operation_hash":  operationHash,
				"already_applied": true,
			})
			return result, false, nil, false, nil
		}
	}
	result, event := r.service.executeAndRecordTool(ctx, r.conversationID, r.messageID, call.Name, call.ArgumentsJSON, r.opts, r.profile)
	event.OperationHash = operationHash
	r.events = append(r.events, event)
	if event.Status == "success" && call.Name == "load_perseus_toolset" {
		markLoadedToolset(r.loadedToolsets, call.ArgumentsJSON, r.allowTask)
	}
	if event.Status == "success" && isWorkspaceMutationTool(call.Name) {
		r.performedMutation = true
		if operationHash != "" {
			r.appliedMutations[operationHash] = event
		}
	}
	if event.Status == "success" {
		r.lastToolFailed = false
		r.consecutiveToolErrors = 0
	} else {
		r.recordToolError()
	}
	emitToolLifecycleEvent(r.opts, "tool_result_returned", eventLevel(event.Status), map[string]any{
		"tool_call_id": call.ID,
		"tool_name":    call.Name,
		"round":        r.currentRound,
		"status":       event.Status,
		"is_error":     event.Status != "success",
	})
	return result, event.Status != "success", nil, false, nil
}

func (r *agentRuntime) alreadyAppliedToolResult(call agentToolCall, previous ToolEvent) (string, ToolEvent) {
	previous.ToolCallID = call.ID
	previous.Status = "success"
	previous.Label = userFriendlyToolLabel(call.Name)
	if previous.Label == "" {
		previous.Label = "正在检查结果"
	}
	if previous.ToolName == "" {
		previous.ToolName = call.Name
	}
	payload := map[string]any{
		"status":          "already_applied",
		"tool_name":       call.Name,
		"operation_hash":  previous.OperationHash,
		"path":            previous.Path,
		"old_path":        previous.OldPath,
		"new_path":        previous.NewPath,
		"changed_paths":   previous.ChangedPaths,
		"version_id":      previous.VersionID,
		"hash":            previous.Hash,
		"message":         "This workspace mutation was already applied in this run. Continue from the verified existing result instead of repeating it.",
		"recovery_hint":   "Do not repeat the same mutation. Use the existing changed_paths and continue, then call attempt_completion when the task is done.",
		"already_applied": true,
	}
	return mustJSON(payload), previous
}

func (r *agentRuntime) validateToolCall(call agentToolCall) string {
	if strings.TrimSpace(call.Name) == "" {
		return "tool name is required"
	}
	if !r.isKnownTool(call.Name) {
		return fmt.Sprintf("unknown or unavailable tool %q", call.Name)
	}
	if strings.TrimSpace(call.ArgumentsJSON) == "" {
		call.ArgumentsJSON = "{}"
	}
	var decoded any
	if err := json.Unmarshal([]byte(call.ArgumentsJSON), &decoded); err != nil {
		return "tool arguments must be valid JSON: " + err.Error()
	}
	if _, ok := decoded.(map[string]any); !ok {
		return "tool arguments must be a JSON object"
	}
	return ""
}

func (r *agentRuntime) isKnownTool(name string) bool {
	for _, tool := range r.tools() {
		switch r.protocol {
		case agentProtocolAnthropic:
			if tool["name"] == name {
				return true
			}
		default:
			fn, _ := tool["function"].(map[string]any)
			if fn["name"] == name {
				return true
			}
		}
	}
	return false
}

func (r *agentRuntime) handleCompletionAttempt(argumentsJSON string) (string, bool, bool) {
	var attempt completionAttempt
	if err := json.Unmarshal([]byte(argumentsJSON), &attempt); err != nil {
		r.recordToolError()
		return r.toolErrorJSON("attempt_completion", "invalid attempt_completion arguments: "+err.Error()), true, false
	}
	attempt.Summary = strings.TrimSpace(attempt.Summary)
	if attempt.Summary == "" {
		r.recordToolError()
		return r.toolErrorJSON("attempt_completion", "summary is required"), true, false
	}
	if r.performedMutation {
		if len(attempt.ChangedPaths) == 0 {
			r.recordToolError()
			reason := "workspace 变更任务完成时必须在 changed_paths 中列出已验证的变更路径。"
			emitToolLifecycleEvent(r.opts, "completion_rejected", "warn", map[string]any{"reason": reason})
			return r.toolErrorJSON("attempt_completion", reason), true, false
		}
		if !r.completionPathsMatchMutation(attempt.ChangedPaths) {
			r.recordToolError()
			reason := "attempt_completion 的 changed_paths 没有匹配到本轮已验证的 workspace 变更。"
			emitToolLifecycleEvent(r.opts, "completion_rejected", "warn", map[string]any{"reason": reason})
			return r.toolErrorJSON("attempt_completion", reason), true, false
		}
	}
	if rejected := r.completionRejection(); rejected != "" {
		r.recordToolError()
		emitToolLifecycleEvent(r.opts, "completion_rejected", "warn", map[string]any{"reason": rejected})
		return r.toolErrorJSON("attempt_completion", rejected), true, false
	}
	content := attempt.Summary
	if strings.TrimSpace(attempt.Notes) != "" {
		content += "\n\n" + strings.TrimSpace(attempt.Notes)
	}
	r.completedContent = content
	r.finalizeActiveAssistantMessage(content)
	emitToolLifecycleEvent(r.opts, "model_message_completed", "info", map[string]any{"content": content})
	return mustJSON(map[string]any{"summary": attempt.Summary, "changed_paths": attempt.ChangedPaths, "failed_items": attempt.FailedItems, "notes": attempt.Notes}), false, true
}

func (r *agentRuntime) completionRejection() string {
	if r.toolLoopActive && r.toolCalls == 0 {
		return "当前任务已经进入工具工作流，但还没有执行任何工具。请先调用合适的工具，再使用 attempt_completion。"
	}
	if r.performedMutation && !r.hasSuccessfulWorkspaceMutation() {
		return "workspace 变更任务尚未产生可验证的成功变更，不能完成。请继续执行或修正 workspace 工具。"
	}
	if r.lastToolFailed {
		return "上一项工具调用失败且尚未修复，不能完成。请根据 tool_result 修正参数或明确说明无法完成。"
	}
	return ""
}

func (r *agentRuntime) hasSuccessfulWorkspaceMutation() bool {
	for _, event := range r.events {
		if event.Status == "success" && isWorkspaceMutationTool(event.ToolName) && len(event.ChangedPaths) > 0 {
			return true
		}
	}
	return false
}

func (r *agentRuntime) completionPathsMatchMutation(paths []string) bool {
	known := map[string]bool{}
	for _, event := range r.events {
		if event.Status != "success" || !isWorkspaceMutationTool(event.ToolName) {
			continue
		}
		for _, path := range append([]string{event.Path, event.OldPath, event.NewPath}, event.ChangedPaths...) {
			normalized := normalizeWorkspaceEventPath(path)
			if normalized != "" {
				known[normalized] = true
			}
		}
	}
	for _, path := range paths {
		normalized := normalizeWorkspaceEventPath(path)
		if normalized != "" && known[normalized] {
			return true
		}
	}
	return false
}

func normalizeWorkspaceEventPath(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	path = strings.Trim(path, "/")
	if path == "." {
		return ""
	}
	return path
}

func (r *agentRuntime) handleNoToolTurn(ctx context.Context, turn agentModelTurn) (Reply, bool, error) {
	content := strings.TrimSpace(turn.Content)
	toolLoopActive := r.requiresCompletion || r.toolLoopActive || r.opts.ForceTool != ""
	r.lastProviderEmptyResponse = content == ""
	if turn.FinishReason == "timeout_after_mutation" || turn.FinishReason == "recoverable_final_response_error" {
		if reply, ok := r.finishWithVerifiedResults("模型最终回复连接中断，已根据工具验证结果收尾。"); ok {
			return reply, true, nil
		}
		if content != "" && !toolLoopActive {
			emitToolLifecycleEvent(r.opts, "model_message_completed", "warn", map[string]any{"content": content})
			return Reply{Content: content, ToolEvents: r.events}, true, nil
		}
	}
	if !toolLoopActive {
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
			r.transcript.AppendUserText(correction, map[string]any{"reason": "empty_model_response"})
			r.service.saveRunCheckpoint(ctx, r.opts, r.checkpoint())
			emitToolLifecycleEvent(r.opts, "completion_rejected", "warn", map[string]any{"reason": "empty model response", "diagnostic_log_id": diagID})
			return Reply{}, false, nil
		}
		r.finalizeActiveAssistantMessage(content)
		emitToolLifecycleEvent(r.opts, "model_message_completed", "info", map[string]any{"content": content})
		return Reply{Content: content, ToolEvents: r.events}, true, nil
	}
	r.invalidTurns++
	if content == "" {
		r.service.recordProviderDiagnostic(ctx, r.opts, r.provider, r.conversationID, r.messageID, "provider_empty_response", "warn", 0, fmt.Errorf("empty model response"))
		if r.invalidTurns > agentMaxInvalidTurns {
			if reply, ok := r.finishWithVerifiedResults("已完成并检查结果。"); ok {
				return reply, true, nil
			}
			reply, err := r.fail("模型返回空内容且没有工具调用，workspace 任务未执行完成", nil)
			return reply, true, err
		}
		r.service.saveRunCheckpoint(ctx, r.opts, r.checkpoint())
		emitToolLifecycleEvent(r.opts, "completion_rejected", "warn", map[string]any{"reason": "empty model response"})
		return Reply{}, false, nil
	}
	if r.invalidTurns > agentMaxInvalidTurns {
		if reply, ok := r.finishWithVerifiedResults("已完成并检查结果。"); ok {
			return reply, true, nil
		}
		errText := "模型没有返回可执行工具调用或 attempt_completion，workspace 任务未执行完成"
		if content == "" {
			errText = "模型返回空内容且没有工具调用，workspace 任务未执行完成"
		}
		reply, err := r.fail(errText, nil)
		return reply, true, err
	}
	correction := r.correctiveMessage(content)
	r.transcript.AppendUserText(correction, map[string]any{"reason": "missing_tool_call"})
	r.service.saveRunCheckpoint(ctx, r.opts, r.checkpoint())
	emitToolLifecycleEvent(r.opts, "completion_rejected", "warn", map[string]any{"reason": "model returned no tool calls", "message": correction})
	return Reply{}, false, nil
}

func (r *agentRuntime) finishWithVerifiedResults(reason string) (Reply, bool) {
	lines := r.verifiedCompletionLines()
	if len(lines) == 0 || r.lastToolFailed {
		return Reply{}, false
	}
	content := "已完成并验证以下结果：\n" + strings.Join(prefixLines(lines, "- "), "\n")
	if strings.TrimSpace(reason) != "" {
		content += "\n\n" + strings.TrimSpace(reason)
	}
	r.completedContent = content
	emitToolLifecycleEvent(r.opts, "model_message_completed", "warn", map[string]any{
		"content":       content,
		"changed_paths": r.verifiedMutationPaths(),
		"recovery":      "verified_tool_result_completion",
	})
	return Reply{Content: content, ToolEvents: r.events}, true
}

func (r *agentRuntime) hasVerifiedCompletionEvidence() bool {
	lines := r.verifiedCompletionLines()
	return len(lines) > 0 && !r.lastToolFailed
}

func (r *agentRuntime) verifiedCompletionLines() []string {
	paths := r.verifiedMutationPaths()
	if len(paths) > 0 {
		return paths
	}
	seen := map[string]bool{}
	var lines []string
	for _, event := range r.events {
		if event.Status != "success" {
			continue
		}
		line := ""
		switch event.ToolName {
		case "install_skill_from_attachment":
			line = "Skill 安装流程已完成"
		case "delete_skill":
			line = "Skill 删除已完成"
		case "read_skill":
			line = "Skill 内容已读取"
		}
		if line != "" && !seen[line] {
			seen[line] = true
			lines = append(lines, line)
		}
	}
	return lines
}

func (r *agentRuntime) verifiedMutationPaths() []string {
	seen := map[string]bool{}
	var paths []string
	for _, event := range r.events {
		if event.Status != "success" || !isWorkspaceMutationTool(event.ToolName) {
			continue
		}
		for _, path := range append([]string{event.Path, event.NewPath}, event.ChangedPaths...) {
			normalized := normalizeWorkspaceEventPath(path)
			if normalized == "" || seen[normalized] {
				continue
			}
			seen[normalized] = true
			paths = append(paths, normalized)
		}
	}
	return paths
}

func prefixLines(items []string, prefix string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			out = append(out, prefix+item)
		}
	}
	return out
}

func (r *agentRuntime) correctiveMessage(content string) string {
	if strings.TrimSpace(content) == "" {
		return "你的上一轮响应没有文本也没有工具调用。当前任务需要通过工具执行并验证；请调用合适的 workspace/search/command/skill 工具，完成后调用 attempt_completion。"
	}
	if r.toolLoopActive {
		return "你的上一轮响应只是普通文本，没有继续调用工具，也没有调用 attempt_completion。当前 run 已经在工具工作流中：如果还需要继续处理，请直接调用下一个合适的工具；如果工具工作已经完成，请调用 attempt_completion 交付结果。"
	}
	return "你的上一轮响应没有调用工具，因此不能完成当前任务。请根据用户请求调用必要工具；工具失败会以 tool_result 返回给你，修正后继续。所有需要改变 workspace 的任务完成前必须调用 attempt_completion。"
}

func (r *agentRuntime) fail(message string, cause error) (Reply, error) {
	err := errors.New(message)
	if cause != nil {
		err = fmt.Errorf("%s: %w", message, cause)
	}
	log.Printf("[ai] agent runtime failed conversation=%s provider=%s model=%s finish_reason=%s empty=%v error=%v", r.conversationID, r.provider.Name, r.provider.ModelID, r.lastProviderFinishReason, r.lastProviderEmptyResponse, err)
	emitToolLifecycleEvent(r.opts, "run_failed", "error", map[string]any{
		"error":         err.Error(),
		"provider":      r.provider.Name,
		"model":         r.provider.ModelID,
		"finish_reason": r.lastProviderFinishReason,
		"stage":         "agent_loop",
	})
	return Reply{}, err
}

func (r *agentRuntime) recordToolError() {
	r.lastToolFailed = true
	r.consecutiveToolErrors++
	if r.consecutiveToolErrors >= agentMaxConsecutiveToolErrs {
		emitToolLifecycleEvent(r.opts, "completion_rejected", "warn", map[string]any{"reason": "too many consecutive tool errors"})
	}
}

func (r *agentRuntime) toolErrorJSON(toolName, message string) string {
	return mustJSON(map[string]any{
		"error":         message,
		"tool_name":     toolName,
		"recovery_hint": "Fix the tool arguments or choose a valid available tool, then continue. Do not claim success until a tool succeeds and completion is accepted.",
	})
}

func mutationOperationHash(toolName, argumentsJSON string) string {
	var decoded any
	if err := json.Unmarshal([]byte(argumentsJSON), &decoded); err != nil {
		decoded = strings.TrimSpace(argumentsJSON)
	}
	canonical, err := json.Marshal(decoded)
	if err != nil {
		canonical = []byte(strings.TrimSpace(argumentsJSON))
	}
	sum := sha256.Sum256([]byte(toolName + "\x00" + string(canonical)))
	return hex.EncodeToString(sum[:])
}

func userFriendlyToolLabel(toolName string) string {
	switch toolName {
	case "inspect_workspace":
		return "正在查看文件"
	case "read_workspace_text":
		return "正在阅读文件"
	case "save_workspace_file":
		return "正在写入文件"
	case "manage_workspace_path":
		return "正在整理文件"
	case "organize_workspace_matches":
		return "正在归档文件"
	case "run_workspace_command":
		return "正在测算"
	case "perseus_search_web", "web_search":
		return "正在查找资料"
	case "web_extract":
		return "正在阅读资料"
	case "list_skills", "read_skill", "install_skill_from_attachment", "delete_skill":
		return "正在准备技能"
	default:
		return "正在处理"
	}
}

func eventLevel(status string) string {
	if status == "success" {
		return "info"
	}
	return "error"
}

func toolRequiresCompletionAfterUse(name string) bool {
	switch name {
	case "install_skill_from_attachment", "delete_skill", "run_workspace_command", "start_background_task":
		return true
	default:
		return isWorkspaceMutationTool(name)
	}
}
