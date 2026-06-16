package ai

import (
	"encoding/json"
	"fmt"
	"strings"

	"webide-backend/internal/store"
)

type ProviderUsage struct {
	InputTokens         int64 `json:"input_tokens,omitempty"`
	OutputTokens        int64 `json:"output_tokens,omitempty"`
	CacheCreationTokens int64 `json:"cache_creation_tokens,omitempty"`
	CacheReadTokens     int64 `json:"cache_read_tokens,omitempty"`
	TotalTokens         int64 `json:"total_tokens,omitempty"`
}

type transcriptEntryKind string

const (
	transcriptEntryAssistant          transcriptEntryKind = "assistant"
	transcriptEntryTool               transcriptEntryKind = "tool"
	transcriptEntryUser               transcriptEntryKind = "user"
	transcriptEntryRuntimeObservation transcriptEntryKind = "runtime_observation"
	transcriptEntryCompactionSummary  transcriptEntryKind = "compaction_summary"
)

type AgentTranscript struct {
	Entries []TranscriptEntry `json:"entries"`
}

type TranscriptEntry struct {
	Kind             transcriptEntryKind    `json:"kind"`
	Text             string                 `json:"text,omitempty"`
	ToolCalls        []agentToolCall        `json:"tool_calls,omitempty"`
	AnthropicContent []map[string]any       `json:"anthropic_content,omitempty"`
	ToolResults      []TranscriptToolResult `json:"tool_results,omitempty"`
	Usage            ProviderUsage          `json:"usage,omitempty"`
	Metadata         map[string]any         `json:"metadata,omitempty"`
}

type TranscriptToolResult struct {
	ToolCallID string    `json:"tool_call_id"`
	ToolName   string    `json:"tool_name"`
	Content    string    `json:"content"`
	IsError    bool      `json:"is_error"`
	Event      ToolEvent `json:"event"`
}

func newAgentTranscript() *AgentTranscript {
	return &AgentTranscript{Entries: make([]TranscriptEntry, 0, 8)}
}

func (t *AgentTranscript) AppendAssistantTurn(turn agentModelTurn) {
	if t == nil {
		return
	}
	entry := TranscriptEntry{
		Kind:             transcriptEntryAssistant,
		Text:             turn.Content,
		ToolCalls:        cloneTranscriptAgentToolCalls(turn.ToolCalls),
		AnthropicContent: cloneMapAnySlice(turn.AnthropicRaw),
		Usage:            turn.Usage,
	}
	if turn.FinishReason != "" {
		entry.Metadata = map[string]any{"finish_reason": turn.FinishReason}
	}
	t.Entries = append(t.Entries, entry)
}

func (t *AgentTranscript) AppendToolResults(results []TranscriptToolResult) {
	if t == nil || len(results) == 0 {
		return
	}
	entry := TranscriptEntry{
		Kind:        transcriptEntryTool,
		ToolResults: cloneTranscriptToolResults(results),
	}
	t.Entries = append(t.Entries, entry)
}

func (t *AgentTranscript) AppendUserText(text string, metadata map[string]any) {
	if t == nil {
		return
	}
	t.Entries = append(t.Entries, TranscriptEntry{
		Kind:     transcriptEntryUser,
		Text:     text,
		Metadata: cloneMapAny(metadata),
	})
}

func (t *AgentTranscript) AppendRuntimeObservation(text string, metadata map[string]any) {
	if t == nil {
		return
	}
	t.Entries = append(t.Entries, TranscriptEntry{
		Kind:     transcriptEntryRuntimeObservation,
		Text:     text,
		Metadata: cloneMapAny(metadata),
	})
}

func (t *AgentTranscript) AppendCompactionSummary(text string, metadata map[string]any) {
	if t == nil {
		return
	}
	t.Entries = append(t.Entries, TranscriptEntry{
		Kind:     transcriptEntryCompactionSummary,
		Text:     text,
		Metadata: cloneMapAny(metadata),
	})
}

func (t *AgentTranscript) Clone() *AgentTranscript {
	if t == nil {
		return newAgentTranscript()
	}
	next := newAgentTranscript()
	next.Entries = make([]TranscriptEntry, 0, len(t.Entries))
	for _, entry := range t.Entries {
		next.Entries = append(next.Entries, TranscriptEntry{
			Kind:             entry.Kind,
			Text:             entry.Text,
			ToolCalls:        cloneAgentToolCalls(entry.ToolCalls),
			AnthropicContent: cloneMapAnySlice(entry.AnthropicContent),
			ToolResults:      cloneTranscriptToolResults(entry.ToolResults),
			Usage:            entry.Usage,
			Metadata:         cloneMapAny(entry.Metadata),
		})
	}
	return next
}

