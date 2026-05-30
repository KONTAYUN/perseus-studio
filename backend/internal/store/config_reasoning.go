package store

import (
	"net/url"
	"strings"
)

const (
	ReasoningCapabilityAuto              = "auto"
	ReasoningCapabilityDisabled          = "disabled"
	ReasoningCapabilityOpenAIReasoning   = "openai_reasoning"
	ReasoningCapabilityAnthropicAdaptive = "anthropic_adaptive"

	ReasoningCapabilitySourceExplicitOverride  = "explicit_override"
	ReasoningCapabilitySourceProviderDiscovery = "provider_discovery"

	ReasoningSupportNone              = "none"
	ReasoningSupportOpenAI            = "openai"
	ReasoningSupportAnthropicAdaptive = "anthropic_adaptive"

	ThinkingWireFamilyNone              = "none"
	ThinkingWireFamilyOpenAIReasoning   = "openai_reasoning"
	ThinkingWireFamilyAnthropicThinking = "anthropic_thinking"

	ThinkingProfileSourceNone              = "none"
	ThinkingProfileSourceExplicitOverride  = "explicit_override"
	ThinkingProfileSourceProviderDiscovery = "provider_discovery"
	ThinkingProfileSourceModelsDev         = "models_dev"
	ThinkingProfileSourceOpenClawProfile   = "openclaw_profile"
)

type ThinkingProfile struct {
	Levels     []string `json:"levels"`
	Default    string   `json:"default"`
	WireFamily string   `json:"wire_family"`
	Source     string   `json:"source"`
}

func NormalizeReasoningCapability(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", ReasoningCapabilityAuto:
		return ReasoningCapabilityAuto
	case ReasoningCapabilityDisabled:
		return ReasoningCapabilityDisabled
	case ReasoningCapabilityOpenAIReasoning:
		return ReasoningCapabilityOpenAIReasoning
	case ReasoningCapabilityAnthropicAdaptive:
		return ReasoningCapabilityAnthropicAdaptive
	default:
		return ReasoningCapabilityAuto
	}
}

func NormalizeReasoningCapabilitySource(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ReasoningCapabilitySourceExplicitOverride:
		return ReasoningCapabilitySourceExplicitOverride
	case ReasoningCapabilitySourceProviderDiscovery:
		return ReasoningCapabilitySourceProviderDiscovery
	default:
		return ""
	}
}

func NormalizeReasoningProfileLevels(value string) string {
	return strings.Join(ParseReasoningProfileLevels(value), ",")
}

func ParseReasoningProfileLevels(value string) []string {
	seen := map[string]bool{}
	var levels []string
	for _, part := range strings.Split(value, ",") {
		level := NormalizeThinkingProfileLevel(part)
		if level == "" || seen[level] {
			continue
		}
		seen[level] = true
		levels = append(levels, level)
	}
	return levels
}

func NormalizeThinkingProfileLevel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "minimal", "low", "light":
		return "light"
	case "medium", "standard":
		return "standard"
	case "high", "deep":
		return "deep"
	case "xhigh", "max", "expert":
		return "expert"
	default:
		return ""
	}
}

func ResolveModelReasoningSupport(provider ModelProvider) string {
	return ReasoningSupportFromThinkingProfile(ResolveModelThinkingProfile(provider))
}

func ReasoningSupportFromThinkingProfile(profile ThinkingProfile) string {
	switch profile.WireFamily {
	case ThinkingWireFamilyOpenAIReasoning:
		return ReasoningSupportOpenAI
	case ThinkingWireFamilyAnthropicThinking:
		return ReasoningSupportAnthropicAdaptive
	default:
		return ReasoningSupportNone
	}
}

func ResolveModelThinkingProfile(provider ModelProvider) ThinkingProfile {
	capability := NormalizeReasoningCapability(provider.ReasoningCapability)
	protocol := strings.ToLower(strings.TrimSpace(provider.Protocol))
	source := NormalizeReasoningCapabilitySource(provider.ReasoningCapabilitySource)
	sourceLevels := ParseReasoningProfileLevels(provider.ReasoningProfileLevels)
	switch capability {
	case ReasoningCapabilityDisabled:
		return noThinkingProfile(ThinkingProfileSourceExplicitOverride)
	case ReasoningCapabilityOpenAIReasoning:
		if protocol == "openai" {
			return openAIThinkingProfile(sourceForCapability(source), sourceLevels)
		}
		return noThinkingProfile(ThinkingProfileSourceExplicitOverride)
	case ReasoningCapabilityAnthropicAdaptive:
		if protocol == "anthropic" {
			return anthropicThinkingProfile(sourceForCapability(source), sourceLevels)
		}
		return noThinkingProfile(ThinkingProfileSourceExplicitOverride)
	}
	return sourceBackedAutoThinkingProfile(provider)
}

