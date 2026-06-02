package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"webide-backend/internal/store"
)

type DiagnosticError struct {
	ID  string
	Err error
}

func (e *DiagnosticError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *DiagnosticError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func WithDiagnosticID(err error, diagnosticID string) error {
	if err == nil || diagnosticID == "" {
		return err
	}
	var existing *DiagnosticError
	if errors.As(err, &existing) {
		return err
	}
	return &DiagnosticError{ID: diagnosticID, Err: err}
}

func DiagnosticIDFromError(err error) string {
	var diagnosticErr *DiagnosticError
	if errors.As(err, &diagnosticErr) {
		return diagnosticErr.ID
	}
	return ""
}

func UserFacingErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	return sanitizeUserFacingAIError(err.Error())
}

func (s *Service) recordProviderDiagnostic(ctx context.Context, opts RunOptions, provider store.ModelProvider, conversationID, messageID, stage, level string, duration time.Duration, err error) string {
	return s.appendProviderDiagnostic(ctx, opts, provider, conversationID, messageID, stage, level, duration, err, "")
}

func (s *Service) recordProviderDiagnosticSummary(ctx context.Context, opts RunOptions, provider store.ModelProvider, conversationID, messageID, stage, level string, duration time.Duration, err error, requestSummary string) string {
	return s.appendProviderDiagnostic(ctx, opts, provider, conversationID, messageID, stage, level, duration, err, requestSummary)
}

func (s *Service) appendProviderDiagnostic(ctx context.Context, opts RunOptions, provider store.ModelProvider, conversationID, messageID, stage, level string, duration time.Duration, err error, requestSummary string) string {
	if s.diag == nil || opts.UserID == "" || err == nil {
		return ""
	}
	requestSummary = strings.TrimSpace(requestSummary)
	if requestSummary == "" {
		requestSummary = fallbackProviderDiagnosticSummary(stage, err)
	}
	input := store.DiagnosticLogInput{
		UserID:          opts.UserID,
		ConversationID:  conversationID,
		MessageID:       messageID,
		TaskID:          opts.TaskID,
		ModelProviderID: provider.ID,
		ModelName:       provider.Name,
		ModelID:         provider.ModelID,
		ProviderName:    provider.Name,
		Protocol:        provider.Protocol,
		Stage:           stage,
		Level:           level,
		ErrorMessage:    err.Error(),
		RequestSummary:  requestSummary,
		DurationMS:      duration.Milliseconds(),
	}
	if parsed, parseErr := url.Parse(strings.TrimSpace(provider.BaseURL)); parseErr == nil {
		input.BaseURLHost = parsed.Host
	}
	var providerErr *ProviderResponseError
	if errors.As(err, &providerErr) {
		input.HTTPStatus = providerErr.StatusCode
		input.ContentType = providerErr.ContentType
		input.ResponseSnippet = providerErr.ResponseSnippet
		if parsed, parseErr := url.Parse(providerErr.Endpoint); parseErr == nil {
			input.BaseURLHost = parsed.Host
			input.FinalURLPath = parsed.EscapedPath()
			if input.FinalURLPath == "" {
				input.FinalURLPath = parsed.Path
			}
		}
	}
	item, appendErr := s.diag.Append(ctx, input)
	if appendErr != nil {
		return ""
	}
	return item.ID
}

func (s *Service) recordToolDiagnostic(ctx context.Context, opts RunOptions, conversationID, messageID, toolName, argumentsJSON, targetSummary string, durationMS int64, err error) {
	if s.diag == nil || opts.UserID == "" || err == nil {
		return
	}
	diag := buildToolDiagnostic(toolName, argumentsJSON, targetSummary, durationMS, err)
	_, _ = s.diag.Append(ctx, store.DiagnosticLogInput{
		UserID:         opts.UserID,
		ConversationID: conversationID,
		MessageID:      messageID,
		TaskID:         opts.TaskID,
		Stage:          "tool_call:" + toolName,
		Level:          diag.Level,
		ErrorMessage:   diag.ErrorMessage,
		RequestSummary: diag.RequestSummary,
		DurationMS:     durationMS,
	})
}

func (s *Service) recordLocalPersistenceDiagnostic(ctx context.Context, opts RunOptions, conversationID, messageID, stage string, err error) string {
	if s.diag == nil || opts.UserID == "" || err == nil {
		return ""
	}
	item, appendErr := s.diag.Append(ctx, store.DiagnosticLogInput{
		UserID:         opts.UserID,
		ConversationID: conversationID,
		MessageID:      messageID,
		TaskID:         opts.TaskID,
		Stage:          stage,
		Level:          "error",
		ErrorMessage:   err.Error(),
	})
	if appendErr != nil {
		return ""
	}
	return item.ID
}

func diagnosticErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	if id := DiagnosticIDFromError(err); id != "" {
		return fmt.Sprintf("%s（已记录诊断日志 ID：%s，可在 设置 → 诊断日志 中导出。）", sanitizeUserFacingAIError(err.Error()), id)
	}
	return sanitizeUserFacingAIError(err.Error())
}

func fallbackProviderDiagnosticSummary(stage string, err error) string {
	payload := map[string]any{
		"failure_class": providerFailureClass(err, ProviderRequestSummary{}),
		"stage":         stage,
	}
	if payload["failure_class"] == "" {
		delete(payload, "failure_class")
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

func sanitizeUserFacingAIError(message string) string {
	text := strings.TrimSpace(message)
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "tool_ledger") || strings.Contains(lower, "run checkpoint") || strings.Contains(lower, "local") && strings.Contains(lower, "persistence"):
		return "本地状态保存失败，本轮已暂停；可以检查磁盘/数据库状态后继续本轮"
	case strings.Contains(lower, "malformed http response"):
		return "模型供应商连接协议异常，本轮请求没有拿到有效响应"
	case strings.Contains(lower, "eof") ||
		strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "connection aborted") ||
		strings.Contains(lower, "server closed idle connection"):
		return "模型供应商连接中断，本轮请求没有拿到有效响应"
	case strings.Contains(lower, "tls: bad record mac"):
		return "模型供应商 TLS 连接中断，本轮请求没有拿到有效响应"
	case strings.Contains(lower, "transport connection broken"):
		return "模型供应商连接中断，本轮请求没有拿到有效响应"
	default:
		return text
	}
}

func diagnosticStage(protocol string, performedMutation bool, hasEvents bool) string {
	switch strings.ToLower(protocol) {
	case "anthropic":
		return anthropicTimeoutStage(performedMutation, hasEvents)
	default:
		return openAITimeoutStage(performedMutation, hasEvents)
	}
}
