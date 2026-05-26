package ai

import "webide-backend/internal/store"

const (
	openAIReasoningEffortLow    = "low"
	openAIReasoningEffortMedium = "medium"
	openAIReasoningEffortHigh   = "high"
	openAIReasoningEffortXHigh  = "xhigh"

	anthropicReasoningEffortMedium = "medium"
	anthropicReasoningEffortHigh   = "high"
	anthropicReasoningEffortMax    = "max"
)

type providerReasoningPolicy struct {
	Support         string
	Profile         store.ThinkingProfile
	ThinkingMode    string
	OpenAIEffort    string
	AnthropicEffort string
}

func nativeReasoningPolicy(provider store.ModelProvider, thinkingMode string) providerReasoningPolicy {
	mode := normalizeThinkingMode(thinkingMode)
	profile := store.ResolveModelThinkingProfile(provider)
	policy := providerReasoningPolicy{
		Support:      store.ReasoningSupportFromThinkingProfile(profile),
		Profile:      profile,
		ThinkingMode: mode,
	}
	switch profile.WireFamily {
	case store.ThinkingWireFamilyOpenAIReasoning:
		policy.OpenAIEffort = openAIReasoningEffort(mode)
		if !thinkingProfileAllowsMode(profile, mode) {
			policy.OpenAIEffort = openAIReasoningEffort(profile.Default)
			policy.ThinkingMode = normalizeThinkingMode(profile.Default)
		}
	case store.ThinkingWireFamilyAnthropicThinking:
		policy.AnthropicEffort = anthropicAdaptiveEffort(mode)
		if !thinkingProfileAllowsMode(profile, mode) {
			policy.AnthropicEffort = anthropicAdaptiveEffort(profile.Default)
			policy.ThinkingMode = normalizeThinkingMode(profile.Default)
		}
	}
	return policy
}

func (p providerReasoningPolicy) OpenAIEnabled() bool {
	return p.OpenAIEffort != ""
}

func (p providerReasoningPolicy) AnthropicAdaptiveEnabled() bool {
	return p.AnthropicEffort != ""
}

func applyOpenAIReasoningPayload(payload map[string]any, provider store.ModelProvider, thinkingMode string) providerReasoningPolicy {
	policy := nativeReasoningPolicy(provider, thinkingMode)
	if policy.OpenAIEnabled() {
		payload["reasoning_effort"] = policy.OpenAIEffort
	}
	return policy
}

func applyAnthropicReasoningPayload(payload map[string]any, provider store.ModelProvider, thinkingMode string) providerReasoningPolicy {
	policy := nativeReasoningPolicy(provider, thinkingMode)
	if policy.AnthropicAdaptiveEnabled() {
		payload["thinking"] = map[string]any{"type": "adaptive"}
		payload["output_config"] = map[string]any{"effort": policy.AnthropicEffort}
	}
	return policy
}

func thinkingProfileAllowsMode(profile store.ThinkingProfile, mode string) bool {
	normalized := normalizeThinkingMode(mode)
	for _, level := range profile.Levels {
		if normalizeThinkingMode(level) == normalized {
			return true
		}
	}
	return false
}

func openAIReasoningEffort(mode string) string {
	switch normalizeThinkingMode(mode) {
	case ThinkingModeLight:
		return openAIReasoningEffortLow
	case ThinkingModeDeep:
		return openAIReasoningEffortHigh
	case ThinkingModeExpert:
		return openAIReasoningEffortXHigh
	default:
		return openAIReasoningEffortMedium
	}
}

func anthropicAdaptiveEffort(mode string) string {
	switch normalizeThinkingMode(mode) {
	case ThinkingModeDeep:
		return anthropicReasoningEffortHigh
	case ThinkingModeExpert:
		return anthropicReasoningEffortMax
	default:
		return anthropicReasoningEffortMedium
	}
}
