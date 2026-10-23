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

func (s *Service) debugOpenAIToolTest(ctx context.Context, provider store.ModelProvider) (DebugToolTestResult, error) {
	tools := openAITools()
	toolChoice := "auto"
	toolNames := openAIToolNames(tools)
	messages := prependOpenAISystem([]map[string]any{
		{
			"role":    "user",
			"content": "请调用 load_perseus_toolset 加载 workspace 工具组",
		},
	})
	payload := map[string]any{
		"model":       provider.ModelID,
		"messages":    messages,
		"tools":       tools,
		"tool_choice": toolChoice,
	}

	log.Printf("[ai-debug] provider=%s protocol=%s base_url=%s model=%s tools_count=%d tools=%s tool_choice=%v",
		provider.Name, provider.Protocol, provider.BaseURL, provider.ModelID, len(toolNames), strings.Join(toolNames, ","), toolChoice,
	)

	raw, _ := json.Marshal(normalizeOpenAIProviderPayload(provider, payload))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIChatEndpoint(provider.BaseURL), bytes.NewReader(raw))
	if err != nil {
		return DebugToolTestResult{}, err
	}
	applyOpenAIHeaders(req, provider)

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

	var data openAIResponse
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

	result := DebugToolTestResult{
		ProviderName:      provider.Name,
		Protocol:          provider.Protocol,
		BaseURL:           provider.BaseURL,
		ModelID:           provider.ModelID,
		RequestToolsCount: len(toolNames),
		RequestToolsNames: toolNames,
		ToolChoice:        toolChoice,
		FinalStatus:       "no_tool_call",
		RawResponseSummary: map[string]any{
			"http_status": resp.StatusCode,
		},
	}

	if len(data.Choices) == 0 {
		result.FinalStatus = "empty_response"
		result.Error = "empty model response"
		return result, nil
	}

	choice := data.Choices[0]
	parsedToolCalls := extractOpenAIToolCalls(choice.Message)
	result.RawResponseToolCallDetected = len(choice.Message.ToolCalls) > 0 || (choice.Message.FunctionCall != nil && choice.Message.FunctionCall.Name != "")
	result.ParsedToolCalls = toDebugParsedToolCallsOpenAI(parsedToolCalls)
	result.RawResponseSummary["finish_reason"] = choice.FinishReason
	result.RawResponseSummary["has_tool_calls"] = len(choice.Message.ToolCalls) > 0
	result.RawResponseSummary["has_function_call"] = choice.Message.FunctionCall != nil && choice.Message.FunctionCall.Name != ""
	result.RawResponseSummary["has_assistant_content"] = strings.TrimSpace(normalizeOpenAIContent(choice.Message.Content)) != ""
	result.RawResponseSummary["message_tool_calls_count"] = len(choice.Message.ToolCalls)
	result.RawResponseSummary["message_function_call_name"] = valueOrEmpty(choice.Message.FunctionCall)
	result.RawResponseSummary["raw_choice_tool_fields"] = map[string]any{
		"finish_reason":             choice.FinishReason,
		"message_has_tool_calls":    len(choice.Message.ToolCalls) > 0,
		"message_has_function_call": choice.Message.FunctionCall != nil && choice.Message.FunctionCall.Name != "",
	}

	logOpenAIResponseSummary("debug-tool-test", choice, parsedToolCalls)

	if resp.StatusCode >= 400 {
		result.FinalStatus = "provider_error"
		result.Error = fmt.Sprintf("model request failed: %v", data.Error)
		return result, nil
	}

	if len(parsedToolCalls) == 0 {
		result.Error = unsupportedToolCallingError().Error()
		return result, nil
	}

	if err := s.executeDebugToolCalls(ctx, parsedToolCalls); err != nil {
		result.FinalStatus = "tool_execution_failed"
		result.Error = err.Error()
		return result, nil
	}
	result.FinalStatus = "tool_executed"
	return result, nil
}

func (s *Service) executeDebugToolCalls(ctx context.Context, toolCalls []openAIToolCall) error {
	for _, call := range toolCalls {
		if _, _, _, _, err := s.executeTool(ctx, call.Function.Name, call.Function.ArgumentsJSON(), RunOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func toDebugParsedToolCallsOpenAI(items []openAIToolCall) []DebugParsedToolCall {
	result := make([]DebugParsedToolCall, 0, len(items))
	for _, item := range items {
		result = append(result, DebugParsedToolCall{
			Name:      item.Function.Name,
			Arguments: item.Function.ArgumentsJSON(),
		})
	}
	return result
}
