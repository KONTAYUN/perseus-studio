package api

import (
	"time"

	"webide-backend/internal/store"
)

type publicModelProviderView struct {
	ID                          string                `json:"id"`
	Name                        string                `json:"name"`
	Visibility                  string                `json:"visibility"`
	Protocol                    string                `json:"protocol"`
	ModelID                     string                `json:"model_id"`
	Enabled                     bool                  `json:"enabled"`
	IsDefault                   bool                  `json:"is_default"`
	ReasoningCapability         string                `json:"reasoning_capability"`
	ReasoningCapabilitySource   string                `json:"reasoning_capability_source,omitempty"`
	ReasoningProfileLevels      string                `json:"reasoning_profile_levels,omitempty"`
	ReasoningSupport            string                `json:"reasoning_support"`
	ThinkingProfile             store.ThinkingProfile `json:"thinking_profile"`
	ContextWindowTokens         int                   `json:"context_window_tokens"`
	AutoCompactEnabled          bool                  `json:"auto_compact_enabled"`
	AutoCompactThresholdPercent int                   `json:"auto_compact_threshold_percent"`
	CompactReservedTokens       int                   `json:"compact_reserved_tokens"`
	PruneToolOutputsEnabled     bool                  `json:"prune_tool_outputs_enabled"`
	NativePatchToolEnabled      bool                  `json:"native_patch_tool_enabled"`
	CreatedAt                   time.Time             `json:"created_at"`
	UpdatedAt                   time.Time             `json:"updated_at"`
}

type publicSearchProviderView struct {
	ID         string    `json:"id"`
	Provider   string    `json:"provider"`
	Visibility string    `json:"visibility"`
	Enabled    bool      `json:"enabled"`
	Priority   int       `json:"priority"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type publicSkillView struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Visibility  string    `json:"visibility"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func modelProvidersForResponse(role string, items []store.ModelProvider) any {
	if role == "admin" {
		return items
	}
	views := make([]publicModelProviderView, 0, len(items))
	for _, item := range items {
		views = append(views, publicModelProviderView{
			ID:                          item.ID,
			Name:                        item.Name,
			Visibility:                  item.Visibility,
			Protocol:                    item.Protocol,
			ModelID:                     item.ModelID,
			Enabled:                     item.Enabled,
			IsDefault:                   item.IsDefault,
			ReasoningCapability:         item.ReasoningCapability,
			ReasoningCapabilitySource:   item.ReasoningCapabilitySource,
			ReasoningProfileLevels:      item.ReasoningProfileLevels,
			ReasoningSupport:            item.ReasoningSupport,
			ThinkingProfile:             item.ThinkingProfile,
			ContextWindowTokens:         item.ContextWindowTokens,
			AutoCompactEnabled:          item.AutoCompactEnabled,
			AutoCompactThresholdPercent: item.AutoCompactThresholdPercent,
			CompactReservedTokens:       item.CompactReservedTokens,
			PruneToolOutputsEnabled:     item.PruneToolOutputsEnabled,
			NativePatchToolEnabled:      item.NativePatchToolEnabled,
			CreatedAt:                   item.CreatedAt,
			UpdatedAt:                   item.UpdatedAt,
		})
	}
	return views
}

func searchProvidersForResponse(role string, items []store.SearchProvider) any {
	if role == "admin" {
		return items
	}
	views := make([]publicSearchProviderView, 0, len(items))
	for _, item := range items {
		views = append(views, publicSearchProviderView{
			ID:         item.ID,
			Provider:   item.Provider,
			Visibility: item.Visibility,
			Enabled:    item.Enabled,
			Priority:   item.Priority,
			CreatedAt:  item.CreatedAt,
			UpdatedAt:  item.UpdatedAt,
		})
	}
	return views
}

func skillForResponse(role string, item store.Skill) any {
	if role == "admin" {
		return item
	}
	return publicSkillView{
		ID:          item.ID,
		Name:        item.Name,
		Description: item.Description,
		Visibility:  item.Visibility,
		Enabled:     item.Enabled,
		CreatedAt:   item.CreatedAt,
		UpdatedAt:   item.UpdatedAt,
	}
}

func skillsForResponse(role string, items []store.Skill) any {
	if role == "admin" {
		return items
	}
	views := make([]publicSkillView, 0, len(items))
	for _, item := range items {
		views = append(views, publicSkillView{
			ID:          item.ID,
			Name:        item.Name,
			Description: item.Description,
			Visibility:  item.Visibility,
			Enabled:     item.Enabled,
			CreatedAt:   item.CreatedAt,
			UpdatedAt:   item.UpdatedAt,
		})
	}
	return views
}
