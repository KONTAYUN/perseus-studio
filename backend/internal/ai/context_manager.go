package ai

import (
	"encoding/json"

	"webide-backend/internal/store"
)

type ContextSnapshot struct {
	OpenAIMessages    []map[string]any
	AnthropicSystem   string
	AnthropicMessages []map[string]any
	Summary           ProviderRequestSummary
}

type ProviderRequestSummary struct {
	Round                     int                  `json:"round"`
	Protocol                  string               `json:"protocol"`
	RequestKind               string               `json:"request_kind,omitempty"`
	Stream                    *bool                `json:"stream,omitempty"`
	ToolChoice                string               `json:"tool_choice,omitempty"`
	RequiresTool              bool                 `json:"requires_tool,omitempty"`
	ThinkingMode              string               `json:"thinking_mode,omitempty"`
	ReasoningSupport          string               `json:"reasoning_support,omitempty"`
	ReasoningEffort           string               `json:"reasoning_effort,omitempty"`
	ReasoningProfileSource    string               `json:"reasoning_profile_source,omitempty"`
	ReasoningProfileLevels    []string             `json:"reasoning_profile_levels,omitempty"`
	ToolsCount                int                  `json:"tools_count,omitempty"`
	ToolNames                 []string             `json:"tool_names,omitempty"`
	ParallelToolCalls         *bool                `json:"parallel_tool_calls,omitempty"`
	PayloadKeys               []string             `json:"payload_keys,omitempty"`
	PayloadBytes              int                  `json:"payload_bytes,omitempty"`
	MessageShapes             []ProviderMessageLog `json:"message_shapes,omitempty"`
	MessageProtocolWarnings   []string             `json:"message_protocol_warnings,omitempty"`
	ToolSchemaDigest          string               `json:"tool_schema_digest,omitempty"`
	ToolSchemaWarnings        []string             `json:"tool_schema_warnings,omitempty"`
	ToolSchemaItems           []ProviderToolLog    `json:"tool_schema_items,omitempty"`
	UnsupportedOpenAIParams   []string             `json:"unsupported_openai_params,omitempty"`
	ProviderHost              string               `json:"provider_host,omitempty"`
	ModelID                   string               `json:"model_id,omitempty"`
	ProviderHTTPProtocol      string               `json:"provider_http_protocol,omitempty"`
	ProviderHTTP2Disabled     bool                 `json:"provider_http2_disabled,omitempty"`
	HasTools                  bool                 `json:"has_tools,omitempty"`
	HasRequiredToolChoice     bool                 `json:"has_required_tool_choice,omitempty"`
	HasReasoningParams        bool                 `json:"has_reasoning_params,omitempty"`
	OutputMaxTokens           int                  `json:"output_max_tokens,omitempty"`
	StreamToolArgsState       string               `json:"stream_tool_args_state,omitempty"`
	StreamFinishReason        string               `json:"stream_finish_reason,omitempty"`
	StreamUsage               ProviderUsage        `json:"stream_usage,omitempty"`
	StreamToolCalls           []StreamToolCallLog  `json:"stream_tool_calls,omitempty"`
	LoadedToolsets            []string             `json:"loaded_toolsets,omitempty"`
	ForcedTool                string               `json:"forced_tool,omitempty"`
	FailureClass              string               `json:"failure_class,omitempty"`
	MessageCount              int                  `json:"message_count"`
	TranscriptEntries         int                  `json:"transcript_entries"`
	ToolResultCount           int                  `json:"tool_result_count"`
	ToolLoopActive            bool                 `json:"tool_loop_active"`
	EstimatedChars            int                  `json:"estimated_chars"`
	EstimatedTokens           int                  `json:"estimated_tokens"`
	ContextWindowTokens       int                  `json:"context_window_tokens"`
	ContextWatermarkPercent   int                  `json:"context_watermark_percent"`
	CompactReservedTokens     int                  `json:"compact_reserved_tokens"`
	CompactionBudgetTokens    int                  `json:"compaction_budget_tokens"`
	AutoCompactEnabled        bool                 `json:"auto_compact_enabled"`
	EstimatedToolResultChars  int                  `json:"estimated_tool_result_chars"`
	EstimatedToolResultTokens int                  `json:"estimated_tool_result_tokens"`
	MaxSegmentChars           int                  `json:"max_segment_chars"`
	CompactionActions         []string             `json:"compaction_actions,omitempty"`
	ProviderAttempts          int                  `json:"provider_attempts,omitempty"`
	StreamFallbackUsed        bool                 `json:"stream_fallback_used,omitempty"`
	CheckpointResumeAttempt   int                  `json:"checkpoint_resume_attempt,omitempty"`
	CheckpointSeq             int64                `json:"checkpoint_seq,omitempty"`
	RecoverableTransientError bool                 `json:"recoverable_transient_error,omitempty"`
	BudgetReason              string               `json:"budget_reason,omitempty"`
	MaxRounds                 int                  `json:"max_rounds,omitempty"`
	CurrentRound              int                  `json:"current_round,omitempty"`
	MaxToolCalls              int                  `json:"max_tool_calls,omitempty"`
	CurrentToolCalls          int                  `json:"current_tool_calls,omitempty"`
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
	Index                int    `json:"index"`
	ID                   string `json:"id,omitempty"`
	Name                 string `json:"name,omitempty"`
	ArgumentsChars       int    `json:"arguments_chars,omitempty"`
	ArgumentsBytes       int    `json:"arguments_bytes,omitempty"`
	ArgumentsJSONValid   bool   `json:"arguments_json_valid"`
	ArgumentsJSONError   string `json:"arguments_json_error,omitempty"`
	ArgumentsHead        string `json:"arguments_head,omitempty"`
	ArgumentsTail        string `json:"arguments_tail,omitempty"`
	TargetSummary        string `json:"target_summary,omitempty"`
	ContentChars         int    `json:"content_chars,omitempty"`
	OldTextChars         int    `json:"old_text_chars,omitempty"`
	NewTextChars         int    `json:"new_text_chars,omitempty"`
	LooksLikeLongWrite   bool   `json:"looks_like_long_write,omitempty"`
	LooksLikeLargeEdit   bool   `json:"looks_like_large_edit,omitempty"`
	RecommendedTool      string `json:"recommended_tool,omitempty"`
	RecommendedChunkSize int    `json:"recommended_chunk_size,omitempty"`
}

