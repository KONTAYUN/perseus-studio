package api

import (
	"context"
	"testing"

	"webide-backend/internal/store"
)

func TestModelProviderContextSettingsRoundTrip(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	configs := store.NewConfigStore(db)
	ctx := context.Background()
	if err := configs.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	autoCompact := false
	pruneOutputs := false
	item, err := configs.CreateModelProvider(ctx, "u1", store.ModelProviderInput{
		Name:                        "Local",
		Protocol:                    "openai",
		BaseURL:                     "http://127.0.0.1",
		APIKey:                      "key",
		ModelID:                     "model",
		Enabled:                     true,
		ContextWindowTokens:         4096,
		AutoCompactEnabled:          &autoCompact,
		AutoCompactThresholdPercent: 75,
		CompactReservedTokens:       512,
		PruneToolOutputsEnabled:     &pruneOutputs,
	})
	if err != nil {
		t.Fatalf("CreateModelProvider returned error: %v", err)
	}
	if item.ContextWindowTokens != 4096 || item.AutoCompactEnabled || item.AutoCompactThresholdPercent != 75 || item.CompactReservedTokens != 512 || item.PruneToolOutputsEnabled {
		t.Fatalf("unexpected context settings: %#v", item)
	}
}

func TestModelProviderContextSettingsDefaults(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	configs := store.NewConfigStore(db)
	ctx := context.Background()
	if err := configs.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	item, err := configs.CreateModelProvider(ctx, "u1", store.ModelProviderInput{
		Name:     "Default",
		Protocol: "openai",
		BaseURL:  "http://127.0.0.1",
		APIKey:   "key",
		ModelID:  "model",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("CreateModelProvider returned error: %v", err)
	}
	if item.ContextWindowTokens != 0 || !item.AutoCompactEnabled || item.AutoCompactThresholdPercent != 80 || item.CompactReservedTokens != 0 || !item.PruneToolOutputsEnabled {
		t.Fatalf("unexpected default context settings: %#v", item)
	}
}

func TestParseDiscoveredModelsSupportsAnthropicDisplayName(t *testing.T) {
	models, err := parseDiscoveredModels([]byte(`{
		"data": [
			{"id": "claude-sonnet-4-5", "display_name": "Claude Sonnet 4.5"},
			{"id": "claude-opus-4-1", "display_name": "Claude Opus 4.1"}
		]
	}`))
	if err != nil {
		t.Fatalf("parseDiscoveredModels returned error: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %#v", models)
	}
	if models[0].ID != "claude-sonnet-4-5" || models[0].Name != "Claude Sonnet 4.5" {
		t.Fatalf("unexpected first model: %#v", models[0])
	}
}

func TestModelDiscoveryEndpointsForAnthropic(t *testing.T) {
	endpoints, headerMode, err := modelDiscoveryEndpoints("anthropic", "https://api.anthropic.com/v1")
	if err != nil {
		t.Fatalf("modelDiscoveryEndpoints returned error: %v", err)
	}
	if headerMode != "anthropic" || len(endpoints) != 1 || endpoints[0] != "https://api.anthropic.com/v1/models" {
		t.Fatalf("unexpected anthropic discovery config: endpoints=%#v headerMode=%s", endpoints, headerMode)
	}
}

func TestModelDiscoveryEndpointsForClaudeCodeProxy(t *testing.T) {
	endpoints, headerMode, err := modelDiscoveryEndpoints("anthropic", "https://example.com/claude-code-proxy")
	if err != nil {
		t.Fatalf("modelDiscoveryEndpoints returned error: %v", err)
	}
	if headerMode != "anthropic_bearer" {
		t.Fatalf("expected anthropic_bearer, got %s", headerMode)
	}
	if len(endpoints) != 2 || endpoints[0] != "https://example.com/claude-code-proxy/v1/models" || endpoints[1] != "https://example.com/claude-code-proxy/models" {
		t.Fatalf("unexpected claude code proxy endpoints: %#v", endpoints)
	}
}
