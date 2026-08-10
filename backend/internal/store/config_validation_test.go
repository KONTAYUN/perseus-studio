package store

import "testing"

func TestModelProviderToolArgumentStreamModeIsExplicit(t *testing.T) {
	if got := normalizeToolArgumentStreamMode("auto", "openai"); got != "delta" {
		t.Fatalf("legacy auto mode should normalize to delta, got %q", got)
	}
	if got := normalizeToolArgumentStreamMode("snapshot", "openai"); got != "snapshot" {
		t.Fatalf("explicit snapshot mode should be preserved, got %q", got)
	}
	if got := normalizeToolArgumentStreamMode("", "anthropic"); got != "delta" {
		t.Fatalf("empty mode should use the protocol delta default, got %q", got)
	}
}

func TestModelProviderValidationRejectsLegacyAutoMode(t *testing.T) {
	input := ModelProviderInput{
		Name:                   "test provider",
		Protocol:               "openai",
		BaseURL:                "https://example.test/v1",
		ModelID:                "test-model",
		ToolArgumentStreamMode: "auto",
	}
	if err := validateModelProvider(input, false); err == nil {
		t.Fatal("legacy auto mode must not be accepted for new provider writes")
	}
}
