package ai

import (
	"encoding/json"
	"sort"
	"strings"

	"webide-backend/internal/store"
)

type ContextSnapshot struct {
	OpenAIMessages    []map[string]any
	AnthropicSystem   string
	AnthropicMessages []map[string]any
	Summary           ProviderRequestSummary
}

type ProviderRequestSummary struct {
	Round                    int      `json:"round"`
	Protocol                 string   `json:"protocol"`
	MessageCount             int      `json:"message_count"`
	TranscriptEntries        int      `json:"transcript_entries"`
	ToolResultCount          int      `json:"tool_result_count"`
	ToolLoopActive           bool     `json:"tool_loop_active"`
	EstimatedChars           int      `json:"estimated_chars"`
	EstimatedToolResultChars int      `json:"estimated_tool_result_chars"`
	MaxSegmentChars          int      `json:"max_segment_chars"`
	CompactionActions        []string `json:"compaction_actions,omitempty"`
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
			Summary:           summarizeAnthropicContext(items, compacted, actions, round),
		}
	default:
		items := prependOpenAISystem(buildOpenAIMessages(messages, attachments))
		items = append(items, openAITranscriptMessages(compacted.Entries)...)
		return ContextSnapshot{
			OpenAIMessages: items,
			Summary:        summarizeOpenAIContext(items, compacted, actions, round),
		}
	}
}

func (m *agentContextManager) compactTranscript(transcript *AgentTranscript, profile providerRuntimeProfile) (*AgentTranscript, []string) {
	compacted := transcript.Clone()
	if compacted == nil {
		return newAgentTranscript(), nil
	}
	var actions []string
	actions = append(actions, compactSupersededReadResults(compacted)...)
	actions = append(actions, compactReadsStaleAfterMutation(compacted)...)
	return compacted, actions
}

func compactSupersededReadResults(transcript *AgentTranscript) []string {
	latestByPath := map[string]bool{}
	var actions []string
	for entryIndex := len(transcript.Entries) - 1; entryIndex >= 0; entryIndex-- {
		entry := &transcript.Entries[entryIndex]
		if entry.Kind != transcriptEntryTool {
			continue
		}
		for resultIndex := len(entry.ToolResults) - 1; resultIndex >= 0; resultIndex-- {
			result := &entry.ToolResults[resultIndex]
			if result.ToolName != "read_workspace_text" || result.IsError {
				continue
			}
			path := normalizeWorkspaceEventPath(result.Event.Path)
			if path == "" {
				continue
			}
			if latestByPath[path] {
				result.Content = compactedToolResultNotice(result.ToolName, result.Event, "A newer read of this file exists later in the transcript. Re-read this path only if exact text is needed again.")
				actions = append(actions, "superseded_read:"+path)
				continue
			}
			latestByPath[path] = true
		}
	}
	return dedupeStrings(actions)
}

func compactReadsStaleAfterMutation(transcript *AgentTranscript) []string {
	mutatedLater := map[string]bool{}
	var actions []string
	for entryIndex := len(transcript.Entries) - 1; entryIndex >= 0; entryIndex-- {
		entry := &transcript.Entries[entryIndex]
		if entry.Kind != transcriptEntryTool {
			continue
		}
		for resultIndex := len(entry.ToolResults) - 1; resultIndex >= 0; resultIndex-- {
			result := &entry.ToolResults[resultIndex]
			if isWorkspaceMutationTool(result.ToolName) && !result.IsError {
				for _, path := range workspaceEventPaths(result.Event) {
					mutatedLater[path] = true
				}
				continue
			}
			if result.ToolName != "read_workspace_text" || result.IsError {
				continue
			}
			path := normalizeWorkspaceEventPath(result.Event.Path)
			if path == "" || !mutatedLater[path] {
				continue
			}
			result.Content = compactedToolResultNotice(result.ToolName, result.Event, "This file was modified later in the transcript. Re-read this path if exact current text is needed again.")
			actions = append(actions, "stale_read_after_mutation:"+path)
		}
	}
	return dedupeStrings(actions)
}

func workspaceEventPaths(event ToolEvent) []string {
	seen := map[string]bool{}
	var paths []string
	for _, path := range append([]string{event.Path, event.NewPath}, event.ChangedPaths...) {
		normalized := normalizeWorkspaceEventPath(path)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		paths = append(paths, normalized)
	}
	return paths
}

func compactedToolResultNotice(toolName string, event ToolEvent, message string) string {
	payload := map[string]any{
		"status":        "compacted_context",
		"tool_name":     toolName,
		"path":          event.Path,
		"changed_paths": event.ChangedPaths,
		"message":       message,
	}
	return mustJSON(payload)
}

