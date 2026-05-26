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

func TestModelProviderReasoningCapabilityRoundTrip(t *testing.T) {
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
		Name:                "GPT 5",
		Protocol:            "openai",
		BaseURL:             "http://127.0.0.1",
		APIKey:              "key",
		ModelID:             "domestic-chat",
		Enabled:             true,
		ReasoningCapability: "openai_reasoning",
	})
	if err != nil {
		t.Fatalf("CreateModelProvider returned error: %v", err)
	}
	if item.ReasoningCapability != "openai_reasoning" || item.ReasoningSupport != "openai" || item.ThinkingProfile.WireFamily != store.ThinkingWireFamilyOpenAIReasoning {
		t.Fatalf("unexpected reasoning config: %#v", item)
	}
}

func TestModelProviderReasoningCapabilityMustMatchProtocol(t *testing.T) {
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
		Name:                "Mismatched",
		Protocol:            "anthropic",
		BaseURL:             "https://api.anthropic.com/v1",
		APIKey:              "key",
		ModelID:             "claude-opus-4-6",
		Enabled:             true,
		ReasoningCapability: "openai_reasoning",
	})
	if err != nil {
		t.Fatalf("CreateModelProvider returned error: %v", err)
	}
	if item.ReasoningSupport != "none" {
		t.Fatalf("expected mismatched protocol/capability to disable support, got %#v", item)
	}
}

func TestModelProviderReasoningAutoIsConservative(t *testing.T) {
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
		Name:     "国产模型",
		Protocol: "openai",
		BaseURL:  "http://127.0.0.1",
		APIKey:   "key",
		ModelID:  "qwen-plus",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("CreateModelProvider returned error: %v", err)
	}
	if item.ReasoningCapability != "auto" || item.ReasoningSupport != "none" {
		t.Fatalf("expected unknown model to disable native reasoning, got %#v", item)
	}
}

func TestModelProviderReasoningAutoUsesSourceBackedOpenAIModels(t *testing.T) {
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
		Name:     "OpenAI GPT",
		Protocol: "openai",
		BaseURL:  "https://example.com/v1",
		APIKey:   "key",
		ModelID:  "gpt-5.4",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("CreateModelProvider returned error: %v", err)
	}
	if item.ReasoningCapability != "auto" || item.ReasoningSupport != "openai" || item.ThinkingProfile.Source != store.ThinkingProfileSourceModelsDev {
		t.Fatalf("expected source-backed OpenAI model to support native reasoning, got %#v", item)
	}
}

