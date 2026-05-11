package ai

import (
	"strings"

	"webide-backend/internal/store"
)

type providerRuntimeProfile struct {
	Name                     string
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
}

func runtimeProfileForProvider(provider store.ModelProvider) providerRuntimeProfile {
	profile := providerRuntimeProfile{
		Name:                     "standard",
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
	return profile
}
