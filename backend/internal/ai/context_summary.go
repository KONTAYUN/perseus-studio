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
	conversationSummaryMinNewMessages  = 8
	conversationSummaryMaxRunes        = 2600
	conversationSummaryTranscriptRunes = 32000
)

type ConversationCompactionResult struct {
	ConversationID        string    `json:"conversation_id"`
	SummaryUntilMessageID string    `json:"summary_until_message_id"`
	CompactedMessageCount int       `json:"compacted_message_count"`
	EstimatedTokensBefore int       `json:"estimated_tokens_before"`
	EstimatedTokensAfter  int       `json:"estimated_tokens_after"`
	ContextWindowTokens   int       `json:"context_window_tokens"`
	ThresholdPercent      int       `json:"threshold_percent"`
	CompactReservedTokens int       `json:"compact_reserved_tokens"`
	SummaryUpdatedAt      time.Time `json:"summary_updated_at"`
	Reason                string    `json:"reason"`
	Error                 string    `json:"error,omitempty"`
	Updated               bool      `json:"updated"`
	Skipped               bool      `json:"skipped"`
	Summary               string    `json:"-"`
}

func (s *Service) prepareMessagesWithConversationSummary(ctx context.Context, conversationID string, provider store.ModelProvider, messages []store.Message, opts RunOptions) []store.Message {
	if s.calls == nil || opts.UserID == "" || conversationID == "" || len(messages) == 0 {
		return messages
	}

	summary, err := s.calls.GetConversationSummary(ctx, opts.UserID, conversationID)
	if err != nil {
		return messages
	}

	if opts.TaskID == "" {
		result := s.maybeRefreshConversationSummary(ctx, conversationID, provider, messages, summary, opts, runtimeProfileForProvider(provider), "auto_threshold", "")
		if result.Updated {
			summary.Summary = result.Summary
			summary.UntilMessageID = result.SummaryUntilMessageID
		}
	}

	return injectConversationSummary(messages, summary.Summary, summary.UntilMessageID)
}

func (s *Service) CompactConversation(ctx context.Context, userID, userRole, conversationID, modelProviderID, reason, instructions string) (ConversationCompactionResult, error) {
	if s.calls == nil || s.configs == nil {
		return ConversationCompactionResult{}, fmt.Errorf("conversation compaction is not available")
	}
	userRole = normalizeRunUserRole(userRole)
	provider, err := s.configs.GetModelProviderForUser(ctx, userID, userRole, modelProviderID)
	if err != nil {
		return ConversationCompactionResult{}, err
	}
	messages, err := s.calls.ListMessages(ctx, userID, conversationID)
	if err != nil {
		return ConversationCompactionResult{}, err
	}
	current, err := s.calls.GetConversationSummary(ctx, userID, conversationID)
	if err != nil {
		return ConversationCompactionResult{}, err
	}
	result := s.maybeRefreshConversationSummary(ctx, conversationID, provider, messages, current, RunOptions{UserID: userID, UserRole: userRole}, runtimeProfileForProvider(provider), firstNonEmpty(reason, "manual"), instructions)
	if result.Reason == "manual" && result.Error != "" {
		return result, fmt.Errorf("%s", result.Error)
	}
	return result, nil
}

