package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func (s *Service) openAISDKPostRaw(ctx context.Context, provider store.ModelProvider, path string, payload map[string]any, opts RunOptions, stage string) (*http.Response, error) {
	client, err := s.openAISDKClient(provider, opts, stage)
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

func (s *Service) anthropicSDKPostRaw(ctx context.Context, provider store.ModelProvider, path string, payload map[string]any, opts RunOptions, stage string) (*http.Response, error) {
	client, err := s.anthropicSDKClient(provider, opts, stage)
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

func (s *Service) openAISDKClient(provider store.ModelProvider, opts RunOptions, stage string) (openaisdk.Client, error) {
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
		openaioption.WithMiddleware(openAISDKAttemptMiddleware(providerAttemptObserver(opts, provider, stage))),
	}
	if apiKey := strings.TrimSpace(provider.APIKey); apiKey != "" {
		options = append(options, openaioption.WithHeader("Authorization", bearerToken(apiKey)))
	}
	return openaisdk.NewClient(options...), nil
}

func (s *Service) anthropicSDKClient(provider store.ModelProvider, opts RunOptions, stage string) (anthropicsdk.Client, error) {
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
		anthropicoption.WithMiddleware(anthropicSDKAttemptMiddleware(providerAttemptObserver(opts, provider, stage))),
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

func openAISDKAttemptMiddleware(observer RequestAttemptObserver) openaioption.Middleware {
	var attempt int64
	return func(req *http.Request, next openaioption.MiddlewareNext) (*http.Response, error) {
		return runSDKAttempt(req, func(r *http.Request) (*http.Response, error) {
			return next(r)
		}, observer, &attempt)
	}
}

func anthropicSDKAttemptMiddleware(observer RequestAttemptObserver) anthropicoption.Middleware {
	var attempt int64
	return func(req *http.Request, next anthropicoption.MiddlewareNext) (*http.Response, error) {
		return runSDKAttempt(req, func(r *http.Request) (*http.Response, error) {
			return next(r)
		}, observer, &attempt)
	}
}

func runSDKAttempt(req *http.Request, next func(*http.Request) (*http.Response, error), observer RequestAttemptObserver, attemptCounter *int64) (*http.Response, error) {
	attempt := int(atomic.AddInt64(attemptCounter, 1))
	maxAttempts := providerSDKMaxRetries + 1
	endpoint := ""
	if req != nil && req.URL != nil {
		endpoint = req.URL.String()
	}
	if observer != nil {
		observer(RequestAttemptEvent{
			Type:        "attempt_started",
			Attempt:     attempt,
			MaxAttempts: maxAttempts,
			Endpoint:    endpoint,
		})
	}
	startedAt := time.Now()
	resp, err := next(req)
	duration := time.Since(startedAt).Milliseconds()
	if observer != nil {
		if err != nil {
			eventType := "attempt_failed"
			if attempt < maxAttempts && !isContextCanceledError(err) {
				eventType = "attempt_retrying"
			}
			observer(RequestAttemptEvent{
				Type:        eventType,
				Attempt:     attempt,
				MaxAttempts: maxAttempts,
				DurationMS:  duration,
				Error:       err.Error(),
				Retriable:   eventType == "attempt_retrying" || isTransientTransportError(err),
				Endpoint:    endpoint,
			})
		} else {
			status := 0
			retriable := false
			if resp != nil {
				status = resp.StatusCode
				retriable = isRetriableProviderStatus(resp.StatusCode)
			}
			eventType := "attempt_completed"
			if retriable && attempt < maxAttempts {
				eventType = "attempt_retrying"
			}
			observer(RequestAttemptEvent{
				Type:        eventType,
				Attempt:     attempt,
				MaxAttempts: maxAttempts,
				StatusCode:  status,
				DurationMS:  duration,
				Retriable:   retriable,
				Endpoint:    endpoint,
			})
		}
	}
	return resp, err
}
