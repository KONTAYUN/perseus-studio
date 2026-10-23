package ai

import (
	"strings"

	"webide-backend/internal/store"
)

func providerNativePatchCapability(provider store.ModelProvider) ProviderNativePatchCapability {
	if provider.NativePatchToolEnabled {
		return ProviderNativePatchEnabled
	}
	// This is an ordinary function tool on both protocols, with no provider-
	// specific API extension. Compatible endpoints need the same write channel.
	switch strings.ToLower(strings.TrimSpace(provider.Protocol)) {
	case "", string(agentProtocolOpenAI), string(agentProtocolAnthropic):
		return ProviderNativePatchEnabled
	}
	return ProviderNativePatchDisabled
}

func providerNativePatchEnabled(provider store.ModelProvider) bool {
	return providerNativePatchCapability(provider) == ProviderNativePatchEnabled
}