func (s *Service) maybeRefreshConversationSummary(ctx context.Context, conversationID string, provider store.ModelProvider, messages []store.Message, current store.ConversationSummary, opts RunOptions, profile providerRuntimeProfile, reason, instructions string) ConversationCompactionResult {
	reason = firstNonEmpty(reason, "auto_threshold")
	result := ConversationCompactionResult{
		ConversationID:        conversationID,
		EstimatedTokensBefore: estimateMessagesTokens(messages),
		ContextWindowTokens:   profile.ContextWindowTokens,
		ThresholdPercent:      profile.CompactThresholdPercent,
		CompactReservedTokens: profile.CompactReservedTokens,
		Reason:                reason,
	}
	if result.ContextWindowTokens <= 0 {
		result.ContextWindowTokens = defaultContextWindowTokens
	}
	if reason != "manual" && !profile.AutoCompactEnabled {
		result.Skipped = true
		return result
	}
	if reason != "manual" && reason != "overflow_retry" && result.EstimatedTokensBefore < contextBudgetTokens(profile) {
		result.Skipped = true
		return result
	}
	keepRecent := conversationSummaryKeepRecent
	isPressureCompaction := reason == "manual" || reason == "overflow_retry" || result.EstimatedTokensBefore >= contextBudgetTokens(profile)
	if len(messages)-keepRecent <= 0 && isPressureCompaction {
		keepRecent = 4
		if len(messages) > 12 {
			keepRecent = 8
		}
	}
	cutoff := len(messages) - keepRecent
	if cutoff <= 0 {
		result.Skipped = true
		return result
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
	minNewMessages := conversationSummaryMinNewMessages
	if isPressureCompaction {
		minNewMessages = 2
	}
	if cutoff-start < minNewMessages {
		result.Skipped = true
		return result
	}

	chunk := messages[start:cutoff]
	untilID := chunk[len(chunk)-1].ID
	result.CompactedMessageCount = len(chunk)
	result.SummaryUntilMessageID = untilID
	emitContextCompactionEvent(opts, "context_compaction_started", "info", result, nil)
	nextSummary, err := s.summarizeConversationChunk(ctx, provider, current.Summary, chunk, opts, instructions)
	if err != nil {
		if !isContextCanceledError(err) {
			log.Printf("[ai] conversation summary skipped provider=%s model=%s error=%v", provider.Name, provider.ModelID, err)
		}
		result.Error = err.Error()
		emitContextCompactionEvent(opts, "context_compaction_failed", "warn", result, err)
		result.Skipped = true
		return result
	}
	nextSummary = strings.TrimSpace(nextSummary)
	if nextSummary == "" {
		result.Skipped = true
		return result
	}
	nextSummary, _ = truncatePlainForModel(nextSummary, conversationSummaryMaxRunes)
	if err := s.calls.UpdateConversationSummary(ctx, opts.UserID, conversationID, nextSummary, untilID); err != nil {
		log.Printf("[ai] conversation summary update failed conversation=%s error=%v", conversationID, err)
		result.Error = err.Error()
		emitContextCompactionEvent(opts, "context_compaction_failed", "warn", result, err)
		result.Skipped = true
		return result
	}
	result.Summary = nextSummary
	result.Updated = true
	result.SummaryUpdatedAt = time.Now().UTC()
	result.EstimatedTokensAfter = estimateMessagesTokens(injectConversationSummary(messages, nextSummary, untilID))
	emitContextCompactionEvent(opts, "context_compaction_completed", "info", result, nil)
	log.Printf("[ai] conversation summary updated conversation=%s until_message=%s chars=%d tokens_before=%d tokens_after=%d",
		conversationID, untilID, len([]rune(nextSummary)), result.EstimatedTokensBefore, result.EstimatedTokensAfter)
	return result
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

func (s *Service) summarizeConversationChunk(ctx context.Context, provider store.ModelProvider, previousSummary string, messages []store.Message, opts RunOptions, instructions string) (string, error) {
	instruction := "You are compressing Perseus Studio conversation history for a coding/agent workspace. Produce a concise, factual memory summary in the user's language when clear. Keep durable facts only: user goals, preferences, constraints, important file paths, model/tool issues already discovered, completed changes, unresolved tasks, and warnings. Do not include full prompts, API keys, full file contents, or long command outputs. Do not invent facts. Maximum 1800 Chinese characters or comparable length."
	prompt := buildConversationSummaryPrompt(previousSummary, messages, instructions)
	switch strings.ToLower(strings.TrimSpace(provider.Protocol)) {
	case "openai":
		return s.summarizeOpenAI(ctx, provider, instruction, prompt, opts)
	case "anthropic":
		return s.summarizeAnthropic(ctx, provider, instruction, prompt, opts)
	default:
		return "", fmt.Errorf("unsupported protocol: %s", provider.Protocol)
	}
}

func buildConversationSummaryPrompt(previousSummary string, messages []store.Message, instructions string) string {
	var b strings.Builder
	if strings.TrimSpace(instructions) != "" {
		b.WriteString("User compaction instructions:\n")
		b.WriteString(truncateWithContextNote(instructions, 1200, "compaction instructions"))
		b.WriteString("\n\n")
	}
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

func emitContextCompactionEvent(opts RunOptions, eventType, level string, result ConversationCompactionResult, err error) {
	if opts.Event == nil {
		return
	}
	payload := map[string]any{
		"conversation_id":          result.ConversationID,
		"reason":                   result.Reason,
		"summary_until_message_id": result.SummaryUntilMessageID,
		"compacted_message_count":  result.CompactedMessageCount,
		"estimated_tokens_before":  result.EstimatedTokensBefore,
		"estimated_tokens_after":   result.EstimatedTokensAfter,
		"context_window_tokens":    result.ContextWindowTokens,
		"threshold_percent":        result.ThresholdPercent,
		"compact_reserved_tokens":  result.CompactReservedTokens,
	}
	if err != nil {
		payload["error"] = err.Error()
	}
	opts.Event(eventType, level, payload)
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
