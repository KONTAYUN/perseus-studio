package ai

import (
	"context"
	"fmt"
	"strings"
	"time"

	"webide-backend/internal/store"
)

const (
	conversationTitleInputMaxRunes  = 2600
	conversationTitleOutputMaxRunes = 40
)

func (s *Service) GenerateConversationTitle(ctx context.Context, userID, userRole, conversationID, messageID, modelProviderID string, userMessage, assistantMessage string) (ConversationTitleResult, error) {
	if s.configs == nil {
		return ConversationTitleResult{}, fmt.Errorf("model provider configuration is not available")
	}
	userRole = normalizeRunUserRole(userRole)
	provider, err := s.configs.GetModelProviderForUser(ctx, userID, userRole, modelProviderID)
	if err != nil {
		return ConversationTitleResult{}, err
	}
	if !provider.Enabled {
		return ConversationTitleResult{}, fmt.Errorf("selected model is disabled")
	}
	instruction := "You generate short conversation titles for an AI workspace. Use the user's language. Return only one concise title, no quotes, no punctuation at the end, no explanation."
	prompt := buildConversationTitlePrompt(userMessage, assistantMessage)
	var title string
	switch strings.ToLower(strings.TrimSpace(provider.Protocol)) {
	case "openai":
		title, err = s.generateOpenAIConversationTitle(ctx, provider, instruction, prompt, RunOptions{UserID: userID, UserRole: userRole}, conversationID, messageID)
	case "anthropic":
		title, err = s.generateAnthropicConversationTitle(ctx, provider, instruction, prompt, RunOptions{UserID: userID, UserRole: userRole}, conversationID, messageID)
	default:
		err = fmt.Errorf("unsupported protocol: %s", provider.Protocol)
	}
	if err != nil {
		return ConversationTitleResult{}, err
	}
	title = normalizeGeneratedConversationTitle(title)
	if title == "" {
		return ConversationTitleResult{}, fmt.Errorf("empty title response")
	}
	return ConversationTitleResult{Title: title}, nil
}

func buildConversationTitlePrompt(userMessage, assistantMessage string) string {
	userMessage = truncateWithContextNote(userMessage, 1200, "user message")
	assistantMessage = truncateWithContextNote(assistantMessage, 1200, "assistant message")
	var b strings.Builder
	b.WriteString("Create a short title for this conversation.\n")
	b.WriteString("Rules:\n")
	b.WriteString("- 5 to 12 Chinese characters when Chinese is natural, or 2 to 6 English words.\n")
	b.WriteString("- Summarize the task or topic; do not copy the first sentence mechanically.\n")
	b.WriteString("- No quotation marks, markdown, emoji, trailing period, or extra text.\n\n")
	b.WriteString("User message:\n")
	b.WriteString(userMessage)
	if strings.TrimSpace(assistantMessage) != "" {
		b.WriteString("\n\nAssistant response:\n")
		b.WriteString(assistantMessage)
	}
	return truncateWithContextNote(b.String(), conversationTitleInputMaxRunes, "title transcript")
}

func (s *Service) generateOpenAIConversationTitle(ctx context.Context, provider store.ModelProvider, instruction, prompt string, opts RunOptions, conversationID, messageID string) (string, error) {
	payload := map[string]any{
		"model": provider.ModelID,
		"messages": []map[string]any{
			{"role": "system", "content": instruction},
			{"role": "user", "content": prompt},
		},
		"stream": false,
	}
	path := openAIChatPath()
	endpoint := providerSDKEndpoint(provider.BaseURL, path)
	startedAt := time.Now()
	resp, err := s.openAISDKPostRaw(ctx, provider, path, payload, opts, "conversation_title")
	duration := time.Since(startedAt)
	if err != nil {
		if !isContextCanceledError(err) {
			s.recordProviderDiagnostic(ctx, opts, provider, conversationID, messageID, "conversation_title", "warn", duration, err)
		}
		return "", err
	}
	var data openAIResponse
	if err := decodeProviderResponse(resp, endpoint, &data); err != nil {
		s.recordProviderDiagnostic(ctx, opts, provider, conversationID, messageID, "conversation_title", "warn", duration, err)
		return "", err
	}
	if data.Error != nil {
		err := formatProviderError(resp.StatusCode, data.Error)
		s.recordProviderDiagnostic(ctx, opts, provider, conversationID, messageID, "conversation_title", "warn", duration, err)
		return "", err
	}
	if len(data.Choices) == 0 {
		return "", fmt.Errorf("empty title response")
	}
	return normalizeOpenAIContent(data.Choices[0].Message.Content), nil
}

func (s *Service) generateAnthropicConversationTitle(ctx context.Context, provider store.ModelProvider, instruction, prompt string, opts RunOptions, conversationID, messageID string) (string, error) {
	payload := map[string]any{
		"model":      provider.ModelID,
		"max_tokens": 64,
		"system":     instruction,
		"messages": []map[string]any{
			{"role": "user", "content": prompt},
		},
		"stream": false,
	}
	path := anthropicMessagesPath(provider)
	endpoint := providerSDKEndpoint(provider.BaseURL, path)
	startedAt := time.Now()
	resp, err := s.anthropicSDKPostRaw(ctx, provider, path, payload, opts, "conversation_title")
	duration := time.Since(startedAt)
	if err != nil {
		if !isContextCanceledError(err) {
			s.recordProviderDiagnostic(ctx, opts, provider, conversationID, messageID, "conversation_title", "warn", duration, err)
		}
		return "", err
	}
	var data anthropicResponse
	if err := decodeProviderResponse(resp, endpoint, &data); err != nil {
		s.recordProviderDiagnostic(ctx, opts, provider, conversationID, messageID, "conversation_title", "warn", duration, err)
		return "", err
	}
	if data.Error != nil {
		err := formatProviderError(resp.StatusCode, data.Error)
		s.recordProviderDiagnostic(ctx, opts, provider, conversationID, messageID, "conversation_title", "warn", duration, err)
		return "", err
	}
	return data.text(), nil
}

func normalizeGeneratedConversationTitle(value string) string {
	value = providerVisibleText(strings.TrimSpace(value))
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"'`“”‘’「」『』《》")
	value = strings.TrimSpace(value)
	for _, prefix := range []string{"标题：", "标题:", "Title:", "Title："} {
		value = strings.TrimSpace(strings.TrimPrefix(value, prefix))
	}
	lines := strings.FieldsFunc(value, func(r rune) bool {
		return r == '\n' || r == '\r'
	})
	if len(lines) > 0 {
		value = strings.TrimSpace(lines[0])
	}
	value = strings.Trim(value, "\"'`“”‘’「」『』《》")
	value = strings.TrimRight(value, "。.!！?？,，;；:：")
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if trimmed, truncated := truncatePlainForModel(value, conversationTitleOutputMaxRunes); truncated {
		value = strings.TrimRight(strings.TrimSpace(trimmed), "。.!！?？,，;；:：")
	}
	return value
}
