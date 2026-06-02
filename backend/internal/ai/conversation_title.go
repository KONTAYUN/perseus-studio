package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	data, stats, mismatch, err := decodeOpenAIConversationTitleResponse(resp, endpoint)
	if err != nil {
		s.recordProviderDiagnostic(ctx, opts, provider, conversationID, messageID, "conversation_title", "warn", duration, err)
		return "", err
	}
	if mismatch && data.titleText() == "" {
		err := &ProviderResponseError{
			StatusCode:  http.StatusOK,
			ContentType: "text/event-stream",
			Endpoint:    endpoint,
			Message:     "provider_protocol_mismatch: conversation title response was SSE but contained no title text",
		}
		s.recordProviderDiagnosticSummary(ctx, opts, provider, conversationID, messageID, "conversation_title", "warn", duration, err, conversationTitleProtocolMismatchSummary(provider, "text/event-stream", stats.Chunks, stats.ChoicesSeen))
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
	return normalizeOpenAIContent(data.titleText()), nil
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

func decodeOpenAIConversationTitleResponse(resp *http.Response, endpoint string) (openAIResponse, openAITitleSSEStats, bool, error) {
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/event-stream") {
		data, stats, err := decodeOpenAITitleSSE(resp.Body)
		if err != nil {
			return openAIResponse{}, stats, true, &ProviderResponseError{
				StatusCode:      resp.StatusCode,
				ContentType:     resp.Header.Get("Content-Type"),
				Endpoint:        endpoint,
				ResponseSnippet: stats.Snippet,
				ParseError:      err.Error(),
			}
		}
		return data, stats, true, nil
	}
	var data openAIResponse
	if err := decodeProviderResponse(resp, endpoint, &data); err != nil {
		return openAIResponse{}, openAITitleSSEStats{}, false, err
	}
	return data, openAITitleSSEStats{}, false, nil
}

type openAITitleSSEStats struct {
	Chunks      int
	ChoicesSeen int
	Snippet     string
}

func decodeOpenAITitleSSE(body io.ReadCloser) (openAIResponse, openAITitleSSEStats, error) {
	defer body.Close()
	var result openAIResponse
	var stats openAITitleSSEStats
	var text strings.Builder
	scanner := bufio.NewScanner(io.LimitReader(body, 2<<20))
	scanner.Buffer(make([]byte, 0, 64*1024), 2<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		dataLine := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if dataLine == "" || dataLine == "[DONE]" {
			continue
		}
		stats.Chunks++
		if stats.Snippet == "" {
			stats.Snippet = truncate(dataLine, 500)
		}
		var chunk struct {
			Error   any `json:"error"`
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
				Delta struct {
					Content any `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(dataLine), &chunk); err != nil {
			return result, stats, err
		}
		if chunk.Error != nil {
			result.Error = chunk.Error
		}
		stats.ChoicesSeen += len(chunk.Choices)
		for _, choice := range chunk.Choices {
			if choice.Message.Content != "" {
				text.WriteString(choice.Message.Content)
			}
			if delta := openAITitleDeltaText(choice.Delta.Content); delta != "" {
				text.WriteString(delta)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return result, stats, err
	}
	content := text.String()
	if content != "" {
		result.Choices = []openAIChoice{{Message: openAIMessage{Content: content}}}
	}
	return result, stats, nil
}

func openAITitleDeltaText(value any) string {
	switch item := value.(type) {
	case string:
		return item
	case []any:
		var b strings.Builder
		for _, part := range item {
			switch value := part.(type) {
			case string:
				b.WriteString(value)
			case map[string]any:
				if text := stringFromAny(value["text"]); text != "" {
					b.WriteString(text)
				}
			}
		}
		return b.String()
	default:
		return ""
	}
}

func (r openAIResponse) titleText() string {
	if len(r.Choices) == 0 {
		return ""
	}
	return normalizeOpenAIContent(r.Choices[0].Message.Content)
}

func conversationTitleProtocolMismatchSummary(provider store.ModelProvider, actualContentType string, chunks, choices int) string {
	raw, _ := json.Marshal(map[string]any{
		"failure_class":       "provider_protocol_mismatch",
		"stage":               "conversation_title",
		"protocol":            strings.TrimSpace(provider.Protocol),
		"provider":            provider.Name,
		"model":               provider.ModelID,
		"provider_host":       providerHost(provider.BaseURL),
		"requested_stream":    false,
		"actual_content_type": actualContentType,
		"sse_chunk_count":     chunks,
		"choices_seen":        choices,
	})
	return string(raw)
}
