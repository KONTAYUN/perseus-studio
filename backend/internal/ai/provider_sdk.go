package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	openaisdk "github.com/openai/openai-go"
	openaioption "github.com/openai/openai-go/option"

	"webide-backend/internal/store"
)

const providerSDKMaxRetries = 0

func (s *Service) openAISDKPostRaw(ctx context.Context, provider store.ModelProvider, path string, payload map[string]any, opts RunOptions, stage string, summary *ProviderRequestSummary) (*http.Response, error) {
	policy := currentProviderRetryPolicy()
	if summary != nil {
		summary.ProviderRetryPolicy = policy.diagnosticMap()
		summary.SDKMaxAttempts = policy.MaxAttempts
	}
	client, err := s.openAISDKClient(provider, opts, stage, policy, summary)
	if err != nil {
		return nil, err
	}
	var resp *http.Response
	err = client.Post(ctx, strings.TrimLeft(path, "/"), payload, &resp)
	if err != nil {
		err = normalizeProviderSDKError(err)
	}
	return resp, err
}

func (s *Service) anthropicSDKPostRaw(ctx context.Context, provider store.ModelProvider, path string, payload map[string]any, opts RunOptions, stage string, summary *ProviderRequestSummary) (*http.Response, error) {
	policy := currentProviderRetryPolicy()
	if summary != nil {
		summary.ProviderRetryPolicy = policy.diagnosticMap()
		summary.SDKMaxAttempts = policy.MaxAttempts
	}
	client, err := s.anthropicSDKClient(provider, opts, stage, policy, summary)
	if err != nil {
		return nil, err
	}
	var resp *http.Response
	err = client.Post(ctx, strings.TrimLeft(path, "/"), payload, &resp)
	if err != nil {
		err = normalizeProviderSDKError(err)
	}
	return resp, err
}

func (s *Service) openAISDKClient(provider store.ModelProvider, opts RunOptions, stage string, policy ProviderRetryPolicy, summary *ProviderRequestSummary) (openaisdk.Client, error) {
	baseURL, err := providerSDKBaseURL(provider.BaseURL)
	if err != nil {
		return openaisdk.Client{}, err
	}
	options := []openaioption.RequestOption{
		openaioption.WithBaseURL(baseURL),
		openaioption.WithHTTPClient(s.sdkHTTPClient()),
		openaioption.WithMaxRetries(providerSDKMaxRetries),
		openaioption.WithHeaderDel("authorization"),
		openaioption.WithHeaderDel("OpenAI-Organization"),
		openaioption.WithHeaderDel("OpenAI-Project"),
		openaioption.WithMiddleware(openAISDKAttemptMiddleware(providerAttemptObserver(opts, provider, stage), policy, summary)),
	}
	if apiKey := strings.TrimSpace(provider.APIKey); apiKey != "" {
		options = append(options, openaioption.WithHeader("Authorization", bearerToken(apiKey)))
	}
	return openaisdk.NewClient(options...), nil
}

func (s *Service) anthropicSDKClient(provider store.ModelProvider, opts RunOptions, stage string, policy ProviderRetryPolicy, summary *ProviderRequestSummary) (anthropicsdk.Client, error) {
	baseURL, err := providerSDKBaseURL(provider.BaseURL)
	if err != nil {
		return anthropicsdk.Client{}, err
	}
	options := []anthropicoption.RequestOption{
		anthropicoption.WithBaseURL(baseURL),
		anthropicoption.WithHTTPClient(s.sdkHTTPClient()),
		anthropicoption.WithMaxRetries(providerSDKMaxRetries),
		anthropicoption.WithHeaderDel("x-api-key"),
		anthropicoption.WithHeaderDel("authorization"),
		anthropicoption.WithHeader("anthropic-version", "2023-06-01"),
		anthropicoption.WithMiddleware(anthropicSDKAttemptMiddleware(providerAttemptObserver(opts, provider, stage), policy, summary)),
	}
	if apiKey := strings.TrimSpace(provider.APIKey); apiKey != "" {
		if providerCompat(provider) == providerCompatClaudeCodeProxy {
			options = append(options, anthropicoption.WithHeader("Authorization", bearerToken(apiKey)))
		} else {
			options = append(options, anthropicoption.WithHeader("x-api-key", apiKey))
		}
	}
	return anthropicsdk.NewClient(options...), nil
}

func (s *Service) sdkHTTPClient() *http.Client {
	if s != nil && s.client != nil {
		return s.client
	}
	return http.DefaultClient
}

func providerSDKBaseURL(baseURL string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", fmt.Errorf("model provider base URL is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid model provider base URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid model provider base URL: %s", baseURL)
	}
	return baseURL, nil
}

func providerSDKEndpoint(baseURL, path string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/" + strings.TrimLeft(path, "/")
}

func normalizeProviderSDKError(err error) error {
	if err == nil {
		return nil
	}
	var openAIErr *openaisdk.Error
	if errors.As(err, &openAIErr) {
		return providerResponseErrorFromSDK(openAIErr.StatusCode, openAIErr.Response, openAIErr.Request, openAIErr.RawJSON(), err)
	}
	var anthropicErr *anthropicsdk.Error
	if errors.As(err, &anthropicErr) {
		return providerResponseErrorFromSDK(anthropicErr.StatusCode, anthropicErr.Response, anthropicErr.Request, anthropicErr.RawJSON(), err)
	}
	return err
}