func cloneAgentToolCalls(items []agentToolCall) []agentToolCall {
	if len(items) == 0 {
		return nil
	}
	next := make([]agentToolCall, 0, len(items))
	for _, item := range items {
		next = append(next, agentToolCall{
			ID:             item.ID,
			Name:           item.Name,
			ArgumentsJSON:  item.ArgumentsJSON,
			AnthropicInput: cloneMapAny(item.AnthropicInput),
			PatchText:      item.PatchText,
		})
	}
	return next
}

func cloneTranscriptAgentToolCalls(items []agentToolCall) []agentToolCall {
	if len(items) == 0 {
		return nil
	}
	next := make([]agentToolCall, 0, len(items))
	for _, item := range items {
		clone := agentToolCall{
			ID:             item.ID,
			Name:           item.Name,
			ArgumentsJSON:  item.ArgumentsJSON,
			AnthropicInput: cloneMapAny(item.AnthropicInput),
		}
		if item.Name == "apply_workspace_patch" && strings.TrimSpace(item.PatchText) != "" {
			clone.ArgumentsJSON = mustJSON(map[string]any{
				"patch":       "[assistant_text_patch captured by backend]",
				"transport":   "assistant_text_patch",
				"patch_chars": len([]rune(item.PatchText)),
			})
			clone.AnthropicInput = map[string]any{
				"patch":       "[assistant_text_patch captured by backend]",
				"transport":   "assistant_text_patch",
				"patch_chars": len([]rune(item.PatchText)),
			}
		}
		next = append(next, clone)
	}
	return next
}

func normalizeAgentToolCallIDs(items []agentToolCall) []agentToolCall {
	if len(items) == 0 {
		return nil
	}
	next := cloneAgentToolCalls(items)
	seen := map[string]bool{}
	for index := range next {
		id := strings.TrimSpace(next[index].ID)
		if id == "" || seen[id] {
			id = fmt.Sprintf("tool_%d", index+1)
			for seen[id] {
				id = fmt.Sprintf("tool_%d_%d", index+1, len(seen)+1)
			}
		}
		next[index].ID = id
		seen[id] = true
	}
	return next
}

func cloneTranscriptToolResults(items []TranscriptToolResult) []TranscriptToolResult {
	if len(items) == 0 {
		return nil
	}
	next := make([]TranscriptToolResult, 0, len(items))
	for _, item := range items {
		next = append(next, item)
	}
	return next
}

func cloneMapAny(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	raw, err := json.Marshal(input)
	if err != nil {
		next := make(map[string]any, len(input))
		for key, value := range input {
			next[key] = value
		}
		return next
	}
	var next map[string]any
	if err := json.Unmarshal(raw, &next); err != nil {
		return nil
	}
	return next
}

func cloneMapAnySlice(input []map[string]any) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	next := make([]map[string]any, 0, len(input))
	for _, item := range input {
		next = append(next, cloneMapAny(item))
	}
	return next
}

type AgentRuntimeCheckpoint struct {
	Transcript                *AgentTranscript                    `json:"transcript"`
	Events                    []ToolEvent                         `json:"events"`
	AppliedMutations          map[string]ToolEvent                `json:"applied_mutations"`
	ChunkWrites               map[string]workspaceChunkWriteState `json:"chunk_writes,omitempty"`
	PerformedMutation         bool                                `json:"performed_mutation"`
	LastToolFailed            bool                                `json:"last_tool_failed"`
	LastToolFailure           runtimeToolFailure                  `json:"last_tool_failure,omitempty"`
	InvalidTurns              int                                 `json:"invalid_turns"`
	ToolCalls                 int                                 `json:"tool_calls"`
	ConsecutiveToolErrors     int                                 `json:"consecutive_tool_errors"`
	RepeatedToolErrorKey      string                              `json:"repeated_tool_error_key,omitempty"`
	RepeatedToolErrorCount    int                                 `json:"repeated_tool_error_count,omitempty"`
	LoadedToolsets            map[string]bool                     `json:"loaded_toolsets"`
	RequiresCompletion        bool                                `json:"requires_completion"`
	ToolLoopActive            bool                                `json:"tool_loop_active"`
	LastProviderFinishReason  string                              `json:"last_provider_finish_reason"`
	LastProviderEmptyResponse bool                                `json:"last_provider_empty_response"`
	CompletedContent          string                              `json:"completed_content"`
	CurrentRound              int                                 `json:"current_round"`
	Seq                       int64                               `json:"seq,omitempty"`
}

