package store

import (
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

func sourceBackedAutoThinkingProfile(_ ModelProvider) ThinkingProfile {
	return noThinkingProfile(ThinkingProfileSourceNone)
}
