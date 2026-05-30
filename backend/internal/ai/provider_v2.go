package ai

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"webide-backend/internal/store"
)

type providerTurnRequest struct {
	ConversationID    string
	MessageID         string
	Provider          store.ModelProvider
	Protocol          agentProtocol
	Tools             []map[string]any
	ToolChoice        providerToolChoice
	DisableTools      bool
	Snapshot          ContextSnapshot
	PerformedMutation bool
	HasToolEvents     bool
	Opts              RunOptions
}

type providerToolChoiceMode string

const (
	providerToolChoiceAuto     providerToolChoiceMode = "auto"
	providerToolChoiceRequired providerToolChoiceMode = "required"
	providerToolChoiceForced   providerToolChoiceMode = "forced"
	providerToolChoiceNone     providerToolChoiceMode = "none"
)

type providerToolChoice struct {
	Mode     providerToolChoiceMode
	ToolName string
}

func (c providerToolChoice) requiresTool() bool {
	return c.Mode == providerToolChoiceRequired || c.Mode == providerToolChoiceForced
}

func normalizeProviderToolChoice(choice providerToolChoice) providerToolChoice {
	switch choice.Mode {
	case providerToolChoiceRequired:
		return providerToolChoice{Mode: providerToolChoiceRequired}
	case providerToolChoiceForced:
		if choice.ToolName != "" {
			return providerToolChoice{Mode: providerToolChoiceForced, ToolName: choice.ToolName}
		}
		return providerToolChoice{Mode: providerToolChoiceRequired}
	case providerToolChoiceNone:
		return providerToolChoice{Mode: providerToolChoiceNone}
	default:
		return providerToolChoice{Mode: providerToolChoiceAuto}
	}
}

func providerToolChoiceSummary(choice providerToolChoice) string {
	choice = normalizeProviderToolChoice(choice)
	switch choice.Mode {
	case providerToolChoiceRequired:
		return "required"
	case providerToolChoiceForced:
		if choice.ToolName != "" {
			return "forced:" + choice.ToolName
		}
		return "forced"
	case providerToolChoiceNone:
		return "none"
	default:
		return "auto"
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func openAIProviderToolChoice(choice providerToolChoice) any {
	choice = normalizeProviderToolChoice(choice)
	switch choice.Mode {
	case providerToolChoiceRequired:
		return "required"
	case providerToolChoiceForced:
		return map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": choice.ToolName,
			},
		}
	case providerToolChoiceNone:
		return "none"
	default:
		return "auto"
	}
}

func anthropicProviderToolChoice(choice providerToolChoice) map[string]any {
	return anthropicProviderToolChoiceForReasoning(choice, false)
}

func anthropicProviderToolChoiceForReasoning(choice providerToolChoice, reasoningEnabled bool) map[string]any {
	choice = normalizeProviderToolChoice(choice)
	payload := map[string]any{"type": "auto", "disable_parallel_tool_use": true}
	if reasoningEnabled && choice.requiresTool() {
		return payload
	}
	switch choice.Mode {
	case providerToolChoiceRequired:
		payload["type"] = "any"
	case providerToolChoiceForced:
		payload["type"] = "tool"
		payload["name"] = choice.ToolName
	case providerToolChoiceNone:
		payload["type"] = "none"
	}
	return payload
}

type providerAdapterV2 interface {
	Turn(ctx context.Context, req providerTurnRequest) (agentModelTurn, error)
}

func (s *Service) providerV2(protocol agentProtocol) providerAdapterV2 {
	switch protocol {
	case agentProtocolAnthropic:
		return anthropicProviderV2{service: s}
	default:
		return openAIProviderV2{service: s}
	}
}

func emitProviderRequestSummary(opts RunOptions, summary ProviderRequestSummary) {
	if opts.Event == nil {
		return
	}
	opts.Event("model_request_summary", "info", map[string]any{
		"round":                        summary.Round,
		"protocol":                     summary.Protocol,
		"request_kind":                 summary.RequestKind,
		"stream":                       summary.Stream,
		"tool_choice":                  summary.ToolChoice,
		"requires_tool":                summary.RequiresTool,
		"thinking_mode":                summary.ThinkingMode,
		"reasoning_support":            summary.ReasoningSupport,
		"reasoning_effort":             summary.ReasoningEffort,
		"reasoning_profile_source":     summary.ReasoningProfileSource,
		"reasoning_profile_levels":     summary.ReasoningProfileLevels,
		"tools_count":                  summary.ToolsCount,
		"tool_names":                   summary.ToolNames,
		"parallel_tool_calls":          summary.ParallelToolCalls,
		"payload_keys":                 summary.PayloadKeys,
		"tool_schema_digest":           summary.ToolSchemaDigest,
		"tool_schema_warnings":         summary.ToolSchemaWarnings,
		"unsupported_openai_params":    summary.UnsupportedOpenAIParams,
		"output_max_tokens":            summary.OutputMaxTokens,
		"stream_tool_args_state":       summary.StreamToolArgsState,
		"loaded_toolsets":              summary.LoadedToolsets,
		"forced_tool":                  summary.ForcedTool,
		"failure_class":                summary.FailureClass,
		"message_count":                summary.MessageCount,
		"transcript_entries":           summary.TranscriptEntries,
		"tool_result_count":            summary.ToolResultCount,
		"tool_loop_active":             summary.ToolLoopActive,
		"estimated_chars":              summary.EstimatedChars,
		"estimated_tokens":             summary.EstimatedTokens,
		"context_window_tokens":        summary.ContextWindowTokens,
		"context_watermark_percent":    summary.ContextWatermarkPercent,
		"compact_reserved_tokens":      summary.CompactReservedTokens,
		"compaction_budget_tokens":     summary.CompactionBudgetTokens,
		"auto_compact_enabled":         summary.AutoCompactEnabled,
		"estimated_tool_result_chars":  summary.EstimatedToolResultChars,
		"estimated_tool_result_tokens": summary.EstimatedToolResultTokens,
		"max_segment_chars":            summary.MaxSegmentChars,
		"compaction_actions":           summary.CompactionActions,
		"provider_attempts":            summary.ProviderAttempts,
		"stream_fallback_used":         summary.StreamFallbackUsed,
		"checkpoint_resume_attempt":    summary.CheckpointResumeAttempt,
		"checkpoint_seq":               summary.CheckpointSeq,
		"recoverable_transient_error":  summary.RecoverableTransientError,
		"budget_reason":                summary.BudgetReason,
		"max_rounds":                   summary.MaxRounds,
		"current_round":                summary.CurrentRound,
		"max_tool_calls":               summary.MaxToolCalls,
		"current_tool_calls":           summary.CurrentToolCalls,
	})
}

