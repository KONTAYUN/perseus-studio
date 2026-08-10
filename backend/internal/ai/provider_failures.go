package ai

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"strings"
	"syscall"
)

type ProviderFailureClass string

const (
	ProviderFailureRateLimited           ProviderFailureClass = "provider_rate_limited"
	ProviderFailureQuotaExhausted        ProviderFailureClass = "provider_quota_exhausted"
	ProviderFailureContextExceeded       ProviderFailureClass = "context_window_exceeded"
	ProviderFailureAuthentication        ProviderFailureClass = "provider_auth_failed"
	ProviderFailureInvalidRequest        ProviderFailureClass = "provider_invalid_request"
	ProviderFailureModelUnavailable      ProviderFailureClass = "provider_model_unavailable"
	ProviderFailureCapabilityUnsupported ProviderFailureClass = "provider_capability_unsupported"
	ProviderFailureConnection            ProviderFailureClass = "provider_connection_failed"
	ProviderFailureStreamInterrupted     ProviderFailureClass = "provider_stream_interrupted"
	ProviderFailureOverloaded            ProviderFailureClass = "provider_overloaded"
	ProviderFailureProtocol              ProviderFailureClass = "provider_protocol_error"
	ProviderFailureUnknown               ProviderFailureClass = "provider_unknown_failure"
)

type ProviderFailure struct {
	Class                ProviderFailureClass `json:"class"`
	Code                 string               `json:"code,omitempty"`
	Type                 string               `json:"type,omitempty"`
	HTTPStatus           int                  `json:"http_status,omitempty"`
	Retryable            bool                 `json:"retryable"`
	ClassificationSource string               `json:"classification_source"`
	RequestID            string               `json:"request_id,omitempty"`
}

type ProviderProtocolError struct {
	Code  string
	Cause error
}

func (e *ProviderProtocolError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return e.Code
}

