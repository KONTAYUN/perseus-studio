package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"webide-backend/internal/store"
)

const anthropicAgentMaxTokens = 65536

func (s *Service) generateAnthropic(ctx context.Context, conversationID, messageID string, provider store.ModelProvider, messages []store.Message, attachments []AttachmentPayload, opts RunOptions) (Reply, error) {
	return s.runAnthropicAgent(ctx, conversationID, messageID, provider, messages, attachments, opts)
}

type anthropicProviderV2 struct {
	service *Service
}

func (p anthropicProviderV2) Turn(ctx context.Context, req providerTurnRequest) (agentModelTurn, error) {
	toolNames := anthropicToolNames(req.Tools)
	toolsEnabled := !req.DisableTools && len(req.Tools) > 0
	req.Snapshot.Summary.RequestKind = "messages"
	req.Snapshot.Summary.Stream = boolPtr(true)
	req.Snapshot.Summary.ToolChoice = "none"
	req.Snapshot.Summary.RequiresTool = false
	req.Snapshot.Summary.ToolsCount = 0
	req.Snapshot.Summary.ToolNames = nil
	if toolsEnabled {
		req.Snapshot.Summary.ToolsCount = len(toolNames)
		req.Snapshot.Summary.ToolNames = toolNames
		req.Snapshot.Summary.ParallelToolCalls = boolPtr(false)
	}
	payload := map[string]any{
		"model":      req.Provider.ModelID,
		"max_tokens": anthropicAgentMaxTokens,
		"messages":   req.Snapshot.AnthropicMessages,
		"stream":     true,
	}
	req.Snapshot.Summary.OutputMaxTokens = anthropicAgentMaxTokens
	reasoningPolicy := applyAnthropicReasoningPayload(payload, req.Provider, req.Opts.ThinkingMode)
	req.Snapshot.Summary.ReasoningSupport = reasoningPolicy.Support
	req.Snapshot.Summary.ReasoningEffort = reasoningPolicy.AnthropicEffort
	req.Snapshot.Summary.ThinkingMode = reasoningPolicy.ThinkingMode
	if toolsEnabled {
		payload["tools"] = req.Tools
		actualChoice := req.ToolChoice
		if reasoningPolicy.AnthropicAdaptiveEnabled() && normalizeProviderToolChoice(actualChoice).requiresTool() {
			actualChoice = providerToolChoice{Mode: providerToolChoiceAuto}
		}
		req.Snapshot.Summary.ToolChoice = providerToolChoiceSummary(actualChoice)
		req.Snapshot.Summary.RequiresTool = normalizeProviderToolChoice(actualChoice).requiresTool()
		payload["tool_choice"] = anthropicProviderToolChoiceForReasoning(req.ToolChoice, reasoningPolicy.AnthropicAdaptiveEnabled())
	}
	if req.Snapshot.AnthropicSystem != "" {
		cacheEnabled := anthropicPromptCacheEnabled()
		req.Snapshot.Summary.AnthropicPromptCacheEnabled = cacheEnabled
		payload["system"] = AnthropicSystemBlockBuilder{}.Build(req.Snapshot.AnthropicSystem, cacheEnabled)
	}
	annotatePayloadDiagnostics(&req.Snapshot.Summary, payload, req.Tools, req.Provider, reasoningPolicy)
	log.Printf("[ai] request conversation=%s provider=%s protocol=%s base_url=%s model=%s tools_count=%d tools=%s tool_choice=%v round=%d",
		req.ConversationID, req.Provider.Name, req.Provider.Protocol, req.Provider.BaseURL, req.Provider.ModelID, len(toolNames), strings.Join(toolNames, ","), payload["tool_choice"], req.Snapshot.Summary.Round,
	)
	path := anthropicMessagesPath(req.Provider)
	endpoint := providerSDKEndpoint(req.Provider.BaseURL, path)

	startedAt := time.Now()
	stage := diagnosticStage(req.Provider.Protocol, req.PerformedMutation, req.HasToolEvents)
	req.Snapshot.Summary.StreamRetryPhase = "stream"
	resp, err := p.service.anthropicSDKPostRaw(ctx, req.Provider, path, payload, req.Opts, stage, &req.Snapshot.Summary)
	duration := time.Since(startedAt)
	if err != nil {
		if isContextCanceledError(err) {
			return agentModelTurn{}, err
		}
		p.service.recordProviderUsage(ctx, providerUsageRecordInput{
			Provider:       req.Provider,
			Opts:           req.Opts,
			ConversationID: req.ConversationID,
			MessageID:      req.MessageID,
			Stage:          stage,
			RequestKind:    "messages",
			Stream:         true,
			Status:         store.UsageStatusFailed,
			ErrorClass:     usageFailureClass(err, req.Snapshot.Summary),
			DurationMS:     duration.Milliseconds(),
		})
		diagID := p.service.recordProviderDiagnosticWithSummary(ctx, req.Opts, req.Provider, req.ConversationID, req.MessageID, stage, "error", duration.Milliseconds(), err, req.Snapshot.Summary)
		return agentModelTurn{}, WithDiagnosticID(err, diagID)
	}
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(strings.ToLower(contentType), "text/event-stream") {
		streamBody := resp.Body
		streamClosed := false
		closeStream := func() {
			if !streamClosed {
				streamClosed = true
				streamBody.Close()
			}
		}
		defer closeStream()
		revealTextAfterAction := req.ActionBoundaryActive || normalizeProviderToolChoice(req.ToolChoice).requiresTool()
		var emitText func(string)
		if req.Opts.Event != nil && providerTurnCanStreamAssistantText(req) {
			emitText = func(text string) {
				emitToolLifecycleEvent(req.Opts, "model_delta", "info", map[string]any{"text": text})
			}
		}
		var emitCompletionResult func(string)
		if req.Opts.OnCompletionResultDelta != nil {
			emitCompletionResult = req.Opts.OnCompletionResultDelta
		}
		turn, err := parseAnthropicStreamWithOptions(streamBody, providerStreamParseOptions{
			EmitText:                  emitText,
			EmitCompletionResultDelta: emitCompletionResult,
			RevealTextAfterTool:       revealTextAfterAction,
		})
		if emitText != nil && strings.TrimSpace(turn.PublicStreamedContent) != "" && (!revealTextAfterAction || len(turn.ToolCalls) > 0) {
			turn.PublicTextStreamed = true
		}
		if err != nil {
			if partial, ok := partialTurnFromStreamError(err); ok {
				if emitText != nil && strings.TrimSpace(partial.PublicStreamedContent) != "" && (!revealTextAfterAction || len(partial.ToolCalls) > 0) {
					partial.PublicTextStreamed = true
				}
				if !partial.CompletionResultStreamed && emitCompletionResult != nil {
					emitStreamedCompletionResultFromToolCalls(partial.ToolCalls, emitCompletionResult)
					partial.CompletionResultStreamed = true
				}
				log.Printf("[ai] anthropic stream_partial_tool_recovered conversation=%s endpoint=%s tool_calls=%d error=%v", req.ConversationID, endpoint, len(partial.ToolCalls), err)
				diagID := p.service.recordProviderDiagnosticWithSummary(ctx, req.Opts, req.Provider, req.ConversationID, req.MessageID, "provider_stream_partial_tool_recovered", "warn", time.Since(startedAt).Milliseconds(), err, req.Snapshot.Summary)
				emitToolLifecycleEvent(req.Opts, "model_stream_recovering", "warn", map[string]any{
					"stage":         "provider_stream_partial_tool_recovered",
					"provider_id":   req.Provider.ID,
					"provider_name": req.Provider.Name,
					"protocol":      req.Provider.Protocol,
					"model_id":      req.Provider.ModelID,
					"error":         err.Error(),
					"diagnostic_id": diagID,
					"recovery":      "use_completed_stream_tool_call",
				})
				closeStream()
				return partial, nil
			}
			if shouldFallbackAfterStreamError(ctx, err) {
				log.Printf("[ai] anthropic stream_parse_error_fallback conversation=%s endpoint=%s error=%v", req.ConversationID, endpoint, err)
				diagID := p.service.recordProviderDiagnosticWithSummary(ctx, req.Opts, req.Provider, req.ConversationID, req.MessageID, "provider_stream_parse", "warn", time.Since(startedAt).Milliseconds(), err, req.Snapshot.Summary)
				emitToolLifecycleEvent(req.Opts, "model_stream_recovering", "warn", map[string]any{
					"stage":         "provider_stream_parse",
					"provider_id":   req.Provider.ID,
					"provider_name": req.Provider.Name,
					"protocol":      req.Provider.Protocol,
					"model_id":      req.Provider.ModelID,
					"error":         err.Error(),
					"diagnostic_id": diagID,
					"recovery":      "retry_non_stream",
				})
				closeStream()
				req.Snapshot.Summary.StreamFallbackUsed = true
				return p.callNonStream(ctx, req, payload, endpoint, "provider_stream_fallback")
			}
			diagID := p.service.recordProviderDiagnosticWithSummary(ctx, req.Opts, req.Provider, req.ConversationID, req.MessageID, "provider_stream_parse", "error", time.Since(startedAt).Milliseconds(), err, req.Snapshot.Summary)
			p.service.recordProviderUsage(ctx, providerUsageRecordInput{
				Provider:       req.Provider,
				Opts:           req.Opts,
				ConversationID: req.ConversationID,
				MessageID:      req.MessageID,
				Stage:          "provider_stream_parse",
				RequestKind:    "messages",
				Stream:         true,
				Status:         store.UsageStatusFailed,
				ErrorClass:     usageFailureClass(err, req.Snapshot.Summary),
				DurationMS:     time.Since(startedAt).Milliseconds(),
			})
			return agentModelTurn{}, WithDiagnosticID(err, diagID)
		}
		if !turn.CompletionResultStreamed && emitCompletionResult != nil {
			emitStreamedCompletionResultFromToolCalls(turn.ToolCalls, emitCompletionResult)
		}
		if hasIncompleteToolCallArguments(turn) {
			req.Snapshot.Summary.StreamToolArgsState = "incomplete_json"
			annotateStreamToolDiagnostics(&req.Snapshot.Summary, turn)
			err := fmt.Errorf("provider stream returned incomplete tool call arguments; max_tokens=%d", anthropicAgentMaxTokens)
			diagID := p.service.recordProviderDiagnosticWithSummary(ctx, req.Opts, req.Provider, req.ConversationID, req.MessageID, "provider_stream_incomplete_tool_arguments", "error", time.Since(startedAt).Milliseconds(), err, req.Snapshot.Summary)
			p.service.recordProviderUsage(ctx, providerUsageRecordInput{
				Provider:       req.Provider,
				Opts:           req.Opts,
				ConversationID: req.ConversationID,
				MessageID:      req.MessageID,
				Stage:          "provider_stream_incomplete_tool_arguments",
				RequestKind:    "messages",
				Stream:         true,
				Status:         store.UsageStatusFailed,
				ErrorClass:     usageFailureClass(err, req.Snapshot.Summary),
				Usage:          turn.Usage,
				DurationMS:     time.Since(startedAt).Milliseconds(),
			})
			return agentModelTurn{}, WithDiagnosticID(err, diagID)
		}
		closeStream()
		if isEmptyModelTurn(turn) {
			err := fmt.Errorf("empty Anthropic stream response")
			if !providerStreamFallbackEnabled() {
				diagID := p.service.recordProviderDiagnosticWithSummary(ctx, req.Opts, req.Provider, req.ConversationID, req.MessageID, "provider_empty_stream_response", "error", time.Since(startedAt).Milliseconds(), err, req.Snapshot.Summary)
				p.service.recordProviderUsage(ctx, providerUsageRecordInput{
					Provider:       req.Provider,
					Opts:           req.Opts,
					ConversationID: req.ConversationID,
					MessageID:      req.MessageID,
					Stage:          "provider_empty_stream_response",
					RequestKind:    "messages",
					Stream:         true,
					Status:         store.UsageStatusFailed,
					ErrorClass:     usageFailureClass(err, req.Snapshot.Summary),
					Usage:          turn.Usage,
					DurationMS:     time.Since(startedAt).Milliseconds(),
				})
				return agentModelTurn{}, WithDiagnosticID(err, diagID)
			}
			diagID := p.service.recordProviderDiagnosticWithSummary(ctx, req.Opts, req.Provider, req.ConversationID, req.MessageID, "provider_empty_stream_response", "warn", time.Since(startedAt).Milliseconds(), err, req.Snapshot.Summary)
			emitToolLifecycleEvent(req.Opts, "model_stream_recovering", "warn", map[string]any{
				"stage":         "provider_empty_stream_response",
				"provider_id":   req.Provider.ID,
				"provider_name": req.Provider.Name,
				"protocol":      req.Provider.Protocol,
				"model_id":      req.Provider.ModelID,
				"error":         err.Error(),
				"diagnostic_id": diagID,
				"recovery":      "retry_non_stream",
			})
			req.Snapshot.Summary.StreamFallbackUsed = true
			return p.callNonStream(ctx, req, payload, endpoint, "provider_empty_stream_fallback")
		}
		p.service.recordProviderUsage(ctx, providerUsageRecordInput{
			Provider:       req.Provider,
			Opts:           req.Opts,
			ConversationID: req.ConversationID,
			MessageID:      req.MessageID,
			Stage:          stage,
			RequestKind:    "messages",
			Stream:         true,
			Status:         store.UsageStatusSucceeded,
			Usage:          turn.Usage,
			DurationMS:     time.Since(startedAt).Milliseconds(),
		})
		return turn, nil
	}

	return p.parseNonStream(ctx, req, resp, endpoint, duration)
}

