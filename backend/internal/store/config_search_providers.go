package store

import (
	"context"
	"time"
)

func (s *ConfigStore) ListSearchProviders(ctx context.Context, userID string) ([]SearchProvider, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, provider, visibility, api_key, enabled, priority, created_at, updated_at
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

func (s *ConfigStore) ListSearchProvidersForUser(ctx context.Context, userID, role string) ([]SearchProvider, error) {
	if role == "admin" {
		return s.ListAllSearchProviders(ctx)
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, provider, visibility, api_key, enabled, priority, created_at, updated_at
FROM search_providers
WHERE enabled = 1 AND visibility = ?
ORDER BY priority ASC, updated_at DESC
`, ResourceVisibilityPublic)
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

func (s *ConfigStore) ListAllSearchProviders(ctx context.Context) ([]SearchProvider, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, provider, visibility, api_key, enabled, priority, created_at, updated_at
FROM search_providers
ORDER BY priority ASC, updated_at DESC
`)
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
	visibility := NormalizeResourceVisibility(input.Visibility)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO search_providers (id, user_id, provider, visibility, api_key, enabled, priority, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
`, id, userID, input.Provider, visibility, input.APIKey, boolToInt(input.Enabled), input.Priority, now, now)
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
	visibility := NormalizeResourceVisibility(input.Visibility)
	_, err = s.db.ExecContext(ctx, `
UPDATE search_providers
SET provider = ?, visibility = ?, api_key = ?, enabled = ?, priority = ?, updated_at = ?
WHERE id = ? AND user_id = ?
`, input.Provider, visibility, apiKey, boolToInt(input.Enabled), input.Priority, now, id, userID)
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
SELECT id, user_id, provider, visibility, api_key, enabled, priority, created_at, updated_at
FROM search_providers
WHERE id = ? AND user_id = ?
`, id, userID)
	return scanSearchProvider(row)
}

func (s *ConfigStore) GetSearchProviderAny(ctx context.Context, id string) (SearchProvider, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, user_id, provider, visibility, api_key, enabled, priority, created_at, updated_at
FROM search_providers
WHERE id = ?
`, id)
	return scanSearchProvider(row)
}
