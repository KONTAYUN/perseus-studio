package ai

import (
	"context"
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

func anthropicProviderToolChoiceForReasoning(choice providerToolChoice, reasoningEnabled bool, parallelEnabled ...bool) map[string]any {
	choice = normalizeProviderToolChoice(choice)
	allowParallel := len(parallelEnabled) > 0 && parallelEnabled[0]
	payload := map[string]any{"type": "auto", "disable_parallel_tool_use": !allowParallel}
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
	if req.ActionBoundaryActive && strings.EqualFold(strings.TrimSpace(req.Snapshot.Summary.KernelMode), agentKernelCodexParity) {
		return false
	}
	if req.ActionBoundaryActive && strings.EqualFold(strings.TrimSpace(req.Snapshot.Summary.KernelMode), agentKernelV3) {
		return false
	}
	return true
}

func emitProviderRequestSummary(opts RunOptions, summary ProviderRequestSummary) {
	if opts.Event == nil {
		return
	}
	opts.Event("model_request_summary", "info", map[string]any{
		"active_context_tokens":             summary.ActiveContextTokens,
		"image_count":                       summary.ImageCount,
		"text_input_tokens":                 summary.TextInputTokens,
		"visual_input_tokens":               summary.VisualInputTokens,
		"visual_token_estimate_source":      summary.VisualTokenEstimateSource,
		"compaction_threshold_tokens":       summary.CompactionThresholdTokens,
		"tool_results_pruned":               summary.ToolResultsPruned,
		"consumed_payloads_demoted":         summary.ConsumedPayloadsDemoted,
		"consumed_payload_bytes_removed":    summary.ConsumedPayloadBytesRemoved,
		"compaction_boundary_seq":           summary.CompactionBoundarySeq,
		"retained_turns":                    summary.RetainedTurns,
		"editor_context_paths":              summary.EditorContextPaths,
		"task_working_set_paths":            summary.TaskWorkingSetPaths,
		"context_window_source":             summary.ContextWindowSource,
		"kernel_version":                    summary.KernelVersion,
		"kernel_mode":                       summary.KernelMode,
		"prompt_policy_version":             summary.PromptPolicyVersion,
		"kernel_progress_events":            summary.KernelProgressEvents,
		"kernel_read_targets":               summary.KernelReadTargets,
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
		"payload_compaction_applied":        summary.PayloadCompactionApplied,
		"context_assembly_strategy":         summary.ContextAssemblyStrategy,
		"payload_before_bytes":              summary.PayloadBeforeBytes,
		"payload_after_bytes":               summary.PayloadAfterBytes,
		"note_view_index_reads":             summary.NoteViewIndexReads,
		"note_view_block_reads":             summary.NoteViewBlockReads,
		"note_view_section_reads":           summary.NoteViewSectionReads,
		"content_only_policy":               summary.ContentOnlyPolicy,
		"accepted_content_only_after_tools": summary.AcceptedContentOnlyAfterTools,
		"anthropic_prompt_cache_enabled":    summary.AnthropicPromptCacheEnabled,
		"provider_round_count":              summary.ProviderRoundCount,
		"cumulative_estimated_input_tokens": summary.CumulativeEstimatedInputTokens,
		"parallel_tool_batches":             summary.ParallelToolBatches,
		"source_read_truncated":             summary.SourceReadTruncated,
		"model_view_truncated":              summary.ModelViewTruncated,
		"provider_context_compacted":        summary.ProviderContextCompacted,
		"ledger_result_truncated":           summary.LedgerResultTruncated,
		"tool_result_bytes":                 summary.ToolResultBytes,
		"read_content_bytes":                summary.ReadContentBytes,
		"largest_tool_results":              summary.LargestToolResults,
		"message_shapes":                    summary.MessageShapes,
		"message_protocol_warnings":         summary.MessageProtocolWarnings,
		"tool_schema_digest":                summary.ToolSchemaDigest,
		"tool_manifest_digest":              summary.ToolManifestDigest,
		"model_action_manifest_digest":      summary.ModelActionManifestDigest,
		"model_terminal_actions":            summary.ModelTerminalActions,
		"tool_schema_warnings":              summary.ToolSchemaWarnings,
		"tool_schema_items":                 summary.ToolSchemaItems,
		"provider_visible_tools":            summary.ProviderVisibleTools,
		"loaded_capabilities":               summary.LoadedCapabilities,
		"internal_edit_protocol":            summary.InternalEditProtocol,
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
		"stream_reconnect_attempts":         summary.StreamReconnectAttempts,
		"stream_max_retries":                summary.StreamMaxRetries,
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
	hasRecoverableProgress := summary.CheckpointSeq > 0 || summary.KernelProgressEvents > 0
	if level == "error" && stage != "checkpoint_resume_failed" && hasRecoverableProgress && isCheckpointRecoverableProviderError(err) {
		return "warn"
	}
	return level
}

func durationFromMilliseconds(ms int64) time.Duration { return time.Duration(ms) * time.Millisecond }

func providerFailureClass(err error, summary ProviderRequestSummary) string {
	_ = summary
	if err == nil {
		return ""
	}
	failure := classifyProviderFailure(err)
	if failure.Class == ProviderFailureUnknown {
		return ""
	}
	return string(failure.Class)
}

func providerTransportFailureClass(err error) string {
	failure := classifyProviderTransportFailure(err)
	if failure.Class == ProviderFailureUnknown {
		return ""
	}
	return string(failure.Class)
}
