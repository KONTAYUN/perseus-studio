package ai

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"webide-backend/internal/store"
)

const (
	conversationSummaryKeepRecent      = 16
	conversationSummaryTriggerMessages = 34
	conversationSummaryMinNewMessages  = 8
	conversationSummaryMaxRunes        = 2600
	conversationSummaryTranscriptRunes = 32000
)

func (s *Service) prepareMessagesWithConversationSummary(ctx context.Context, conversationID string, provider store.ModelProvider, messages []store.Message, opts RunOptions) []store.Message {
	if s.calls == nil || opts.UserID == "" || conversationID == "" || len(messages) == 0 {
		return messages
	}

	summary, err := s.calls.GetConversationSummary(ctx, opts.UserID, conversationID)
	if err != nil {
		return messages
	}

	if opts.TaskID == "" {
		if nextSummary, untilID, ok := s.maybeRefreshConversationSummary(ctx, provider, messages, summary, opts); ok {
			summary.Summary = nextSummary
			summary.UntilMessageID = untilID
			if err := s.calls.UpdateConversationSummary(ctx, opts.UserID, conversationID, nextSummary, untilID); err != nil {
				log.Printf("[ai] conversation summary update failed conversation=%s error=%v", conversationID, err)
			} else {
				log.Printf("[ai] conversation summary updated conversation=%s until_message=%s chars=%d", conversationID, untilID, len([]rune(nextSummary)))
			}
		}
	}

	return injectConversationSummary(messages, summary.Summary, summary.UntilMessageID)
}

func (s *Service) maybeRefreshConversationSummary(ctx context.Context, provider store.ModelProvider, messages []store.Message, current store.ConversationSummary, opts RunOptions) (string, string, bool) {
	if len(messages) < conversationSummaryTriggerMessages {
		return "", "", false
	}
	cutoff := len(messages) - conversationSummaryKeepRecent
	if cutoff <= 0 {
		return "", "", false
	}

	start := 0
	if current.UntilMessageID != "" {
		for index, message := range messages {
			if message.ID == current.UntilMessageID {
				start = index + 1
				break
			}
		}
	}
	if cutoff-start < conversationSummaryMinNewMessages {
		return "", "", false
	}

	chunk := messages[start:cutoff]
	untilID := chunk[len(chunk)-1].ID
	nextSummary, err := s.summarizeConversationChunk(ctx, provider, current.Summary, chunk, opts)
	if err != nil {
		if !isContextCanceledError(err) {
			log.Printf("[ai] conversation summary skipped provider=%s model=%s error=%v", provider.Name, provider.ModelID, err)
		}
		return "", "", false
	}
	nextSummary = strings.TrimSpace(nextSummary)
	if nextSummary == "" {
		return "", "", false
	}
	nextSummary, _ = truncatePlainForModel(nextSummary, conversationSummaryMaxRunes)
	return nextSummary, untilID, true
}

func injectConversationSummary(messages []store.Message, summary, untilMessageID string) []store.Message {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return messages
	}
	trimmed := messages
	if untilMessageID != "" {
		for index, message := range messages {
			if message.ID == untilMessageID {
				trimmed = messages[index+1:]
				break
			}
		}
	}
	content := "Conversation memory summary for stability. This is a compact summary of earlier messages, not a substitute for reading workspace files with tools when file contents are needed.\n\n" + summary
	item := store.Message{
		Role:    "system",
		Content: content,
	}
	next := make([]store.Message, 0, len(trimmed)+1)
	next = append(next, item)
	next = append(next, trimmed...)
	return next
}

func (s *Service) summarizeConversationChunk(ctx context.Context, provider store.ModelProvider, previousSummary string, messages []store.Message, opts RunOptions) (string, error) {
	instruction := "You are compressing Perseus Studio conversation history for a coding/agent workspace. Produce a concise, factual memory summary in the user's language when clear. Keep durable facts only: user goals, preferences, constraints, important file paths, model/tool issues already discovered, completed changes, unresolved tasks, and warnings. Do not include full prompts, API keys, full file contents, or long command outputs. Do not invent facts. Maximum 1800 Chinese characters or comparable length."
	prompt := buildConversationSummaryPrompt(previousSummary, messages)
	switch strings.ToLower(strings.TrimSpace(provider.Protocol)) {
	case "openai":
		return s.summarizeOpenAI(ctx, provider, instruction, prompt, opts)
	case "anthropic":
		return s.summarizeAnthropic(ctx, provider, instruction, prompt, opts)
	default:
		return "", fmt.Errorf("unsupported protocol: %s", provider.Protocol)
	}
}

