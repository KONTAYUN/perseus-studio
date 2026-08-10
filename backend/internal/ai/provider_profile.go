package ai

import (
	"strings"

	"webide-backend/internal/store"
)

type providerRuntimeProfile struct {
	Name                      string
	ModelID                   string
	AdapterMode               string
	SupportsTools             string
	SupportsParallelToolCalls string
	ParallelCapabilitySource  string
	ToolArgumentStreamMode    string
	SupportsVision            string
	CapabilitySource          string
	ContextWindowTokens       int
	ContextWindowSource       string
	AutoCompactEnabled        bool
	CompactThresholdPercent   int
	CompactReservedTokens     int
	PruneToolOutputsEnabled   bool
}

func runtimeProfileForProvider(provider store.ModelProvider) providerRuntimeProfile {
	windowTokens, windowSource := providerContextWindowProfile(provider)
	profile := providerRuntimeProfile{
		Name:                      "token_budget",
		ModelID:                   provider.ModelID,
		AdapterMode:               provider.AdapterMode,
		SupportsTools:             provider.SupportsTools,
		SupportsParallelToolCalls: provider.SupportsParallelToolCalls,
		ParallelCapabilitySource:  provider.ParallelCapabilitySource,
		ToolArgumentStreamMode:    provider.ToolArgumentStreamMode,
		SupportsVision:            provider.SupportsVision,
		CapabilitySource:          provider.CapabilitySource,
		ContextWindowTokens:       windowTokens,
		ContextWindowSource:       windowSource,
		AutoCompactEnabled:        provider.AutoCompactEnabled,
		CompactThresholdPercent:   provider.AutoCompactThresholdPercent,
		CompactReservedTokens:     provider.CompactReservedTokens,
		PruneToolOutputsEnabled:   provider.PruneToolOutputsEnabled,
	}
	if profile.CompactThresholdPercent == 0 {
		profile.CompactThresholdPercent = 80
	}
	if profile.ContextWindowTokens <= 0 {
		profile.ContextWindowTokens = defaultContextWindowTokens
	}
	if profile.CompactReservedTokens == 0 {
		profile.CompactReservedTokens = defaultCompactReservedTokens(profile.ContextWindowTokens)
	}
	return profile
}

func normalizedToolArgumentStreamMode(provider store.ModelProvider) string {
	switch strings.ToLower(strings.TrimSpace(provider.ToolArgumentStreamMode)) {
	case "delta", "snapshot":
		return strings.ToLower(strings.TrimSpace(provider.ToolArgumentStreamMode))
	}
	return "delta"
}

const defaultContextWindowTokens = 128000

func defaultCompactReservedTokens(windowTokens int) int {
	switch {
	case windowTokens >= 900000:
		return 128000
	case windowTokens >= 500000:
		return 80000
	case windowTokens >= 200000:
		return 48000
	case windowTokens >= 100000:
		return 24000
	case windowTokens >= 64000:
		return 16000
	default:
		reserved := windowTokens / 4
		if reserved < 4000 {
			return 4000
		}
		return reserved
	}
}

var builtInContextWindows = map[string]int{
	"gpt-4o":                   128000,
	"gpt-4o-mini":              128000,
	"gpt-4.1":                  1000000,
	"gpt-4.1-mini":             1000000,
	"gpt-4.1-nano":             1000000,
	"o3":                       200000,
	"o3-mini":                  200000,
	"o4-mini":                  200000,
	"claude-3-5-sonnet-latest": 200000,
	"claude-3-7-sonnet-latest": 200000,
	"claude-sonnet-4-20250514": 200000,
	"claude-opus-4-20250514":   200000,
}

type visualTokenStrategy string

const (
	visualTokenStrategyOpenAI    visualTokenStrategy = "openai_tiles"
	visualTokenStrategyAnthropic visualTokenStrategy = "anthropic_pixels"
)

var builtInVisualTokenStrategies = map[string]visualTokenStrategy{
	"gpt-4o":                   visualTokenStrategyOpenAI,
	"gpt-4o-mini":              visualTokenStrategyOpenAI,
	"gpt-4.1":                  visualTokenStrategyOpenAI,
	"gpt-4.1-mini":             visualTokenStrategyOpenAI,
	"gpt-4.1-nano":             visualTokenStrategyOpenAI,
	"o3":                       visualTokenStrategyOpenAI,
	"o4-mini":                  visualTokenStrategyOpenAI,
	"claude-3-5-sonnet-latest": visualTokenStrategyAnthropic,
	"claude-3-7-sonnet-latest": visualTokenStrategyAnthropic,
	"claude-sonnet-4-20250514": visualTokenStrategyAnthropic,
	"claude-opus-4-20250514":   visualTokenStrategyAnthropic,
}

func providerContextWindowProfile(provider store.ModelProvider) (int, string) {
	if provider.ContextWindowTokens > 0 {
		return provider.ContextWindowTokens, "user"
	}
	modelID := strings.ToLower(strings.TrimSpace(provider.ModelID))
	if tokens, ok := builtInContextWindows[modelID]; ok {
		return tokens, "builtin"
	}
	return defaultContextWindowTokens, "default"
}

func providerContextWindowTokens(provider store.ModelProvider) int {
	tokens, _ := providerContextWindowProfile(provider)
	return tokens
}

func ContextWindowProfileInfo(provider store.ModelProvider) (int, string) {
	return providerContextWindowProfile(provider)
}
