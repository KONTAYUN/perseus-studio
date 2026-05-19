package ai

import (
	"encoding/json"
	"sort"
)

func summarizeOpenAIContext(messages []map[string]any, transcript *AgentTranscript, actions []string, round int, profile providerRuntimeProfile) ProviderRequestSummary {
	return summarizeProviderContext("openai", messages, transcript, actions, round, profile)
}

func summarizeAnthropicContext(messages []map[string]any, transcript *AgentTranscript, actions []string, round int, profile providerRuntimeProfile) ProviderRequestSummary {
	return summarizeProviderContext("anthropic", messages, transcript, actions, round, profile)
}

func summarizeProviderContext(protocol string, messages []map[string]any, transcript *AgentTranscript, actions []string, round int, profile providerRuntimeProfile) ProviderRequestSummary {
	summary := ProviderRequestSummary{
		Round:                   round,
		Protocol:                protocol,
		MessageCount:            len(messages),
		TranscriptEntries:       len(transcript.Entries),
		ContextWindowTokens:     profile.ContextWindowTokens,
		ContextWatermarkPercent: 0,
		CompactReservedTokens:   profile.CompactReservedTokens,
		CompactionBudgetTokens:  contextBudgetTokens(profile),
		AutoCompactEnabled:      profile.AutoCompactEnabled,
		CompactionActions:       dedupeStrings(actions),
	}
	for _, entry := range transcript.Entries {
		if entry.Kind == transcriptEntryTool {
			summary.ToolLoopActive = true
		}
		for _, result := range entry.ToolResults {
			summary.ToolResultCount++
			size := len([]rune(result.Content))
			summary.EstimatedToolResultChars += size
			summary.EstimatedToolResultTokens += estimateTextTokens(result.Content)
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
	summary.EstimatedTokens = estimateRuntimeMessagesTokens(messages)
	if summary.ContextWindowTokens <= 0 {
		summary.ContextWindowTokens = defaultContextWindowTokens
	}
	summary.ContextWatermarkPercent = contextWatermarkPercent(summary.EstimatedTokens, summary.ContextWindowTokens)
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