func openAITranscriptMessages(entries []TranscriptEntry) []map[string]any {
	items := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		switch entry.Kind {
		case transcriptEntryAssistant:
			message := openAIMessage{Role: "assistant"}
			if strings.TrimSpace(entry.Text) != "" {
				message.Content = entry.Text
			}
			if len(entry.ToolCalls) > 0 {
				for _, call := range entry.ToolCalls {
					message.ToolCalls = append(message.ToolCalls, openAIToolCall{
						ID:   call.ID,
						Type: "function",
						Function: openAIFunctionCall{
							Name:      call.Name,
							Arguments: json.RawMessage(strconvQuoteJSON(call.ArgumentsJSON)),
						},
					})
				}
			}
			items = append(items, message.toRuntimeMessage())
		case transcriptEntryTool:
			for _, result := range entry.ToolResults {
				items = append(items, map[string]any{
					"role":         "tool",
					"tool_call_id": result.ToolCallID,
					"content":      result.Content,
				})
			}
		case transcriptEntryUser:
			items = append(items, map[string]any{
				"role":    "user",
				"content": entry.Text,
			})
		}
	}
	return items
}

func anthropicTranscriptMessages(entries []TranscriptEntry) []map[string]any {
	items := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		switch entry.Kind {
		case transcriptEntryAssistant:
			content := make([]map[string]any, 0, 1+len(entry.ToolCalls))
			if strings.TrimSpace(entry.Text) != "" {
				content = append(content, map[string]any{"type": "text", "text": entry.Text})
			}
			for _, call := range entry.ToolCalls {
				input := cloneMapAny(call.AnthropicInput)
				if input == nil {
					var decoded map[string]any
					if err := json.Unmarshal([]byte(call.ArgumentsJSON), &decoded); err == nil {
						input = decoded
					}
				}
				if input == nil {
					input = map[string]any{}
				}
				content = append(content, map[string]any{"type": "tool_use", "id": call.ID, "name": call.Name, "input": input})
			}
			items = append(items, map[string]any{"role": "assistant", "content": content})
		case transcriptEntryTool:
			content := make([]map[string]any, 0, len(entry.ToolResults))
			for _, result := range entry.ToolResults {
				item := map[string]any{
					"type":        "tool_result",
					"tool_use_id": result.ToolCallID,
					"content":     result.Content,
				}
				if result.IsError {
					item["is_error"] = true
				}
				content = append(content, item)
			}
			items = append(items, map[string]any{"role": "user", "content": content})
		case transcriptEntryUser:
			items = append(items, map[string]any{"role": "user", "content": entry.Text})
		}
	}
	return items
}

func summarizeOpenAIContext(messages []map[string]any, transcript *AgentTranscript, actions []string, round int) ProviderRequestSummary {
	return summarizeProviderContext("openai", messages, transcript, actions, round)
}

func summarizeAnthropicContext(messages []map[string]any, transcript *AgentTranscript, actions []string, round int) ProviderRequestSummary {
	return summarizeProviderContext("anthropic", messages, transcript, actions, round)
}

func summarizeProviderContext(protocol string, messages []map[string]any, transcript *AgentTranscript, actions []string, round int) ProviderRequestSummary {
	summary := ProviderRequestSummary{
		Round:             round,
		Protocol:          protocol,
		MessageCount:      len(messages),
		TranscriptEntries: len(transcript.Entries),
		CompactionActions: dedupeStrings(actions),
	}
	for _, entry := range transcript.Entries {
		if entry.Kind == transcriptEntryTool {
			summary.ToolLoopActive = true
		}
		for _, result := range entry.ToolResults {
			summary.ToolResultCount++
			size := len([]rune(result.Content))
			summary.EstimatedToolResultChars += size
			if size > summary.MaxSegmentChars {
				summary.MaxSegmentChars = size
			}
		}
	}
	for _, item := range messages {
		size := estimateMessageChars(item)
		summary.EstimatedChars += size
		if size > summary.MaxSegmentChars {
			summary.MaxSegmentChars = size
		}
	}
	return summary
}

func estimateMessageChars(message map[string]any) int {
	raw, err := json.Marshal(message)
	if err != nil {
		return 0
	}
	return len([]rune(string(raw)))
}

func dedupeStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := map[string]bool{}
	next := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		next = append(next, item)
	}
	sort.Strings(next)
	return next
}

func strconvQuoteJSON(value string) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(raw)
}
