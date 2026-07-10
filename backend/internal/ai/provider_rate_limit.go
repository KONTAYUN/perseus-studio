package ai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"webide-backend/internal/store"
)

const defaultProviderRateLimitCooldown = 60 * time.Second

type ProviderRateLimitKind string

const (
	ProviderRateLimitTPM     ProviderRateLimitKind = "tpm"
	ProviderRateLimitRPM     ProviderRateLimitKind = "rpm"
	ProviderRateLimitQuota   ProviderRateLimitKind = "quota"
	ProviderRateLimitUnknown ProviderRateLimitKind = "unknown"
)

type ProviderRateLimitState struct {
	Kind              ProviderRateLimitKind `json:"kind"`
	UserID            string                `json:"-"`
	ProviderID        string                `json:"provider_id"`
	ProviderName      string                `json:"provider_name,omitempty"`
	ModelID           string                `json:"model_id"`
	StatusCode        int                   `json:"status_code"`
	RetryAfterMS      int64                 `json:"retry_after_ms"`
	ResetAt           time.Time             `json:"reset_at"`
	ResetTimeInferred bool                  `json:"reset_time_inferred"`
	RequestID         string                `json:"request_id,omitempty"`
	ObservedAt        time.Time             `json:"observed_at"`
}

func (s ProviderRateLimitState) Active(now time.Time) bool {
	return !s.ResetAt.IsZero() && now.Before(s.ResetAt)
}

func (s ProviderRateLimitState) Remaining(now time.Time) time.Duration {
	if !s.Active(now) {
		return 0
	}
	return s.ResetAt.Sub(now)
}

type ProviderRateLimitError struct {
	State ProviderRateLimitState
	Err   error
}

func (e *ProviderRateLimitError) Error() string {
	if e == nil {
		return ""
	}
	label := "模型供应商限流"
	switch e.State.Kind {
	case ProviderRateLimitTPM:
		label = "模型供应商 TPM 限流"
	case ProviderRateLimitRPM:
		label = "模型供应商 RPM 限流"
	case ProviderRateLimitQuota:
		label = "模型供应商额度不足"
	}
	remaining := e.State.Remaining(time.Now().UTC())
	if remaining > 0 {
		seconds := int64(remaining.Round(time.Second) / time.Second)
		if seconds < 1 {
			seconds = 1
		}
		return fmt.Sprintf("%s，请等待约 %d 秒后继续，或切换模型", label, seconds)
	}
	return label
}

