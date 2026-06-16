package ai

import (
	"context"
	"log"

	"webide-backend/internal/store"
)

type providerUsageRecordInput struct {
	Provider       store.ModelProvider
	Opts           RunOptions
	ConversationID string
	MessageID      string
	Stage          string
	RequestKind    string
	Stream         bool
	Status         string
	ErrorClass     string
	Usage          ProviderUsage
	DurationMS     int64
}

func (s *Service) recordProviderUsage(ctx context.Context, input providerUsageRecordInput) {
	if s == nil || s.usage == nil || input.Opts.UserID == "" {
		return
	}
	usageSource := store.UsageSourceProviderMissing
	if providerUsageReported(input.Usage) {
		usageSource = store.UsageSourceProviderReported
	}
	totalTokens := input.Usage.TotalTokens
	if totalTokens == 0 {
		totalTokens = input.Usage.InputTokens + input.Usage.OutputTokens + input.Usage.CacheCreationTokens + input.Usage.CacheReadTokens
	}
	if input.Status == "" {
		input.Status = store.UsageStatusSucceeded
	}
	writeCtx := ctx
	if writeCtx == nil || writeCtx.Err() != nil {
		writeCtx = context.Background()
	}
	if _, err := s.usage.AppendEvent(writeCtx, store.LLMUsageEventInput{
		UserID:              input.Opts.UserID,
		RunID:               input.Opts.RunID,
		ConversationID:      input.ConversationID,
		MessageID:           input.MessageID,
		TaskID:              input.Opts.TaskID,
		ModelProviderID:     input.Provider.ID,
		ProviderName:        input.Provider.Name,
		Protocol:            input.Provider.Protocol,
		ModelID:             input.Provider.ModelID,
		BaseURLHost:         providerHost(input.Provider.BaseURL),
		Stage:               input.Stage,
		RequestKind:         input.RequestKind,
		Stream:              input.Stream,
		Status:              input.Status,
		ErrorClass:          input.ErrorClass,
		InputTokens:         input.Usage.InputTokens,
		OutputTokens:        input.Usage.OutputTokens,
		CacheCreationTokens: input.Usage.CacheCreationTokens,
		CacheReadTokens:     input.Usage.CacheReadTokens,
		TotalTokens:         totalTokens,
		UsageSource:         usageSource,
		DurationMS:          input.DurationMS,
	}); err != nil {
		log.Printf("[ai] usage record failed user=%s provider=%s model=%s stage=%s error=%v", input.Opts.UserID, input.Provider.Name, input.Provider.ModelID, input.Stage, err)
	}
}

func providerUsageReported(usage ProviderUsage) bool {
	return usage.InputTokens > 0 ||
		usage.OutputTokens > 0 ||
		usage.CacheCreationTokens > 0 ||
		usage.CacheReadTokens > 0 ||
		usage.TotalTokens > 0
}

func usageStatusForError(err error) string {
	if err != nil {
		return store.UsageStatusFailed
	}
	return store.UsageStatusSucceeded
}

func usageFailureClass(err error, summary ProviderRequestSummary) string {
	if err == nil {
		return ""
	}
	return providerFailureClass(err, summary)
}
