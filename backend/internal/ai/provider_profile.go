package ai

import (
	"os"
	"strconv"
	"strings"

	"webide-backend/internal/store"
)

type providerRuntimeProfile struct {
	Name                     string
	ContextWindowTokens      int
	AutoCompactEnabled       bool
	CompactThresholdPercent  int
	CompactReservedTokens    int
	PruneToolOutputsEnabled  bool
	AggressiveContextPruning bool
	MaxHistoryMessages       int
	MaxRecentMessageRunes    int
	MaxOlderMessageRunes     int
	MaxAttachmentTextRunes   int
	MaxSkillTextRunes        int
	MaxToolResultRunes       int
	MaxCommandOutputRunes    int
	MaxReadFileContentRunes  int
	MaxWebSearchSnippetRunes int
	MaxWebExtractRunes       int
	PayloadSoftBudgetBytes   int
	PayloadHardBudgetBytes   int
}

func runtimeProfileForProvider(provider store.ModelProvider) providerRuntimeProfile {
	profile := providerRuntimeProfile{
		Name:                     "standard",
		ContextWindowTokens:      providerContextWindowTokens(provider),
		AutoCompactEnabled:       provider.AutoCompactEnabled,
		CompactThresholdPercent:  provider.AutoCompactThresholdPercent,
		CompactReservedTokens:    provider.CompactReservedTokens,
		PruneToolOutputsEnabled:  provider.PruneToolOutputsEnabled,
		MaxHistoryMessages:       60,
		MaxRecentMessageRunes:    12000,
		MaxOlderMessageRunes:     3000,
		MaxAttachmentTextRunes:   12000,
		MaxSkillTextRunes:        12000,
		MaxToolResultRunes:       14000,
		MaxCommandOutputRunes:    6000,
		MaxReadFileContentRunes:  18000,
		MaxWebSearchSnippetRunes: 700,
		MaxWebExtractRunes:       12000,
		PayloadSoftBudgetBytes:   envIntDefault("PERSEUS_PROVIDER_PAYLOAD_SOFT_BYTES", 96*1024),
		PayloadHardBudgetBytes:   envIntDefault("PERSEUS_PROVIDER_PAYLOAD_HARD_BYTES", 128*1024),
	}

	protocol := strings.ToLower(strings.TrimSpace(provider.Protocol))
	modelText := strings.ToLower(provider.Name + " " + provider.ModelID + " " + provider.BaseURL)
	if protocol == "anthropic" || strings.Contains(modelText, "claude") || usesClaudeCodeProxy(provider.BaseURL) {
		profile.Name = "claude_like"
		profile.MaxHistoryMessages = 50
		profile.MaxRecentMessageRunes = 10000
		profile.MaxOlderMessageRunes = 2200
		profile.MaxAttachmentTextRunes = 10000
		profile.MaxSkillTextRunes = 10000
		profile.MaxToolResultRunes = 10000
		profile.MaxCommandOutputRunes = 4000
		profile.MaxReadFileContentRunes = 14000
		profile.MaxWebSearchSnippetRunes = 500
		profile.MaxWebExtractRunes = 8000
	}
	if strings.Contains(modelText, "kat-coder") || strings.Contains(modelText, "claude-code-proxy") {
		profile.Name = "strict_claude_proxy"
		profile.MaxHistoryMessages = 44
		profile.MaxRecentMessageRunes = 9000
		profile.MaxOlderMessageRunes = 1800
		profile.MaxAttachmentTextRunes = 9000
		profile.MaxSkillTextRunes = 9000
		profile.MaxToolResultRunes = 8500
		profile.MaxCommandOutputRunes = 3500
		profile.MaxReadFileContentRunes = 12000
		profile.MaxWebSearchSnippetRunes = 450
		profile.MaxWebExtractRunes = 7000
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
	if profile.PayloadSoftBudgetBytes <= 0 {
		profile.PayloadSoftBudgetBytes = 96 * 1024
	}
	if profile.PayloadHardBudgetBytes <= profile.PayloadSoftBudgetBytes {
		profile.PayloadHardBudgetBytes = profile.PayloadSoftBudgetBytes + 32*1024
	}
	return profile
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

func providerContextWindowTokens(provider store.ModelProvider) int {
	if provider.ContextWindowTokens > 0 {
		return provider.ContextWindowTokens
	}
	modelText := strings.ToLower(provider.Name + " " + provider.ModelID + " " + provider.BaseURL)
	switch {
	case strings.Contains(modelText, "gpt-4.1") || strings.Contains(modelText, "gpt-5"):
		return 1000000
	case strings.Contains(modelText, "gemini-1.5") || strings.Contains(modelText, "gemini-2"):
		return 1000000
	case strings.Contains(modelText, "claude"):
		return 200000
	case strings.Contains(modelText, "gpt-4o") || strings.Contains(modelText, "o3") || strings.Contains(modelText, "o4"):
		return 128000
	default:
		return defaultContextWindowTokens
	}
}

func envIntDefault(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
