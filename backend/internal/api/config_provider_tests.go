package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func runModelProviderTest(r *http.Request, req testModelProviderRequest) modelProviderTestReport {
	started := time.Now()
	report := modelProviderTestReport{StartedAt: started}
	req.Protocol = strings.ToLower(strings.TrimSpace(req.Protocol))
	if req.Protocol == "" {
		req.Protocol = "openai"
	}
	if req.BaseURL == "" || req.APIKey == "" || req.ModelID == "" {
		report.Results = append(report.Results, modelProviderTestResult{Name: "config", Label: "基础配置", Status: "failed", Error: "base_url、api_key、model_id 都是必填项"})
		report.EndedAt = time.Now()
		report.Rating, report.Summary = rateModelProviderTest(report.Results)
		return report
	}
	tests := []struct {
		name  string
		label string
		run   func(context.Context, testModelProviderRequest) error
	}{
		{"non_stream", "普通回复", testProviderNonStream},
		{"stream", "流式回复", testProviderStream},
		{"tool_call", "工具调用", testProviderToolCall},
		{"forced_tool_call", "强制工具调用", testProviderForcedToolCall},
		{"stream_tool_call", "流式工具调用", testProviderStreamToolCall},
	}
	for _, item := range tests {
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		startedAt := time.Now()
		err := item.run(ctx, req)
		cancel()
		result := modelProviderTestResult{Name: item.name, Label: item.label, Status: "passed", DurationMS: time.Since(startedAt).Milliseconds()}
		if err != nil {
			result.Status = "failed"
			result.Error = sanitizeProviderTestError(err)
		}
		report.Results = append(report.Results, result)
	}
	report.EndedAt = time.Now()
	report.Rating, report.Summary = rateModelProviderTest(report.Results)
	return report
}

func rateModelProviderTest(results []modelProviderTestResult) (string, string) {
	passed := 0
	for _, item := range results {
		if item.Status == "passed" {
			passed++
		}
	}
	switch {
	case passed == len(results) && passed > 0:
		return "agent_ready", "适合 Agent"
	case passed >= 2:
		return "usable", "基本可用"
	case passed >= 1:
		return "chat_only", "不建议用于工具任务"
	default:
		return "failed", "连接失败"
	}
}

func testProviderNonStream(ctx context.Context, req testModelProviderRequest) error {
	body := providerTestPayload(req, false, false)
	raw, err := providerTestRequest(ctx, req, body)
	if err != nil {
		return err
	}
	if providerTestHasText(req.Protocol, raw) {
		return nil
	}
	return fmt.Errorf("模型没有返回有效文本")
}

func testProviderStream(ctx context.Context, req testModelProviderRequest) error {
	body := providerTestPayload(req, true, false)
	raw, err := providerTestRequest(ctx, req, body)
	if err != nil {
		return err
	}
	if !bytes.Contains(raw, []byte("data:")) {
		return fmt.Errorf("供应商没有返回 SSE data 事件")
	}
	if providerStreamHasText(raw) {
		return nil
	}
	return fmt.Errorf("流式响应没有文本增量")
}

func testProviderToolCall(ctx context.Context, req testModelProviderRequest) error {
	body := providerTestPayload(req, false, true)
	raw, err := providerTestRequest(ctx, req, body)
	if err != nil {
		return err
	}
	if providerTestHasToolCall(req.Protocol, raw) {
		return nil
	}
	return fmt.Errorf("模型没有返回 native tool call")
}

func testProviderForcedToolCall(ctx context.Context, req testModelProviderRequest) error {
	body := providerTestPayload(req, false, true)
	if req.Protocol == "anthropic" {
		body["tool_choice"] = map[string]any{"type": "tool", "name": "perseus_test_echo"}
	} else {
		body["tool_choice"] = map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": "perseus_test_echo",
			},
		}
	}
	raw, err := providerTestRequest(ctx, req, body)
	if err != nil {
		return err
	}
	if providerTestHasToolCall(req.Protocol, raw) {
		return nil
	}
	return fmt.Errorf("供应商不支持或没有遵守 forced tool_choice")
}

func testProviderStreamToolCall(ctx context.Context, req testModelProviderRequest) error {
	body := providerTestPayload(req, true, true)
	raw, err := providerTestRequest(ctx, req, body)
	if err != nil {
		return err
	}
	if !bytes.Contains(raw, []byte("data:")) {
		return fmt.Errorf("供应商没有返回 SSE data 事件")
	}
	if providerStreamHasToolCall(raw) {
		return nil
	}
	return fmt.Errorf("流式响应没有返回 tool call 增量")
}