func (s *Service) recordProviderDiagnosticWithSummary(ctx context.Context, opts RunOptions, provider store.ModelProvider, conversationID, messageID, stage, level string, durationMs int64, err error, summary ProviderRequestSummary) string {
	if s.diag == nil || opts.UserID == "" || err == nil {
		return ""
	}
	summary.FailureClass = providerFailureClass(err, summary)
	level = providerDiagnosticLevel(stage, level, err, summary)
	return s.recordProviderDiagnosticSummary(ctx, opts, provider, conversationID, messageID, stage, level, durationFromMilliseconds(durationMs), err, summary.JSON())
}

func providerDiagnosticLevel(stage, level string, err error, summary ProviderRequestSummary) string {
	if level == "error" && stage != "checkpoint_resume_failed" && summary.CheckpointSeq > 0 && isCheckpointRecoverableProviderError(err) {
		return "warn"
	}
	return level
}

func durationFromMilliseconds(ms int64) time.Duration { return time.Duration(ms) * time.Millisecond }

func providerFailureClass(err error, summary ProviderRequestSummary) string {
	if err == nil {
		return ""
	}
	if isContextCanceledError(err) {
		return "user_or_client_canceled"
	}
	var providerErr *ProviderResponseError
	if errors.As(err, &providerErr) {
		if failureClass := providerResponseFailureClass(providerErr, err); failureClass != "" {
			return failureClass
		}
		if providerErr.StatusCode == http.StatusBadRequest {
			if providerRejectedRequiredToolChoice(providerErr, err) {
				return "provider_rejected_required_tool_choice"
			}
			if len(summary.UnsupportedOpenAIParams) > 0 {
				return "provider_rejected_unsupported_model_params"
			}
			if summary.ReasoningEffort != "" {
				return "provider_bad_request_with_reasoning"
			}
			if summary.ToolsCount > 0 {
				return "provider_bad_request_with_tools"
			}
			return "provider_bad_request_without_tools"
		}
		if isRetriableProviderStatus(providerErr.StatusCode) || providerErr.StatusCode == 529 {
			return "provider_transient_http"
		}
		return "provider_http_error"
	}
	if isCheckpointRecoverableProviderError(err) {
		return "provider_transient_transport"
	}
	if strings.Contains(strings.ToLower(err.Error()), "incomplete tool call arguments") {
		return "provider_incomplete_tool_arguments"
	}
	if failureClass := providerTransportFailureClass(err); failureClass != "" {
		return failureClass
	}
	return ""
}

func providerRejectedRequiredToolChoice(providerErr *ProviderResponseError, err error) bool {
	text := strings.ToLower(strings.Join([]string{providerErr.Message, providerErr.ResponseSnippet, errString(err)}, "\n"))
	return strings.Contains(text, "tool_choice") &&
		strings.Contains(text, "required") &&
		(strings.Contains(text, "not support") ||
			strings.Contains(text, "unsupported") ||
			strings.Contains(text, "invalid") ||
			strings.Contains(text, "not allowed") ||
			strings.Contains(text, "must be"))
}

func providerResponseFailureClass(providerErr *ProviderResponseError, err error) string {
	if providerErr == nil {
		return ""
	}
	text := strings.ToLower(strings.Join([]string{providerErr.Message, providerErr.ResponseSnippet, errString(err)}, "\n"))
	switch {
	case strings.Contains(text, "incomplete tool call arguments"):
		return "provider_incomplete_tool_arguments"
	case strings.Contains(text, "model_not_found") ||
		strings.Contains(text, "model not found") ||
		strings.Contains(text, "no available channel for model") ||
		strings.Contains(text, "does not exist") && strings.Contains(text, "model"):
		return "provider_model_unavailable"
	case providerErr.StatusCode == http.StatusUnauthorized || providerErr.StatusCode == http.StatusForbidden:
		return "provider_auth_failed"
	case providerErr.StatusCode == http.StatusNotFound:
		return "provider_endpoint_or_model_not_found"
	default:
		return ""
	}
}

func providerTransportFailureClass(err error) string {
	if err == nil {
		return ""
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "no such host") || strings.Contains(text, "lookup ") && strings.Contains(text, ":53"):
		return "provider_dns_lookup_failed"
	case strings.Contains(text, "connection refused"):
		return "provider_connection_refused"
	case strings.Contains(text, "context deadline exceeded") || strings.Contains(text, "client.timeout exceeded"):
		return "provider_transport_timeout"
	default:
		return ""
	}
}