func sourceForCapability(source string) string {
	if source == ReasoningCapabilitySourceProviderDiscovery {
		return ThinkingProfileSourceProviderDiscovery
	}
	return ThinkingProfileSourceExplicitOverride
}

func noThinkingProfile(source string) ThinkingProfile {
	if source == "" {
		source = ThinkingProfileSourceNone
	}
	return ThinkingProfile{
		Levels:     []string{},
		Default:    "standard",
		WireFamily: ThinkingWireFamilyNone,
		Source:     source,
	}
}

func defaultThinkingLevel(levels []string) string {
	for _, preferred := range []string{"standard", "light", "deep", "expert"} {
		for _, level := range levels {
			if level == preferred {
				return preferred
			}
		}
	}
	if len(levels) > 0 {
		return levels[0]
	}
	return "standard"
}

func openAIThinkingProfile(source string, sourceLevels []string) ThinkingProfile {
	if source == ThinkingProfileSourceProviderDiscovery && len(sourceLevels) > 0 {
		return ThinkingProfile{
			Levels:     sourceLevels,
			Default:    defaultThinkingLevel(sourceLevels),
			WireFamily: ThinkingWireFamilyOpenAIReasoning,
			Source:     source,
		}
	}
	levels := []string{"light", "standard", "deep"}
	if source == ThinkingProfileSourceExplicitOverride || openAIProfileSourceAllowsExpert(source) {
		levels = append(levels, "expert")
	}
	return ThinkingProfile{
		Levels:     levels,
		Default:    "standard",
		WireFamily: ThinkingWireFamilyOpenAIReasoning,
		Source:     source,
	}
}

func openAIThinkingProfileWithExpert(source string) ThinkingProfile {
	return ThinkingProfile{
		Levels:     []string{"light", "standard", "deep", "expert"},
		Default:    "standard",
		WireFamily: ThinkingWireFamilyOpenAIReasoning,
		Source:     source,
	}
}

func openAIProfileSourceAllowsExpert(source string) bool {
	return source == ThinkingProfileSourceProviderDiscovery
}

func anthropicThinkingProfile(source string, sourceLevels []string) ThinkingProfile {
	if source == ThinkingProfileSourceProviderDiscovery && len(sourceLevels) > 0 {
		return ThinkingProfile{
			Levels:     sourceLevels,
			Default:    defaultThinkingLevel(sourceLevels),
			WireFamily: ThinkingWireFamilyAnthropicThinking,
			Source:     source,
		}
	}
	return ThinkingProfile{
		Levels:     []string{"standard", "deep", "expert"},
		Default:    "standard",
		WireFamily: ThinkingWireFamilyAnthropicThinking,
		Source:     source,
	}
}

func sourceBackedAutoThinkingProfile(provider ModelProvider) ThinkingProfile {
	protocol := strings.ToLower(strings.TrimSpace(provider.Protocol))
	modelID := normalizeModelID(provider.ModelID)
	switch protocol {
	case "openai":
		if isOfficialOpenAIBaseURL(provider.BaseURL) {
			if profile, ok := staticOpenAIThinkingProfiles[modelID]; ok {
				return profile
			}
		}
	case "anthropic":
		if _, ok := staticAnthropicAdaptiveModels[modelID]; ok {
			return anthropicThinkingProfile(ThinkingProfileSourceOpenClawProfile, nil)
		}
	}
	return noThinkingProfile(ThinkingProfileSourceNone)
}

func IsOfficialOpenAIBaseURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "api.openai.com"
}

func isOfficialOpenAIBaseURL(value string) bool {
	return IsOfficialOpenAIBaseURL(value)
}