func (e *ProviderRateLimitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type AgentRunFailureMetadata struct {
	FailureClass      string    `json:"failure_class,omitempty"`
	RetryAfterMS      int64     `json:"retry_after_ms,omitempty"`
	ResetAt           time.Time `json:"reset_at,omitempty"`
	ProviderID        string    `json:"provider_id,omitempty"`
	ModelID           string    `json:"model_id,omitempty"`
	CanResumeNow      bool      `json:"can_resume_now"`
	RecommendedAction string    `json:"recommended_action,omitempty"`
}

func FailureMetadataFromError(err error) AgentRunFailureMetadata {
	metadata := AgentRunFailureMetadata{CanResumeNow: true, RecommendedAction: "continue"}
	if err == nil {
		return metadata
	}
	var limited *ProviderRateLimitError
	if errors.As(err, &limited) {
		metadata.FailureClass = rateLimitFailureClass(limited.State.Kind)
		metadata.RetryAfterMS = limited.State.Remaining(time.Now().UTC()).Milliseconds()
		metadata.ResetAt = limited.State.ResetAt
		metadata.ProviderID = limited.State.ProviderID
		metadata.ModelID = limited.State.ModelID
		metadata.CanResumeNow = metadata.RetryAfterMS <= 0
		metadata.RecommendedAction = "wait_or_switch_model"
		return metadata
	}
	if isContextOverflowError(err) {
		metadata.FailureClass = "context_window_exceeded"
		metadata.RecommendedAction = "compact_then_continue"
		return metadata
	}
	if class := providerTransportFailureClass(err); class != "" {
		metadata.FailureClass = class
		metadata.RecommendedAction = "continue"
		return metadata
	}
	var providerErr *ProviderResponseError
	if errors.As(err, &providerErr) {
		metadata.FailureClass = providerResponseFailureClass(providerErr, err)
		if metadata.FailureClass == "" {
			metadata.FailureClass = "provider_http_error"
		}
		metadata.RecommendedAction = "review_provider_error"
	}
	return metadata
}

func rateLimitFailureClass(kind ProviderRateLimitKind) string {
	switch kind {
	case ProviderRateLimitTPM:
		return "provider_tpm_rate_limited"
	case ProviderRateLimitRPM:
		return "provider_rpm_rate_limited"
	case ProviderRateLimitQuota:
		return "provider_quota_exhausted"
	default:
		return "provider_rate_limited"
	}
}

func (s *Service) classifyProviderError(opts RunOptions, provider store.ModelProvider, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := ProviderRateLimitStateFromError(err); ok {
		return err
	}
	var providerErr *ProviderResponseError
	if !errors.As(err, &providerErr) && !looksLikeProviderRateLimit(err.Error()) {
		return err
	}
	if providerErr != nil && providerErr.StatusCode != http.StatusTooManyRequests && !looksLikeProviderRateLimit(providerErr.Error()) {
		return err
	}
	now := time.Now().UTC()
	state := ProviderRateLimitState{
		Kind:         classifyProviderRateLimitKind(err),
		UserID:       opts.UserID,
		ProviderID:   provider.ID,
		ProviderName: provider.Name,
		ModelID:      provider.ModelID,
		StatusCode:   http.StatusTooManyRequests,
		ObservedAt:   now,
	}
	if providerErr != nil {
		if providerErr.StatusCode > 0 {
			state.StatusCode = providerErr.StatusCode
		}
		state.RequestID = providerErr.RequestID
		state.ResetAt = parseProviderRateLimitReset(providerErr.RetryAfter, providerErr.RateLimitReset, now)
	}
	if state.ResetAt.IsZero() {
		state.ResetAt = now.Add(defaultProviderRateLimitCooldown)
		state.ResetTimeInferred = true
	}
	state.RetryAfterMS = state.Remaining(now).Milliseconds()
	s.setProviderRateLimitState(state)
	emitToolLifecycleEvent(opts, "provider_rate_limited", "error", providerRateLimitEventPayload(state, now))
	emitToolLifecycleEvent(opts, "provider_cooldown_updated", "warn", providerRateLimitEventPayload(state, now))
	return &ProviderRateLimitError{State: state, Err: err}
}

func ProviderRateLimitStateFromError(err error) (ProviderRateLimitState, bool) {
	var limited *ProviderRateLimitError
	if !errors.As(err, &limited) || limited == nil {
		return ProviderRateLimitState{}, false
	}
	return limited.State, true
}

func (s *Service) ActiveProviderRateLimit(ctx context.Context, userID, role, providerID string) (ProviderRateLimitState, bool, error) {
	provider, err := s.configs.GetModelProviderForUser(ctx, userID, normalizeRunUserRole(role), providerID)
	if err != nil {
		return ProviderRateLimitState{}, false, err
	}
	state, ok := s.activeProviderRateLimitState(userID, provider.ID, provider.ModelID, time.Now().UTC())
	return state, ok, nil
}

func (s *Service) activeProviderRateLimitState(userID, providerID, modelID string, now time.Time) (ProviderRateLimitState, bool) {
	if s == nil {
		return ProviderRateLimitState{}, false
	}
	key := providerRateLimitKey(userID, providerID, modelID)
	s.rateLimitMu.Lock()
	defer s.rateLimitMu.Unlock()
	state, ok := s.rateLimits[key]
	if !ok {
		return ProviderRateLimitState{}, false
	}
	if !state.Active(now) {
		delete(s.rateLimits, key)
		return ProviderRateLimitState{}, false
	}
	state.RetryAfterMS = state.Remaining(now).Milliseconds()
	return state, true
}

func (s *Service) setProviderRateLimitState(state ProviderRateLimitState) {
	if s == nil {
		return
	}
	s.rateLimitMu.Lock()
	defer s.rateLimitMu.Unlock()
	if s.rateLimits == nil {
		s.rateLimits = map[string]ProviderRateLimitState{}
	}
	s.rateLimits[providerRateLimitKey(state.UserID, state.ProviderID, state.ModelID)] = state
}

func providerRateLimitKey(userID, providerID, modelID string) string {
	return strings.Join([]string{strings.TrimSpace(userID), strings.TrimSpace(providerID), strings.TrimSpace(modelID)}, "\x00")
}

func providerRateLimitEventPayload(state ProviderRateLimitState, now time.Time) map[string]any {
	return map[string]any{
		"failure_class":       rateLimitFailureClass(state.Kind),
		"kind":                state.Kind,
		"provider_id":         state.ProviderID,
		"provider_name":       state.ProviderName,
		"model_id":            state.ModelID,
		"status_code":         state.StatusCode,
		"retry_after_ms":      state.Remaining(now).Milliseconds(),
		"reset_at":            state.ResetAt,
		"reset_time_inferred": state.ResetTimeInferred,
		"request_id":          state.RequestID,
		"can_resume_now":      !state.Active(now),
		"recommended_action":  "wait_or_switch_model",
	}
}

func classifyProviderRateLimitKind(err error) ProviderRateLimitKind {
	text := strings.ToLower(errString(err))
	switch {
	case strings.Contains(text, "insufficient_quota") || strings.Contains(text, "quota exceeded") || strings.Contains(text, "额度"):
		return ProviderRateLimitQuota
	case strings.Contains(text, "tpm") || strings.Contains(text, "token per minute") || strings.Contains(text, "tokens per minute"):
		return ProviderRateLimitTPM
	case strings.Contains(text, "rpm") || strings.Contains(text, "request per minute") || strings.Contains(text, "requests per minute"):
		return ProviderRateLimitRPM
	default:
		return ProviderRateLimitUnknown
	}
}

func looksLikeProviderRateLimit(value string) bool {
	text := strings.ToLower(value)
	return strings.Contains(text, "rate limit") || strings.Contains(text, "too many requests") || strings.Contains(text, "tpm") || strings.Contains(text, "rpm")
}

func providerRateLimitResetHeader(header http.Header) string {
	for _, name := range []string{"x-ratelimit-reset-tokens", "x-ratelimit-reset-requests", "x-ratelimit-reset"} {
		if value := strings.TrimSpace(header.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

func providerRequestIDHeader(header http.Header) string {
	for _, name := range []string{"x-request-id", "request-id", "x-tencent-request-id"} {
		if value := strings.TrimSpace(header.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

func parseProviderRateLimitReset(retryAfter, reset string, now time.Time) time.Time {
	if delay := parseRetryAfter(retryAfter, now); delay > 0 {
		return now.Add(delay)
	}
	reset = strings.TrimSpace(reset)
	if reset == "" {
		return time.Time{}
	}
	if delay, err := time.ParseDuration(reset); err == nil && delay > 0 {
		return now.Add(delay)
	}
	if seconds, err := strconv.ParseFloat(reset, 64); err == nil {
		if seconds > float64(now.Unix()) {
			return time.Unix(int64(seconds), 0).UTC()
		}
		if seconds > 0 {
			return now.Add(time.Duration(seconds * float64(time.Second)))
		}
	}
	if at, err := http.ParseTime(reset); err == nil && at.After(now) {
		return at.UTC()
	}
	return time.Time{}
}
