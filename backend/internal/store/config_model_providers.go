package store

import (
	"context"
	"strings"
	"time"
)

func (s *ConfigStore) ListModelProviders(ctx context.Context, userID string) ([]ModelProvider, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, name, visibility, protocol, adapter_mode, auth_mode, supports_tools, supports_vision, capability_source, base_url, api_key, model_id, enabled, is_default,
       context_window_tokens, auto_compact_enabled, auto_compact_threshold_percent, compact_reserved_tokens, prune_tool_outputs_enabled,
       native_patch_tool_enabled,
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

func (s *ConfigStore) ListModelProvidersForUser(ctx context.Context, userID, role string) ([]ModelProvider, error) {
	if role == "admin" {
		return s.ListAllModelProviders(ctx)
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, name, visibility, protocol, adapter_mode, auth_mode, supports_tools, supports_vision, capability_source, base_url, api_key, model_id, enabled, is_default,
       context_window_tokens, auto_compact_enabled, auto_compact_threshold_percent, compact_reserved_tokens, prune_tool_outputs_enabled,
       native_patch_tool_enabled,
       reasoning_capability, reasoning_capability_source, reasoning_profile_levels,
       created_at, updated_at
FROM model_providers
WHERE enabled = 1 AND visibility = ?
ORDER BY is_default DESC, updated_at DESC
`, ResourceVisibilityPublic)
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

func (s *ConfigStore) ListAllModelProviders(ctx context.Context) ([]ModelProvider, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, name, visibility, protocol, adapter_mode, auth_mode, supports_tools, supports_vision, capability_source, base_url, api_key, model_id, enabled, is_default,
       context_window_tokens, auto_compact_enabled, auto_compact_threshold_percent, compact_reserved_tokens, prune_tool_outputs_enabled,
       native_patch_tool_enabled,
       reasoning_capability, reasoning_capability_source, reasoning_profile_levels,
       created_at, updated_at
FROM model_providers
ORDER BY is_default DESC, updated_at DESC
`)
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
		if _, err := tx.ExecContext(ctx, `UPDATE model_providers SET is_default = 0, updated_at = ?`, now); err != nil {
			return ModelProvider{}, err
		}
	} else if !input.Enabled {
		input.IsDefault = false
	}

	settings := normalizeModelProviderContextSettings(input)
	visibility := NormalizeResourceVisibility(input.Visibility)
	adapterMode := normalizeModelAdapterMode(input.AdapterMode, input.Protocol)
	authMode := normalizeModelAuthMode(input.AuthMode, adapterMode)
	capabilitySource := strings.TrimSpace(input.CapabilitySource)
	if capabilitySource == "" {
		capabilitySource = "user"
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO model_providers (
  id, user_id, name, visibility, protocol, adapter_mode, auth_mode, supports_tools, supports_vision, capability_source, base_url, api_key, model_id, enabled, is_default,
  context_window_tokens, auto_compact_enabled, auto_compact_threshold_percent, compact_reserved_tokens, prune_tool_outputs_enabled, native_patch_tool_enabled, reasoning_capability, reasoning_capability_source, reasoning_profile_levels,
  created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, id, userID, strings.TrimSpace(input.Name), visibility, input.Protocol, adapterMode, authMode, normalizeModelCapability(input.SupportsTools), normalizeModelCapability(input.SupportsVision), capabilitySource, strings.TrimSpace(input.BaseURL), input.APIKey, strings.TrimSpace(input.ModelID), boolToInt(input.Enabled), boolToInt(input.IsDefault),
		settings.ContextWindowTokens, boolToInt(settings.AutoCompactEnabled), settings.AutoCompactThresholdPercent, settings.CompactReservedTokens, boolToInt(settings.PruneToolOutputsEnabled), boolToInt(settings.NativePatchToolEnabled), NormalizeReasoningCapability(input.ReasoningCapability), NormalizeReasoningCapabilitySource(input.ReasoningCapabilitySource), NormalizeReasoningProfileLevels(input.ReasoningProfileLevels), now, now)
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
		if _, err := tx.ExecContext(ctx, `UPDATE model_providers SET is_default = 0, updated_at = ?`, now); err != nil {
			return ModelProvider{}, err
		}
	} else if !input.Enabled {
		input.IsDefault = false
	}

	settings := normalizeModelProviderContextSettings(input)
	visibility := NormalizeResourceVisibility(input.Visibility)
	adapterMode := normalizeModelAdapterMode(input.AdapterMode, input.Protocol)
	authMode := normalizeModelAuthMode(input.AuthMode, adapterMode)
	capabilitySource := strings.TrimSpace(input.CapabilitySource)
	if capabilitySource == "" {
		capabilitySource = "user"
	}
	_, err = tx.ExecContext(ctx, `
UPDATE model_providers
SET name = ?, visibility = ?, protocol = ?, adapter_mode = ?, auth_mode = ?, supports_tools = ?, supports_vision = ?, capability_source = ?, base_url = ?, api_key = ?, model_id = ?, enabled = ?, is_default = ?,
    context_window_tokens = ?, auto_compact_enabled = ?, auto_compact_threshold_percent = ?, compact_reserved_tokens = ?, prune_tool_outputs_enabled = ?, native_patch_tool_enabled = ?, reasoning_capability = ?, reasoning_capability_source = ?, reasoning_profile_levels = ?,
    updated_at = ?
WHERE id = ? AND user_id = ?
`, strings.TrimSpace(input.Name), visibility, input.Protocol, adapterMode, authMode, normalizeModelCapability(input.SupportsTools), normalizeModelCapability(input.SupportsVision), capabilitySource, strings.TrimSpace(input.BaseURL), apiKey, strings.TrimSpace(input.ModelID), boolToInt(input.Enabled), boolToInt(input.IsDefault),
		settings.ContextWindowTokens, boolToInt(settings.AutoCompactEnabled), settings.AutoCompactThresholdPercent, settings.CompactReservedTokens, boolToInt(settings.PruneToolOutputsEnabled), boolToInt(settings.NativePatchToolEnabled),
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

func (s *ConfigStore) SetDefaultModelProviderAny(ctx context.Context, id string) (ModelProvider, error) {
	if _, err := s.GetModelProviderAny(ctx, id); err != nil {
		return ModelProvider{}, err
	}
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ModelProvider{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE model_providers SET is_default = 0, updated_at = ?`, now); err != nil {
		return ModelProvider{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE model_providers SET is_default = 1, enabled = 1, updated_at = ? WHERE id = ?`, now, id); err != nil {
		return ModelProvider{}, err
	}
	if err := tx.Commit(); err != nil {
		return ModelProvider{}, err
	}
	return s.GetModelProviderAny(ctx, id)
}

func (s *ConfigStore) GetModelProvider(ctx context.Context, userID, id string) (ModelProvider, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, user_id, name, visibility, protocol, adapter_mode, auth_mode, supports_tools, supports_vision, capability_source, base_url, api_key, model_id, enabled, is_default,
       context_window_tokens, auto_compact_enabled, auto_compact_threshold_percent, compact_reserved_tokens, prune_tool_outputs_enabled,
       native_patch_tool_enabled,
       reasoning_capability, reasoning_capability_source, reasoning_profile_levels,
       created_at, updated_at
FROM model_providers
WHERE id = ? AND user_id = ?
`, id, userID)
	return scanModelProvider(row)
}

func (s *ConfigStore) GetModelProviderForUser(ctx context.Context, userID, role, id string) (ModelProvider, error) {
	if role == "admin" {
		return s.GetModelProviderAny(ctx, id)
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, user_id, name, visibility, protocol, adapter_mode, auth_mode, supports_tools, supports_vision, capability_source, base_url, api_key, model_id, enabled, is_default,
       context_window_tokens, auto_compact_enabled, auto_compact_threshold_percent, compact_reserved_tokens, prune_tool_outputs_enabled,
       native_patch_tool_enabled,
       reasoning_capability, reasoning_capability_source, reasoning_profile_levels,
       created_at, updated_at
FROM model_providers
WHERE id = ? AND enabled = 1 AND visibility = ?
`, id, ResourceVisibilityPublic)
	return scanModelProvider(row)
}

func (s *ConfigStore) GetModelProviderAny(ctx context.Context, id string) (ModelProvider, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, user_id, name, visibility, protocol, adapter_mode, auth_mode, supports_tools, supports_vision, capability_source, base_url, api_key, model_id, enabled, is_default,
       context_window_tokens, auto_compact_enabled, auto_compact_threshold_percent, compact_reserved_tokens, prune_tool_outputs_enabled,
       native_patch_tool_enabled,
       reasoning_capability, reasoning_capability_source, reasoning_profile_levels,
       created_at, updated_at
FROM model_providers
WHERE id = ?
`, id)
	return scanModelProvider(row)
}
