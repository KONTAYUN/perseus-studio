package api

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultJSONBodyLimit = int64(2 << 20)
	largeJSONBodyLimit   = int64(16 << 20)
)

type fixedWindowEntry struct {
	started time.Time
	count   int
}

type fixedWindowLimiter struct {
	mu      sync.Mutex
	entries map[string]fixedWindowEntry
}

type tokenBucketEntry struct {
	tokens float64
	last   time.Time
}

type tokenBucketLimiter struct {
	mu      sync.Mutex
	entries map[string]tokenBucketEntry
}

func newTokenBucketLimiter() *tokenBucketLimiter {
	return &tokenBucketLimiter{entries: map[string]tokenBucketEntry{}}
}

func (l *tokenBucketLimiter) Allow(key string, ratePerMinute, burst int, now time.Time) (bool, time.Duration) {
	if l == nil || ratePerMinute <= 0 || burst <= 0 {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.entries[key]
	if !ok {
		entry = tokenBucketEntry{tokens: float64(burst), last: now}
	}
	perSecond := float64(ratePerMinute) / 60
	entry.tokens += now.Sub(entry.last).Seconds() * perSecond
	if entry.tokens > float64(burst) {
		entry.tokens = float64(burst)
	}
	entry.last = now
	if len(l.entries) > 4096 {
		for itemKey, item := range l.entries {
			if now.Sub(item.last) >= time.Hour {
				delete(l.entries, itemKey)
			}
		}
	}
	if entry.tokens < 1 {
		l.entries[key] = entry
		return false, time.Duration((1 - entry.tokens) / perSecond * float64(time.Second))
	}
	entry.tokens--
	l.entries[key] = entry
	return true, 0
}

func newFixedWindowLimiter() *fixedWindowLimiter {
	return &fixedWindowLimiter{entries: map[string]fixedWindowEntry{}}
}

func (l *fixedWindowLimiter) Allow(key string, limit int, window time.Duration, now time.Time) (bool, time.Duration) {
	if l == nil || limit <= 0 || window <= 0 {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := l.entries[key]
	if entry.started.IsZero() || now.Sub(entry.started) >= window {
		entry = fixedWindowEntry{started: now}
	}
	if entry.count >= limit {
		return false, positiveDuration(window - now.Sub(entry.started))
	}
	entry.count++
	l.entries[key] = entry
	if len(l.entries) > 4096 {
		for itemKey, item := range l.entries {
			if now.Sub(item.started) >= window {
				delete(l.entries, itemKey)
			}
		}
	}
	return true, 0
}

func positiveDuration(value time.Duration) time.Duration {
	if value < 0 {
		return 0
	}
	return value
}

func (s *Server) withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'; object-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withRequestBodyLimits(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
		if contentType != "application/json" || r.Body == nil {
			next.ServeHTTP(w, r)
			return
		}
		limit := defaultJSONBodyLimit
		if r.URL.Path == "/api/files/write" {
			limit = largeJSONBodyLimit
		}
		if r.ContentLength > limit {
			writeCodedError(w, http.StatusRequestEntityTooLarge, "request_body_too_large", "request body is too large")
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
		if err != nil {
			writeCodedError(w, http.StatusBadRequest, "request_body_read_failed", "request body could not be read")
			return
		}
		if int64(len(raw)) > limit {
			writeCodedError(w, http.StatusRequestEntityTooLarge, "request_body_too_large", "request body is too large")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
		r.ContentLength = int64(len(raw))
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withJSONRequestTimeout(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
		if contentType != "application/json" || s.cfg.HTTPRequestTimeout <= 0 {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		http.TimeoutHandler(
			next,
			s.cfg.HTTPRequestTimeout,
			`{"error":"request timed out","code":"request_timeout"}`+"\n",
		).ServeHTTP(w, r)
	})
}

func (s *Server) withCSRFOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isUnsafeMethod(r.Method) && UserFromContext(r.Context()) != nil {
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			if origin == "" || !s.originAllowed(r, origin) {
				writeCodedError(w, http.StatusForbidden, "csrf_origin_rejected", "request origin is not allowed")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func (s *Server) originAllowed(r *http.Request, origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") {
		return false
	}
	normalized := strings.ToLower(parsed.Scheme + "://" + parsed.Host)
	if normalized == strings.ToLower(s.requestOrigin(r)) {
		return true
	}
	return s.allowedOrigins[normalized]
}

func (s *Server) requestOrigin(r *http.Request) string {
	scheme := "http"
	if s.requestIsHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (s *Server) requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if !s.remoteIsTrustedProxy(r.RemoteAddr) {
		return false
	}
	forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])
	return strings.EqualFold(forwarded, "https")
}

func (s *Server) remoteIsTrustedProxy(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return false
	}
	for _, network := range s.trustedProxyNetworks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func (s *Server) clientIP(r *http.Request) string {
	remote := r.RemoteAddr
	if host, _, err := net.SplitHostPort(remote); err == nil {
		remote = host
	}
	remote = strings.TrimSpace(remote)
	if !s.remoteIsTrustedProxy(r.RemoteAddr) {
		return remote
	}
	forwarded := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	if len(forwarded) > 0 {
		if ip := net.ParseIP(strings.TrimSpace(forwarded[0])); ip != nil {
			return ip.String()
		}
	}
	return remote
}

func (s *Server) allowRate(w http.ResponseWriter, limiter *fixedWindowLimiter, key string, limit int, window time.Duration, code string) bool {
	allowed, retryAfter := limiter.Allow(key, limit, window, time.Now().UTC())
	if allowed {
		return true
	}
	seconds := int(retryAfter.Round(time.Second) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	writeJSON(w, http.StatusTooManyRequests, map[string]any{
		"error":          "too many requests",
		"code":           code,
		"retry_after_ms": retryAfter.Milliseconds(),
	})
	return false
}

func (s *Server) allowLoginRate(w http.ResponseWriter, key string) bool {
	allowed, retryAfter := s.loginLimiter.Allow(key, s.cfg.LoginRatePerMinute, s.cfg.LoginRateBurst, time.Now().UTC())
	if allowed {
		return true
	}
	seconds := int(retryAfter.Round(time.Second) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	writeJSON(w, http.StatusTooManyRequests, map[string]any{
		"error": "too many authentication attempts", "code": "auth_rate_limited", "retry_after_ms": retryAfter.Milliseconds(),
	})
	return false
}
