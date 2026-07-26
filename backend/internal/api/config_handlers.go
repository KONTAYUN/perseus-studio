package api

import (
	"time"

	"webide-backend/internal/store"
)

type discoverModelsRequest struct {
	ProviderID string `json:"provider_id"`
	Protocol   string `json:"protocol"`
	AdapterMode string `json:"adapter_mode"`
	AuthMode    string `json:"auth_mode"`
	BaseURL    string `json:"base_url"`
	APIKey     string `json:"api_key"`
}

type discoveredModel struct {
	ID                        string                 `json:"id"`
	Name                      string                 `json:"name"`
	ReasoningCapability       string                 `json:"reasoning_capability,omitempty"`
	ReasoningCapabilitySource string                 `json:"reasoning_capability_source,omitempty"`
	ReasoningProfileLevels    string                 `json:"reasoning_profile_levels,omitempty"`
	ReasoningSupport          string                 `json:"reasoning_support,omitempty"`
	ThinkingProfile           *store.ThinkingProfile `json:"thinking_profile,omitempty"`
}

type testModelProviderRequest struct {
	ProviderID                string `json:"provider_id"`
	Protocol                  string `json:"protocol"`
	AdapterMode               string `json:"adapter_mode"`
	AuthMode                  string `json:"auth_mode"`
	BaseURL                   string `json:"base_url"`
	APIKey                    string `json:"api_key"`
	ModelID                   string `json:"model_id"`
	ReasoningCapability       string `json:"reasoning_capability,omitempty"`
	ReasoningCapabilitySource string `json:"reasoning_capability_source,omitempty"`
	ReasoningProfileLevels    string `json:"reasoning_profile_levels,omitempty"`
}

type modelProviderTestResult struct {
	Name       string `json:"name"`
	Label      string `json:"label"`
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

type modelProviderTestReport struct {
	Rating    string                    `json:"rating"`
	Summary   string                    `json:"summary"`
	Results   []modelProviderTestResult `json:"results"`
	StartedAt time.Time                 `json:"started_at"`
	EndedAt   time.Time                 `json:"ended_at"`
}
