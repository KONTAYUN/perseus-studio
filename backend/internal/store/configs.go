package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type ModelProvider struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Protocol  string    `json:"protocol"`
	BaseURL   string    `json:"base_url"`
	APIKey    string    `json:"-"`
	MaskedKey string    `json:"masked_key"`
	ModelID   string    `json:"model_id"`
	Enabled   bool      `json:"enabled"`
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SearchProvider struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Provider  string    `json:"provider"`
	APIKey    string    `json:"-"`
	MaskedKey string    `json:"masked_key"`
	Enabled   bool      `json:"enabled"`
	Priority  int       `json:"priority"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ModelProviderInput struct {
	Name      string `json:"name"`
	Protocol  string `json:"protocol"`
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
	ModelID   string `json:"model_id"`
	Enabled   bool   `json:"enabled"`
	IsDefault bool   `json:"is_default"`
}

type SearchProviderInput struct {
	Provider string `json:"provider"`
	APIKey   string `json:"api_key"`
	Enabled  bool   `json:"enabled"`
	Priority int    `json:"priority"`
}

type ConfigStore struct {
	db *sql.DB
}

func NewConfigStore(db *sql.DB) *ConfigStore {
	return &ConfigStore{db: db}
}

func (s *ConfigStore) Migrate(ctx context.Context) error {
	tables := `
CREATE TABLE IF NOT EXISTS model_providers (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  protocol TEXT NOT NULL,
  base_url TEXT NOT NULL,
  api_key TEXT NOT NULL,
  model_id TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  is_default INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS search_providers (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL DEFAULT '',
  provider TEXT NOT NULL,
  api_key TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  priority INTEGER NOT NULL DEFAULT 100,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
`
	if _, err := s.db.ExecContext(ctx, tables); err != nil {
		return err
	}
	for _, tbl := range []string{"model_providers", "search_providers"} {
		if err := AddColumnIfNotExists(ctx, s.db, tbl, "user_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
	}
	indexes := `
CREATE INDEX IF NOT EXISTS idx_model_providers_enabled_default ON model_providers(enabled, is_default);
CREATE INDEX IF NOT EXISTS idx_model_providers_user_id ON model_providers(user_id);
CREATE INDEX IF NOT EXISTS idx_search_providers_priority ON search_providers(priority);
CREATE INDEX IF NOT EXISTS idx_search_providers_user_id ON search_providers(user_id);
`
	_, err := s.db.ExecContext(ctx, indexes)
	return err
}

func (s *ConfigStore) ListModelProviders(ctx context.Context, userID string) ([]ModelProvider, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, name, protocol, base_url, api_key, model_id, enabled, is_default, created_at, updated_at
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

	_, err = tx.ExecContext(ctx, `
INSERT INTO model_providers (id, user_id, name, protocol, base_url, api_key, model_id, enabled, is_default, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, id, userID, strings.TrimSpace(input.Name), input.Protocol, strings.TrimSpace(input.BaseURL), input.APIKey, strings.TrimSpace(input.ModelID), boolToInt(input.Enabled), boolToInt(input.IsDefault), now, now)
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

	_, err = tx.ExecContext(ctx, `
UPDATE model_providers
SET name = ?, protocol = ?, base_url = ?, api_key = ?, model_id = ?, enabled = ?, is_default = ?, updated_at = ?
WHERE id = ? AND user_id = ?
`, strings.TrimSpace(input.Name), input.Protocol, strings.TrimSpace(input.BaseURL), apiKey, strings.TrimSpace(input.ModelID), boolToInt(input.Enabled), boolToInt(input.IsDefault), now, id, userID)
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
SELECT id, user_id, name, protocol, base_url, api_key, model_id, enabled, is_default, created_at, updated_at
FROM model_providers
WHERE id = ? AND user_id = ?
`, id, userID)
	return scanModelProvider(row)
}

func (s *ConfigStore) ListSearchProviders(ctx context.Context, userID string) ([]SearchProvider, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, provider, api_key, enabled, priority, created_at, updated_at
FROM search_providers
WHERE user_id = ?
ORDER BY priority ASC, updated_at DESC
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []SearchProvider
	for rows.Next() {
		item, err := scanSearchProvider(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *ConfigStore) CreateSearchProvider(ctx context.Context, userID string, input SearchProviderInput) (SearchProvider, error) {
	if err := validateSearchProvider(input, true); err != nil {
		return SearchProvider{}, err
	}
	now := time.Now().UTC()
	id := nextStoreID("sp", now)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO search_providers (id, user_id, provider, api_key, enabled, priority, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, id, userID, input.Provider, input.APIKey, boolToInt(input.Enabled), input.Priority, now, now)
	if err != nil {
		return SearchProvider{}, err
	}
	return s.GetSearchProvider(ctx, userID, id)
}

func (s *ConfigStore) UpdateSearchProvider(ctx context.Context, userID, id string, input SearchProviderInput) (SearchProvider, error) {
	if err := validateSearchProvider(input, false); err != nil {
		return SearchProvider{}, err
	}
	existing, err := s.GetSearchProvider(ctx, userID, id)
	if err != nil {
		return SearchProvider{}, err
	}
	apiKey := existing.APIKey
	if input.APIKey != "" {
		apiKey = input.APIKey
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
UPDATE search_providers
SET provider = ?, api_key = ?, enabled = ?, priority = ?, updated_at = ?
WHERE id = ? AND user_id = ?
`, input.Provider, apiKey, boolToInt(input.Enabled), input.Priority, now, id, userID)
	if err != nil {
		return SearchProvider{}, err
	}
	return s.GetSearchProvider(ctx, userID, id)
}