func normalizeModelID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// Static OpenAI model entries are an exact-id snapshot from models.dev's
// official OpenAI provider records where `reasoning=true` (checked
// 2026-05-22). The optional expert/xhigh level follows OpenClaw's
// provider-owned OpenAI thinking profile, but is expanded here as exact model
// IDs so this project does not add local prefix guessing.
var staticOpenAIThinkingProfiles = map[string]ThinkingProfile{
	"gpt-5":                 openAIThinkingProfile(ThinkingProfileSourceModelsDev, nil),
	"gpt-5-mini":            openAIThinkingProfile(ThinkingProfileSourceModelsDev, nil),
	"gpt-5-nano":            openAIThinkingProfile(ThinkingProfileSourceModelsDev, nil),
	"gpt-5-pro":             openAIThinkingProfile(ThinkingProfileSourceModelsDev, nil),
	"gpt-5-codex":           openAIThinkingProfile(ThinkingProfileSourceModelsDev, nil),
	"gpt-5-chat-latest":     openAIThinkingProfile(ThinkingProfileSourceModelsDev, nil),
	"gpt-5.1":               openAIThinkingProfile(ThinkingProfileSourceModelsDev, nil),
	"gpt-5.1-chat-latest":   openAIThinkingProfile(ThinkingProfileSourceModelsDev, nil),
	"gpt-5.1-codex":         openAIThinkingProfile(ThinkingProfileSourceModelsDev, nil),
	"gpt-5.1-codex-max":     openAIThinkingProfile(ThinkingProfileSourceModelsDev, nil),
	"gpt-5.1-codex-mini":    openAIThinkingProfile(ThinkingProfileSourceModelsDev, nil),
	"gpt-5.2":               openAIThinkingProfileWithExpert(ThinkingProfileSourceModelsDev),
	"gpt-5.2-chat-latest":   openAIThinkingProfileWithExpert(ThinkingProfileSourceModelsDev),
	"gpt-5.2-codex":         openAIThinkingProfileWithExpert(ThinkingProfileSourceModelsDev),
	"gpt-5.2-pro":           openAIThinkingProfileWithExpert(ThinkingProfileSourceModelsDev),
	"gpt-5.3-codex":         openAIThinkingProfile(ThinkingProfileSourceModelsDev, nil),
	"gpt-5.3-codex-spark":   openAIThinkingProfile(ThinkingProfileSourceModelsDev, nil),
	"gpt-5.4":               openAIThinkingProfileWithExpert(ThinkingProfileSourceModelsDev),
	"gpt-5.4-mini":          openAIThinkingProfileWithExpert(ThinkingProfileSourceModelsDev),
	"gpt-5.4-nano":          openAIThinkingProfileWithExpert(ThinkingProfileSourceModelsDev),
	"gpt-5.4-pro":           openAIThinkingProfileWithExpert(ThinkingProfileSourceModelsDev),
	"gpt-5.5":               openAIThinkingProfileWithExpert(ThinkingProfileSourceModelsDev),
	"gpt-5.5-pro":           openAIThinkingProfileWithExpert(ThinkingProfileSourceModelsDev),
	"o1":                    openAIThinkingProfile(ThinkingProfileSourceModelsDev, nil),
	"o1-mini":               openAIThinkingProfile(ThinkingProfileSourceModelsDev, nil),
	"o1-preview":            openAIThinkingProfile(ThinkingProfileSourceModelsDev, nil),
	"o1-pro":                openAIThinkingProfile(ThinkingProfileSourceModelsDev, nil),
	"o3":                    openAIThinkingProfile(ThinkingProfileSourceModelsDev, nil),
	"o3-mini":               openAIThinkingProfile(ThinkingProfileSourceModelsDev, nil),
	"o3-pro":                openAIThinkingProfile(ThinkingProfileSourceModelsDev, nil),
	"o3-deep-research":      openAIThinkingProfile(ThinkingProfileSourceModelsDev, nil),
	"o4-mini":               openAIThinkingProfile(ThinkingProfileSourceModelsDev, nil),
	"o4-mini-deep-research": openAIThinkingProfile(ThinkingProfileSourceModelsDev, nil),
}

// Anthropic adaptive thinking support follows OpenClaw's Claude thinking
// profile: Claude 4.6 defaults to adaptive, and Opus 4.7 exposes adaptive/max
// effort. Older Claude reasoning models are not enabled here because this
// runtime does not implement the older budget_tokens wire shape in normal
// chat.
var staticAnthropicAdaptiveModels = map[string]struct{}{
	"claude-opus-4-6":   {},
	"claude-opus-4.6":   {},
	"claude-sonnet-4-6": {},
	"claude-sonnet-4.6": {},
	"claude-opus-4-7":   {},
	"claude-opus-4.7":   {},
}
