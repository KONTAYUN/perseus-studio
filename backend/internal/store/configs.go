package store

import (
	"context"
	"database/sql"
	"time"
)

type ModelProvider struct {
	ID                          string          `json:"id"`
	UserID                      string          `json:"user_id"`
	Name                        string          `json:"name"`
	Visibility                  string          `json:"visibility"`
	Protocol                    string          `json:"protocol"`
	BaseURL                     string          `json:"base_url"`
	APIKey                      string          `json:"-"`
	MaskedKey                   string          `json:"masked_key"`
	ModelID                     string          `json:"model_id"`
	Enabled                     bool            `json:"enabled"`
	IsDefault                   bool            `json:"is_default"`
	ReasoningCapability         string          `json:"reasoning_capability"`
	ReasoningCapabilitySource   string          `json:"reasoning_capability_source,omitempty"`
	ReasoningProfileLevels      string          `json:"reasoning_profile_levels,omitempty"`
	ReasoningSupport            string          `json:"reasoning_support"`
	ThinkingProfile             ThinkingProfile `json:"thinking_profile"`
	ContextWindowTokens         int             `json:"context_window_tokens"`
	AutoCompactEnabled          bool            `json:"auto_compact_enabled"`
	AutoCompactThresholdPercent int             `json:"auto_compact_threshold_percent"`
	CompactReservedTokens       int             `json:"compact_reserved_tokens"`
	PruneToolOutputsEnabled     bool            `json:"prune_tool_outputs_enabled"`
	CreatedAt                   time.Time       `json:"created_at"`
	UpdatedAt                   time.Time       `json:"updated_at"`
}

type SearchProvider struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	Provider   string    `json:"provider"`
	Visibility string    `json:"visibility"`
	APIKey     string    `json:"-"`
	MaskedKey  string    `json:"masked_key"`
	Enabled    bool      `json:"enabled"`
	Priority   int       `json:"priority"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ModelProviderInput struct {
	Name                        string `json:"name"`
	Visibility                  string `json:"visibility"`
	Protocol                    string `json:"protocol"`
	BaseURL                     string `json:"base_url"`
	APIKey                      string `json:"api_key"`
	ModelID                     string `json:"model_id"`
	Enabled                     bool   `json:"enabled"`
	IsDefault                   bool   `json:"is_default"`
	ReasoningCapability         string `json:"reasoning_capability"`
	ReasoningCapabilitySource   string `json:"reasoning_capability_source"`
	ReasoningProfileLevels      string `json:"reasoning_profile_levels"`
	ContextWindowTokens         int    `json:"context_window_tokens"`
	AutoCompactEnabled          *bool  `json:"auto_compact_enabled"`
	AutoCompactThresholdPercent int    `json:"auto_compact_threshold_percent"`
	CompactReservedTokens       int    `json:"compact_reserved_tokens"`
	PruneToolOutputsEnabled     *bool  `json:"prune_tool_outputs_enabled"`
}

type SearchProviderInput struct {
	Provider   string `json:"provider"`
	Visibility string `json:"visibility"`
	APIKey     string `json:"api_key"`
	Enabled    bool   `json:"enabled"`
	Priority   int    `json:"priority"`
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
		if err := AddColumnIfNotExists(ctx, s.db, tbl, "visibility", "TEXT NOT NULL DEFAULT 'private'"); err != nil {
			return err
		}
	}
	modelProviderColumns := []struct {
		name string
		def  string
	}{
		{"context_window_tokens", "INTEGER NOT NULL DEFAULT 0"},
		{"auto_compact_enabled", "INTEGER NOT NULL DEFAULT 1"},
		{"auto_compact_threshold_percent", "INTEGER NOT NULL DEFAULT 80"},
		{"compact_reserved_tokens", "INTEGER NOT NULL DEFAULT 0"},
		{"prune_tool_outputs_enabled", "INTEGER NOT NULL DEFAULT 1"},
		{"reasoning_capability", "TEXT NOT NULL DEFAULT 'auto'"},
		{"reasoning_capability_source", "TEXT NOT NULL DEFAULT ''"},
		{"reasoning_profile_levels", "TEXT NOT NULL DEFAULT ''"},
	}
	for _, column := range modelProviderColumns {
		if err := AddColumnIfNotExists(ctx, s.db, "model_providers", column.name, column.def); err != nil {
			return err
		}
	}
	indexes := `
CREATE INDEX IF NOT EXISTS idx_model_providers_enabled_default ON model_providers(enabled, is_default);
CREATE INDEX IF NOT EXISTS idx_model_providers_user_id ON model_providers(user_id);
CREATE INDEX IF NOT EXISTS idx_model_providers_visibility_enabled ON model_providers(visibility, enabled);
CREATE INDEX IF NOT EXISTS idx_search_providers_priority ON search_providers(priority);
CREATE INDEX IF NOT EXISTS idx_search_providers_user_id ON search_providers(user_id);
CREATE INDEX IF NOT EXISTS idx_search_providers_visibility_enabled ON search_providers(visibility, enabled);
`
	_, err := s.db.ExecContext(ctx, indexes)
	return err
}
