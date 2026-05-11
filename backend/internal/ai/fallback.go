package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"webide-backend/internal/store"
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

func isContextCanceledError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.Canceled) || strings.Contains(strings.ToLower(err.Error()), "context canceled")
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

func (s *Service) tryFallbackTextReplacement(ctx context.Context, conversationID, messageID string, messages []store.Message, events []ToolEvent, opts RunOptions) (Reply, bool) {
	path := lastSuccessfulReadPath(events)
	if path == "" {
		return Reply{}, false
	}
	oldText, newText, ok := parseExplicitReplacement(lastUserMessageContent(messages))
	if !ok || oldText == "" || oldText == newText {
		return Reply{}, false
	}

	files, err := s.factory.Workspace(opts.UserID)
	if err != nil {
		return Reply{}, false
	}
	file, err := files.ReadFile(path)
	if err != nil || !strings.Contains(file.Content, oldText) {
		return Reply{}, false
	}
	nextContent := strings.ReplaceAll(file.Content, oldText, newText)
	payload, _ := json.Marshal(map[string]string{
		"path":    path,
		"content": nextContent,
		"mode":    "overwrite",
	})
	_, event := s.executeAndRecordTool(ctx, conversationID, messageID, "save_workspace_file", string(payload), opts, providerRuntimeProfile{})
	nextEvents := append(append([]ToolEvent{}, events...), event)
	if event.Status != "success" {
		return Reply{}, false
	}

	return Reply{
		Content:    fmt.Sprintf("模型处理文件内容时超时；已根据你的明确替换指令将 `%s` 中的 `%s` 替换为 `%s`。", path, oldText, newText),
		ToolEvents: nextEvents,
	}, true
}

func lastSuccessfulReadPath(events []ToolEvent) string {
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if event.ToolName == "read_workspace_text" && event.Status == "success" && event.Path != "" {
			return event.Path
		}
	}
	return ""
}

func parseExplicitReplacement(content string) (string, string, bool) {
	text := strings.TrimSpace(content)
	if text == "" {
		return "", "", false
	}
	versionPattern := regexp.MustCompile(`\d+(?:\.\d+){1,4}`)
	versions := versionPattern.FindAllString(text, -1)
	if len(versions) >= 2 && containsAny(text, []string{"版本", "版本号", "写成", "改成", "改为", "替换为", "换成"}) {
		return versions[0], versions[len(versions)-1], true
	}

	patterns := []*regexp.Regexp{
		regexp.MustCompile(`把\s*[“"']?(.+?)[”"']?\s*(?:改成|改为|替换为|换成|写成)\s*[“"']?(.+?)[”"']?\s*$`),
		regexp.MustCompile(`将\s*[“"']?(.+?)[”"']?\s*(?:改成|改为|替换为|换成|写成)\s*[“"']?(.+?)[”"']?\s*$`),
	}
	for _, pattern := range patterns {
		match := pattern.FindStringSubmatch(text)
		if len(match) == 3 {
			oldText := cleanReplacementText(match[1])
			newText := cleanReplacementText(match[2])
			if oldText != "" && newText != "" {
				return oldText, newText, true
			}
		}
	}
	return "", "", false
}

func cleanReplacementText(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, " \t\r\n。.!！?？,，;；:：`'\"“”‘’")
	return strings.TrimSpace(value)
}
