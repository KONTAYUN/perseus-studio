package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (s *UsageStore) AppendEvent(ctx context.Context, input LLMUsageEventInput) (LLMUsageEvent, error) {
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		return LLMUsageEvent{}, fmt.Errorf("user_id is required")
	}
	now := time.Now().UTC()
	item := LLMUsageEvent{
		ID:                  nextStoreID("usage", now),
		UserID:              userID,
		RunID:               trimUsageField(input.RunID, 200),
		ConversationID:      trimUsageField(input.ConversationID, 200),
		MessageID:           trimUsageField(input.MessageID, 200),
		TaskID:              trimUsageField(input.TaskID, 200),
		ModelProviderID:     trimUsageField(input.ModelProviderID, 200),
		ProviderName:        trimUsageField(input.ProviderName, 200),
		Protocol:            trimUsageField(input.Protocol, 32),
		ModelID:             trimUsageField(input.ModelID, 200),
		BaseURLHost:         trimUsageField(input.BaseURLHost, 255),
		Stage:               trimUsageField(input.Stage, 200),
		RequestKind:         trimUsageField(input.RequestKind, 64),
		Stream:              input.Stream,
		Status:              normalizeUsageStatus(input.Status),
		ErrorClass:          trimUsageField(input.ErrorClass, 200),
		InputTokens:         maxInt64(0, input.InputTokens),
		OutputTokens:        maxInt64(0, input.OutputTokens),
		CacheCreationTokens: maxInt64(0, input.CacheCreationTokens),
		CacheReadTokens:     maxInt64(0, input.CacheReadTokens),
		TotalTokens:         maxInt64(0, input.TotalTokens),
		UsageSource:         normalizeUsageSource(input.UsageSource),
		DurationMS:          maxInt64(0, input.DurationMS),
		CreatedAt:           now,
	}
	if item.TotalTokens == 0 {
		item.TotalTokens = item.InputTokens + item.OutputTokens + item.CacheCreationTokens + item.CacheReadTokens
	}
	if price, ok := s.priceForEvent(ctx, item.ModelProviderID, item.ModelID); ok {
		item.Currency = price.Currency
		item.PricingSource = "manual"
		item.EstimatedCostMicro = estimateUsageCostMicro(item, price)
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO llm_usage_events (
  id, user_id, run_id, conversation_id, message_id, task_id, model_provider_id, provider_name,
  protocol, model_id, base_url_host, stage, request_kind, stream, status, error_class,
  input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, total_tokens,
  usage_source, estimated_cost_micro, currency, pricing_source, duration_ms, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.UserID, item.RunID, item.ConversationID, item.MessageID, item.TaskID, item.ModelProviderID, item.ProviderName,
		item.Protocol, item.ModelID, item.BaseURLHost, item.Stage, item.RequestKind, boolToInt(item.Stream), item.Status, item.ErrorClass,
		item.InputTokens, item.OutputTokens, item.CacheCreationTokens, item.CacheReadTokens, item.TotalTokens,
		item.UsageSource, item.EstimatedCostMicro, item.Currency, item.PricingSource, item.DurationMS, item.CreatedAt)
	if err != nil {
		return LLMUsageEvent{}, err
	}
	_ = s.CleanupBefore(ctx, now.AddDate(0, 0, -180))
	return item, nil
}

func (s *UsageStore) ListEvents(ctx context.Context, query UsageQuery) ([]LLMUsageEvent, error) {
	filters, args := usageFilters(query)
	where := ""
	if len(filters) > 0 {
		where = "WHERE " + strings.Join(filters, " AND ")
	}
	limit := normalizeUsageLimit(query.Limit, 100)
	offset := query.Offset
	if offset < 0 {
		offset = 0
	}
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, run_id, conversation_id, message_id, task_id, model_provider_id, provider_name,
  protocol, model_id, base_url_host, stage, request_kind, stream, status, error_class,
  input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, total_tokens,
  usage_source, estimated_cost_micro, currency, pricing_source, duration_ms, created_at
FROM llm_usage_events `+where+`
ORDER BY created_at DESC
LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []LLMUsageEvent
	for rows.Next() {
		item, err := scanUsageEvent(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *UsageStore) DeleteUser(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM llm_usage_events WHERE user_id = ?`, strings.TrimSpace(userID))
	return err
}

func (s *UsageStore) CleanupBefore(ctx context.Context, cutoff time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM llm_usage_events WHERE created_at < ?`, cutoff.UTC())
	return err
}

func scanUsageEvent(s scanner) (LLMUsageEvent, error) {
	var item LLMUsageEvent
	var stream int
	err := s.Scan(&item.ID, &item.UserID, &item.RunID, &item.ConversationID, &item.MessageID, &item.TaskID,
		&item.ModelProviderID, &item.ProviderName, &item.Protocol, &item.ModelID, &item.BaseURLHost, &item.Stage,
		&item.RequestKind, &stream, &item.Status, &item.ErrorClass, &item.InputTokens, &item.OutputTokens,
		&item.CacheCreationTokens, &item.CacheReadTokens, &item.TotalTokens, &item.UsageSource,
		&item.EstimatedCostMicro, &item.Currency, &item.PricingSource, &item.DurationMS, &item.CreatedAt)
	item.Stream = stream != 0
	return item, err
}
