package ai

import (
	"context"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultProviderRequestMaxAttempts = 3
	defaultProviderRetryBaseDelayMS   = 750
	defaultProviderRetryMaxDelayMS    = 5000
)

type ProviderRetryPolicy struct {
	MaxAttempts int           `json:"max_attempts"`
	BaseDelay   time.Duration `json:"-"`
	MaxDelay    time.Duration `json:"-"`
}

func (p ProviderRetryPolicy) diagnosticMap() map[string]any {
	return map[string]any{
		"request_max_attempts": p.MaxAttempts,
		"base_delay_ms":        p.BaseDelay.Milliseconds(),
		"max_delay_ms":         p.MaxDelay.Milliseconds(),
		"retry_statuses":       []int{408, 425, 429, 500, 502, 503, 504, 529},
		"retry_transport":      true,
		"stream_fallback":      providerStreamFallbackEnabled(),
	}
}

func currentProviderRetryPolicy() ProviderRetryPolicy {
	maxAttempts := envInt("PERSEUS_PROVIDER_REQUEST_MAX_ATTEMPTS", defaultProviderRequestMaxAttempts)
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	if maxAttempts > 10 {
		maxAttempts = 10
	}
	baseDelayMS := envInt("PERSEUS_PROVIDER_RETRY_BASE_DELAY_MS", defaultProviderRetryBaseDelayMS)
	if baseDelayMS < 0 {
		baseDelayMS = 0
	}
	maxDelayMS := envInt("PERSEUS_PROVIDER_RETRY_MAX_DELAY_MS", defaultProviderRetryMaxDelayMS)
	if maxDelayMS < 0 {
		maxDelayMS = 0
	}
	if maxDelayMS > 0 && baseDelayMS > maxDelayMS {
		baseDelayMS = maxDelayMS
	}
	return ProviderRetryPolicy{
		MaxAttempts: maxAttempts,
		BaseDelay:   time.Duration(baseDelayMS) * time.Millisecond,
		MaxDelay:    time.Duration(maxDelayMS) * time.Millisecond,
	}
}

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func shouldRetryProviderAttempt(err error, statusCode int, attempt int, policy ProviderRetryPolicy) bool {
	return isRetryableProviderAttemptError(err, statusCode) && attempt < policy.MaxAttempts
}

func isRetryableProviderAttemptError(err error, statusCode int) bool {
	if isContextCanceledError(err) {
		return false
	}
	if statusCode > 0 {
		return isRetriableProviderStatusIncluding529(statusCode)
	}
	if err != nil {
		return isTransientTransportError(err)
	}
	return false
}

func providerRetryDelay(policy ProviderRetryPolicy, attempt int, resp *http.Response) (time.Duration, time.Duration) {
	if resp != nil {
		if retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()); retryAfter > 0 {
			return retryAfter, retryAfter
		}
	}
	if policy.BaseDelay <= 0 {
		return 0, 0
	}
	exp := math.Pow(2, float64(max(0, attempt-1)))
	delay := time.Duration(float64(policy.BaseDelay) * exp)
	if policy.MaxDelay > 0 && delay > policy.MaxDelay {
		delay = policy.MaxDelay
	}
	delay = jitterProviderRetryDelay(delay, attempt)
	if policy.MaxDelay > 0 && delay > policy.MaxDelay {
		delay = policy.MaxDelay
	}
	return delay, 0
}

func jitterProviderRetryDelay(delay time.Duration, attempt int) time.Duration {
	if delay <= 0 {
		return 0
	}
	seed := time.Now().UnixNano() + int64(attempt*37)
	percent := 90 + seed%21
	return time.Duration(int64(delay) * percent / 100)
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseFloat(value, 64); err == nil {
		if math.IsNaN(seconds) || seconds <= 0 {
			return 0
		}
		if math.IsInf(seconds, 1) || seconds >= float64(math.MaxInt64)/float64(time.Second) {
			return time.Duration(math.MaxInt64)
		}
		return time.Duration(seconds * float64(time.Second))
	}
	if at, err := http.ParseTime(value); err == nil {
		if at.After(now) {
			return at.Sub(now)
		}
	}
	return 0
}

func sleepProviderRetry(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return true
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
