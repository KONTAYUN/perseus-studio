package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"webide-backend/internal/store"
)

func TestModelCreateReusesStoredCredentialWithoutReturningIt(t *testing.T) {
	env := newAPITestAIEnv(t)
	input := store.ModelProviderInput{Name: "original", Protocol: "openai", BaseURL: "http://example.invalid/v1", APIKey: "test-source-credential", ModelID: "original", Enabled: true, IsDefault: true}
	source, err := env.configs.CreateModelProvider(t.Context(), env.admin.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{configs: env.configs}
	for _, model := range []string{"new-one", "new-two"} {
		body, _ := json.Marshal(map[string]any{"credential_provider_id": source.ID, "name": model, "model_id": model, "protocol": "openai", "base_url": source.BaseURL, "enabled": true})
		req := httptest.NewRequest(http.MethodPost, "/api/model-providers", strings.NewReader(string(body)))
		req = req.WithContext(context.WithValue(req.Context(), ctxSessionKey, &SessionInfo{UserID: env.admin.ID, Role: "admin"}))
		rec := httptest.NewRecorder()
		server.handleModelProviders(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create failed: %d %s", rec.Code, rec.Body.String())
		}
		var created store.ModelProvider
		if err = json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(rec.Body.String(), input.APIKey) || strings.Contains(rec.Body.String(), `"api_key"`) {
			t.Fatal("credential leaked")
		}
		stored, err := env.configs.GetModelProvider(t.Context(), env.admin.ID, created.ID)
		if err != nil || stored.APIKey != input.APIKey || stored.ModelID != model || stored.IsDefault {
			t.Fatal("incorrect new model configuration")
		}
	}
	original, err := env.configs.GetModelProvider(t.Context(), env.admin.ID, source.ID)
	if err != nil || !original.IsDefault || original.ModelID != "original" {
		t.Fatal("source/default changed")
	}
}

func TestModelCredentialReuseRejectsUnauthorizedAndMissingSource(t *testing.T) {
	for _, tc := range []struct {
		role string
		want int
	}{{"user", http.StatusForbidden}, {"admin", http.StatusBadRequest}} {
		t.Run(tc.role, func(t *testing.T) {
			env := newAPITestAIEnv(t)
			server := &Server{configs: env.configs}
			req := httptest.NewRequest(http.MethodPost, "/api/model-providers", strings.NewReader(`{"credential_provider_id":"missing","name":"new","model_id":"new","protocol":"openai","base_url":"http://example.invalid","enabled":true}`))
			req = req.WithContext(context.WithValue(req.Context(), ctxSessionKey, &SessionInfo{UserID: env.user.ID, Role: tc.role}))
			rec := httptest.NewRecorder()
			server.handleModelProviders(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("unexpected status %d", rec.Code)
			}
			items, err := env.configs.ListAllModelProviders(t.Context())
			if err != nil || len(items) != 0 {
				t.Fatal("failed create changed data")
			}
		})
	}
}