func (s *ConfigStore) DeleteSearchProvider(ctx context.Context, userID, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM search_providers WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

func (s *ConfigStore) GetSearchProvider(ctx context.Context, userID, id string) (SearchProvider, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, user_id, provider, api_key, enabled, priority, created_at, updated_at
FROM search_providers
WHERE id = ? AND user_id = ?
`, id, userID)
	return scanSearchProvider(row)
}

func validateModelProvider(input ModelProviderInput, requireKey bool) error {
	if strings.TrimSpace(input.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if input.Protocol != "openai" && input.Protocol != "anthropic" {
		return fmt.Errorf("protocol must be openai or anthropic")
	}
	if strings.TrimSpace(input.BaseURL) == "" {
		return fmt.Errorf("base_url is required")
	}
	if strings.TrimSpace(input.ModelID) == "" {
		return fmt.Errorf("model_id is required")
	}
	if requireKey && input.APIKey == "" {
		return fmt.Errorf("api_key is required")
	}
	return nil
}

func validateSearchProvider(input SearchProviderInput, requireKey bool) error {
	if input.Provider != "tavily" && input.Provider != "brave" {
		return fmt.Errorf("provider must be tavily or brave")
	}
	if requireKey && input.APIKey == "" {
		return fmt.Errorf("api_key is required")
	}
	return nil
}

func scanModelProvider(s scanner) (ModelProvider, error) {
	var item ModelProvider
	var enabled int
	var isDefault int
	err := s.Scan(&item.ID, &item.UserID, &item.Name, &item.Protocol, &item.BaseURL, &item.APIKey, &item.ModelID, &enabled, &isDefault, &item.CreatedAt, &item.UpdatedAt)
	item.Enabled = enabled == 1
	item.IsDefault = isDefault == 1
	item.MaskedKey = maskKey(item.APIKey)
	return item, err
}

func scanSearchProvider(s scanner) (SearchProvider, error) {
	var item SearchProvider
	var enabled int
	err := s.Scan(&item.ID, &item.UserID, &item.Provider, &item.APIKey, &enabled, &item.Priority, &item.CreatedAt, &item.UpdatedAt)
	item.Enabled = enabled == 1
	item.MaskedKey = maskKey(item.APIKey)
	return item, err
}

func maskKey(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "••••"
	}
	return value[:4] + "••••" + value[len(value)-4:]
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
