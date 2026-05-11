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

func (s *Service) generateAnthropic(ctx context.Context, conversationID, messageID string, provider store.ModelProvider, messages []store.Message, attachments []AttachmentPayload, opts RunOptions) (Reply, error) {
	return s.runAnthropicAgent(ctx, conversationID, messageID, provider, messages, attachments, opts)
}

type anthropicProviderV2 struct {
	service *Service
}

func (p anthropicProviderV2) Turn(ctx context.Context, req providerTurnRequest) (agentModelTurn, error) {
	toolNames := anthropicToolNames(req.Tools)
	payload := map[string]any{
		"model":       req.Provider.ModelID,
		"max_tokens":  1024,
		"messages":    req.Snapshot.AnthropicMessages,
		"tools":       req.Tools,
		"tool_choice": map[string]any{"type": "auto", "disable_parallel_tool_use": true},
		"stream":      true,
	}
	if req.Snapshot.AnthropicSystem != "" {
		payload["system"] = req.Snapshot.AnthropicSystem
	}
	log.Printf("[ai] request conversation=%s provider=%s protocol=%s base_url=%s model=%s tools_count=%d tools=%s tool_choice=%v round=%d",
		req.ConversationID, req.Provider.Name, req.Provider.Protocol, req.Provider.BaseURL, req.Provider.ModelID, len(toolNames), strings.Join(toolNames, ","), payload["tool_choice"], req.Snapshot.Summary.Round,
	)
	path := anthropicMessagesPath(req.Provider)
	endpoint := providerSDKEndpoint(req.Provider.BaseURL, path)

	startedAt := time.Now()
	stage := diagnosticStage(req.Provider.Protocol, req.PerformedMutation, req.HasToolEvents)
	resp, err := p.service.anthropicSDKPostRaw(ctx, req.Provider, path, payload, req.Opts, stage)
	duration := time.Since(startedAt)
	if err != nil {
		if isContextCanceledError(err) {
			return agentModelTurn{}, err
		}
		if isRecoverableFinalProviderError(err) {
			diagID := p.service.recordProviderDiagnosticWithSummary(ctx, req.Opts, req.Provider, req.ConversationID, req.MessageID, stage, "warn", duration.Milliseconds(), err, req.Snapshot.Summary)
			emitToolLifecycleEvent(req.Opts, "model_stream_recovering", "warn", map[string]any{
				"stage":         "provider_stream_transport",
				"provider_id":   req.Provider.ID,
				"provider_name": req.Provider.Name,
				"protocol":      req.Provider.Protocol,
				"model_id":      req.Provider.ModelID,
				"error":         err.Error(),
				"diagnostic_id": diagID,
				"recovery":      "retry_non_stream",
			})
			return p.callNonStream(ctx, req, payload, endpoint, "provider_stream_transport_fallback")
		}
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
		turn, err := parseAnthropicStream(streamBody, func(text string) {
			emitToolLifecycleEvent(req.Opts, "model_delta", "info", map[string]any{"text": text})
		})
		if err != nil {
			if partial, ok := partialTurnFromStreamError(err); ok {
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
				return p.callNonStream(ctx, req, payload, endpoint, "provider_stream_fallback")
			}
			diagID := p.service.recordProviderDiagnosticWithSummary(ctx, req.Opts, req.Provider, req.ConversationID, req.MessageID, "provider_stream_parse", "error", time.Since(startedAt).Milliseconds(), err, req.Snapshot.Summary)
			return agentModelTurn{}, WithDiagnosticID(err, diagID)
		}
		closeStream()
		return turn, nil
	}

	return p.parseNonStream(ctx, req, resp, endpoint, duration)
}

func (s *Service) callAnthropicAgentTurn(ctx context.Context, runtime *agentRuntime, round int) (agentModelTurn, error) {
	return s.providerV2(agentProtocolAnthropic).Turn(ctx, providerTurnRequest{
		ConversationID:                runtime.conversationID,
		MessageID:                     runtime.messageID,
		Provider:                      runtime.provider,
		Protocol:                      runtime.protocol,
		Tools:                         runtime.tools(),
		Snapshot:                      runtime.lastContextSnapshot,
		PerformedMutation:             runtime.performedMutation,
		HasToolEvents:                 len(runtime.events) > 0,
		HasVerifiedCompletionEvidence: runtime.hasVerifiedCompletionEvidence(),
		Opts:                          runtime.opts,
	})
}

func (p anthropicProviderV2) callNonStream(ctx context.Context, reqState providerTurnRequest, payload map[string]any, endpoint, stage string) (agentModelTurn, error) {
	fallbackPayload := clonePayloadWithStream(payload, false)
	startedAt := time.Now()
	resp, err := p.service.anthropicSDKPostRaw(ctx, reqState.Provider, anthropicMessagesPath(reqState.Provider), fallbackPayload, reqState.Opts, stage)
	duration := time.Since(startedAt)
	if err != nil {
		if isContextCanceledError(err) {
			return agentModelTurn{}, err
		}
		if reqState.HasVerifiedCompletionEvidence && isRecoverableFinalProviderError(err) {
			diagID := p.service.recordProviderDiagnosticWithSummary(ctx, reqState.Opts, reqState.Provider, reqState.ConversationID, reqState.MessageID, stage, "warn", duration.Milliseconds(), err, reqState.Snapshot.Summary)
			emitToolLifecycleEvent(reqState.Opts, "model_stream_recovering", "warn", map[string]any{
				"stage":         "final_response_recoverable_transport",
				"provider_id":   reqState.Provider.ID,
				"provider_name": reqState.Provider.Name,
				"protocol":      reqState.Provider.Protocol,
				"model_id":      reqState.Provider.ModelID,
				"error":         err.Error(),
				"diagnostic_id": diagID,
				"recovery":      "verified_tool_result_completion",
			})
			return agentModelTurn{Content: "已完成并检查结果。", FinishReason: "recoverable_final_response_error"}, nil
		}
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
		return agentModelTurn{}, WithDiagnosticID(fmt.Errorf("模型响应解析失败（阶段：%s）: %w", anthropicTimeoutStage(req.PerformedMutation, req.HasToolEvents), err), diagID)
	}
	if data.Error != nil {
		log.Printf("[ai] anthropic provider_error conversation=%s status=%d error=%v", req.ConversationID, resp.StatusCode, data.Error)
		err := formatProviderError(resp.StatusCode, data.Error)
		stage := diagnosticStage(req.Provider.Protocol, req.PerformedMutation, req.HasToolEvents)
		diagID := p.service.recordProviderDiagnosticWithSummary(ctx, req.Opts, req.Provider, req.ConversationID, req.MessageID, stage, "error", duration.Milliseconds(), err, req.Snapshot.Summary)
		return agentModelTurn{}, WithDiagnosticID(err, diagID)
	}
	toolUses := data.toolUses()
	logAnthropicResponseSummary(req.ConversationID, data, toolUses)
	if strings.TrimSpace(data.text()) == "" && len(toolUses) == 0 {
		err := fmt.Errorf("empty model response")
		p.service.recordProviderDiagnosticWithSummary(ctx, req.Opts, req.Provider, req.ConversationID, req.MessageID, "provider_empty_response", "warn", duration.Milliseconds(), err, req.Snapshot.Summary)
		return agentModelTurn{FinishReason: "empty_response"}, nil
	}
	turn := agentModelTurn{
		Content:      data.text(),
		FinishReason: data.StopReason,
		AnthropicRaw: data.Content,
		ToolCalls:    make([]agentToolCall, 0, len(toolUses)),
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
	return turn, nil
}