func (r *agentRuntime) checkpoint() AgentRuntimeCheckpoint {
	checkpoint := AgentRuntimeCheckpoint{
		Transcript:                r.transcript.Clone(),
		Events:                    append([]ToolEvent(nil), r.events...),
		AppliedMutations:          map[string]ToolEvent{},
		ChunkWrites:               map[string]workspaceChunkWriteState{},
		PerformedMutation:         r.performedMutation,
		LastToolFailed:            r.lastToolFailed,
		LastToolFailure:           r.lastToolFailure,
		InvalidTurns:              r.invalidTurns,
		ToolCalls:                 r.toolCalls,
		ConsecutiveToolErrors:     r.consecutiveToolErrors,
		RepeatedToolErrorKey:      r.repeatedToolErrorKey,
		RepeatedToolErrorCount:    r.repeatedToolErrorCount,
		LoadedToolsets:            map[string]bool{},
		RequiresCompletion:        r.requiresCompletion,
		ToolLoopActive:            r.toolLoopActive,
		LastProviderFinishReason:  r.lastProviderFinishReason,
		LastProviderEmptyResponse: r.lastProviderEmptyResponse,
		CompletedContent:          r.completedContent,
		CurrentRound:              r.currentRound,
	}
	for key, value := range r.appliedMutations {
		checkpoint.AppliedMutations[key] = value
	}
	for key, value := range r.chunkWrites {
		checkpoint.ChunkWrites[key] = value
	}
	for key, value := range r.loadedToolsets {
		checkpoint.LoadedToolsets[key] = value
	}
	return checkpoint
}

func (r *agentRuntime) restoreCheckpoint(checkpoint AgentRuntimeCheckpoint) {
	r.transcript = checkpoint.Transcript.Clone()
	r.events = append([]ToolEvent(nil), checkpoint.Events...)
	r.appliedMutations = map[string]ToolEvent{}
	for key, value := range checkpoint.AppliedMutations {
		r.appliedMutations[key] = value
	}
	r.chunkWrites = map[string]workspaceChunkWriteState{}
	for key, value := range checkpoint.ChunkWrites {
		r.chunkWrites[key] = value
	}
	r.performedMutation = checkpoint.PerformedMutation
	r.lastToolFailed = checkpoint.LastToolFailed
	r.lastToolFailure = checkpoint.LastToolFailure
	r.invalidTurns = checkpoint.InvalidTurns
	r.toolCalls = checkpoint.ToolCalls
	r.consecutiveToolErrors = checkpoint.ConsecutiveToolErrors
	r.repeatedToolErrorKey = checkpoint.RepeatedToolErrorKey
	r.repeatedToolErrorCount = checkpoint.RepeatedToolErrorCount
	r.loadedToolsets = map[string]bool{}
	for key, value := range checkpoint.LoadedToolsets {
		r.loadedToolsets[key] = value
	}
	r.requiresCompletion = checkpoint.RequiresCompletion
	r.toolLoopActive = checkpoint.ToolLoopActive || checkpoint.ToolCalls > 0 || checkpoint.RequiresCompletion
	r.lastProviderFinishReason = checkpoint.LastProviderFinishReason
	r.lastProviderEmptyResponse = checkpoint.LastProviderEmptyResponse
	r.completedContent = checkpoint.CompletedContent
	r.currentRound = checkpoint.CurrentRound
}

func initialRuntimeCheckpoint(messages []store.Message, attachments []AttachmentPayload, allowTask bool, requiresCompletion bool) AgentRuntimeCheckpoint {
	return AgentRuntimeCheckpoint{
		Transcript:         newAgentTranscript(),
		AppliedMutations:   map[string]ToolEvent{},
		ChunkWrites:        map[string]workspaceChunkWriteState{},
		LoadedToolsets:     allAgentToolsets(allowTask),
		RequiresCompletion: requiresCompletion,
		ToolLoopActive:     requiresCompletion,
	}
}
