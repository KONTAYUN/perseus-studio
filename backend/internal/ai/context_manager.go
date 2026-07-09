package ai

import (
	"encoding/json"

	"webide-backend/internal/store"
)

type ContextSnapshot struct {
	OpenAIMessages    []map[string]any
	AnthropicSystem   string
	AnthropicMessages []map[string]any
	Transcript        *AgentTranscript
	Summary           ProviderRequestSummary
}

type ProviderRequestSummary struct {
	Round                         int                  `json:"round"`
	Protocol                      string               `json:"protocol"`
	ProviderMessagePhase          string               `json:"provider_message_phase,omitempty"`
	RequestKind                   string               `json:"request_kind,omitempty"`
	Stream                        *bool                `json:"stream,omitempty"`
	ToolChoice                    string               `json:"tool_choice,omitempty"`
	RequiresTool                  bool                 `json:"requires_tool,omitempty"`
	ThinkingMode                  string               `json:"thinking_mode,omitempty"`
	ReasoningSupport              string               `json:"reasoning_support,omitempty"`
	ReasoningEffort               string               `json:"reasoning_effort,omitempty"`
	ReasoningProfileSource        string               `json:"reasoning_profile_source,omitempty"`
	ReasoningProfileLevels        []string             `json:"reasoning_profile_levels,omitempty"`
	ToolsCount                    int                  `json:"tools_count,omitempty"`
	ToolNames                     []string             `json:"tool_names,omitempty"`
	ParallelToolCalls             *bool                `json:"parallel_tool_calls,omitempty"`
	PayloadKeys                   []string             `json:"payload_keys,omitempty"`
	PayloadBytes                  int                  `json:"payload_bytes,omitempty"`
	PayloadSoftBudgetBytes        int                  `json:"payload_soft_budget_bytes,omitempty"`
	PayloadHardBudgetBytes        int                  `json:"payload_hard_budget_bytes,omitempty"`
	PayloadBudgetExceeded         bool                 `json:"payload_budget_exceeded,omitempty"`
	PayloadCompactionApplied      bool                 `json:"payload_compaction_applied,omitempty"`
	MessageBuilderApplied         bool                 `json:"message_builder_applied,omitempty"`
	MessageBuilderStrategy        string               `json:"message_builder_strategy,omitempty"`
	PayloadBeforeBytes            int                  `json:"payload_before_bytes,omitempty"`
	PayloadAfterBytes             int                  `json:"payload_after_bytes,omitempty"`
	CompactedEntries              int                  `json:"compacted_entries,omitempty"`
	PreservedRecentEntries        int                  `json:"preserved_recent_entries,omitempty"`
	ContinuationSummaryChars      int                  `json:"continuation_summary_chars,omitempty"`
	PreservedChangedPaths         []string             `json:"preserved_changed_paths,omitempty"`
	PreservedReadRanges           []string             `json:"preserved_read_ranges,omitempty"`
	CurrentReadWorkingSetPaths    []string             `json:"current_read_working_set_paths,omitempty"`
	CurrentReadWorkingSetRanges   []string             `json:"current_read_working_set_ranges,omitempty"`
	NoteViewIndexReads            int                  `json:"note_view_index_reads,omitempty"`
	NoteViewBlockReads            int                  `json:"note_view_block_reads,omitempty"`
	NoteViewSectionReads          int                  `json:"note_view_section_reads,omitempty"`
	RecentReadMemoryPaths         []string             `json:"recent_read_memory_paths,omitempty"`
	RecentReadInlinedChars        int                  `json:"recent_read_inlined_chars,omitempty"`
	ContentOnlyPolicy             string               `json:"content_only_policy,omitempty"`
	AcceptedContentOnlyAfterTools bool                 `json:"accepted_content_only_after_tools,omitempty"`
	AnthropicPromptCacheEnabled   bool                 `json:"anthropic_prompt_cache_enabled,omitempty"`
	ProviderRoundCount            int                  `json:"provider_round_count,omitempty"`
	SourceReadTruncated           bool                 `json:"source_read_truncated,omitempty"`
	ModelViewTruncated            bool                 `json:"model_view_truncated,omitempty"`
	ProviderContextCompacted      bool                 `json:"provider_context_compacted,omitempty"`
	LedgerResultTruncated         bool                 `json:"ledger_result_truncated,omitempty"`
	ToolResultBytes               int                  `json:"tool_result_bytes,omitempty"`
	ReadContentBytes              int                  `json:"read_content_bytes,omitempty"`
	LargestToolResults            []ToolResultSizeLog  `json:"largest_tool_results,omitempty"`
	MessageShapes                 []ProviderMessageLog `json:"message_shapes,omitempty"`
	MessageProtocolWarnings       []string             `json:"message_protocol_warnings,omitempty"`
	ToolSchemaDigest              string               `json:"tool_schema_digest,omitempty"`
	ToolManifestDigest            string               `json:"tool_manifest_digest,omitempty"`
	ModelActionManifestDigest     string               `json:"model_action_manifest_digest,omitempty"`
	ModelTextActions              []string             `json:"model_text_actions,omitempty"`
	ModelTerminalActions          []string             `json:"model_terminal_actions,omitempty"`
	ToolSchemaWarnings            []string             `json:"tool_schema_warnings,omitempty"`
	ToolSchemaItems               []ProviderToolLog    `json:"tool_schema_items,omitempty"`
	ProviderVisibleTools          []string             `json:"provider_visible_tools,omitempty"`
	LoadedCapabilities            map[string]any       `json:"loaded_capabilities,omitempty"`
	InternalEditProtocol          string               `json:"internal_edit_protocol,omitempty"`
	NativePatchToolEnabled        bool                 `json:"native_patch_tool_enabled,omitempty"`
	ActionSelectionState          ActionSelectionState `json:"action_selection_state,omitempty"`
	LoopGuardState                AgentLoopGuard       `json:"loop_guard_state,omitempty"`
	UnsupportedOpenAIParams       []string             `json:"unsupported_openai_params,omitempty"`
	ProviderHost                  string               `json:"provider_host,omitempty"`
	ModelID                       string               `json:"model_id,omitempty"`
	ProviderHTTPProtocol          string               `json:"provider_http_protocol,omitempty"`
	ProviderHTTP2Disabled         bool                 `json:"provider_http2_disabled,omitempty"`
	HasTools                      bool                 `json:"has_tools,omitempty"`
	HasRequiredToolChoice         bool                 `json:"has_required_tool_choice,omitempty"`
	HasReasoningParams            bool                 `json:"has_reasoning_params,omitempty"`
	OutputMaxTokens               int                  `json:"output_max_tokens,omitempty"`
	StreamToolArgsState           string               `json:"stream_tool_args_state,omitempty"`
	StreamFinishReason            string               `json:"stream_finish_reason,omitempty"`
	StreamUsage                   ProviderUsage        `json:"stream_usage,omitempty"`
	StreamToolCalls               []StreamToolCallLog  `json:"stream_tool_calls,omitempty"`
	LoadedToolsets                []string             `json:"loaded_toolsets,omitempty"`
	ForcedTool                    string               `json:"forced_tool,omitempty"`
	AgentExecutionMode            string               `json:"agent_execution_mode,omitempty"`
	FailureClass                  string               `json:"failure_class,omitempty"`
	MessageCount                  int                  `json:"message_count"`
	TranscriptEntries             int                  `json:"transcript_entries"`
	ToolResultCount               int                  `json:"tool_result_count"`
	ToolLoopActive                bool                 `json:"tool_loop_active"`
	ActionBoundaryActive          bool                 `json:"action_boundary_active,omitempty"`
	EstimatedChars                int                  `json:"estimated_chars"`
	EstimatedTokens               int                  `json:"estimated_tokens"`
	ContextWindowTokens           int                  `json:"context_window_tokens"`
	ContextWatermarkPercent       int                  `json:"context_watermark_percent"`
	CompactReservedTokens         int                  `json:"compact_reserved_tokens"`
	CompactionBudgetTokens        int                  `json:"compaction_budget_tokens"`
	AutoCompactEnabled            bool                 `json:"auto_compact_enabled"`
	EstimatedToolResultChars      int                  `json:"estimated_tool_result_chars"`
	EstimatedToolResultTokens     int                  `json:"estimated_tool_result_tokens"`
	MaxSegmentChars               int                  `json:"max_segment_chars"`
	CompactionActions             []string             `json:"compaction_actions,omitempty"`
	ProviderAttempts              int                  `json:"provider_attempts,omitempty"`
	ProviderRetryPolicy           map[string]any       `json:"provider_retry_policy,omitempty"`
	SDKMaxAttempts                int                  `json:"sdk_max_attempts,omitempty"`
	RequestAttempts               int                  `json:"request_attempts,omitempty"`
	RetryableError                bool                 `json:"retryable_error,omitempty"`
	RetryAfterMS                  int64                `json:"retry_after_ms,omitempty"`
	StreamRetryPhase              string               `json:"stream_retry_phase,omitempty"`
	FallbackAttempts              int                  `json:"fallback_attempts,omitempty"`
	StreamFallbackUsed            bool                 `json:"stream_fallback_used,omitempty"`
	CheckpointResumeAttempt       int                  `json:"checkpoint_resume_attempt,omitempty"`
	CheckpointSeq                 int64                `json:"checkpoint_seq,omitempty"`
	RecoverableTransientError     bool                 `json:"recoverable_transient_error,omitempty"`
	BudgetReason                  string               `json:"budget_reason,omitempty"`
	MaxRounds                     int                  `json:"max_rounds,omitempty"`
	CurrentRound                  int                  `json:"current_round,omitempty"`
	MaxToolCalls                  int                  `json:"max_tool_calls,omitempty"`
	CurrentToolCalls              int                  `json:"current_tool_calls,omitempty"`
}

