package ai

import (
	"context"
	"os"
	"strings"
)

func providerStreamFallbackEnabled() bool {
	value := strings.TrimSpace(os.Getenv("PERSEUS_PROVIDER_STREAM_FALLBACK_ENABLED"))
	return value == "" || !strings.EqualFold(value, "false")
}

func shouldFallbackAfterStreamError(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	return providerStreamFallbackEnabled()
}

func clonePayloadWithStream(payload map[string]any, stream bool) map[string]any {
	next := make(map[string]any, len(payload))
	for key, value := range payload {
		next[key] = value
	}
	next["stream"] = stream
	return next
}
