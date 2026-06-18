package store

import (
	"fmt"
	"strings"
)

const (
	ResourceVisibilityPrivate = "private"
	ResourceVisibilityPublic  = "public"
)

func NormalizeResourceVisibility(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ResourceVisibilityPublic:
		return ResourceVisibilityPublic
	default:
		return ResourceVisibilityPrivate
	}
}

func validateModelProvider(input ModelProviderInput, requireKey bool) error {
	if strings.TrimSpace(input.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(input.Visibility) != "" && NormalizeResourceVisibility(input.Visibility) != strings.ToLower(strings.TrimSpace(input.Visibility)) {
		return fmt.Errorf("visibility must be public or private")
	}
	if input.Protocol != "openai" && input.Protocol != "anthropic" {
		return fmt.Errorf("protocol must be openai or anthropic")
	}
	if strings.TrimSpace(input.ReasoningCapability) != "" && NormalizeReasoningCapability(input.ReasoningCapability) != strings.ToLower(strings.TrimSpace(input.ReasoningCapability)) {
		return fmt.Errorf("reasoning_capability must be auto, disabled, openai_reasoning, or anthropic_adaptive")
	}
	if strings.TrimSpace(input.ReasoningCapabilitySource) != "" && NormalizeReasoningCapabilitySource(input.ReasoningCapabilitySource) != strings.ToLower(strings.TrimSpace(input.ReasoningCapabilitySource)) {
		return fmt.Errorf("reasoning_capability_source must be provider_discovery or explicit_override")
	}
	if strings.TrimSpace(input.ReasoningProfileLevels) != "" && NormalizeReasoningProfileLevels(input.ReasoningProfileLevels) == "" {
		return fmt.Errorf("reasoning_profile_levels contains no supported levels")
	}
	if NormalizeReasoningCapabilitySource(input.ReasoningCapabilitySource) != ReasoningCapabilitySourceProviderDiscovery && strings.TrimSpace(input.ReasoningProfileLevels) != "" {
		return fmt.Errorf("reasoning_profile_levels requires provider_discovery source")
	}
	if strings.TrimSpace(input.BaseURL) == "" {
		return fmt.Errorf("base_url is required")
	}
	if strings.TrimSpace(input.ModelID) == "" {
		return fmt.Errorf("model_id is required")
	}
	if requireKey && input.APIKey == "" {
		return fmt.Errorf("api_key is required")
	}
	settings := normalizeModelProviderContextSettings(input)
	if settings.ContextWindowTokens < 0 {
		return fmt.Errorf("context_window_tokens must be >= 0")
	}
	if settings.AutoCompactThresholdPercent < 50 || settings.AutoCompactThresholdPercent > 95 {
		return fmt.Errorf("auto_compact_threshold_percent must be between 50 and 95")
	}
	if settings.CompactReservedTokens < 0 {
		return fmt.Errorf("compact_reserved_tokens must be >= 0")
	}
	return nil
}

func validateSearchProvider(input SearchProviderInput, requireKey bool) error {
	if input.Provider != "tavily" && input.Provider != "brave" {
		return fmt.Errorf("provider must be tavily or brave")
	}
	if strings.TrimSpace(input.Visibility) != "" && NormalizeResourceVisibility(input.Visibility) != strings.ToLower(strings.TrimSpace(input.Visibility)) {
		return fmt.Errorf("visibility must be public or private")
	}
	if requireKey && input.APIKey == "" {
		return fmt.Errorf("api_key is required")
	}
	return nil
}

func scanModelProvider(s scanner) (ModelProvider, error) {
	var item ModelProvider
	var enabled int
	var isDefault int
	var autoCompactEnabled int
	var pruneToolOutputsEnabled int
	var nativePatchToolEnabled int
	err := s.Scan(
		&item.ID,
		&item.UserID,
		&item.Name,
		&item.Visibility,
		&item.Protocol,
		&item.BaseURL,
		&item.APIKey,
		&item.ModelID,
		&enabled,
		&isDefault,
		&item.ContextWindowTokens,
		&autoCompactEnabled,
		&item.AutoCompactThresholdPercent,
		&item.CompactReservedTokens,
		&pruneToolOutputsEnabled,
		&nativePatchToolEnabled,
		&item.ReasoningCapability,
		&item.ReasoningCapabilitySource,
		&item.ReasoningProfileLevels,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	item.Enabled = enabled == 1
	item.IsDefault = isDefault == 1
	item.Visibility = NormalizeResourceVisibility(item.Visibility)
	item.AutoCompactEnabled = autoCompactEnabled == 1
	item.PruneToolOutputsEnabled = pruneToolOutputsEnabled == 1
	item.NativePatchToolEnabled = nativePatchToolEnabled == 1
	item.ReasoningCapability = NormalizeReasoningCapability(item.ReasoningCapability)
	item.ReasoningCapabilitySource = NormalizeReasoningCapabilitySource(item.ReasoningCapabilitySource)
	item.ReasoningProfileLevels = NormalizeReasoningProfileLevels(item.ReasoningProfileLevels)
	item.ThinkingProfile = ResolveModelThinkingProfile(item)
	item.ReasoningSupport = ReasoningSupportFromThinkingProfile(item.ThinkingProfile)
	item.MaskedKey = maskKey(item.APIKey)
	return item, err
}

type modelProviderContextSettings struct {
	ContextWindowTokens         int
	AutoCompactEnabled          bool
	AutoCompactThresholdPercent int
	CompactReservedTokens       int
	PruneToolOutputsEnabled     bool
	NativePatchToolEnabled      bool
}

func normalizeModelProviderContextSettings(input ModelProviderInput) modelProviderContextSettings {
	autoCompactEnabled := true
	if input.AutoCompactEnabled != nil {
		autoCompactEnabled = *input.AutoCompactEnabled
	}
	pruneToolOutputsEnabled := true
	if input.PruneToolOutputsEnabled != nil {
		pruneToolOutputsEnabled = *input.PruneToolOutputsEnabled
	}
	nativePatchToolEnabled := false
	if input.NativePatchToolEnabled != nil {
		nativePatchToolEnabled = *input.NativePatchToolEnabled
	}
	threshold := input.AutoCompactThresholdPercent
	if threshold == 0 {
		threshold = 80
	}
	return modelProviderContextSettings{
		ContextWindowTokens:         input.ContextWindowTokens,
		AutoCompactEnabled:          autoCompactEnabled,
		AutoCompactThresholdPercent: threshold,
		CompactReservedTokens:       input.CompactReservedTokens,
		PruneToolOutputsEnabled:     pruneToolOutputsEnabled,
		NativePatchToolEnabled:      nativePatchToolEnabled,
	}
}

func scanSearchProvider(s scanner) (SearchProvider, error) {
	var item SearchProvider
	var enabled int
	err := s.Scan(&item.ID, &item.UserID, &item.Provider, &item.Visibility, &item.APIKey, &enabled, &item.Priority, &item.CreatedAt, &item.UpdatedAt)
	item.Visibility = NormalizeResourceVisibility(item.Visibility)
	item.Enabled = enabled == 1
	item.MaskedKey = maskKey(item.APIKey)
	return item, err
}

func maskKey(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "••••"
	}
	return value[:4] + "••••" + value[len(value)-4:]
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
