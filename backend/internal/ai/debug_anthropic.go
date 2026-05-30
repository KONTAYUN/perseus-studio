package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"webide-backend/internal/store"
)

func (s *Service) debugAnthropicToolTest(ctx context.Context, provider store.ModelProvider) (DebugToolTestResult, error) {
	tools := anthropicTools()
	toolChoice := map[string]any{"type": "auto"}
	toolNames := anthropicToolNames(tools)
	systemText := mergeToolSystemPrompt("")
	payload := map[string]any{
		"model": provider.ModelID,
		"messages": []map[string]any{
			{
				"role":    "user",
				"content": "请调用 load_perseus_toolset 加载 workspace 工具组",
			},
		},
		"tools":       tools,
		"tool_choice": toolChoice,
		"max_tokens":  anthropicAgentMaxTokens,
		"system":      systemText,
	}

	log.Printf("[ai-debug] provider=%s protocol=%s base_url=%s model=%s tools_count=%d tools=%s tool_choice=%v",
		provider.Name, provider.Protocol, provider.BaseURL, provider.ModelID, len(toolNames), strings.Join(toolNames, ","), toolChoice,
	)

	raw, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicMessagesEndpoint(provider.BaseURL), bytes.NewReader(raw))
	if err != nil {
		return DebugToolTestResult{}, err
	}
	applyAnthropicHeaders(req, provider)

	resp, err := s.client.Do(req)
	if err != nil {
		return DebugToolTestResult{
			ProviderName:      provider.Name,
			Protocol:          provider.Protocol,
			BaseURL:           provider.BaseURL,
			ModelID:           provider.ModelID,
			RequestToolsCount: len(toolNames),
			RequestToolsNames: toolNames,
			ToolChoice:        toolChoice,
			FinalStatus:       "request_failed",
			Error:             err.Error(),
		}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return DebugToolTestResult{}, err
	}

	var data anthropicResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return DebugToolTestResult{
			ProviderName:      provider.Name,
			Protocol:          provider.Protocol,
			BaseURL:           provider.BaseURL,
			ModelID:           provider.ModelID,
			RequestToolsCount: len(toolNames),
			RequestToolsNames: toolNames,
			ToolChoice:        toolChoice,
			FinalStatus:       "decode_failed",
			Error:             err.Error(),
			RawResponseSummary: map[string]any{
				"http_status": resp.StatusCode,
				"raw_body":    truncate(string(body), 2000),
			},
		}, nil
	}

	toolUses := data.toolUses()
	result := DebugToolTestResult{
		ProviderName:                provider.Name,
		Protocol:                    provider.Protocol,
		BaseURL:                     provider.BaseURL,
		ModelID:                     provider.ModelID,
		RequestToolsCount:           len(toolNames),
		RequestToolsNames:           toolNames,
		ToolChoice:                  toolChoice,
		RawResponseToolCallDetected: len(toolUses) > 0,
		ParsedToolCalls:             toDebugParsedToolCallsAnthropic(toolUses),
		FinalStatus:                 "no_tool_call",
		RawResponseSummary: map[string]any{
			"http_status":      resp.StatusCode,
			"stop_reason":      data.StopReason,
			"has_tool_use":     len(toolUses) > 0,
			"has_text_content": strings.TrimSpace(data.text()) != "",
			"content_blocks":   truncate(mustJSON(data.Content), 2000),
		},
	}

	logAnthropicResponseSummary("debug-tool-test", data, toolUses)

	if resp.StatusCode >= 400 {
		result.FinalStatus = "provider_error"
		result.Error = fmt.Sprintf("model request failed: %v", data.Error)
		return result, nil
	}

	if len(toolUses) == 0 {
		result.Error = unsupportedToolCallingError().Error()
		return result, nil
	}

	if err := s.executeDebugAnthropicToolCalls(ctx, toolUses); err != nil {
		result.FinalStatus = "tool_execution_failed"
		result.Error = err.Error()
		return result, nil
	}
	result.FinalStatus = "tool_executed"
	return result, nil
}

func (s *Service) executeDebugAnthropicToolCalls(ctx context.Context, toolUses []anthropicToolUse) error {
	for _, call := range toolUses {
		argsRaw, _ := json.Marshal(call.Input)
		if _, _, _, _, err := s.executeTool(ctx, call.Name, string(argsRaw), RunOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func toDebugParsedToolCallsAnthropic(items []anthropicToolUse) []DebugParsedToolCall {
	result := make([]DebugParsedToolCall, 0, len(items))
	for _, item := range items {
		argsRaw, _ := json.Marshal(item.Input)
		result = append(result, DebugParsedToolCall{
			Name:      item.Name,
			Arguments: string(argsRaw),
		})
	}
	return result
}
