package ai

import (
	"net/url"
	"strings"

	"webide-backend/internal/store"
)

func providerNativePatchCapability(provider store.ModelProvider) ProviderNativePatchCapability {
	if provider.NativePatchToolEnabled {
		return ProviderNativePatchEnabled
	}
	if strings.ToLower(strings.TrimSpace(provider.Protocol)) != string(agentProtocolOpenAI) {
		return ProviderNativePatchDisabled
	}
	parsed, err := url.Parse(strings.TrimSpace(provider.BaseURL))
	if err != nil {
		return ProviderNativePatchDisabled
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "api.openai.com" {
		return ProviderNativePatchEnabled
	}
	return ProviderNativePatchDisabled
}

func providerNativePatchEnabled(provider store.ModelProvider) bool {
	return providerNativePatchCapability(provider) == ProviderNativePatchEnabled
}
