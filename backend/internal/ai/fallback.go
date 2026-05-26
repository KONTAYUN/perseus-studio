package ai

import (
	"context"
	"errors"
	"os"
	"strings"
)

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "timeout") ||
		strings.Contains(strings.ToLower(err.Error()), "deadline exceeded")
}

func isRecoverableFinalProviderError(err error) bool {
	return isTimeoutError(err) || isTransientTransportError(err)
}

func isCheckpointRecoverableProviderError(err error) bool {
	if err == nil || isContextCanceledError(err) || isContextOverflowError(err) {
		return false
	}
	var providerErr *ProviderResponseError
	if errors.As(err, &providerErr) {
		return isRetriableProviderStatus(providerErr.StatusCode) || providerErr.StatusCode == 529
	}
	if isRecoverableFinalProviderError(err) {
		return true
	}
	lower := strings.ToLower(err.Error())
	markers := []string{
		"empty model response",
		"empty stream",
		"empty provider",
		"unexpected eof",
		"http 408",
		"http 409",
		"http 429",
		"http 500",
		"http 502",
		"http 503",
		"http 504",
		"http 529",
		"status 408",
		"status 409",
		"status 429",
		"status 500",
		"status 502",
		"status 503",
		"status 504",
		"status 529",
		"too many requests",
		"rate limit",
		"overloaded",
		"capacity",
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func isContextCanceledError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.Canceled) || strings.Contains(strings.ToLower(err.Error()), "context canceled")
}

func IsContextCanceledError(err error) bool {
	return isContextCanceledError(err)
}

func openAITimeoutStage(performedMutation, hasToolEvents bool) string {
	if performedMutation {
		return "等待模型最终回复"
	}
	if hasToolEvents {
		return "等待模型继续处理 workspace 工具结果"
	}
	return "等待模型调用 workspace 工具"
}

func anthropicTimeoutStage(performedMutation, hasToolEvents bool) string {
	if performedMutation {
		return "等待模型最终回复"
	}
	if hasToolEvents {
		return "等待模型继续处理 workspace 工具结果"
	}
	return "等待模型调用 workspace 工具"
}
