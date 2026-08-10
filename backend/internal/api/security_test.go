package api

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"webide-backend/internal/config"
)

func TestCORSRejectsUnknownCredentialedOrigin(t *testing.T) {
	server := &Server{allowedOrigins: map[string]bool{"http://localhost:5173": true}}
	handler := server.withCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))

	rejected := httptest.NewRequest(http.MethodOptions, "http://127.0.0.1:8080/api/files/tree", nil)
	rejected.Header.Set("Origin", "https://attacker.example")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, rejected)
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "cors_origin_rejected") {
		t.Fatalf("unexpected rejected origin response: %d %s", recorder.Code, recorder.Body.String())
	}

	allowed := httptest.NewRequest(http.MethodOptions, "http://127.0.0.1:8080/api/files/tree", nil)
	allowed.Header.Set("Origin", "http://localhost:5173")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, allowed)
	if recorder.Code != http.StatusNoContent || recorder.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("unexpected allowed origin response: %d %#v", recorder.Code, recorder.Header())
	}
}

func TestCSRFAllowsSameOriginAndRejectsCrossOrigin(t *testing.T) {
	server := &Server{}
	handler := server.withCSRFOrigin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))

	same := httptest.NewRequest(http.MethodPost, "http://perseus.local/api/files/write", nil)
	same = same.WithContext(context.WithValue(same.Context(), ctxSessionKey, &SessionInfo{UserID: "user-1"}))
	same.Header.Set("Origin", "http://perseus.local")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, same)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("same-origin request rejected: %d %s", recorder.Code, recorder.Body.String())
	}

	cross := httptest.NewRequest(http.MethodPost, "http://perseus.local/api/files/write", nil)
	cross = cross.WithContext(context.WithValue(cross.Context(), ctxSessionKey, &SessionInfo{UserID: "user-1"}))
	cross.Header.Set("Origin", "https://attacker.example")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, cross)
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "csrf_origin_rejected") {
		t.Fatalf("cross-origin request not rejected: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestCSRFRejectsMissingOriginForAuthenticatedMutation(t *testing.T) {
	server := &Server{}
	called := false
	handler := server.withCSRFOrigin(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	request := httptest.NewRequest(http.MethodPost, "http://perseus.local/api/files/write", nil)
	request = request.WithContext(context.WithValue(request.Context(), ctxSessionKey, &SessionInfo{UserID: "user-1"}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if called || recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "csrf_origin_rejected") {
		t.Fatalf("missing authenticated origin was not rejected: called=%v status=%d body=%s", called, recorder.Code, recorder.Body.String())
	}
}

func TestSecureCookieAutoTrustsOnlyConfiguredProxy(t *testing.T) {
	_, trusted, err := net.ParseCIDR("10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{cfg: config.Config{SessionCookieSecure: "auto"}, trustedProxyNetworks: []*net.IPNet{trusted}}

	request := httptest.NewRequest(http.MethodPost, "http://perseus.local/api/auth/login", nil)
	request.RemoteAddr = "10.1.2.3:1234"
	request.Header.Set("X-Forwarded-Proto", "https")
	if !server.sessionCookieSecure(request) {
		t.Fatal("trusted HTTPS proxy should produce a Secure cookie")
	}
	request.RemoteAddr = "192.0.2.10:1234"
	if server.sessionCookieSecure(request) {
		t.Fatal("untrusted proxy headers must not produce a Secure decision")
	}
}

func TestJSONBodyLimitRejectsKnownOversizeRequest(t *testing.T) {
	server := &Server{}
	called := false
	handler := server.withRequestBodyLimits(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	body := bytes.NewReader(make([]byte, defaultJSONBodyLimit+1))
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if called || recorder.Code != http.StatusRequestEntityTooLarge || !strings.Contains(recorder.Body.String(), "request_body_too_large") {
		t.Fatalf("oversize request was not rejected: called=%v status=%d body=%s", called, recorder.Code, recorder.Body.String())
	}
}

func TestJSONBodyLimitRejectsChunkedOversizeRequestBeforeRoute(t *testing.T) {
	server := &Server{}
	called := false
	handler := server.withRequestBodyLimits(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	body := bytes.NewReader(make([]byte, defaultJSONBodyLimit+1))
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	request.ContentLength = -1
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if called || recorder.Code != http.StatusRequestEntityTooLarge || !strings.Contains(recorder.Body.String(), "request_body_too_large") {
		t.Fatalf("chunked oversize request reached route: called=%v status=%d body=%s", called, recorder.Code, recorder.Body.String())
	}
}

func TestJSONRequestTimeoutDoesNotWrapEventStreams(t *testing.T) {
	server := &Server{cfg: config.Config{HTTPRequestTimeout: 10 * time.Millisecond}}
	handler := server.withJSONRequestTimeout(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/events" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		<-r.Context().Done()
	}))

	jsonRequest := httptest.NewRequest(http.MethodPost, "/json", nil)
	jsonRequest.Header.Set("Content-Type", "application/json")
	jsonRecorder := httptest.NewRecorder()
	handler.ServeHTTP(jsonRecorder, jsonRequest)
	if jsonRecorder.Code != http.StatusServiceUnavailable || !strings.Contains(jsonRecorder.Body.String(), "request_timeout") {
		t.Fatalf("expected JSON timeout, status=%d body=%s", jsonRecorder.Code, jsonRecorder.Body.String())
	}

	eventRequest := httptest.NewRequest(http.MethodGet, "/events", nil)
	eventRequest.Header.Set("Accept", "text/event-stream")
	eventRecorder := httptest.NewRecorder()
	handler.ServeHTTP(eventRecorder, eventRequest)
	if eventRecorder.Code != http.StatusNoContent {
		t.Fatalf("event stream was unexpectedly wrapped: %d", eventRecorder.Code)
	}
}

func TestLoginTokenBucketSupportsBurstAndRefill(t *testing.T) {
	limiter := newTokenBucketLimiter()
	now := time.Unix(1000, 0)
	for index := 0; index < 10; index++ {
		if allowed, _ := limiter.Allow("ip-user", 5, 10, now); !allowed {
			t.Fatalf("burst request %d was rejected", index+1)
		}
	}
	if allowed, _ := limiter.Allow("ip-user", 5, 10, now); allowed {
		t.Fatal("request beyond burst should be rejected")
	}
	if allowed, _ := limiter.Allow("ip-user", 5, 10, now.Add(12*time.Second)); !allowed {
		t.Fatal("one token should refill after twelve seconds")
	}
}
