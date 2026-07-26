package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"webide-backend/internal/store"
)

type ProviderResponseError struct {
	StatusCode      int
	ContentType     string
	Endpoint        string
	ResponseSnippet string
	ParseError      string
	Message         string
	ErrorCode       string
	ErrorType       string
	ErrorParam      string
	StructuredError map[string]any
	RetryAfter      string
	RateLimitReset  string
	RequestID       string
}

type RequestAttemptEvent struct {
	Type         string
	Attempt      int
	MaxAttempts  int
	StatusCode   int
	DurationMS   int64
	DelayMS      int64
	RetryAfterMS int64
	Error        string
	Retriable    bool
	Endpoint     string
}

type RequestAttemptObserver func(RequestAttemptEvent)

func (e *ProviderResponseError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.ParseError != "" {
		return fmt.Sprintf("%s；HTTP %d；content-type=%s；url=%s；响应前300字符=%q",
			e.ParseError,
			e.StatusCode,
			valueOrUnknown(e.ContentType),
			e.Endpoint,
			e.ResponseSnippet,
		)
	}
	return providerHTTPErrorMessage(e.StatusCode, e.ContentType, e.Endpoint, e.ResponseSnippet)
}

func (s *Service) pickSearchProvider(ctx context.Context, userID string) (store.SearchProvider, error) {
	return s.pickSearchProviderForRole(ctx, userID, "admin")
}

func (s *Service) pickSearchProviderForRole(ctx context.Context, userID, role string) (store.SearchProvider, error) {
	items, err := s.configs.ListSearchProvidersForUser(ctx, userID, normalizeRunUserRole(role))
	if err != nil {
		return store.SearchProvider{}, err
	}
	for _, item := range items {
		if item.Enabled {
			return item, nil
		}
	}
	return store.SearchProvider{}, fmt.Errorf("未配置可用搜索工具，请先在设置页配置 Tavily 或 Brave。")
}

func (s *Service) webSearch(ctx context.Context, userID, query string, maxResults int) ([]WebSearchResult, string, error) {
	return s.webSearchForRole(ctx, userID, "admin", query, maxResults)
}

func (s *Service) webSearchForRole(ctx context.Context, userID, role, query string, maxResults int) ([]WebSearchResult, string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, "", fmt.Errorf("query is required")
	}
	if maxResults <= 0 {
		maxResults = 5
	}
	if maxResults > 10 {
		maxResults = 10
	}
	provider, err := s.pickSearchProviderForRole(ctx, userID, role)
	if err != nil {
		return nil, "", err
	}
	switch strings.ToLower(provider.Provider) {
	case "tavily":
		results, err := s.tavilySearch(ctx, provider.APIKey, query, maxResults)
		return results, "tavily", err
	case "brave":
		results, err := s.braveSearch(ctx, provider.APIKey, query, maxResults)
		return results, "brave", err
	default:
		return nil, provider.Provider, fmt.Errorf("unsupported search provider: %s", provider.Provider)
	}
}

func (s *Service) webExtract(ctx context.Context, userID, rawURL string) (WebExtractResult, error) {
	return s.webExtractForRole(ctx, userID, "admin", rawURL)
}

func (s *Service) webExtractForRole(ctx context.Context, userID, role, rawURL string) (WebExtractResult, error) {
	parsedURL, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return WebExtractResult{}, fmt.Errorf("invalid url")
	}
	provider, err := s.pickSearchProviderForRole(ctx, userID, role)
	if err != nil {
		return WebExtractResult{}, err
	}
	switch strings.ToLower(provider.Provider) {
	case "tavily":
		return s.tavilyExtract(ctx, provider.APIKey, parsedURL.String())
	case "brave":
		return WebExtractResult{}, fmt.Errorf("当前 provider 不支持 extract")
	default:
		return WebExtractResult{}, fmt.Errorf("unsupported search provider: %s", provider.Provider)
	}
}

func (s *Service) postJSON(ctx context.Context, endpoint string, payload any, headers map[string]string, target any) error {
	raw, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	return s.doJSON(req, target)
}

func (s *Service) getJSON(ctx context.Context, endpoint string, headers map[string]string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	return s.doJSON(req, target)
}

func (s *Service) doJSON(req *http.Request, target any) error {
	resp, err := s.doRequestWithRetry(req, nil)
	if err != nil {
		return err
	}
	return decodeProviderResponse(resp, req.URL.String(), target)
}

