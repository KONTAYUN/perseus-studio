package ai

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"webide-backend/internal/store"
)

const defaultProviderStreamMaxRetries = 5

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

func providerStreamMaxRetries() int {
	value := strings.TrimSpace(os.Getenv("PERSEUS_PROVIDER_STREAM_MAX_RETRIES"))
	if value == "" {
		return defaultProviderStreamMaxRetries
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultProviderStreamMaxRetries
	}
	if parsed < 0 {
		return 0
	}
	if parsed > 100 {
		return 100
	}
	return parsed
}

func shouldRetryProviderStreamError(ctx context.Context, err error) bool {
	if err == nil || isContextCanceledError(err) || isContextOverflowError(err) {
		return false
	}
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	if _, limited := ProviderRateLimitStateFromError(err); limited {
		return false
	}
	var providerErr *ProviderResponseError
	if errors.As(err, &providerErr) {
		return false
	}
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "empty stream") || strings.Contains(text, "empty anthropic stream") {
		return true
	}
	return isTransientTransportError(err)
}

func providerStreamTurnHasStarted(turn agentModelTurn) bool {
	return strings.TrimSpace(turn.Content) != "" ||
		strings.TrimSpace(turn.PublicStreamedContent) != "" ||
		len(turn.ToolCalls) > 0 ||
		len(turn.OpenAIMessage.ToolCalls) > 0 ||
		len(turn.AnthropicRaw) > 0
}

func providerStreamRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := 200 * time.Millisecond
	for index := 1; index < attempt; index++ {
		delay *= 2
	}
	if delay > 5*time.Second {
		delay = 5 * time.Second
	}
	return jitterProviderRetryDelay(delay, attempt)
}

func emitProviderStreamReconnect(opts RunOptions, provider store.ModelProvider, attempt, maxRetries int, delay time.Duration, err error) {
	payload := map[string]any{
		"attempt":       attempt,
		"max_attempts":  maxRetries,
		"delay_ms":      delay.Milliseconds(),
		"provider_id":   provider.ID,
		"provider_name": provider.Name,
		"model_id":      provider.ModelID,
		"failure_class": providerTransportFailureClass(err),
		"error":         errString(err),
	}
	emitToolLifecycleEvent(opts, "stream_reconnecting", "warn", payload)
}

func clonePayloadWithStream(payload map[string]any, stream bool) map[string]any {
	next := make(map[string]any, len(payload))
	for key, value := range payload {
		next[key] = value
	}
	next["stream"] = stream
	return next
}
