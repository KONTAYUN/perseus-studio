package ai

import (
	"context"
	"errors"
	"net/http"
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
	choice = normalizeProviderToolChoice(choice)
	payload := map[string]any{"type": "auto", "disable_parallel_tool_use": true}
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
		"tools_count":                  summary.ToolsCount,
		"tool_names":                   summary.ToolNames,
		"parallel_tool_calls":          summary.ParallelToolCalls,
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
		if providerErr.StatusCode == http.StatusBadRequest {
			if summary.RequiresTool && summary.ToolChoice == "required" {
				return "provider_rejected_required_tool_choice"
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
	return ""
}