func TestModelProviderReasoningAutoRejectsUnsupportedThinkingNames(t *testing.T) {
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
		Name:     "Thinking Name",
		Protocol: "openai",
		BaseURL:  "https://example.com/v1",
		APIKey:   "key",
		ModelID:  "qwen3-235b-a22b-thinking",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("CreateModelProvider returned error: %v", err)
	}
	if item.ReasoningSupport != "none" {
		t.Fatalf("expected unsupported thinking-like model name to avoid guessed reasoning, got %#v", item)
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

func TestParseDiscoveredModelsIgnoresBroadReasoningMetadata(t *testing.T) {
	models, err := parseDiscoveredModelsForRequest([]byte(`{
		"data": [
			{"id": "provider/reasoner", "name": "Reasoner", "model_info": {"supports_reasoning": true}},
			{"id": "provider/chat", "name": "Chat", "model_info": {"supports_reasoning": false}}
		]
	}`), discoverModelsRequest{
		Protocol: "openai",
		BaseURL:  "https://proxy.example/v1",
	})
	if err != nil {
		t.Fatalf("parseDiscoveredModelsForRequest returned error: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %#v", models)
	}
	if models[0].ReasoningCapability != "" || models[0].ReasoningSupport != "" {
		t.Fatalf("expected broad reasoning flag not to enable effort controls, got %#v", models[0])
	}
	if models[1].ReasoningCapability != "" || models[1].ReasoningSupport != "" {
		t.Fatalf("expected unsupported model to omit reasoning metadata, got %#v", models[1])
	}
}

func TestParseDiscoveredModelsUsesSupportedParametersMetadata(t *testing.T) {
	models, err := parseDiscoveredModelsForRequest([]byte(`{
		"data": [
			{"id": "provider/gpt", "name": "GPT", "supported_parameters": ["tools", "reasoning_effort"]}
		]
	}`), discoverModelsRequest{
		Protocol: "openai",
		BaseURL:  "https://proxy.example/v1",
	})
	if err != nil {
		t.Fatalf("parseDiscoveredModelsForRequest returned error: %v", err)
	}
	if len(models) != 1 || models[0].ReasoningCapability != "openai_reasoning" || models[0].ReasoningCapabilitySource != store.ReasoningCapabilitySourceProviderDiscovery || models[0].ReasoningSupport != "openai" {
		t.Fatalf("expected supported_parameters-backed reasoning support, got %#v", models)
	}
}

func TestParseDiscoveredModelsUsesReasoningEffortOptions(t *testing.T) {
	models, err := parseDiscoveredModelsForRequest([]byte(`{
		"data": [
			{"id": "provider/gpt", "name": "GPT", "model_info": {"reasoning_effort_options": ["low", "medium", "high"]}}
		]
	}`), discoverModelsRequest{
		Protocol: "openai",
		BaseURL:  "https://proxy.example/v1",
	})
	if err != nil {
		t.Fatalf("parseDiscoveredModelsForRequest returned error: %v", err)
	}
	if len(models) != 1 || models[0].ReasoningCapability != "openai_reasoning" || models[0].ThinkingProfile.WireFamily != store.ThinkingWireFamilyOpenAIReasoning {
		t.Fatalf("expected effort-options-backed reasoning support, got %#v", models)
	}
	if models[0].ReasoningProfileLevels != "light,standard,deep" || len(models[0].ThinkingProfile.Levels) != 3 || models[0].ThinkingProfile.Levels[2] != "deep" {
		t.Fatalf("expected provider effort options to drive exact profile levels, got %#v", models[0])
	}
}

func TestParseDiscoveredModelsUsesVariableReasoningEffortOptions(t *testing.T) {
	models, err := parseDiscoveredModelsForRequest([]byte(`{
		"data": [
			{"id": "provider/gpt", "name": "GPT", "model_info": {"reasoning_effort_options": ["minimal", "low", "medium", "high", "xhigh"]}}
		]
	}`), discoverModelsRequest{
		Protocol: "openai",
		BaseURL:  "https://proxy.example/v1",
	})
	if err != nil {
		t.Fatalf("parseDiscoveredModelsForRequest returned error: %v", err)
	}
	if len(models) != 1 || models[0].ReasoningProfileLevels != "light,standard,deep,expert" {
		t.Fatalf("expected variable effort options to be preserved, got %#v", models)
	}
	if len(models[0].ThinkingProfile.Levels) != 4 || models[0].ThinkingProfile.Levels[0] != "light" || models[0].ThinkingProfile.Levels[3] != "expert" {
		t.Fatalf("expected thinking profile to mirror provider options, got %#v", models[0].ThinkingProfile)
	}
}

func TestParseDiscoveredModelsDoesNotTreatThinkingParameterAsOpenAIReasoning(t *testing.T) {
	models, err := parseDiscoveredModelsForRequest([]byte(`{
		"data": [
			{"id": "provider/chat", "name": "Chat", "supported_parameters": ["tools", "thinking"]}
		]
	}`), discoverModelsRequest{
		Protocol: "openai",
		BaseURL:  "https://proxy.example/v1",
	})
	if err != nil {
		t.Fatalf("parseDiscoveredModelsForRequest returned error: %v", err)
	}
	if len(models) != 1 || models[0].ReasoningCapability != "" || models[0].ReasoningSupport != "" {
		t.Fatalf("expected generic thinking parameter not to enable controls, got %#v", models)
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
