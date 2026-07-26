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
	switch provider.AdapterMode {
	case store.ModelAdapterAnthropicBearerProxy:
		return providerCompatClaudeCodeProxy
	case store.ModelAdapterAnthropicNative:
		return providerCompatAnthropic
	case store.ModelAdapterOpenAICompatible:
		return providerCompatOpenAICompatible
	}
	if strings.EqualFold(strings.TrimSpace(provider.Protocol), "anthropic") {
		return providerCompatAnthropic
	}
	return providerCompatOpenAICompatible
}

func openAIChatEndpoint(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/chat/completions"
}

func anthropicMessagesEndpointForProvider(provider store.ModelProvider) string {
	baseURL := provider.BaseURL
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if providerCompat(provider) == providerCompatClaudeCodeProxy {
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

	if provider.AuthMode == store.ModelAuthBearer {
		req.Header.Set("Authorization", bearerToken(apiKey))
		return
	}

	req.Header.Set("x-api-key", apiKey)
}

func bearerToken(value string) string {
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return value
	}
	return "Bearer " + value
}
