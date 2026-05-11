package ai

import (
	"net/http"
	"strings"

	"webide-backend/internal/store"
)

type providerCompatKind string

const (
	providerCompatOpenAICompatible providerCompatKind = "openai_compatible"
	providerCompatAnthropic        providerCompatKind = "anthropic"
	providerCompatClaudeCodeProxy  providerCompatKind = "claude_code_proxy"
)

func providerCompat(provider store.ModelProvider) providerCompatKind {
	if usesClaudeCodeProxy(provider.BaseURL) {
		return providerCompatClaudeCodeProxy
	}
	if strings.EqualFold(strings.TrimSpace(provider.Protocol), "anthropic") {
		return providerCompatAnthropic
	}
	return providerCompatOpenAICompatible
}

func openAIChatEndpoint(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/chat/completions"
}

func anthropicMessagesEndpoint(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if usesClaudeCodeProxy(base) {
		return base + "/v1/messages"
	}
	return base + "/messages"
}

func applyOpenAIHeaders(req *http.Request, provider store.ModelProvider) {
	req.Header.Set("Content-Type", "application/json")

	apiKey := strings.TrimSpace(provider.APIKey)
	if apiKey == "" {
		return
	}
	req.Header.Set("Authorization", bearerToken(apiKey))
}

func applyAnthropicHeaders(req *http.Request, provider store.ModelProvider) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")

	apiKey := strings.TrimSpace(provider.APIKey)
	if apiKey == "" {
		return
	}

	if providerCompat(provider) == providerCompatClaudeCodeProxy {
		req.Header.Set("Authorization", bearerToken(apiKey))
		return
	}

	req.Header.Set("x-api-key", apiKey)
}

func usesClaudeCodeProxy(baseURL string) bool {
	return strings.Contains(strings.ToLower(baseURL), "claude-code-proxy")
}

func bearerToken(value string) string {
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return value
	}
	return "Bearer " + value
}