type ToolResultSizeLog struct {
	ToolName     string `json:"tool_name"`
	Path         string `json:"path,omitempty"`
	Bytes        int    `json:"bytes"`
	ReadContent  bool   `json:"read_content,omitempty"`
	Compacted    bool   `json:"compacted,omitempty"`
	ContentChars int    `json:"content_chars,omitempty"`
}

type ProviderMessageLog struct {
	Index        int      `json:"index"`
	Role         string   `json:"role,omitempty"`
	ContentKind  string   `json:"content_kind,omitempty"`
	TextChars    int      `json:"text_chars,omitempty"`
	PartTypes    []string `json:"part_types,omitempty"`
	ToolCalls    int      `json:"tool_calls,omitempty"`
	ToolCallIDs  []string `json:"tool_call_ids,omitempty"`
	ToolCallName []string `json:"tool_call_names,omitempty"`
	ToolCallID   string   `json:"tool_call_id,omitempty"`
}

type ProviderToolLog struct {
	Name             string   `json:"name"`
	Strict           bool     `json:"strict,omitempty"`
	Required         []string `json:"required,omitempty"`
	Properties       []string `json:"properties,omitempty"`
	UnsupportedPaths []string `json:"unsupported_paths,omitempty"`
	SchemaDigest     string   `json:"schema_digest,omitempty"`
}

