package ai

import (
	"context"
	"fmt"

	"webide-backend/internal/store"
)

type DebugToolTestResult struct {
	ProviderName                string                `json:"provider_name"`
	Protocol                    string                `json:"protocol"`
	BaseURL                     string                `json:"base_url"`
	ModelID                     string                `json:"model_id"`
	RequestToolsCount           int                   `json:"request_tools_count"`
	RequestToolsNames           []string              `json:"request_tools_names"`
	ToolChoice                  any                   `json:"tool_choice"`
	RawResponseToolCallDetected bool                  `json:"raw_response_tool_call_detected"`
	ParsedToolCalls             []DebugParsedToolCall `json:"parsed_tool_calls"`
	RawResponseSummary          map[string]any        `json:"raw_response_summary"`
	FinalStatus                 string                `json:"final_status"`
	Error                       string                `json:"error,omitempty"`
}

type DebugParsedToolCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func (s *Service) DebugModelToolTest(ctx context.Context, userID string) (DebugToolTestResult, error) {
	provider, err := s.pickPreferredModelProvider(ctx, userID)
	if err != nil {
		return DebugToolTestResult{
			FinalStatus: "provider_unavailable",
			Error:       err.Error(),
		}, nil
	}

	switch provider.Protocol {
	case "openai":
		return s.debugOpenAIToolTest(ctx, provider)
	case "anthropic":
		return s.debugAnthropicToolTest(ctx, provider)
	default:
		return DebugToolTestResult{
			ProviderName: provider.Name,
			Protocol:     provider.Protocol,
			BaseURL:      provider.BaseURL,
			ModelID:      provider.ModelID,
			FinalStatus:  "unsupported_protocol",
			Error:        fmt.Sprintf("unsupported protocol: %s", provider.Protocol),
		}, nil
	}
}

func (s *Service) pickPreferredModelProvider(ctx context.Context, userID string) (store.ModelProvider, error) {
	items, err := s.configs.ListModelProvidersForUser(ctx, userID, "admin")
	if err != nil {
		return store.ModelProvider{}, err
	}
	for _, item := range items {
		if item.Enabled && item.IsDefault {
			return item, nil
		}
	}
	for _, item := range items {
		if item.Enabled {
			return item, nil
		}
	}
	return store.ModelProvider{}, fmt.Errorf("no enabled model provider configured")
}
