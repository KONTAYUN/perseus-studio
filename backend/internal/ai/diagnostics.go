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
	var runtimeErr *AgentRuntimeError
	if errors.As(err, &runtimeErr) {
		return strings.TrimSpace(runtimeErr.Error())
	}
	var toolErr *ToolError
	if errors.As(err, &toolErr) {
		return strings.TrimSpace(toolErr.Error())
	}
	var limited *ProviderRateLimitError
	if errors.As(err, &limited) {
		return limited.Error()
	}
	var contextErr *ContextWindowError
	if errors.As(err, &contextErr) {
		if contextErr.EstimatedTokens > 0 && contextErr.BudgetTokens > 0 {
			return fmt.Sprintf("上下文窗口已超过预算：预计 %d tokens，预算 %d tokens", contextErr.EstimatedTokens, contextErr.BudgetTokens)
		}
		return "上下文窗口已超过预算"
	}
	var capabilityErr *ProviderCapabilityError
	if errors.As(err, &capabilityErr) {
		return "当前模型不支持本轮需要的能力：" + capabilityErr.Capability
	}
	failure := classifyProviderFailure(err)
	switch failure.Class {
	case ProviderFailureAuthentication:
		return "模型供应商认证失败，请检查 API Key 和认证方式"
	case ProviderFailureQuotaExhausted:
		return "模型供应商额度不足"
	case ProviderFailureRateLimited:
		return "模型供应商限流，请稍后继续"
	case ProviderFailureModelUnavailable:
		return "当前模型不可用，请检查模型配置"
	case ProviderFailureCapabilityUnsupported:
		return "当前模型不支持本轮需要的能力"
	case ProviderFailureInvalidRequest:
		return "模型供应商拒绝了本轮请求，请查看诊断日志"
	case ProviderFailureOverloaded:
		return "模型供应商当前繁忙，请稍后继续"
	case ProviderFailureConnection, ProviderFailureStreamInterrupted:
		return "模型供应商连接中断，本轮请求没有拿到完整响应"
	case ProviderFailureProtocol:
		return "模型供应商返回了无法处理的响应"
	default:
		return strings.TrimSpace(err.Error())
	}
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
		return fmt.Sprintf("%s（已记录诊断日志 ID：%s，可在 设置 → 诊断日志 中导出。）", UserFacingErrorMessage(err), id)
	}
	return UserFacingErrorMessage(err)
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
	return strings.TrimSpace(message)
}

func diagnosticStage(protocol string, performedMutation bool, hasEvents bool) string {
	switch strings.ToLower(protocol) {
	case "anthropic":
		return anthropicTimeoutStage(performedMutation, hasEvents)
	default:
		return openAITimeoutStage(performedMutation, hasEvents)
	}
}