type StreamToolCallLog struct {
	Index              int    `json:"index"`
	ID                 string `json:"id,omitempty"`
	Name               string `json:"name,omitempty"`
	ArgumentsChars     int    `json:"arguments_chars,omitempty"`
	ArgumentsBytes     int    `json:"arguments_bytes,omitempty"`
	ArgumentsJSONValid bool   `json:"arguments_json_valid"`
	ArgumentsJSONError string `json:"arguments_json_error,omitempty"`
	ArgumentsHead      string `json:"arguments_head,omitempty"`
	ArgumentsTail      string `json:"arguments_tail,omitempty"`
	TargetSummary      string `json:"target_summary,omitempty"`
	ContentChars       int    `json:"content_chars,omitempty"`
	OldTextChars       int    `json:"old_text_chars,omitempty"`
	NewTextChars       int    `json:"new_text_chars,omitempty"`
	LooksLikeLongWrite bool   `json:"looks_like_long_write,omitempty"`
	LooksLikeLargeEdit bool   `json:"looks_like_large_edit,omitempty"`
	RecommendedTool    string `json:"recommended_tool,omitempty"`
}

func (s ProviderRequestSummary) JSON() string {
	raw, _ := json.Marshal(s)
	return string(raw)
}

type agentContextManager struct{}

func newAgentContextManager() *agentContextManager {
	return &agentContextManager{}
}

type ProviderMessageBuildOptions struct {
	ForceCompact   bool
	Reason         string
	Phase          string
	TargetBytes    int
	TriggerBytes   int
	TriggerEntries int
	PreserveRecent int
}

func (m *agentContextManager) Build(protocol agentProtocol, messages []store.Message, attachments []AttachmentPayload, transcript *AgentTranscript, profile providerRuntimeProfile, round int) ContextSnapshot {
	return m.BuildWithOptions(protocol, messages, attachments, transcript, profile, round, ProviderMessageBuildOptions{})
}

