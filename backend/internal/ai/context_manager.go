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
	Round                     int      `json:"round"`
	Protocol                  string   `json:"protocol"`
	RequestKind               string   `json:"request_kind,omitempty"`
	Stream                    *bool    `json:"stream,omitempty"`
	ToolChoice                string   `json:"tool_choice,omitempty"`
	RequiresTool              bool     `json:"requires_tool,omitempty"`
	ThinkingMode              string   `json:"thinking_mode,omitempty"`
	ReasoningSupport          string   `json:"reasoning_support,omitempty"`
	ReasoningEffort           string   `json:"reasoning_effort,omitempty"`
	ReasoningProfileSource    string   `json:"reasoning_profile_source,omitempty"`
	ReasoningProfileLevels    []string `json:"reasoning_profile_levels,omitempty"`
	ToolsCount                int      `json:"tools_count,omitempty"`
	ToolNames                 []string `json:"tool_names,omitempty"`
	ParallelToolCalls         *bool    `json:"parallel_tool_calls,omitempty"`
	PayloadKeys               []string `json:"payload_keys,omitempty"`
	ToolSchemaDigest          string   `json:"tool_schema_digest,omitempty"`
	ToolSchemaWarnings        []string `json:"tool_schema_warnings,omitempty"`
	UnsupportedOpenAIParams   []string `json:"unsupported_openai_params,omitempty"`
	OutputMaxTokens           int      `json:"output_max_tokens,omitempty"`
	StreamToolArgsState       string   `json:"stream_tool_args_state,omitempty"`
	LoadedToolsets            []string `json:"loaded_toolsets,omitempty"`
	ForcedTool                string   `json:"forced_tool,omitempty"`
	FailureClass              string   `json:"failure_class,omitempty"`
	MessageCount              int      `json:"message_count"`
	TranscriptEntries         int      `json:"transcript_entries"`
	ToolResultCount           int      `json:"tool_result_count"`
	ToolLoopActive            bool     `json:"tool_loop_active"`
	EstimatedChars            int      `json:"estimated_chars"`
	EstimatedTokens           int      `json:"estimated_tokens"`
	ContextWindowTokens       int      `json:"context_window_tokens"`
	ContextWatermarkPercent   int      `json:"context_watermark_percent"`
	CompactReservedTokens     int      `json:"compact_reserved_tokens"`
	CompactionBudgetTokens    int      `json:"compaction_budget_tokens"`
	AutoCompactEnabled        bool     `json:"auto_compact_enabled"`
	EstimatedToolResultChars  int      `json:"estimated_tool_result_chars"`
	EstimatedToolResultTokens int      `json:"estimated_tool_result_tokens"`
	MaxSegmentChars           int      `json:"max_segment_chars"`
	CompactionActions         []string `json:"compaction_actions,omitempty"`
	ProviderAttempts          int      `json:"provider_attempts,omitempty"`
	StreamFallbackUsed        bool     `json:"stream_fallback_used,omitempty"`
	CheckpointResumeAttempt   int      `json:"checkpoint_resume_attempt,omitempty"`
	CheckpointSeq             int64    `json:"checkpoint_seq,omitempty"`
	RecoverableTransientError bool     `json:"recoverable_transient_error,omitempty"`
	BudgetReason              string   `json:"budget_reason,omitempty"`
	MaxRounds                 int      `json:"max_rounds,omitempty"`
	CurrentRound              int      `json:"current_round,omitempty"`
	MaxToolCalls              int      `json:"max_tool_calls,omitempty"`
	CurrentToolCalls          int      `json:"current_tool_calls,omitempty"`
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
