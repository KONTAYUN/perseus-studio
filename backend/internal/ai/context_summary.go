package ai

import (
	"context"
	"fmt"
	"time"

	"webide-backend/internal/store"
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

func (s *Service) CompactConversation(ctx context.Context, userID, userRole, conversationID, modelProviderID, reason, instructions string) (ConversationCompactionResult, error) {
	if s.calls == nil || s.configs == nil {
		return ConversationCompactionResult{}, fmt.Errorf("conversation compaction is not available")
	}
	userRole = normalizeRunUserRole(userRole)
	provider, err := s.configs.GetModelProviderForUser(ctx, userID, userRole, modelProviderID)
	if err != nil {
		return ConversationCompactionResult{}, err
	}
	if err := s.calls.EnsureAgentTurnItems(ctx, userID, conversationID); err != nil {
		return ConversationCompactionResult{}, err
	}
	items, err := s.calls.ListAgentTurnItems(ctx, userID, conversationID)
	if err != nil {
		return ConversationCompactionResult{}, err
	}
	profile := runtimeProfileForProvider(provider)
	active, currentBoundary := activeAgentContextItems(items)
	boundary := selectAgentCompactionBoundary(active, minInt(contextRecentTurnSuffixTokens, contextAssemblyBudgetTokens(profile)/2))
	result := ConversationCompactionResult{
		ConversationID: conversationID, EstimatedTokensBefore: estimateAgentItemsTokens(active),
		ContextWindowTokens: profile.ContextWindowTokens, ThresholdPercent: profile.CompactThresholdPercent,
		CompactReservedTokens: profile.CompactReservedTokens, Reason: firstNonEmpty(reason, "manual"),
	}
	if boundary <= currentBoundary {
		result.Skipped = true
		return result, nil
	}
	var chunk []store.AgentTurnItem
	for _, item := range active {
		if item.Seq > currentBoundary && item.Seq <= boundary {
			chunk = append(chunk, item)
		}
	}
	result.CompactedMessageCount = len(chunk)
	summary, err := s.summarizeAgentItemChunk(ctx, provider, chunk, RunOptions{UserID: userID, UserRole: userRole}, instructions)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	turnID, _ := s.calls.LatestAgentTurnID(ctx, userID, conversationID)
	_, err = s.calls.AppendAgentTurnItem(ctx, store.AgentTurnItemInput{
		UserID: userID, ConversationID: conversationID, TurnID: firstNonEmpty(turnID, "manual-compact"),
		Kind: "compaction", Role: "system", Status: "completed", Visibility: store.AgentItemVisibilityModel,
		SourceID:    fmt.Sprintf("compaction:manual:%d", boundary),
		PayloadJSON: mustJSON(map[string]any{"summary": summary, "reason": result.Reason, "replaces_through_seq": boundary}),
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		return result, err
	}
	result.Summary = summary
	result.Updated = true
	result.SummaryUpdatedAt = time.Now().UTC()
	remaining, _ := activeAgentContextItems(append(items, store.AgentTurnItem{Seq: boundary + 1, Kind: "compaction", Visibility: store.AgentItemVisibilityModel, PayloadJSON: mustJSON(map[string]any{"replaces_through_seq": boundary, "summary": summary})}))
	result.EstimatedTokensAfter = estimateAgentItemsTokens(remaining)
	return result, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
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
	resp, err := s.openAISDKPostRaw(ctx, provider, path, payload, opts, "conversation_summary", nil)
	duration := time.Since(startedAt)
	if err != nil {
		s.recordProviderUsage(ctx, providerUsageRecordInput{
			Provider:    provider,
			Opts:        opts,
			Stage:       "conversation_summary",
			RequestKind: "chat_completions",
			Stream:      false,
			Status:      store.UsageStatusFailed,
			ErrorClass:  usageFailureClass(err, ProviderRequestSummary{}),
			DurationMS:  duration.Milliseconds(),
		})
		if !isContextCanceledError(err) {
			s.recordProviderDiagnostic(ctx, opts, provider, "", "", "conversation_summary", "warn", duration, err)
		}
		return "", err
	}
	var data openAIResponse
	if err := decodeProviderResponse(resp, endpoint, &data); err != nil {
		s.recordProviderUsage(ctx, providerUsageRecordInput{
			Provider:    provider,
			Opts:        opts,
			Stage:       "conversation_summary",
			RequestKind: "chat_completions",
			Stream:      false,
			Status:      store.UsageStatusFailed,
			ErrorClass:  usageFailureClass(err, ProviderRequestSummary{}),
			DurationMS:  duration.Milliseconds(),
		})
		s.recordProviderDiagnostic(ctx, opts, provider, "", "", "conversation_summary", "warn", duration, err)
		return "", err
	}
	if data.Error != nil {
		err := formatProviderError(resp.StatusCode, data.Error)
		s.recordProviderUsage(ctx, providerUsageRecordInput{
			Provider:    provider,
			Opts:        opts,
			Stage:       "conversation_summary",
			RequestKind: "chat_completions",
			Stream:      false,
			Status:      store.UsageStatusFailed,
			ErrorClass:  usageFailureClass(err, ProviderRequestSummary{}),
			DurationMS:  duration.Milliseconds(),
		})
		s.recordProviderDiagnostic(ctx, opts, provider, "", "", "conversation_summary", "warn", duration, err)
		return "", err
	}
	if len(data.Choices) == 0 {
		return "", fmt.Errorf("empty summary response")
	}
	usage := ProviderUsage{}
	updateProviderUsageFromAny(&usage, data.Usage)
	s.recordProviderUsage(ctx, providerUsageRecordInput{
		Provider:    provider,
		Opts:        opts,
		Stage:       "conversation_summary",
		RequestKind: "chat_completions",
		Stream:      false,
		Status:      store.UsageStatusSucceeded,
		Usage:       usage,
		DurationMS:  duration.Milliseconds(),
	})
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
	resp, err := s.anthropicSDKPostRaw(ctx, provider, path, payload, opts, "conversation_summary", nil)
	duration := time.Since(startedAt)
	if err != nil {
		s.recordProviderUsage(ctx, providerUsageRecordInput{
			Provider:    provider,
			Opts:        opts,
			Stage:       "conversation_summary",
			RequestKind: "messages",
			Stream:      false,
			Status:      store.UsageStatusFailed,
			ErrorClass:  usageFailureClass(err, ProviderRequestSummary{}),
			DurationMS:  duration.Milliseconds(),
		})
		if !isContextCanceledError(err) {
			s.recordProviderDiagnostic(ctx, opts, provider, "", "", "conversation_summary", "warn", duration, err)
		}
		return "", err
	}
	var data anthropicResponse
	if err := decodeProviderResponse(resp, endpoint, &data); err != nil {
		s.recordProviderUsage(ctx, providerUsageRecordInput{
			Provider:    provider,
			Opts:        opts,
			Stage:       "conversation_summary",
			RequestKind: "messages",
			Stream:      false,
			Status:      store.UsageStatusFailed,
			ErrorClass:  usageFailureClass(err, ProviderRequestSummary{}),
			DurationMS:  duration.Milliseconds(),
		})
		s.recordProviderDiagnostic(ctx, opts, provider, "", "", "conversation_summary", "warn", duration, err)
		return "", err
	}
	if data.Error != nil {
		err := formatProviderError(resp.StatusCode, data.Error)
		s.recordProviderUsage(ctx, providerUsageRecordInput{
			Provider:    provider,
			Opts:        opts,
			Stage:       "conversation_summary",
			RequestKind: "messages",
			Stream:      false,
			Status:      store.UsageStatusFailed,
			ErrorClass:  usageFailureClass(err, ProviderRequestSummary{}),
			DurationMS:  duration.Milliseconds(),
		})
		s.recordProviderDiagnostic(ctx, opts, provider, "", "", "conversation_summary", "warn", duration, err)
		return "", err
	}
	usage := ProviderUsage{}
	updateProviderUsageFromAny(&usage, data.Usage)
	s.recordProviderUsage(ctx, providerUsageRecordInput{
		Provider:    provider,
		Opts:        opts,
		Stage:       "conversation_summary",
		RequestKind: "messages",
		Stream:      false,
		Status:      store.UsageStatusSucceeded,
		Usage:       usage,
		DurationMS:  duration.Milliseconds(),
	})
	return data.text(), nil
}