func (m *agentContextManager) BuildWithOptions(protocol agentProtocol, messages []store.Message, attachments []AttachmentPayload, transcript *AgentTranscript, profile providerRuntimeProfile, round int, options ProviderMessageBuildOptions) ContextSnapshot {
	options = normalizeProviderMessageBuildOptions(options)
	compacted, actions := m.compactTranscript(transcript, profile)
	snapshot := m.buildFromCompacted(protocol, messages, attachments, compacted, actions, profile, round)
	snapshot.Summary.ProviderMessagePhase = options.Phase
	payloadActions := m.compactTranscriptForPayloadPressure(compacted, profile, estimatedContextPayloadBytes(snapshot))
	if len(payloadActions) == 0 {
		return m.applyProviderMessageBuilder(protocol, messages, attachments, compacted, actions, profile, round, options, snapshot)
	}
	actions = append(actions, payloadActions...)
	snapshot = m.buildFromCompacted(protocol, messages, attachments, compacted, actions, profile, round)
	snapshot.Summary.ProviderMessagePhase = options.Phase
	return m.applyProviderMessageBuilder(protocol, messages, attachments, compacted, actions, profile, round, options, snapshot)
}

func (m *agentContextManager) applyProviderMessageBuilder(protocol agentProtocol, messages []store.Message, attachments []AttachmentPayload, transcript *AgentTranscript, actions []string, profile providerRuntimeProfile, round int, options ProviderMessageBuildOptions, snapshot ContextSnapshot) ContextSnapshot {
	builder := ProviderMessageBuilder{}
	result := builder.Build(ProviderMessageBuildInput{
		Protocol:      protocol,
		Messages:      messages,
		Attachments:   attachments,
		Transcript:    transcript,
		Actions:       actions,
		Profile:       profile,
		Round:         round,
		Options:       options,
		BeforePayload: estimatedContextPayloadBytes(snapshot),
	})
	if !result.Applied {
		return snapshot
	}
	next := m.buildFromCompacted(protocol, result.Messages, attachments, result.Transcript, result.Actions, profile, round)
	next.Summary.ProviderMessagePhase = result.Phase
	next.Summary.MessageBuilderApplied = true
	next.Summary.MessageBuilderStrategy = result.Strategy
	next.Summary.PayloadBeforeBytes = result.PayloadBeforeBytes
	next.Summary.PayloadAfterBytes = estimatedContextPayloadBytes(next)
	next.Summary.CompactedEntries = result.CompactedEntries
	next.Summary.PreservedRecentEntries = result.PreservedRecentEntries
	next.Summary.ContinuationSummaryChars = result.ContinuationSummaryChars
	next.Summary.PreservedChangedPaths = append([]string(nil), result.PreservedChangedPaths...)
	next.Summary.PreservedReadRanges = append([]string(nil), result.PreservedReadRanges...)
	next.Summary.PayloadCompactionApplied = true
	next.Summary.ProviderContextCompacted = true
	return next
}

func estimatedContextPayloadBytes(snapshot ContextSnapshot) int {
	messageBytes := 0
	switch {
	case len(snapshot.OpenAIMessages) > 0:
		raw, err := json.Marshal(snapshot.OpenAIMessages)
		if err != nil {
			return 0
		}
		messageBytes = len(raw)
	case len(snapshot.AnthropicMessages) > 0 || snapshot.AnthropicSystem != "":
		raw, err := json.Marshal(map[string]any{
			"system":   snapshot.AnthropicSystem,
			"messages": snapshot.AnthropicMessages,
		})
		if err != nil {
			return 0
		}
		messageBytes = len(raw)
	default:
		return 0
	}
	return messageBytes + 16*1024
}

func (m *agentContextManager) buildFromCompacted(protocol agentProtocol, messages []store.Message, attachments []AttachmentPayload, compacted *AgentTranscript, actions []string, profile providerRuntimeProfile, round int) ContextSnapshot {
	switch protocol {
	case agentProtocolAnthropic:
		systemText, runtimeMessages := buildAnthropicMessages(messages, attachments)
		items := append([]map[string]any{}, runtimeMessages...)
		items = append(items, anthropicTranscriptMessages(compacted.Entries)...)
		return ContextSnapshot{
			AnthropicSystem:   mergeToolSystemPrompt(systemText),
			AnthropicMessages: items,
			Transcript:        compacted,
			Summary:           summarizeAnthropicContext(items, compacted, actions, round, profile),
		}
	default:
		items := prependOpenAISystem(buildOpenAIMessages(messages, attachments))
		items = append(items, openAITranscriptMessages(compacted.Entries)...)
		return ContextSnapshot{
			OpenAIMessages: items,
			Transcript:     compacted,
			Summary:        summarizeOpenAIContext(items, compacted, actions, round, profile),
		}
	}
}