func providerResponseErrorFromSDK(statusCode int, resp *http.Response, req *http.Request, rawJSON string, fallback error) error {
	contentType := ""
	endpoint := ""
	if resp != nil {
		contentType = resp.Header.Get("Content-Type")
		if resp.Request != nil && resp.Request.URL != nil {
			endpoint = resp.Request.URL.String()
		}
	}
	if endpoint == "" && req != nil && req.URL != nil {
		endpoint = req.URL.String()
	}
	snippet := responseSnippet([]byte(rawJSON), 500)
	message := ""
	var raw any
	if strings.TrimSpace(rawJSON) != "" && json.Unmarshal([]byte(rawJSON), &raw) == nil && raw != nil {
		message = formatProviderError(statusCode, raw).Error()
	}
	if message == "" && fallback != nil {
		message = fallback.Error()
	}
	return &ProviderResponseError{
		StatusCode:      statusCode,
		ContentType:     contentType,
		Endpoint:        endpoint,
		ResponseSnippet: snippet,
		Message:         message,
	}
}

func openAIChatPath() string {
	return "chat/completions"
}

func anthropicMessagesPath(provider store.ModelProvider) string {
	if providerCompat(provider) == providerCompatClaudeCodeProxy {
		return "v1/messages"
	}
	return "messages"
}

func openAISDKAttemptMiddleware(observer RequestAttemptObserver, policy ProviderRetryPolicy, summary *ProviderRequestSummary) openaioption.Middleware {
	var attempt int64
	return func(req *http.Request, next openaioption.MiddlewareNext) (*http.Response, error) {
		return runSDKAttempt(req, func(r *http.Request) (*http.Response, error) {
			return next(r)
		}, observer, &attempt, policy, summary)
	}
}

func anthropicSDKAttemptMiddleware(observer RequestAttemptObserver, policy ProviderRetryPolicy, summary *ProviderRequestSummary) anthropicoption.Middleware {
	var attempt int64
	return func(req *http.Request, next anthropicoption.MiddlewareNext) (*http.Response, error) {
		return runSDKAttempt(req, func(r *http.Request) (*http.Response, error) {
			return next(r)
		}, observer, &attempt, policy, summary)
	}
}

func runSDKAttempt(req *http.Request, next func(*http.Request) (*http.Response, error), observer RequestAttemptObserver, attemptCounter *int64, policy ProviderRetryPolicy, summary *ProviderRequestSummary) (*http.Response, error) {
	if req != nil && req.Body != nil && req.GetBody == nil {
		raw, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		_ = req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(raw))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(raw)), nil
		}
	}
	var lastResp *http.Response
	var lastErr error
	for {
		if req != nil && req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			req.Body = body
		}
		attempt := int(atomic.AddInt64(attemptCounter, 1))
		endpoint := ""
		if req != nil && req.URL != nil {
			endpoint = req.URL.String()
		}
		if summary != nil {
			summary.RequestAttempts = attempt
		}
		if observer != nil {
			observer(RequestAttemptEvent{
				Type:        "attempt_started",
				Attempt:     attempt,
				MaxAttempts: policy.MaxAttempts,
				Endpoint:    endpoint,
			})
		}
		startedAt := time.Now()
		resp, err := next(req)
		duration := time.Since(startedAt).Milliseconds()
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		retryableError := isRetryableProviderAttemptError(err, status)
		retryable := retryableError && attempt < policy.MaxAttempts
		if summary != nil {
			summary.RetryableError = retryableError
		}
		if !retryable {
			emitSDKAttemptDone(observer, attempt, policy.MaxAttempts, duration, endpoint, status, err, retryable, 0, 0)
			return resp, err
		}
		delay, retryAfter := providerRetryDelay(policy, attempt, resp)
		if summary != nil {
			summary.RetryAfterMS = retryAfter.Milliseconds()
		}
		emitSDKAttemptDone(observer, attempt, policy.MaxAttempts, duration, endpoint, status, err, retryable, delay, retryAfter)
		lastResp, lastErr = resp, err
		discardRetryResponse(resp)
		if req != nil && req.Context() != nil {
			if !sleepProviderRetry(req.Context(), delay) {
				if req.Context().Err() != nil {
					return nil, req.Context().Err()
				}
				return lastResp, lastErr
			}
		}
	}
}

func emitSDKAttemptDone(observer RequestAttemptObserver, attempt, maxAttempts int, duration int64, endpoint string, status int, err error, retryable bool, delay, retryAfter time.Duration) {
	if observer == nil {
		return
	}
	if retryable {
		event := RequestAttemptEvent{
			Type:         "attempt_retrying",
			Attempt:      attempt,
			MaxAttempts:  maxAttempts,
			StatusCode:   status,
			DurationMS:   duration,
			DelayMS:      delay.Milliseconds(),
			RetryAfterMS: retryAfter.Milliseconds(),
			Retriable:    true,
			Endpoint:     endpoint,
		}
		if err != nil {
			event.Error = err.Error()
		}
		observer(event)
		return
	}
	if err != nil {
		observer(RequestAttemptEvent{
			Type:        "attempt_failed",
			Attempt:     attempt,
			MaxAttempts: maxAttempts,
			StatusCode:  status,
			DurationMS:  duration,
			Error:       err.Error(),
			Retriable:   false,
			Endpoint:    endpoint,
		})
		return
	}
	observer(RequestAttemptEvent{
		Type:        "attempt_completed",
		Attempt:     attempt,
		MaxAttempts: maxAttempts,
		StatusCode:  status,
		DurationMS:  duration,
		Retriable:   false,
		Endpoint:    endpoint,
	})
}

func discardRetryResponse(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
}
