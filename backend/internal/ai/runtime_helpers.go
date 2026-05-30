package ai

import (
	"fmt"
	"strings"

	"webide-backend/internal/store"
)

func lastUserMessageContent(messages []store.Message) string {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == "user" {
			return messages[index].Content
		}
	}
	return ""
}

func formatProviderError(statusCode int, raw any) error {
	message := providerErrorMessage(raw)
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "inappropriate content") || strings.Contains(lower, "data_inspection_failed") ||
		strings.Contains(lower, "content policy") || strings.Contains(lower, "safety"):
		return fmt.Errorf("模型供应商内容安全检查拦截了本轮输入：%s。通常是搜索结果或上下文里包含敏感内容，可换一个模型、减少搜索结果、或改成更中性的摘要请求后重试", message)
	case strings.Contains(message, "容量已满") || strings.Contains(message, "繁忙") ||
		strings.Contains(lower, "capacity") || strings.Contains(lower, "overloaded") || strings.Contains(lower, "rate limit"):
		return fmt.Errorf("模型供应商暂时繁忙：%s。请稍后重试，或切换到其它可用模型", message)
	case strings.Contains(lower, "tool") || strings.Contains(lower, "function"):
		return fmt.Errorf("当前模型或中转可能不支持 tool calling：%s", message)
	case message != "":
		return fmt.Errorf("模型供应商请求失败（HTTP %d）：%s", statusCode, message)
	default:
		return fmt.Errorf("模型供应商请求失败（HTTP %d）", statusCode)
	}
}

func providerErrorMessage(raw any) string {
	switch value := raw.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(value)
	case map[string]any:
		for _, key := range []string{"message", "error", "detail", "code"} {
			if item, ok := value[key]; ok {
				text := providerErrorMessage(item)
				if text != "" {
					return text
				}
			}
		}
	}
	text := strings.TrimSpace(fmt.Sprint(raw))
	text = strings.ReplaceAll(text, "map[", "")
	text = strings.TrimSuffix(text, "]")
	return text
}

func isContextOverflowError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	markers := []string{
		"context length exceeded",
		"maximum context",
		"context window",
		"input too long",
		"prompt is too long",
		"token limit",
		"too many tokens",
		"exceeds the context",
		"reduce the length",
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func isWorkspaceMutationTool(name string) bool {
	switch name {
	case "edit_workspace_lines", "write_workspace_text", "manage_workspace_path", "organize_workspace_matches":
		return true
	default:
		return false
	}
}

func unsupportedToolCallingError() error {
	return fmt.Errorf("当前模型或中转不支持 tool calling，无法让 AI 操作工作区")
}

func missingWorkspaceMutationError() error {
	return fmt.Errorf("模型没有调用所需的 workspace 变更工具，操作未执行")
}

func attachmentsRequireToolLoop(attachments []AttachmentPayload) bool {
	for _, attachment := range attachments {
		switch attachment.Kind {
		case "workspace_file":
			return true
		}
	}
	return false
}
