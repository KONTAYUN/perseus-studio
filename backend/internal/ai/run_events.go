package ai

import (
	"net/url"
	"strings"

	"webide-backend/internal/store"
)

func providerAttemptObserver(opts RunOptions, provider store.ModelProvider, stage string) RequestAttemptObserver {
	if opts.Event == nil {
		return nil
	}
	return func(event RequestAttemptEvent) {
		if event.Type == "attempt_started" && opts.OnProviderAttempt != nil {
			opts.OnProviderAttempt()
		}
		eventType := "model_call_attempt_started"
		level := "info"
		switch event.Type {
		case "attempt_retrying":
			eventType = "model_call_retrying"
			level = "warn"
		case "attempt_completed":
			eventType = "model_call_attempt_completed"
			if event.StatusCode >= 400 {
				level = "warn"
			}
		case "attempt_failed":
			eventType = "model_call_attempt_failed"
			level = "error"
		}
		payload := map[string]any{
			"stage":          stage,
			"provider_id":    provider.ID,
			"provider_name":  provider.Name,
			"protocol":       provider.Protocol,
			"model_id":       provider.ModelID,
			"attempt":        event.Attempt,
			"max_attempts":   event.MaxAttempts,
			"duration_ms":    event.DurationMS,
			"delay_ms":       event.DelayMS,
			"retry_after_ms": event.RetryAfterMS,
			"retriable":      event.Retriable,
		}
		if event.StatusCode > 0 {
			payload["http_status"] = event.StatusCode
		}
		if strings.TrimSpace(event.Error) != "" {
			payload["error"] = event.Error
		}
		if parsed, err := url.Parse(strings.TrimSpace(event.Endpoint)); err == nil {
			payload["base_url_host"] = parsed.Host
			path := parsed.EscapedPath()
			if path == "" {
				path = parsed.Path
			}
			payload["final_url_path"] = path
		}
		opts.Event(eventType, level, payload)
	}
}