func providerTestPayload(req testModelProviderRequest, stream, withTool bool) map[string]any {
	if req.Protocol == "anthropic" {
		payload := map[string]any{
			"model":      req.ModelID,
			"max_tokens": 128,
			"stream":     stream,
			"system":     "You are running a short capability check. Keep responses tiny.",
			"messages": []map[string]any{{
				"role":    "user",
				"content": "Reply with exactly: pong",
			}},
		}
		if withTool {
			payload["tools"] = []map[string]any{providerAnthropicTestTool()}
			payload["messages"] = []map[string]any{{
				"role":    "user",
				"content": "Use the perseus_test_echo tool with text ok. Do not answer normally.",
			}}
			payload["tool_choice"] = map[string]any{"type": "auto"}
		}
		return payload
	}
	payload := map[string]any{
		"model":  req.ModelID,
		"stream": stream,
		"messages": []map[string]any{
			{"role": "system", "content": "You are running a short capability check. Keep responses tiny."},
			{"role": "user", "content": "Reply with exactly: pong"},
		},
	}
	if withTool {
		payload["tools"] = []map[string]any{providerOpenAITestTool()}
		payload["tool_choice"] = "auto"
		payload["messages"] = []map[string]any{
			{"role": "system", "content": "You are running a short capability check."},
			{"role": "user", "content": "Use the perseus_test_echo tool with text ok. Do not answer normally."},
		}
	}
	return payload
}

func providerOpenAITestTool() map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "perseus_test_echo",
			"description": "Return a short echo for capability testing.",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{"text": map[string]any{"type": "string"}},
				"required":   []string{"text"},
			},
		},
	}
}

func providerAnthropicTestTool() map[string]any {
	return map[string]any{
		"name":        "perseus_test_echo",
		"description": "Return a short echo for capability testing.",
		"input_schema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"text": map[string]any{"type": "string"}},
			"required":   []string{"text"},
		},
	}
}

func providerTestRequest(ctx context.Context, req testModelProviderRequest, payload map[string]any) ([]byte, error) {
	raw, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, providerTestEndpoint(req), bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	if req.Protocol == "anthropic" {
		if strings.Contains(strings.ToLower(req.BaseURL), "claude-code-proxy") {
			httpReq.Header.Set("Authorization", bearerToken(req.APIKey))
		} else {
			httpReq.Header.Set("x-api-key", req.APIKey)
		}
		httpReq.Header.Set("anthropic-version", "2023-06-01")
	} else {
		httpReq.Header.Set("Authorization", bearerToken(req.APIKey))
	}
	client := &http.Client{Timeout: 50 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("请求失败：%w", err)
	}
	defer resp.Body.Close()
	rawResp, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("读取响应失败：%w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d：%s", resp.StatusCode, truncateForError(string(rawResp), 240))
	}
	return rawResp, nil
}

func providerTestEndpoint(req testModelProviderRequest) string {
	baseURL := strings.TrimRight(req.BaseURL, "/")
	if req.Protocol == "anthropic" {
		if strings.Contains(strings.ToLower(baseURL), "claude-code-proxy") {
			return baseURL + "/v1/messages"
		}
		return baseURL + "/messages"
	}
	return baseURL + "/chat/completions"
}

func providerTestHasText(protocol string, raw []byte) bool {
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return false
	}
	if protocol == "anthropic" {
		if content, ok := data["content"].([]any); ok {
			for _, item := range content {
				block, _ := item.(map[string]any)
				if text, _ := block["text"].(string); strings.TrimSpace(text) != "" {
					return true
				}
			}
		}
		return false
	}
	choices, _ := data["choices"].([]any)
	for _, item := range choices {
		choice, _ := item.(map[string]any)
		message, _ := choice["message"].(map[string]any)
		if text, _ := message["content"].(string); strings.TrimSpace(text) != "" {
			return true
		}
	}
	return false
}

func providerTestHasToolCall(protocol string, raw []byte) bool {
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return false
	}
	if protocol == "anthropic" {
		content, _ := data["content"].([]any)
		for _, item := range content {
			block, _ := item.(map[string]any)
			if block["type"] == "tool_use" && block["name"] == "perseus_test_echo" {
				input, _ := block["input"].(map[string]any)
				text, _ := input["text"].(string)
				return strings.TrimSpace(text) != ""
			}
		}
		return false
	}
	choices, _ := data["choices"].([]any)
	for _, item := range choices {
		choice, _ := item.(map[string]any)
		message, _ := choice["message"].(map[string]any)
		if calls, ok := message["tool_calls"].([]any); ok && len(calls) > 0 {
			call, _ := calls[0].(map[string]any)
			function, _ := call["function"].(map[string]any)
			if function["name"] != "perseus_test_echo" {
				continue
			}
			argsText, _ := function["arguments"].(string)
			var args map[string]any
			if err := json.Unmarshal([]byte(argsText), &args); err != nil {
				continue
			}
			text, _ := args["text"].(string)
			if strings.TrimSpace(text) != "" {
				return true
			}
		}
	}
	return false
}

func providerStreamHasText(raw []byte) bool {
	return scanSSEJSON(raw, func(data string) bool {
		return strings.Contains(data, "content") || strings.Contains(data, "text_delta")
	})
}

func providerStreamHasToolCall(raw []byte) bool {
	return scanSSEJSON(raw, func(data string) bool {
		return strings.Contains(data, "tool_calls") || strings.Contains(data, "tool_use") || strings.Contains(data, "input_json_delta")
	})
}

func scanSSEJSON(raw []byte, match func(string) bool) bool {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 2<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		if match(data) {
			return true
		}
	}
	return false
}

func sanitizeProviderTestError(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	text = strings.ReplaceAll(text, "\n", " ")
	return truncateForError(text, 260)
}