func (s *Service) doRequestWithRetry(req *http.Request, body []byte, observers ...RequestAttemptObserver) (*http.Response, error) {
	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if body != nil {
			req.Body = io.NopCloser(bytes.NewReader(body))
			req.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(body)), nil
			}
		}
		emitRequestAttempt(observers, RequestAttemptEvent{
			Type:        "attempt_started",
			Attempt:     attempt,
			MaxAttempts: maxAttempts,
			Endpoint:    req.URL.String(),
		})
		startedAt := time.Now()
		resp, err := s.client.Do(req)
		duration := time.Since(startedAt).Milliseconds()
		if err == nil {
			if resp != nil && isRetriableProviderStatus(resp.StatusCode) && attempt < maxAttempts {
				log.Printf("[ai] transient provider http status endpoint=%s attempt=%d/%d status=%d", req.URL.String(), attempt, maxAttempts, resp.StatusCode)
				emitRequestAttempt(observers, RequestAttemptEvent{
					Type:        "attempt_retrying",
					Attempt:     attempt,
					MaxAttempts: maxAttempts,
					StatusCode:  resp.StatusCode,
					DurationMS:  duration,
					Retriable:   true,
					Endpoint:    req.URL.String(),
				})
				io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
				resp.Body.Close()
				if !sleepBeforeRetry(req.Context(), attempt) {
					return nil, req.Context().Err()
				}
				continue
			}
			emitRequestAttempt(observers, RequestAttemptEvent{
				Type:        "attempt_completed",
				Attempt:     attempt,
				MaxAttempts: maxAttempts,
				StatusCode:  resp.StatusCode,
				DurationMS:  duration,
				Retriable:   resp != nil && isRetriableProviderStatus(resp.StatusCode),
				Endpoint:    req.URL.String(),
			})
			return resp, nil
		}
		lastErr = err
		if attempt == maxAttempts || !isTransientTransportError(err) {
			emitRequestAttempt(observers, RequestAttemptEvent{
				Type:        "attempt_failed",
				Attempt:     attempt,
				MaxAttempts: maxAttempts,
				DurationMS:  duration,
				Error:       err.Error(),
				Retriable:   isTransientTransportError(err),
				Endpoint:    req.URL.String(),
			})
			return nil, err
		}
		log.Printf("[ai] transient provider transport error endpoint=%s attempt=%d/%d error=%v", req.URL.String(), attempt, maxAttempts, err)
		emitRequestAttempt(observers, RequestAttemptEvent{
			Type:        "attempt_retrying",
			Attempt:     attempt,
			MaxAttempts: maxAttempts,
			DurationMS:  duration,
			Error:       err.Error(),
			Retriable:   true,
			Endpoint:    req.URL.String(),
		})
		if !sleepBeforeRetry(req.Context(), attempt) {
			return nil, req.Context().Err()
		}
	}
	return nil, lastErr
}

func emitRequestAttempt(observers []RequestAttemptObserver, event RequestAttemptEvent) {
	for _, observer := range observers {
		if observer != nil {
			observer(event)
		}
	}
}

func sleepBeforeRetry(ctx context.Context, attempt int) bool {
	delay := 750 * time.Millisecond
	for i := 1; i < attempt; i++ {
		delay *= 2
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

func isRetriableProviderStatus(statusCode int) bool {
	return statusCode >= http.StatusInternalServerError && statusCode <= 599
}

func isRetriableProviderStatusIncluding529(statusCode int) bool {
	return isRetriableProviderStatus(statusCode) || statusCode == 529
}

func isTransientTransportError(err error) bool {
	if err == nil {
		return false
	}
	return classifyProviderTransportFailure(err).Retryable
}

func decodeProviderResponse(resp *http.Response, endpoint string, target any) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	contentType := resp.Header.Get("Content-Type")
	isJSON := strings.Contains(strings.ToLower(contentType), "json")
	if resp.StatusCode >= 400 {
		if isJSON && len(body) > 0 {
			var data struct {
				Error any `json:"error"`
			}
			if err := json.Unmarshal(body, &data); err == nil && data.Error != nil {
				errorCode, errorType, errorParam, errorMessage, structured := providerErrorFields(data.Error)
				return &ProviderResponseError{
					StatusCode:      resp.StatusCode,
					ContentType:     contentType,
					Endpoint:        endpoint,
					ResponseSnippet: responseSnippet(body, 500),
					Message:         errorMessage,
					ErrorCode:       errorCode,
					ErrorType:       errorType,
					ErrorParam:      errorParam,
					StructuredError: structured,
					RetryAfter:      resp.Header.Get("Retry-After"),
					RateLimitReset:  providerRateLimitResetHeader(resp.Header),
					RequestID:       providerRequestIDHeader(resp.Header),
				}
			}
		}
		return &ProviderResponseError{
			StatusCode:      resp.StatusCode,
			ContentType:     contentType,
			Endpoint:        endpoint,
			ResponseSnippet: responseSnippet(body, 500),
			Message:         providerHTTPErrorMessage(resp.StatusCode, contentType, endpoint, responseSnippet(body, 300)),
			RetryAfter:      resp.Header.Get("Retry-After"),
			RateLimitReset:  providerRateLimitResetHeader(resp.Header),
			RequestID:       providerRequestIDHeader(resp.Header),
		}
	}
	if err := json.Unmarshal(body, target); err != nil {
		return &ProviderResponseError{
			StatusCode:      resp.StatusCode,
			ContentType:     contentType,
			Endpoint:        endpoint,
			ResponseSnippet: responseSnippet(body, 500),
			ParseError:      err.Error(),
		}
	}
	return nil
}

func providerHTTPError(statusCode int, contentType, endpoint string, body []byte) error {
	return fmt.Errorf("%s", providerHTTPErrorMessage(statusCode, contentType, endpoint, responseSnippet(body, 300)))
}

func providerHTTPErrorMessage(statusCode int, contentType, endpoint, snippet string) string {
	return fmt.Sprintf("模型供应商返回非成功响应；HTTP %d；content-type=%s；url=%s；响应前300字符=%q",
		statusCode,
		valueOrUnknown(contentType),
		endpoint,
		snippet,
	)
}

func responseSnippet(body []byte, limit int) string {
	text := strings.TrimSpace(string(body))
	text = strings.Join(strings.Fields(text), " ")
	return truncateRunes(text, limit)
}