func (e *ProviderProtocolError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type ProviderCapabilityError struct {
	Capability string
	ModelID    string
}

func (e *ProviderCapabilityError) Error() string {
	if e == nil {
		return ""
	}
	return "configured model does not support required capability: " + e.Capability
}

type ContextWindowError struct {
	EstimatedTokens int
	BudgetTokens    int
	Stage           string
}

func (e *ContextWindowError) Error() string {
	if e == nil {
		return ""
	}
	return "context window budget exceeded"
}

func classifyProviderFailure(err error) ProviderFailure {
	if err == nil {
		return ProviderFailure{}
	}
	var limited *ProviderRateLimitError
	if errors.As(err, &limited) {
		class := ProviderFailureRateLimited
		if limited.State.Kind == ProviderRateLimitQuota {
			class = ProviderFailureQuotaExhausted
		}
		return ProviderFailure{
			Class:                class,
			HTTPStatus:           limited.State.StatusCode,
			Retryable:            false,
			ClassificationSource: "rate_limit_state",
			RequestID:            limited.State.RequestID,
		}
	}
	var contextErr *ContextWindowError
	if errors.As(err, &contextErr) {
		return ProviderFailure{Class: ProviderFailureContextExceeded, Retryable: false, ClassificationSource: "local_context_budget"}
	}
	var responseErr *ProviderResponseError
	if errors.As(err, &responseErr) {
		return classifyProviderResponseFailure(responseErr)
	}
	var protocolErr *ProviderProtocolError
	if errors.As(err, &protocolErr) {
		class := ProviderFailureProtocol
		if protocolErr.Code == "stream_interrupted" {
			class = ProviderFailureStreamInterrupted
		}
		return ProviderFailure{
			Class:                class,
			Code:                 protocolErr.Code,
			Retryable:            protocolErr.Code == "stream_interrupted" || protocolErr.Code == "empty_provider_response",
			ClassificationSource: "protocol_error",
		}
	}
	var capabilityErr *ProviderCapabilityError
	if errors.As(err, &capabilityErr) {
		return ProviderFailure{
			Class:                ProviderFailureCapabilityUnsupported,
			Code:                 capabilityErr.Capability,
			Retryable:            false,
			ClassificationSource: "provider_capability",
		}
	}
	return classifyProviderTransportFailure(err)
}

func classifyProviderResponseFailure(err *ProviderResponseError) ProviderFailure {
	failure := ProviderFailure{
		Code:                 strings.ToLower(strings.TrimSpace(err.ErrorCode)),
		Type:                 strings.ToLower(strings.TrimSpace(err.ErrorType)),
		HTTPStatus:           err.StatusCode,
		Retryable:            false,
		ClassificationSource: "provider_structured_error",
		RequestID:            err.RequestID,
	}
	switch {
	case err.StatusCode == 429:
		failure.Class = ProviderFailureRateLimited
	case isQuotaErrorCode(failure.Code):
		failure.Class = ProviderFailureQuotaExhausted
	case isContextLimitErrorCode(failure.Code):
		failure.Class = ProviderFailureContextExceeded
	case err.StatusCode == 401 || err.StatusCode == 403:
		failure.Class = ProviderFailureAuthentication
	case isModelUnavailableErrorCode(failure.Code):
		failure.Class = ProviderFailureModelUnavailable
	case isCapabilityErrorCode(failure.Code):
		failure.Class = ProviderFailureCapabilityUnsupported
	case err.StatusCode == 404:
		failure.Class = ProviderFailureModelUnavailable
	case err.StatusCode == 529 || err.StatusCode == 503:
		failure.Class = ProviderFailureOverloaded
		failure.Retryable = true
	case err.StatusCode >= 500:
		failure.Class = ProviderFailureConnection
		failure.Retryable = true
	case err.StatusCode >= 400:
		failure.Class = ProviderFailureInvalidRequest
	default:
		failure.Class = ProviderFailureUnknown
	}
	if failure.Code == "" && failure.Type == "" && err.StatusCode == 0 {
		failure.Class = ProviderFailureUnknown
		failure.Code = "provider_unknown_failure"
		failure.ClassificationSource = "unstructured_provider_response"
	}
	return failure
}

func classifyProviderTransportFailure(err error) ProviderFailure {
	failure := ProviderFailure{Class: ProviderFailureUnknown, Retryable: false, ClassificationSource: "unclassified_error"}
	if errors.Is(err, context.Canceled) {
		failure.Class = "user_or_client_canceled"
		failure.ClassificationSource = "context"
		return failure
	}
	if errors.Is(err, context.DeadlineExceeded) {
		failure.Class = ProviderFailureConnection
		failure.Code = "deadline_exceeded"
		failure.Retryable = true
		failure.ClassificationSource = "context"
		return failure
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		failure.Class = ProviderFailureStreamInterrupted
		failure.Code = "eof"
		failure.Retryable = true
		failure.ClassificationSource = "io"
		return failure
	}
	if certificateFailure, ok := classifyProviderCertificateFailure(err); ok {
		return certificateFailure
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		failure.Class = ProviderFailureConnection
		failure.Code = "dns_lookup_failed"
		failure.Retryable = dnsErr.IsTemporary || dnsErr.IsTimeout
		failure.ClassificationSource = "net_dns"
		return failure
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		nested := classifyProviderTransportFailure(urlErr.Err)
		if nested.Class != ProviderFailureUnknown {
			return nested
		}
		// net/http wraps TLS record failures and other request transport
		// failures in url.Error. Some TLS alerts do not implement net.Error,
		// so Timeout/Temporary alone would incorrectly make them terminal.
		failure.Class = ProviderFailureConnection
		failure.Code = "http_transport_error"
		failure.Retryable = true
		failure.ClassificationSource = "url_error"
		return failure
	}
	for code, label := range map[error]string{
		syscall.ECONNREFUSED: "connection_refused",
		syscall.ECONNRESET:   "connection_reset",
		syscall.ECONNABORTED: "connection_aborted",
	} {
		if errors.Is(err, code) {
			failure.Class = ProviderFailureConnection
			failure.Code = label
			failure.Retryable = true
			failure.ClassificationSource = "syscall"
			return failure
		}
	}
	var tlsRecordErr tls.RecordHeaderError
	if errors.As(err, &tlsRecordErr) {
		failure.Class = ProviderFailureConnection
		failure.Code = "tls_record_error"
		failure.Retryable = true
		failure.ClassificationSource = "tls"
		return failure
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		nested := classifyProviderTransportFailure(opErr.Err)
		if nested.Class != ProviderFailureUnknown {
			return nested
		}
		failure.Class = ProviderFailureConnection
		failure.Code = "network_operation_failed"
		failure.Retryable = true
		failure.ClassificationSource = "net_op"
		return failure
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		failure.Class = ProviderFailureConnection
		failure.Code = "network_error"
		failure.Retryable = netErr.Timeout() || netErr.Temporary()
		failure.ClassificationSource = "net_error"
		return failure
	}
	return failure
}

func classifyProviderCertificateFailure(err error) (ProviderFailure, bool) {
	certificateFailure := ProviderFailure{
		Class:                ProviderFailureConnection,
		Code:                 "tls_certificate_invalid",
		Retryable:            false,
		ClassificationSource: "tls_certificate",
	}
	var verificationErr *tls.CertificateVerificationError
	if errors.As(err, &verificationErr) {
		return certificateFailure, true
	}
	var unknownAuthorityErr x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthorityErr) {
		return certificateFailure, true
	}
	var hostnameErr x509.HostnameError
	if errors.As(err, &hostnameErr) {
		return certificateFailure, true
	}
	var invalidErr x509.CertificateInvalidError
	if errors.As(err, &invalidErr) {
		return certificateFailure, true
	}
	var rootsErr x509.SystemRootsError
	if errors.As(err, &rootsErr) {
		return certificateFailure, true
	}
	return ProviderFailure{}, false
}

func isQuotaErrorCode(code string) bool {
	switch code {
	case "insufficient_quota", "quota_exceeded", "billing_hard_limit_reached":
		return true
	default:
		return false
	}
}

func isContextLimitErrorCode(code string) bool {
	switch code {
	case "context_length_exceeded", "prompt_too_long", "request_too_large", "max_tokens_exceeded":
		return true
	default:
		return false
	}
}

func isModelUnavailableErrorCode(code string) bool {
	switch code {
	case "model_not_found", "unknown_model", "model_unavailable":
		return true
	default:
		return false
	}
}

func isCapabilityErrorCode(code string) bool {
	switch code {
	case "unsupported_capability", "unsupported_content_type", "image_input_not_supported", "tool_use_not_supported":
		return true
	default:
		return false
	}
}

func providerErrorFields(raw any) (code, errorType, param, message string, structured map[string]any) {
	switch value := raw.(type) {
	case map[string]any:
		structured = value
	case nil:
		return "", "", "", "", nil
	default:
		if encoded, err := json.Marshal(value); err == nil {
			_ = json.Unmarshal(encoded, &structured)
		}
	}
	if structured == nil {
		return "", "", "", providerErrorMessage(raw), nil
	}
	if nested, ok := structured["error"].(map[string]any); ok {
		structured = nested
	}
	code = strings.TrimSpace(stringFromAny(structured["code"]))
	errorType = strings.TrimSpace(stringFromAny(structured["type"]))
	param = strings.TrimSpace(stringFromAny(structured["param"]))
	message = strings.TrimSpace(stringFromAny(structured["message"]))
	if message == "" {
		message = providerErrorMessage(raw)
	}
	return code, errorType, param, message, structured
}
