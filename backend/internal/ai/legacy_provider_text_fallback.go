package ai

import "strings"

// Some OpenAI-compatible gateways return only a display message without an
// HTTP status or structured error fields. This compatibility path is isolated
// and never marks an error as safely retryable.
func legacyProviderTextFailure(values ...string) ProviderFailure {
	text := strings.ToLower(strings.Join(values, "\n"))
	failure := ProviderFailure{Retryable: false, ClassificationSource: "legacy_text_fallback"}
	switch {
	case strings.Contains(text, "rate limit") || strings.Contains(text, "too many requests"):
		failure.Class = ProviderFailureRateLimited
	case strings.Contains(text, "context length") || strings.Contains(text, "context window"):
		failure.Class = ProviderFailureContextExceeded
	case strings.Contains(text, "model not found") || strings.Contains(text, "no available channel for model"):
		failure.Class = ProviderFailureModelUnavailable
	default:
		failure.Class = ProviderFailureUnknown
	}
	return failure
}
