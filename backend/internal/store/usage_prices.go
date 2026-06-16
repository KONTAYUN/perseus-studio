package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (s *UsageStore) ListPrices(ctx context.Context) ([]LLMModelPrice, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, model_provider_id, model_id, currency, input_price_micro, output_price_micro,
  cache_creation_price_micro, cache_read_price_micro, created_at, updated_at
FROM llm_model_prices
ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []LLMModelPrice
	for rows.Next() {
		item, err := scanUsagePrice(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *UsageStore) CreatePrice(ctx context.Context, input LLMModelPriceInput) (LLMModelPrice, error) {
	if err := validateUsagePriceInput(input); err != nil {
		return LLMModelPrice{}, err
	}
	now := time.Now().UTC()
	id := nextStoreID("price", now)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO llm_model_prices (
  id, model_provider_id, model_id, currency, input_price_micro, output_price_micro,
  cache_creation_price_micro, cache_read_price_micro, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, strings.TrimSpace(input.ModelProviderID), strings.TrimSpace(input.ModelID), normalizeCurrency(input.Currency),
		maxInt64(0, input.InputPriceMicro), maxInt64(0, input.OutputPriceMicro),
		maxInt64(0, input.CacheCreationPriceMicro), maxInt64(0, input.CacheReadPriceMicro), now, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return LLMModelPrice{}, fmt.Errorf("price already exists for provider and model")
		}
		return LLMModelPrice{}, err
	}
	return s.GetPrice(ctx, id)
}

func (s *UsageStore) UpdatePrice(ctx context.Context, id string, input LLMModelPriceInput) (LLMModelPrice, error) {
	if err := validateUsagePriceInput(input); err != nil {
		return LLMModelPrice{}, err
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
UPDATE llm_model_prices
SET model_provider_id = ?, model_id = ?, currency = ?, input_price_micro = ?, output_price_micro = ?,
    cache_creation_price_micro = ?, cache_read_price_micro = ?, updated_at = ?
WHERE id = ?`,
		strings.TrimSpace(input.ModelProviderID), strings.TrimSpace(input.ModelID), normalizeCurrency(input.Currency),
		maxInt64(0, input.InputPriceMicro), maxInt64(0, input.OutputPriceMicro),
		maxInt64(0, input.CacheCreationPriceMicro), maxInt64(0, input.CacheReadPriceMicro), now, strings.TrimSpace(id))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return LLMModelPrice{}, fmt.Errorf("price already exists for provider and model")
		}
		return LLMModelPrice{}, err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return LLMModelPrice{}, fmt.Errorf("price not found")
	}
	return s.GetPrice(ctx, id)
}

func (s *UsageStore) DeletePrice(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM llm_model_prices WHERE id = ?`, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return fmt.Errorf("price not found")
	}
	return nil
}

func (s *UsageStore) GetPrice(ctx context.Context, id string) (LLMModelPrice, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, model_provider_id, model_id, currency, input_price_micro, output_price_micro,
  cache_creation_price_micro, cache_read_price_micro, created_at, updated_at
FROM llm_model_prices WHERE id = ?`, strings.TrimSpace(id))
	return scanUsagePrice(row)
}

func (s *UsageStore) priceForEvent(ctx context.Context, providerID, modelID string) (LLMModelPrice, bool) {
	providerID = strings.TrimSpace(providerID)
	modelID = strings.TrimSpace(modelID)
	if providerID == "" || modelID == "" {
		return LLMModelPrice{}, false
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, model_provider_id, model_id, currency, input_price_micro, output_price_micro,
  cache_creation_price_micro, cache_read_price_micro, created_at, updated_at
FROM llm_model_prices
WHERE model_provider_id = ? AND model_id = ?`, providerID, modelID)
	item, err := scanUsagePrice(row)
	return item, err == nil
}

func scanUsagePrice(s scanner) (LLMModelPrice, error) {
	var item LLMModelPrice
	err := s.Scan(&item.ID, &item.ModelProviderID, &item.ModelID, &item.Currency, &item.InputPriceMicro, &item.OutputPriceMicro,
		&item.CacheCreationPriceMicro, &item.CacheReadPriceMicro, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func validateUsagePriceInput(input LLMModelPriceInput) error {
	if strings.TrimSpace(input.ModelProviderID) == "" {
		return fmt.Errorf("model_provider_id is required")
	}
	if strings.TrimSpace(input.ModelID) == "" {
		return fmt.Errorf("model_id is required")
	}
	if input.InputPriceMicro < 0 || input.OutputPriceMicro < 0 || input.CacheCreationPriceMicro < 0 || input.CacheReadPriceMicro < 0 {
		return fmt.Errorf("prices must be non-negative")
	}
	return nil
}
