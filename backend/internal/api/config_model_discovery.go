package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func discoverModels(r *http.Request, req discoverModelsRequest) ([]discoveredModel, error) {
	if req.Protocol == "" {
		req.Protocol = "openai"
	}
	if req.BaseURL == "" {
		return nil, fmt.Errorf("base_url is required")
	}
	if req.APIKey == "" {
		return nil, fmt.Errorf("api_key is required")
	}

	baseURL := strings.TrimRight(req.BaseURL, "/")
	endpoints, headerMode, err := modelDiscoveryEndpoints(req.Protocol, baseURL)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, endpoint := range endpoints {
		models, err := fetchDiscoveredModels(r, endpoint, req.APIKey, headerMode)
		if err == nil {
			return models, nil
		}
		lastErr = err
	}
	if req.Protocol == "anthropic" {
		return nil, fmt.Errorf("Anthropic 协议自动获取模型列表失败：%w；部分中转不支持 models 接口，请手动填写模型 ID", lastErr)
	}
	return nil, lastErr
}

func modelDiscoveryEndpoints(protocol string, baseURL string) ([]string, string, error) {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "", "openai":
		return []string{baseURL + "/models"}, "openai", nil
	case "anthropic":
		if strings.Contains(strings.ToLower(baseURL), "claude-code-proxy") {
			return []string{baseURL + "/v1/models", baseURL + "/models"}, "anthropic_bearer", nil
		}
		return []string{baseURL + "/models"}, "anthropic", nil
	default:
		return nil, "", fmt.Errorf("不支持的模型列表协议：%s", protocol)
	}
}

func fetchDiscoveredModels(r *http.Request, endpoint string, apiKey string, headerMode string) ([]discoveredModel, error) {
	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "application/json")
	switch headerMode {
	case "anthropic":
		httpReq.Header.Set("x-api-key", apiKey)
		httpReq.Header.Set("anthropic-version", "2023-06-01")
	case "anthropic_bearer":
		httpReq.Header.Set("Authorization", bearerToken(apiKey))
		httpReq.Header.Set("anthropic-version", "2023-06-01")
	default:
		httpReq.Header.Set("Authorization", bearerToken(apiKey))
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("获取模型列表失败：%w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("读取模型列表失败：%w", err)
	}
	contentType := resp.Header.Get("Content-Type")
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("供应商返回非成功响应：HTTP %d；content-type=%s；响应前300字符=%q", resp.StatusCode, contentType, truncateForError(string(raw), 300))
	}
	if strings.Contains(strings.ToLower(contentType), "text/html") || bytes.HasPrefix(bytes.TrimSpace(raw), []byte("<")) {
		return nil, fmt.Errorf("供应商返回 HTML 页面，base_url 可能不是 API 地址；请确认是否应填写类似 https://example.com/v1")
	}

	models, err := parseDiscoveredModels(raw)
	if err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("未从模型列表响应中找到模型 ID")
	}
	return models, nil
}

func bearerToken(value string) string {
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return value
	}
	return "Bearer " + value
}

func contextWithTimeout(r *http.Request, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), timeout)
}

func parseDiscoveredModels(raw []byte) ([]discoveredModel, error) {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("模型列表不是有效 JSON：%w", err)
	}
	seen := map[string]bool{}
	var models []discoveredModel
	add := func(id, name string) {
		id = strings.TrimSpace(id)
		name = strings.TrimSpace(name)
		if id == "" || seen[id] {
			return
		}
		if name == "" {
			name = id
		}
		seen[id] = true
		models = append(models, discoveredModel{ID: id, Name: name})
	}

	var walkItem func(any)
	walkItem = func(item any) {
		switch value := item.(type) {
		case string:
			add(value, value)
		case map[string]any:
			id, _ := value["id"].(string)
			name, _ := value["name"].(string)
			if name == "" {
				name, _ = value["display_name"].(string)
			}
			if name == "" {
				name, _ = value["owned_by"].(string)
			}
			add(id, name)
		}
	}

	switch value := payload.(type) {
	case map[string]any:
		if data, ok := value["data"].([]any); ok {
			for _, item := range data {
				walkItem(item)
			}
		}
		if data, ok := value["models"].([]any); ok {
			for _, item := range data {
				walkItem(item)
			}
		}
	case []any:
		for _, item := range value {
			walkItem(item)
		}
	}
	return models, nil
}

func truncateForError(value string, limit int) string {
	runes := []rune(strings.ReplaceAll(value, "\n", " "))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}