func buildConversationSummaryPrompt(previousSummary string, messages []store.Message) string {
	var b strings.Builder
	if strings.TrimSpace(previousSummary) != "" {
		b.WriteString("Existing memory summary:\n")
		b.WriteString(truncateWithContextNote(previousSummary, conversationSummaryMaxRunes, "previous summary"))
		b.WriteString("\n\n")
	}
	b.WriteString("New conversation messages to merge into memory:\n")
	for _, message := range messages {
		role := strings.ToUpper(strings.TrimSpace(message.Role))
		if role == "" {
			role = "MESSAGE"
		}
		content := truncateWithContextNote(message.Content, 1800, "message")
		if strings.TrimSpace(message.AttachmentsJSON) != "" && strings.TrimSpace(message.AttachmentsJSON) != "[]" {
			content += "\n[attachments metadata present; do not treat as file content unless explicitly described]"
		}
		b.WriteString("\n---\n")
		b.WriteString(role)
		b.WriteString(" ")
		if message.ID != "" {
			b.WriteString(message.ID)
		}
		b.WriteString(":\n")
		b.WriteString(content)
	}
	return truncateWithContextNote(b.String(), conversationSummaryTranscriptRunes, "summary transcript")
}

func (s *Service) summarizeOpenAI(ctx context.Context, provider store.ModelProvider, instruction, prompt string, opts RunOptions) (string, error) {
	payload := map[string]any{
		"model": provider.ModelID,
		"messages": []map[string]any{
			{"role": "system", "content": instruction},
			{"role": "user", "content": prompt},
		},
	}
	path := openAIChatPath()
	endpoint := providerSDKEndpoint(provider.BaseURL, path)
	startedAt := time.Now()
	resp, err := s.openAISDKPostRaw(ctx, provider, path, payload, opts, "conversation_summary")
	duration := time.Since(startedAt)
	if err != nil {
		if !isContextCanceledError(err) {
			s.recordProviderDiagnostic(ctx, opts, provider, "", "", "conversation_summary", "warn", duration, err)
		}
		return "", err
	}
	var data openAIResponse
	if err := decodeProviderResponse(resp, endpoint, &data); err != nil {
		s.recordProviderDiagnostic(ctx, opts, provider, "", "", "conversation_summary", "warn", duration, err)
		return "", err
	}
	if data.Error != nil {
		err := formatProviderError(resp.StatusCode, data.Error)
		s.recordProviderDiagnostic(ctx, opts, provider, "", "", "conversation_summary", "warn", duration, err)
		return "", err
	}
	if len(data.Choices) == 0 {
		return "", fmt.Errorf("empty summary response")
	}
	return normalizeOpenAIContent(data.Choices[0].Message.Content), nil
}

func (s *Service) summarizeAnthropic(ctx context.Context, provider store.ModelProvider, instruction, prompt string, opts RunOptions) (string, error) {
	payload := map[string]any{
		"model":      provider.ModelID,
		"max_tokens": 2048,
		"system":     instruction,
		"messages": []map[string]any{
			{"role": "user", "content": prompt},
		},
	}
	path := anthropicMessagesPath(provider)
	endpoint := providerSDKEndpoint(provider.BaseURL, path)
	startedAt := time.Now()
	resp, err := s.anthropicSDKPostRaw(ctx, provider, path, payload, opts, "conversation_summary")
	duration := time.Since(startedAt)
	if err != nil {
		if !isContextCanceledError(err) {
			s.recordProviderDiagnostic(ctx, opts, provider, "", "", "conversation_summary", "warn", duration, err)
		}
		return "", err
	}
	var data anthropicResponse
	if err := decodeProviderResponse(resp, endpoint, &data); err != nil {
		s.recordProviderDiagnostic(ctx, opts, provider, "", "", "conversation_summary", "warn", duration, err)
		return "", err
	}
	if data.Error != nil {
		err := formatProviderError(resp.StatusCode, data.Error)
		s.recordProviderDiagnostic(ctx, opts, provider, "", "", "conversation_summary", "warn", duration, err)
		return "", err
	}
	return data.text(), nil
}
