package store

import (
	"context"
	"strings"
	"time"
)

func (s *ConfigStore) ListModelProviders(ctx context.Context, userID string) ([]ModelProvider, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, name, protocol, base_url, api_key, model_id, enabled, is_default,
       context_window_tokens, auto_compact_enabled, auto_compact_threshold_percent, compact_reserved_tokens, prune_tool_outputs_enabled,
       reasoning_capability, reasoning_capability_source, reasoning_profile_levels,
       created_at, updated_at
FROM model_providers
WHERE user_id = ?
ORDER BY is_default DESC, updated_at DESC
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ModelProvider
	for rows.Next() {
		item, err := scanModelProvider(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *ConfigStore) CreateModelProvider(ctx context.Context, userID string, input ModelProviderInput) (ModelProvider, error) {
	if err := validateModelProvider(input, true); err != nil {
		return ModelProvider{}, err
	}
	now := time.Now().UTC()
	id := nextStoreID("mp", now)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ModelProvider{}, err
	}
	defer tx.Rollback()

	if input.IsDefault {
		if _, err := tx.ExecContext(ctx, `UPDATE model_providers SET is_default = 0, updated_at = ? WHERE user_id = ?`, now, userID); err != nil {
			return ModelProvider{}, err
		}
	}

	settings := normalizeModelProviderContextSettings(input)
	_, err = tx.ExecContext(ctx, `
INSERT INTO model_providers (
  id, user_id, name, protocol, base_url, api_key, model_id, enabled, is_default,
  context_window_tokens, auto_compact_enabled, auto_compact_threshold_percent, compact_reserved_tokens, prune_tool_outputs_enabled, reasoning_capability, reasoning_capability_source, reasoning_profile_levels,
  created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, id, userID, strings.TrimSpace(input.Name), input.Protocol, strings.TrimSpace(input.BaseURL), input.APIKey, strings.TrimSpace(input.ModelID), boolToInt(input.Enabled), boolToInt(input.IsDefault),
		settings.ContextWindowTokens, boolToInt(settings.AutoCompactEnabled), settings.AutoCompactThresholdPercent, settings.CompactReservedTokens, boolToInt(settings.PruneToolOutputsEnabled), NormalizeReasoningCapability(input.ReasoningCapability), NormalizeReasoningCapabilitySource(input.ReasoningCapabilitySource), NormalizeReasoningProfileLevels(input.ReasoningProfileLevels), now, now)
	if err != nil {
		return ModelProvider{}, err
	}
	if err := tx.Commit(); err != nil {
		return ModelProvider{}, err
	}
	return s.GetModelProvider(ctx, userID, id)
}

func (s *ConfigStore) UpdateModelProvider(ctx context.Context, userID, id string, input ModelProviderInput) (ModelProvider, error) {
	if err := validateModelProvider(input, false); err != nil {
		return ModelProvider{}, err
	}
	existing, err := s.GetModelProvider(ctx, userID, id)
	if err != nil {
		return ModelProvider{}, err
	}
	apiKey := existing.APIKey
	if input.APIKey != "" {
		apiKey = input.APIKey
	}
	now := time.Now().UTC()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ModelProvider{}, err
	}
	defer tx.Rollback()

	if input.IsDefault {
		if _, err := tx.ExecContext(ctx, `UPDATE model_providers SET is_default = 0, updated_at = ? WHERE user_id = ?`, now, userID); err != nil {
			return ModelProvider{}, err
		}
	}

	settings := normalizeModelProviderContextSettings(input)
	_, err = tx.ExecContext(ctx, `
UPDATE model_providers
SET name = ?, protocol = ?, base_url = ?, api_key = ?, model_id = ?, enabled = ?, is_default = ?,
    context_window_tokens = ?, auto_compact_enabled = ?, auto_compact_threshold_percent = ?, compact_reserved_tokens = ?, prune_tool_outputs_enabled = ?, reasoning_capability = ?, reasoning_capability_source = ?, reasoning_profile_levels = ?,
    updated_at = ?
WHERE id = ? AND user_id = ?
`, strings.TrimSpace(input.Name), input.Protocol, strings.TrimSpace(input.BaseURL), apiKey, strings.TrimSpace(input.ModelID), boolToInt(input.Enabled), boolToInt(input.IsDefault),
		settings.ContextWindowTokens, boolToInt(settings.AutoCompactEnabled), settings.AutoCompactThresholdPercent, settings.CompactReservedTokens, boolToInt(settings.PruneToolOutputsEnabled),
		NormalizeReasoningCapability(input.ReasoningCapability), NormalizeReasoningCapabilitySource(input.ReasoningCapabilitySource), NormalizeReasoningProfileLevels(input.ReasoningProfileLevels), now, id, userID)
	if err != nil {
		return ModelProvider{}, err
	}
	if err := tx.Commit(); err != nil {
		return ModelProvider{}, err
	}
	return s.GetModelProvider(ctx, userID, id)
}

func (s *ConfigStore) DeleteModelProvider(ctx context.Context, userID, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM model_providers WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

func (s *ConfigStore) SetDefaultModelProvider(ctx context.Context, userID, id string) (ModelProvider, error) {
	if _, err := s.GetModelProvider(ctx, userID, id); err != nil {
		return ModelProvider{}, err
	}
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ModelProvider{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE model_providers SET is_default = 0, updated_at = ? WHERE user_id = ?`, now, userID); err != nil {
		return ModelProvider{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE model_providers SET is_default = 1, enabled = 1, updated_at = ? WHERE id = ? AND user_id = ?`, now, id, userID); err != nil {
		return ModelProvider{}, err
	}
	if err := tx.Commit(); err != nil {
		return ModelProvider{}, err
	}
	return s.GetModelProvider(ctx, userID, id)
}

func (s *ConfigStore) GetModelProvider(ctx context.Context, userID, id string) (ModelProvider, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, user_id, name, protocol, base_url, api_key, model_id, enabled, is_default,
       context_window_tokens, auto_compact_enabled, auto_compact_threshold_percent, compact_reserved_tokens, prune_tool_outputs_enabled,
       reasoning_capability, reasoning_capability_source, reasoning_profile_levels,
       created_at, updated_at
FROM model_providers
WHERE id = ? AND user_id = ?
`, id, userID)
	return scanModelProvider(row)
}
