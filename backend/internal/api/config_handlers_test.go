package api

import "testing"

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
