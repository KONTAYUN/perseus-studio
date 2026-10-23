package store

import (
	"path/filepath"
	"testing"
)

func TestUsageSummaryEmptyAndUnmatchedFiltersReturnZero(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "usage-empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	users := NewUserStore(db)
	if err := users.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	usage := NewUsageStore(db)
	if err := usage.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	check := func(query UsageQuery) {
		t.Helper()
		result, err := usage.Summary(t.Context(), query)
		if err != nil {
			t.Fatalf("empty summary: %v", err)
		}
		if result.Totals != (UsageAggregate{}) || len(result.ByUser) != 0 || len(result.ByModel) != 0 {
			t.Fatalf("expected zero summary, got %#v", result)
		}
	}
	check(UsageQuery{})
	user, err := users.CreateUser(t.Context(), "summary-user", "fixture-password", "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := usage.AppendEvent(t.Context(), LLMUsageEventInput{
		UserID: user.ID, ModelProviderID: "fixture-model", Status: UsageStatusSucceeded,
		UsageSource: UsageSourceProviderReported, InputTokens: 10,
	}); err != nil {
		t.Fatal(err)
	}
	check(UsageQuery{ModelProviderID: "unmatched-model"})
	check(UsageQuery{UserID: "unmatched-user"})
}
