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
	ConversationID       string
	MessageID            string
	Provider             store.ModelProvider
	Protocol             agentProtocol
	Tools                []map[string]any
	ToolChoice           providerToolChoice
	DisableTools         bool
	Snapshot             ContextSnapshot
	PerformedMutation    bool
	HasToolEvents        bool
	ActionBoundaryActive bool
	Opts                 RunOptions
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

func providerTurnCanStreamAssistantText(req providerTurnRequest) bool {
	if strings.TrimSpace(req.Opts.TaskID) != "" {
		return false
	}
	return true
}

func emitProviderRequestSummary(opts RunOptions, summary ProviderRequestSummary) {
	if opts.Event == nil {
		return
	}
	opts.Event("model_request_summary", "info", map[string]any{
		"round":                             summary.Round,
		"protocol":                          summary.Protocol,
		"provider_message_phase":            summary.ProviderMessagePhase,
		"request_kind":                      summary.RequestKind,
		"stream":                            summary.Stream,
		"tool_choice":                       summary.ToolChoice,
		"requires_tool":                     summary.RequiresTool,
		"thinking_mode":                     summary.ThinkingMode,
		"reasoning_support":                 summary.ReasoningSupport,
		"reasoning_effort":                  summary.ReasoningEffort,
		"reasoning_profile_source":          summary.ReasoningProfileSource,
		"reasoning_profile_levels":          summary.ReasoningProfileLevels,
		"tools_count":                       summary.ToolsCount,
		"tool_names":                        summary.ToolNames,
		"parallel_tool_calls":               summary.ParallelToolCalls,
		"payload_keys":                      summary.PayloadKeys,
		"payload_bytes":                     summary.PayloadBytes,
		"payload_soft_budget_bytes":         summary.PayloadSoftBudgetBytes,
		"payload_hard_budget_bytes":         summary.PayloadHardBudgetBytes,
		"payload_budget_exceeded":           summary.PayloadBudgetExceeded,
		"payload_compaction_applied":        summary.PayloadCompactionApplied,
		"message_builder_applied":           summary.MessageBuilderApplied,
		"message_builder_strategy":          summary.MessageBuilderStrategy,
		"payload_before_bytes":              summary.PayloadBeforeBytes,
		"payload_after_bytes":               summary.PayloadAfterBytes,
		"compacted_entries":                 summary.CompactedEntries,
		"preserved_recent_entries":          summary.PreservedRecentEntries,
		"continuation_summary_chars":        summary.ContinuationSummaryChars,
		"preserved_changed_paths":           summary.PreservedChangedPaths,
		"preserved_read_ranges":             summary.PreservedReadRanges,
		"current_read_working_set_paths":    summary.CurrentReadWorkingSetPaths,
		"current_read_working_set_ranges":   summary.CurrentReadWorkingSetRanges,
		"recent_read_memory_paths":          summary.RecentReadMemoryPaths,
		"recent_read_inlined_chars":         summary.RecentReadInlinedChars,
		"content_only_policy":               summary.ContentOnlyPolicy,
		"accepted_content_only_after_tools": summary.AcceptedContentOnlyAfterTools,
		"anthropic_prompt_cache_enabled":    summary.AnthropicPromptCacheEnabled,
		"provider_round_count":              summary.ProviderRoundCount,
		"source_read_truncated":             summary.SourceReadTruncated,
		"provider_context_compacted":        summary.ProviderContextCompacted,
		"tool_result_bytes":                 summary.ToolResultBytes,
		"read_content_bytes":                summary.ReadContentBytes,
		"largest_tool_results":              summary.LargestToolResults,
		"message_shapes":                    summary.MessageShapes,
		"message_protocol_warnings":         summary.MessageProtocolWarnings,
		"tool_schema_digest":                summary.ToolSchemaDigest,
		"tool_manifest_digest":              summary.ToolManifestDigest,
		"model_action_manifest_digest":      summary.ModelActionManifestDigest,
		"model_text_actions":                summary.ModelTextActions,
		"model_terminal_actions":            summary.ModelTerminalActions,
		"tool_schema_warnings":              summary.ToolSchemaWarnings,
		"tool_schema_items":                 summary.ToolSchemaItems,
		"provider_visible_tools":            summary.ProviderVisibleTools,
		"loaded_capabilities":               summary.LoadedCapabilities,
		"internal_edit_protocol":            summary.InternalEditProtocol,
		"native_patch_tool_enabled":         summary.NativePatchToolEnabled,
		"action_selection_state":            summary.ActionSelectionState,
		"loop_guard_state":                  summary.LoopGuardState,
		"unsupported_openai_params":         summary.UnsupportedOpenAIParams,
		"provider_host":                     summary.ProviderHost,
		"model_id":                          summary.ModelID,
		"provider_http_protocol":            summary.ProviderHTTPProtocol,
		"provider_http2_disabled":           summary.ProviderHTTP2Disabled,
		"has_tools":                         summary.HasTools,
		"has_required_tool_choice":          summary.HasRequiredToolChoice,
		"has_reasoning_params":              summary.HasReasoningParams,
		"output_max_tokens":                 summary.OutputMaxTokens,
		"stream_tool_args_state":            summary.StreamToolArgsState,
		"stream_finish_reason":              summary.StreamFinishReason,
		"stream_usage":                      summary.StreamUsage,
		"stream_tool_calls":                 summary.StreamToolCalls,
		"loaded_toolsets":                   summary.LoadedToolsets,
		"forced_tool":                       summary.ForcedTool,
		"agent_execution_mode":              summary.AgentExecutionMode,
		"failure_class":                     summary.FailureClass,
		"message_count":                     summary.MessageCount,
		"transcript_entries":                summary.TranscriptEntries,
		"tool_result_count":                 summary.ToolResultCount,
		"tool_loop_active":                  summary.ToolLoopActive,
		"action_boundary_active":            summary.ActionBoundaryActive,
		"estimated_chars":                   summary.EstimatedChars,
		"estimated_tokens":                  summary.EstimatedTokens,
		"context_window_tokens":             summary.ContextWindowTokens,
		"context_watermark_percent":         summary.ContextWatermarkPercent,
		"compact_reserved_tokens":           summary.CompactReservedTokens,
		"compaction_budget_tokens":          summary.CompactionBudgetTokens,
		"auto_compact_enabled":              summary.AutoCompactEnabled,
		"estimated_tool_result_chars":       summary.EstimatedToolResultChars,
		"estimated_tool_result_tokens":      summary.EstimatedToolResultTokens,
		"max_segment_chars":                 summary.MaxSegmentChars,
		"compaction_actions":                summary.CompactionActions,
		"provider_attempts":                 summary.ProviderAttempts,
		"provider_retry_policy":             summary.ProviderRetryPolicy,
		"sdk_max_attempts":                  summary.SDKMaxAttempts,
		"request_attempts":                  summary.RequestAttempts,
		"retryable_error":                   summary.RetryableError,
		"retry_after_ms":                    summary.RetryAfterMS,
		"stream_retry_phase":                summary.StreamRetryPhase,
		"fallback_attempts":                 summary.FallbackAttempts,
		"stream_fallback_used":              summary.StreamFallbackUsed,
		"checkpoint_resume_attempt":         summary.CheckpointResumeAttempt,
		"checkpoint_seq":                    summary.CheckpointSeq,
		"recoverable_transient_error":       summary.RecoverableTransientError,
		"budget_reason":                     summary.BudgetReason,
		"max_rounds":                        summary.MaxRounds,
		"current_round":                     summary.CurrentRound,
		"max_tool_calls":                    summary.MaxToolCalls,
		"current_tool_calls":                summary.CurrentToolCalls,
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
	if strings.Contains(strings.ToLower(err.Error()), "incomplete tool call arguments") {
		return "provider_incomplete_tool_arguments"
	}
	if failureClass := providerTransportFailureClass(err); failureClass != "" {
		return failureClass
	}
	if isCheckpointRecoverableProviderError(err) {
		return "provider_transient_transport"
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
	case strings.Contains(text, "eof"):
		return "provider_transport_eof"
	case strings.Contains(text, "malformed http response"):
		return "provider_transport_protocol_error"
	case strings.Contains(text, "connection refused"):
		return "provider_connection_refused"
	case strings.Contains(text, "connection reset") ||
		strings.Contains(text, "connection aborted") ||
		strings.Contains(text, "transport connection broken") ||
		strings.Contains(text, "server closed idle connection"):
		return "provider_connection_interrupted"
	case strings.Contains(text, "context deadline exceeded") || strings.Contains(text, "client.timeout exceeded"):
		return "provider_transport_timeout"
	case strings.Contains(text, "tls: bad record mac"):
		return "provider_tls_interrupted"
	default:
		return ""
	}
}
