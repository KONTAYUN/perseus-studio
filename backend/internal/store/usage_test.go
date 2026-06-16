package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestUsageStoreAggregatesAndEstimatesCost(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()

	users := NewUserStore(db)
	if err := users.Migrate(ctx); err != nil {
		t.Fatalf("users.Migrate returned error: %v", err)
	}
	user, err := users.CreateUser(ctx, "usage-user", "password", "user")
	if err != nil {
		t.Fatalf("CreateUser returned error: %v", err)
	}
	store := NewUsageStore(db)
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("usage.Migrate returned error: %v", err)
	}
	if _, err := store.CreatePrice(ctx, LLMModelPriceInput{
		ModelProviderID:  "mp1",
		ModelID:          "model-a",
		Currency:         "USD",
		InputPriceMicro:  2_000_000,
		OutputPriceMicro: 6_000_000,
	}); err != nil {
		t.Fatalf("CreatePrice returned error: %v", err)
	}
	if _, err := store.AppendEvent(ctx, LLMUsageEventInput{
		UserID:          user.ID,
		ModelProviderID: "mp1",
		ProviderName:    "Provider",
		Protocol:        "openai",
		ModelID:         "model-a",
		Status:          UsageStatusSucceeded,
		UsageSource:     UsageSourceProviderReported,
		InputTokens:     1_000_000,
		OutputTokens:    500_000,
	}); err != nil {
		t.Fatalf("AppendEvent success returned error: %v", err)
	}
	if _, err := store.AppendEvent(ctx, LLMUsageEventInput{
		UserID:          user.ID,
		ModelProviderID: "mp1",
		ModelID:         "model-a",
		Status:          UsageStatusFailed,
		UsageSource:     UsageSourceProviderMissing,
		ErrorClass:      "provider_tls_interrupted",
	}); err != nil {
		t.Fatalf("AppendEvent failure returned error: %v", err)
	}

	summary, err := store.Summary(ctx, UsageQuery{})
	if err != nil {
		t.Fatalf("Summary returned error: %v", err)
	}
	if summary.Totals.RequestCount != 2 || summary.Totals.SuccessCount != 1 || summary.Totals.FailedCount != 1 {
		t.Fatalf("unexpected totals counts: %#v", summary.Totals)
	}
	if summary.Totals.TotalTokens != 1_500_000 {
		t.Fatalf("expected total tokens 1500000, got %#v", summary.Totals)
	}
	if summary.Totals.EstimatedCostMicro != 5_000_000 || summary.Totals.Currency != "USD" {
		t.Fatalf("unexpected estimated cost: %#v", summary.Totals)
	}
	if len(summary.ByUser) != 1 || summary.ByUser[0].Username != "usage-user" {
		t.Fatalf("expected user aggregate with username, got %#v", summary.ByUser)
	}
	if len(summary.ByModel) != 1 || summary.ByModel[0].ModelID != "model-a" {
		t.Fatalf("expected model aggregate, got %#v", summary.ByModel)
	}
}
