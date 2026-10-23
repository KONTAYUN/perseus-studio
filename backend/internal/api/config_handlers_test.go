package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"webide-backend/internal/ai"
	"webide-backend/internal/store"
	"webide-backend/internal/usersvc"
)

type apiTestAIEnv struct {
	db            *sql.DB
	factory       *usersvc.Factory
	conversations *store.ConversationStore
	configs       *store.ConfigStore
	ai            *ai.Service
	admin         store.User
	user          store.User
}

func newAPITestAIEnv(t *testing.T) apiTestAIEnv {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	users := store.NewUserStore(db)
	if err := users.Migrate(t.Context()); err != nil {
		t.Fatalf("users.Migrate returned error: %v", err)
	}
	admin, err := users.CreateUser(t.Context(), "admin-ai", "password", "admin")
	if err != nil {
		t.Fatalf("CreateUser admin returned error: %v", err)
	}
	ordinary, err := users.CreateUser(t.Context(), "user-ai", "password", "user")
	if err != nil {
		t.Fatalf("CreateUser user returned error: %v", err)
	}
	configs := store.NewConfigStore(db)
	if err := configs.Migrate(t.Context()); err != nil {
		t.Fatalf("configs.Migrate returned error: %v", err)
	}
	skills := store.NewSkillStore(db)
	if err := skills.Migrate(t.Context()); err != nil {
		t.Fatalf("skills.Migrate returned error: %v", err)
	}
	factory := usersvc.NewFactory(db, filepath.Join(t.TempDir(), "users"), skills)
	conversations := store.NewConversationStore(db)
	if err := conversations.Migrate(t.Context()); err != nil {
		t.Fatalf("conversations.Migrate returned error: %v", err)
	}
	return apiTestAIEnv{db: db, factory: factory, conversations: conversations,
		configs: configs,
		ai:      ai.NewService(configs, factory, conversations),
		admin:   admin,
		user:    ordinary,
	}
}

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
		BaseURL:  "https://api.openai.com/v1",
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

