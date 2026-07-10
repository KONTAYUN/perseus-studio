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
		// HTTP failures belong to request classification. Checkpoint recovery is
		// reserved for a broken transport after verified tool progress.
		return false
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