func (s *Service) callAnthropicAgentTurn(ctx context.Context, runtime *agentRuntime, round int) (agentModelTurn, error) {
	return s.providerV2(agentProtocolAnthropic).Turn(ctx, providerTurnRequest{
		ConversationID:       runtime.conversationID,
		MessageID:            runtime.messageID,
		Provider:             runtime.provider,
		Protocol:             runtime.protocol,
		Tools:                runtime.tools(),
		ToolChoice:           runtime.toolChoice(),
		Snapshot:             runtime.lastContextSnapshot,
		PerformedMutation:    runtime.performedMutation,
		HasToolEvents:        len(runtime.events) > 0,
		ActionBoundaryActive: runtime.actionBoundaryActive(),
		Opts:                 runtime.opts,
	})
}

func (p anthropicProviderV2) callNonStream(ctx context.Context, reqState providerTurnRequest, payload map[string]any, endpoint, stage string) (agentModelTurn, error) {
	fallbackPayload := clonePayloadWithStream(payload, false)
	reqState.Snapshot.Summary.Stream = boolPtr(false)
	reqState.Snapshot.Summary.StreamFallbackUsed = true
	reqState.Snapshot.Summary.StreamRetryPhase = "non_stream_fallback"
	reqState.Snapshot.Summary.FallbackAttempts++
	startedAt := time.Now()
	resp, err := p.service.anthropicSDKPostRaw(ctx, reqState.Provider, anthropicMessagesPath(reqState.Provider), fallbackPayload, reqState.Opts, stage, &reqState.Snapshot.Summary)
	duration := time.Since(startedAt)
	if err != nil {
		if isContextCanceledError(err) {
			return agentModelTurn{}, err
		}
		p.service.recordProviderUsage(ctx, providerUsageRecordInput{
			Provider:       reqState.Provider,
			Opts:           reqState.Opts,
			ConversationID: reqState.ConversationID,
			MessageID:      reqState.MessageID,
			Stage:          stage,
			RequestKind:    "messages",
			Stream:         false,
			Status:         store.UsageStatusFailed,
			ErrorClass:     usageFailureClass(err, reqState.Snapshot.Summary),
			DurationMS:     duration.Milliseconds(),
		})
		diagID := p.service.recordProviderDiagnosticWithSummary(ctx, reqState.Opts, reqState.Provider, reqState.ConversationID, reqState.MessageID, stage, "error", duration.Milliseconds(), err, reqState.Snapshot.Summary)
		return agentModelTurn{}, WithDiagnosticID(err, diagID)
	}
	return p.parseNonStream(ctx, reqState, resp, endpoint, duration)
}