func TestModelProviderReasoningAutoDoesNotUseOpenAITableForCompatibleProxy(t *testing.T) {
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
		Name:     "Proxy GPT",
		Protocol: "openai",
		BaseURL:  "https://api.jellix.top/v1",
		APIKey:   "key",
		ModelID:  "gpt-5.5",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("CreateModelProvider returned error: %v", err)
	}
	if item.ReasoningCapability != "auto" || item.ReasoningSupport != "none" {
		t.Fatalf("expected compatible proxy to avoid official OpenAI reasoning params unless discovered/explicit, got %#v", item)
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

func TestResourceVisibilityFiltersModelProvidersForOrdinaryUsers(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	users := store.NewUserStore(db)
	if err := users.Migrate(t.Context()); err != nil {
		t.Fatalf("users.Migrate returned error: %v", err)
	}
	admin, err := users.CreateUser(t.Context(), "admin", "password", "admin")
	if err != nil {
		t.Fatalf("CreateUser admin returned error: %v", err)
	}
	ordinary, err := users.CreateUser(t.Context(), "user", "password", "user")
	if err != nil {
		t.Fatalf("CreateUser user returned error: %v", err)
	}
	sessions := store.NewSessionStore(db)
	if err := sessions.Migrate(t.Context()); err != nil {
		t.Fatalf("sessions.Migrate returned error: %v", err)
	}
	configs := store.NewConfigStore(db)
	if err := configs.Migrate(t.Context()); err != nil {
		t.Fatalf("configs.Migrate returned error: %v", err)
	}
	skills := store.NewSkillStore(db)
	if err := skills.Migrate(t.Context()); err != nil {
		t.Fatalf("skills.Migrate returned error: %v", err)
	}
	if _, err := configs.CreateModelProvider(t.Context(), admin.ID, store.ModelProviderInput{
		Name:       "private",
		Visibility: store.ResourceVisibilityPrivate,
		Protocol:   "openai",
		BaseURL:    "https://example.com/v1",
		APIKey:     "private-key",
		ModelID:    "private-model",
		Enabled:    true,
	}); err != nil {
		t.Fatalf("CreateModelProvider private returned error: %v", err)
	}
	publicProvider, err := configs.CreateModelProvider(t.Context(), admin.ID, store.ModelProviderInput{
		Name:       "public",
		Visibility: store.ResourceVisibilityPublic,
		Protocol:   "openai",
		BaseURL:    "https://example.com/v1",
		APIKey:     "public-key",
		ModelID:    "public-model",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("CreateModelProvider public returned error: %v", err)
	}

	server := &Server{
		configs:  configs,
		factory:  usersvc.NewFactory(db, filepath.Join(t.TempDir(), "users"), skills),
		mux:      http.NewServeMux(),
		sessions: sessions,
		users:    users,
	}
	server.register()

	req := httptest.NewRequest(http.MethodGet, "/api/model-providers", nil)
	attachTestSession(t, server, req, "ordinary-token", ordinary)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if len(payload.Items) != 1 || payload.Items[0].ID != publicProvider.ID {
		t.Fatalf("expected ordinary user to see only public provider, got %#v", payload.Items)
	}
	for _, forbidden := range []string{"public-key", "private-key", "base_url", "masked_key", "user_id"} {
		if strings.Contains(rec.Body.String(), forbidden) {
			t.Fatalf("model provider response leaked %q: %s", forbidden, rec.Body.String())
		}
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/model-providers", strings.NewReader(`{}`))
	createReq.Header.Set("Content-Type", "application/json")
	attachTestSession(t, server, createReq, "ordinary-token-create", ordinary)
	createRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusForbidden {
		t.Fatalf("expected ordinary create 403, got %d: %s", createRec.Code, createRec.Body.String())
	}
}

func TestResourceVisibilityFiltersSearchAndSkillsForOrdinaryUsers(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	users := store.NewUserStore(db)
	if err := users.Migrate(t.Context()); err != nil {
		t.Fatalf("users.Migrate returned error: %v", err)
	}
	admin, err := users.CreateUser(t.Context(), "admin", "password", "admin")
	if err != nil {
		t.Fatalf("CreateUser admin returned error: %v", err)
	}
	ordinary, err := users.CreateUser(t.Context(), "user", "password", "user")
	if err != nil {
		t.Fatalf("CreateUser user returned error: %v", err)
	}
	sessions := store.NewSessionStore(db)
	if err := sessions.Migrate(t.Context()); err != nil {
		t.Fatalf("sessions.Migrate returned error: %v", err)
	}
	configs := store.NewConfigStore(db)
	if err := configs.Migrate(t.Context()); err != nil {
		t.Fatalf("configs.Migrate returned error: %v", err)
	}
	skillStore := store.NewSkillStore(db)
	if err := skillStore.Migrate(t.Context()); err != nil {
		t.Fatalf("skillStore.Migrate returned error: %v", err)
	}
	userDataRoot := filepath.Join(t.TempDir(), "users")
	adminSkillRoot := filepath.Join(userDataRoot, admin.ID, "skills")
	if _, err := configs.CreateSearchProvider(t.Context(), admin.ID, store.SearchProviderInput{
		Provider:   "tavily",
		Visibility: store.ResourceVisibilityPrivate,
		APIKey:     "private-search-key",
		Enabled:    true,
		Priority:   10,
	}); err != nil {
		t.Fatalf("CreateSearchProvider private returned error: %v", err)
	}
	publicSearch, err := configs.CreateSearchProvider(t.Context(), admin.ID, store.SearchProviderInput{
		Provider:   "brave",
		Visibility: store.ResourceVisibilityPublic,
		APIKey:     "public-search-key",
		Enabled:    true,
		Priority:   5,
	})
	if err != nil {
		t.Fatalf("CreateSearchProvider public returned error: %v", err)
	}
	if _, err := skillStore.UpsertSkill(t.Context(), store.UpsertSkillInput{
		ID:          "private-skill",
		UserID:      admin.ID,
		Name:        "Private Skill",
		Description: "private",
		Visibility:  store.ResourceVisibilityPrivate,
		InstallPath: filepath.Join(adminSkillRoot, "private-skill"),
		Enabled:     true,
	}); err != nil {
		t.Fatalf("UpsertSkill private returned error: %v", err)
	}
	publicSkill, err := skillStore.UpsertSkill(t.Context(), store.UpsertSkillInput{
		ID:          "public-skill",
		UserID:      admin.ID,
		Name:        "Public Skill",
		Description: "public",
		Visibility:  store.ResourceVisibilityPublic,
		InstallPath: filepath.Join(adminSkillRoot, "public-skill"),
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("UpsertSkill public returned error: %v", err)
	}
	if err := os.MkdirAll(publicSkill.InstallPath, 0o755); err != nil {
		t.Fatalf("MkdirAll public skill returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(publicSkill.InstallPath, "SKILL.md"), []byte("# Public Skill\n\nUse public instructions."), 0o644); err != nil {
		t.Fatalf("WriteFile public SKILL.md returned error: %v", err)
	}

	server := &Server{
		configs:  configs,
		factory:  usersvc.NewFactory(db, userDataRoot, skillStore),
		mux:      http.NewServeMux(),
		sessions: sessions,
		users:    users,
	}
	server.register()

	searchReq := httptest.NewRequest(http.MethodGet, "/api/search-providers", nil)
	attachTestSession(t, server, searchReq, "ordinary-search-token", ordinary)
	searchRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(searchRec, searchReq)
	if searchRec.Code != http.StatusOK {
		t.Fatalf("expected search list 200, got %d: %s", searchRec.Code, searchRec.Body.String())
	}
	var searchPayload struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(searchRec.Body.Bytes(), &searchPayload); err != nil {
		t.Fatalf("Unmarshal search returned error: %v", err)
	}
	if len(searchPayload.Items) != 1 || searchPayload.Items[0].ID != publicSearch.ID {
		t.Fatalf("expected ordinary user to see only public search provider, got %#v", searchPayload.Items)
	}
	for _, forbidden := range []string{"public-search-key", "private-search-key", "masked_key", "user_id"} {
		if strings.Contains(searchRec.Body.String(), forbidden) {
			t.Fatalf("search provider response leaked %q: %s", forbidden, searchRec.Body.String())
		}
	}

	skillReq := httptest.NewRequest(http.MethodGet, "/api/skills", nil)
	attachTestSession(t, server, skillReq, "ordinary-skill-token", ordinary)
	skillRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(skillRec, skillReq)
	if skillRec.Code != http.StatusOK {
		t.Fatalf("expected skills list 200, got %d: %s", skillRec.Code, skillRec.Body.String())
	}
	var skillPayload struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(skillRec.Body.Bytes(), &skillPayload); err != nil {
		t.Fatalf("Unmarshal skills returned error: %v", err)
	}
	if len(skillPayload.Items) != 1 || skillPayload.Items[0].ID != publicSkill.ID {
		t.Fatalf("expected ordinary user to see only public skill, got %#v", skillPayload.Items)
	}
	for _, forbidden := range []string{"private-skill", "install_path", "source_path", "user_id"} {
		if strings.Contains(skillRec.Body.String(), forbidden) {
			t.Fatalf("skill response leaked %q: %s", forbidden, skillRec.Body.String())
		}
	}

	readSkillReq := httptest.NewRequest(http.MethodGet, "/api/skills/"+publicSkill.ID+"/read", nil)
	attachTestSession(t, server, readSkillReq, "ordinary-read-public-skill-token", ordinary)
	readSkillRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(readSkillRec, readSkillReq)
	if readSkillRec.Code != http.StatusOK {
		t.Fatalf("expected ordinary user to read public skill, got %d: %s", readSkillRec.Code, readSkillRec.Body.String())
	}
	if !strings.Contains(readSkillRec.Body.String(), "Use public instructions.") {
		t.Fatalf("expected public skill content, got %s", readSkillRec.Body.String())
	}

	for _, item := range []struct {
		path string
		body string
	}{
		{"/api/search-providers", `{}`},
		{"/api/skills/upload", ``},
	} {
		createReq := httptest.NewRequest(http.MethodPost, item.path, strings.NewReader(item.body))
		createReq.Header.Set("Content-Type", "application/json")
		attachTestSession(t, server, createReq, "ordinary-token-create-"+item.path, ordinary)
		createRec := httptest.NewRecorder()
		server.Routes().ServeHTTP(createRec, createReq)
		if createRec.Code != http.StatusForbidden {
			t.Fatalf("expected ordinary create %s 403, got %d: %s", item.path, createRec.Code, createRec.Body.String())
		}
	}
}

func TestAIRejectsPrivateModelForOrdinaryUser(t *testing.T) {
	env := newAPITestAIEnv(t)
	ctx := context.Background()
	provider, err := env.configs.CreateModelProvider(ctx, env.admin.ID, store.ModelProviderInput{
		Name:       "private",
		Visibility: store.ResourceVisibilityPrivate,
		Protocol:   "openai",
		BaseURL:    "https://example.com/v1",
		APIKey:     "key",
		ModelID:    "model",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("CreateModelProvider returned error: %v", err)
	}
	_, err = env.ai.GenerateReply(ctx, "conv", "msg", provider.ID, []store.Message{{Role: "user", Content: "hi"}}, nil, ai.RunOptions{
		UserID:   env.user.ID,
		UserRole: "user",
	})
	if err == nil {
		t.Fatal("expected private model to be unavailable to ordinary user")
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
