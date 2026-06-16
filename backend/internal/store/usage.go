package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

const (
	UsageSourceProviderReported = "provider_reported"
	UsageSourceProviderMissing  = "provider_missing"
	UsageStatusSucceeded        = "succeeded"
	UsageStatusFailed           = "failed"
)

type LLMUsageEvent struct {
	ID                  string    `json:"id"`
	UserID              string    `json:"user_id"`
	RunID               string    `json:"run_id,omitempty"`
	ConversationID      string    `json:"conversation_id,omitempty"`
	MessageID           string    `json:"message_id,omitempty"`
	TaskID              string    `json:"task_id,omitempty"`
	ModelProviderID     string    `json:"model_provider_id,omitempty"`
	ProviderName        string    `json:"provider_name,omitempty"`
	Protocol            string    `json:"protocol,omitempty"`
	ModelID             string    `json:"model_id,omitempty"`
	BaseURLHost         string    `json:"base_url_host,omitempty"`
	Stage               string    `json:"stage,omitempty"`
	RequestKind         string    `json:"request_kind,omitempty"`
	Stream              bool      `json:"stream"`
	Status              string    `json:"status"`
	ErrorClass          string    `json:"error_class,omitempty"`
	InputTokens         int64     `json:"input_tokens"`
	OutputTokens        int64     `json:"output_tokens"`
	CacheCreationTokens int64     `json:"cache_creation_tokens"`
	CacheReadTokens     int64     `json:"cache_read_tokens"`
	TotalTokens         int64     `json:"total_tokens"`
	UsageSource         string    `json:"usage_source"`
	EstimatedCostMicro  int64     `json:"estimated_cost_micro,omitempty"`
	Currency            string    `json:"currency,omitempty"`
	PricingSource       string    `json:"pricing_source,omitempty"`
	DurationMS          int64     `json:"duration_ms,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
}

type LLMUsageEventInput struct {
	UserID              string
	RunID               string
	ConversationID      string
	MessageID           string
	TaskID              string
	ModelProviderID     string
	ProviderName        string
	Protocol            string
	ModelID             string
	BaseURLHost         string
	Stage               string
	RequestKind         string
	Stream              bool
	Status              string
	ErrorClass          string
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	TotalTokens         int64
	UsageSource         string
	DurationMS          int64
}

type LLMModelPrice struct {
	ID                      string    `json:"id"`
	ModelProviderID         string    `json:"model_provider_id"`
	ModelID                 string    `json:"model_id"`
	Currency                string    `json:"currency"`
	InputPriceMicro         int64     `json:"input_price_micro"`
	OutputPriceMicro        int64     `json:"output_price_micro"`
	CacheCreationPriceMicro int64     `json:"cache_creation_price_micro"`
	CacheReadPriceMicro     int64     `json:"cache_read_price_micro"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

type LLMModelPriceInput struct {
	ModelProviderID         string `json:"model_provider_id"`
	ModelID                 string `json:"model_id"`
	Currency                string `json:"currency"`
	InputPriceMicro         int64  `json:"input_price_micro"`
	OutputPriceMicro        int64  `json:"output_price_micro"`
	CacheCreationPriceMicro int64  `json:"cache_creation_price_micro"`
	CacheReadPriceMicro     int64  `json:"cache_read_price_micro"`
}

type UsageQuery struct {
	From            time.Time
	To              time.Time
	UserID          string
	ModelProviderID string
	ModelID         string
	Stage           string
	Limit           int
	Offset          int
}

type UsageSummary struct {
	Totals  UsageAggregate        `json:"totals"`
	ByUser  []UsageUserAggregate  `json:"by_user"`
	ByModel []UsageModelAggregate `json:"by_model"`
}

type UsageAggregate struct {
	RequestCount        int64  `json:"request_count"`
	SuccessCount        int64  `json:"success_count"`
	FailedCount         int64  `json:"failed_count"`
	InputTokens         int64  `json:"input_tokens"`
	OutputTokens        int64  `json:"output_tokens"`
	CacheCreationTokens int64  `json:"cache_creation_tokens"`
	CacheReadTokens     int64  `json:"cache_read_tokens"`
	TotalTokens         int64  `json:"total_tokens"`
	EstimatedCostMicro  int64  `json:"estimated_cost_micro"`
	Currency            string `json:"currency,omitempty"`
}

type UsageUserAggregate struct {
	UserID   string `json:"user_id"`
	Username string `json:"username,omitempty"`
	Role     string `json:"role,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
	UsageAggregate
}

type UsageModelAggregate struct {
	ModelProviderID string `json:"model_provider_id"`
	ProviderName    string `json:"provider_name,omitempty"`
	Protocol        string `json:"protocol,omitempty"`
	ModelID         string `json:"model_id,omitempty"`
	UsageAggregate
}

type UsageStore struct {
	db *sql.DB
}

func NewUsageStore(db *sql.DB) *UsageStore {
	return &UsageStore{db: db}
}

func (s *UsageStore) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS llm_usage_events (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  run_id TEXT NOT NULL DEFAULT '',
  conversation_id TEXT NOT NULL DEFAULT '',
  message_id TEXT NOT NULL DEFAULT '',
  task_id TEXT NOT NULL DEFAULT '',
  model_provider_id TEXT NOT NULL DEFAULT '',
  provider_name TEXT NOT NULL DEFAULT '',
  protocol TEXT NOT NULL DEFAULT '',
  model_id TEXT NOT NULL DEFAULT '',
  base_url_host TEXT NOT NULL DEFAULT '',
  stage TEXT NOT NULL DEFAULT '',
  request_kind TEXT NOT NULL DEFAULT '',
  stream INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL,
  error_class TEXT NOT NULL DEFAULT '',
  input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
  cache_read_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  usage_source TEXT NOT NULL DEFAULT 'provider_missing',
  estimated_cost_micro INTEGER NOT NULL DEFAULT 0,
  currency TEXT NOT NULL DEFAULT '',
  pricing_source TEXT NOT NULL DEFAULT '',
  duration_ms INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_llm_usage_events_created ON llm_usage_events(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_llm_usage_events_user_created ON llm_usage_events(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_llm_usage_events_model_created ON llm_usage_events(model_provider_id, model_id, created_at DESC);

CREATE TABLE IF NOT EXISTS llm_model_prices (
  id TEXT PRIMARY KEY,
  model_provider_id TEXT NOT NULL,
  model_id TEXT NOT NULL,
  currency TEXT NOT NULL DEFAULT 'USD',
  input_price_micro INTEGER NOT NULL DEFAULT 0,
  output_price_micro INTEGER NOT NULL DEFAULT 0,
  cache_creation_price_micro INTEGER NOT NULL DEFAULT 0,
  cache_read_price_micro INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_model_prices_provider_model ON llm_model_prices(model_provider_id, model_id);
`)
	return err
}

func estimateUsageCostMicro(event LLMUsageEvent, price LLMModelPrice) int64 {
	return event.InputTokens*price.InputPriceMicro/1_000_000 +
		event.OutputTokens*price.OutputPriceMicro/1_000_000 +
		event.CacheCreationTokens*price.CacheCreationPriceMicro/1_000_000 +
		event.CacheReadTokens*price.CacheReadPriceMicro/1_000_000
}

func normalizeUsageStatus(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), UsageStatusSucceeded) {
		return UsageStatusSucceeded
	}
	return UsageStatusFailed
}

func normalizeUsageSource(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), UsageSourceProviderReported) {
		return UsageSourceProviderReported
	}
	return UsageSourceProviderMissing
}

func normalizeCurrency(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return "USD"
	}
	if len(value) > 12 {
		return value[:12]
	}
	return value
}

func normalizeUsageLimit(limit, fallback int) int {
	if limit <= 0 {
		limit = fallback
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func trimUsageField(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

func maxInt64(minimum, value int64) int64 {
	if value < minimum {
		return minimum
	}
	return value
}