func (s ProviderRequestSummary) JSON() string {
	raw, _ := json.Marshal(s)
	return string(raw)
}

type agentContextManager struct{}

func newAgentContextManager() *agentContextManager {
	return &agentContextManager{}
}

func (m *agentContextManager) Build(protocol agentProtocol, messages []store.Message, attachments []AttachmentPayload, transcript *AgentTranscript, profile providerRuntimeProfile, round int) ContextSnapshot {
	compacted, actions := m.compactTranscript(transcript, profile)
	switch protocol {
	case agentProtocolAnthropic:
		systemText, runtimeMessages := buildAnthropicMessages(messages, attachments)
		items := append([]map[string]any{}, runtimeMessages...)
		items = append(items, anthropicTranscriptMessages(compacted.Entries)...)
		return ContextSnapshot{
			AnthropicSystem:   mergeToolSystemPrompt(systemText),
			AnthropicMessages: items,
			Summary:           summarizeAnthropicContext(items, compacted, actions, round, profile),
		}
	default:
		items := prependOpenAISystem(buildOpenAIMessages(messages, attachments))
		items = append(items, openAITranscriptMessages(compacted.Entries)...)
		return ContextSnapshot{
			OpenAIMessages: items,
			Summary:        summarizeOpenAIContext(items, compacted, actions, round, profile),
		}
	}
}