func (p anthropicProviderV2) parseNonStream(ctx context.Context, req providerTurnRequest, resp *http.Response, endpoint string, duration time.Duration) (agentModelTurn, error) {
	var data anthropicResponse
	if err := decodeProviderResponse(resp, endpoint, &data); err != nil {
		log.Printf("[ai] anthropic provider_response_error conversation=%s endpoint=%s error=%v", req.ConversationID, endpoint, err)
		stage := diagnosticStage(req.Provider.Protocol, req.PerformedMutation, req.HasToolEvents)
		diagID := p.service.recordProviderDiagnosticWithSummary(ctx, req.Opts, req.Provider, req.ConversationID, req.MessageID, stage, "error", duration.Milliseconds(), err, req.Snapshot.Summary)
		p.service.recordProviderUsage(ctx, providerUsageRecordInput{
			Provider:       req.Provider,
			Opts:           req.Opts,
			ConversationID: req.ConversationID,
			MessageID:      req.MessageID,
			Stage:          stage,
			RequestKind:    "messages",
			Stream:         req.Snapshot.Summary.Stream != nil && *req.Snapshot.Summary.Stream,
			Status:         store.UsageStatusFailed,
			ErrorClass:     usageFailureClass(err, req.Snapshot.Summary),
			DurationMS:     duration.Milliseconds(),
		})
		return agentModelTurn{}, WithDiagnosticID(fmt.Errorf("模型响应解析失败（阶段：%s）: %w", anthropicTimeoutStage(req.PerformedMutation, req.HasToolEvents), err), diagID)
	}
	if data.Error != nil {
		log.Printf("[ai] anthropic provider_error conversation=%s status=%d error=%v", req.ConversationID, resp.StatusCode, data.Error)
		err := formatProviderError(resp.StatusCode, data.Error)
		stage := diagnosticStage(req.Provider.Protocol, req.PerformedMutation, req.HasToolEvents)
		diagID := p.service.recordProviderDiagnosticWithSummary(ctx, req.Opts, req.Provider, req.ConversationID, req.MessageID, stage, "error", duration.Milliseconds(), err, req.Snapshot.Summary)
		p.service.recordProviderUsage(ctx, providerUsageRecordInput{
			Provider:       req.Provider,
			Opts:           req.Opts,
			ConversationID: req.ConversationID,
			MessageID:      req.MessageID,
			Stage:          stage,
			RequestKind:    "messages",
			Stream:         req.Snapshot.Summary.Stream != nil && *req.Snapshot.Summary.Stream,
			Status:         store.UsageStatusFailed,
			ErrorClass:     usageFailureClass(err, req.Snapshot.Summary),
			DurationMS:     duration.Milliseconds(),
		})
		return agentModelTurn{}, WithDiagnosticID(err, diagID)
	}
	toolUses := data.toolUses()
	logAnthropicResponseSummary(req.ConversationID, data, toolUses)
	content := providerVisibleText(data.text())
	if strings.TrimSpace(content) == "" && len(toolUses) == 0 {
		err := fmt.Errorf("empty model response")
		p.service.recordProviderDiagnosticWithSummary(ctx, req.Opts, req.Provider, req.ConversationID, req.MessageID, "provider_empty_response", "warn", duration.Milliseconds(), err, req.Snapshot.Summary)
		return agentModelTurn{FinishReason: "empty_response"}, nil
	}
	data.Content = sanitizeAnthropicVisibleTextBlocks(data.Content)
	usage := ProviderUsage{}
	updateProviderUsageFromAny(&usage, data.Usage)
	turn := agentModelTurn{
		Content:      content,
		FinishReason: data.StopReason,
		AnthropicRaw: data.Content,
		ToolCalls:    make([]agentToolCall, 0, len(toolUses)),
		Usage:        usage,
	}
	for _, toolUse := range toolUses {
		rawArgs, _ := json.Marshal(toolUse.Input)
		turn.ToolCalls = append(turn.ToolCalls, agentToolCall{
			ID:             toolUse.ID,
			Name:           toolUse.Name,
			ArgumentsJSON:  string(rawArgs),
			AnthropicInput: toolUse.Input,
		})
	}
	p.service.recordProviderUsage(ctx, providerUsageRecordInput{
		Provider:       req.Provider,
		Opts:           req.Opts,
		ConversationID: req.ConversationID,
		MessageID:      req.MessageID,
		Stage:          diagnosticStage(req.Provider.Protocol, req.PerformedMutation, req.HasToolEvents),
		RequestKind:    "messages",
		Stream:         false,
		Status:         store.UsageStatusSucceeded,
		Usage:          turn.Usage,
		DurationMS:     duration.Milliseconds(),
	})
	return turn, nil
}

func isEmptyModelTurn(turn agentModelTurn) bool {
	return strings.TrimSpace(turn.Content) == "" && len(turn.ToolCalls) == 0
}
