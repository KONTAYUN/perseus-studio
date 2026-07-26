package store

import (
	"fmt"
	"strings"
)

const (
	ResourceVisibilityPrivate = "private"
	ResourceVisibilityPublic  = "public"

	ModelAdapterOpenAICompatible     = "openai_compatible"
	ModelAdapterAnthropicNative      = "anthropic_native"
	ModelAdapterAnthropicBearerProxy = "anthropic_bearer_proxy"
	ModelAuthBearer                  = "bearer"
	ModelAuthAPIKey                  = "x_api_key"
	ModelCapabilitySupported         = "supported"
	ModelCapabilityUnsupported       = "unsupported"
	ModelCapabilityUnknown           = "unknown"
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
	if !validModelAdapterMode(input.AdapterMode) {
		return fmt.Errorf("adapter_mode is invalid")
	}
	if !validModelAuthMode(input.AuthMode) {
		return fmt.Errorf("auth_mode is invalid")
	}
	if !validModelCapability(input.SupportsTools) || !validModelCapability(input.SupportsVision) {
		return fmt.Errorf("model capabilities must be supported, unsupported, or unknown")
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
		&item.AdapterMode,
		&item.AuthMode,
		&item.SupportsTools,
		&item.SupportsVision,
		&item.CapabilitySource,
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
	item.AdapterMode = normalizeModelAdapterMode(item.AdapterMode, item.Protocol)
	item.AuthMode = normalizeModelAuthMode(item.AuthMode, item.AdapterMode)
	item.SupportsTools = normalizeModelCapability(item.SupportsTools)
	item.SupportsVision = normalizeModelCapability(item.SupportsVision)
	if strings.TrimSpace(item.CapabilitySource) == "" {
		item.CapabilitySource = "default"
	}
	item.ThinkingProfile = ResolveModelThinkingProfile(item)
	item.ReasoningSupport = ReasoningSupportFromThinkingProfile(item.ThinkingProfile)
	item.MaskedKey = maskKey(item.APIKey)
	return item, err
}

func validModelAdapterMode(value string) bool {
	switch strings.TrimSpace(value) {
	case "", ModelAdapterOpenAICompatible, ModelAdapterAnthropicNative, ModelAdapterAnthropicBearerProxy:
		return true
	default:
		return false
	}
}

func normalizeModelAdapterMode(value, protocol string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	if strings.EqualFold(protocol, "anthropic") {
		return ModelAdapterAnthropicNative
	}
	return ModelAdapterOpenAICompatible
}

func validModelAuthMode(value string) bool {
	switch strings.TrimSpace(value) {
	case "", ModelAuthBearer, ModelAuthAPIKey:
		return true
	default:
		return false
	}
}

func normalizeModelAuthMode(value, adapterMode string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	if adapterMode == ModelAdapterAnthropicNative {
		return ModelAuthAPIKey
	}
	return ModelAuthBearer
}

func validModelCapability(value string) bool {
	switch strings.TrimSpace(value) {
	case "", ModelCapabilitySupported, ModelCapabilityUnsupported, ModelCapabilityUnknown:
		return true
	default:
		return false
	}
}

func normalizeModelCapability(value string) string {
	switch strings.TrimSpace(value) {
	case ModelCapabilitySupported:
		return ModelCapabilitySupported
	case ModelCapabilityUnsupported:
		return ModelCapabilityUnsupported
	default:
		return ModelCapabilityUnknown
	}
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
