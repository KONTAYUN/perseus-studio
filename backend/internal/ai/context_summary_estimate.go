package ai

import (
	"encoding/json"
	"sort"
	"strings"
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
		ToolManifestDigest:      toolManifestDigest(),
		InternalEditProtocol:    string(ToolTransportAssistantTextPatch),
		PayloadSoftBudgetBytes:  profile.PayloadSoftBudgetBytes,
		PayloadHardBudgetBytes:  profile.PayloadHardBudgetBytes,
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
			charSize := len([]rune(result.Content))
			byteSize := len([]byte(result.Content))
			summary.EstimatedToolResultChars += charSize
			summary.EstimatedToolResultTokens += estimateTextTokens(result.Content)
			summary.ToolResultBytes += byteSize
			if isReadWorkspaceTool(result.ToolName) {
				summary.ReadContentBytes += readToolResultContentBytes(result.Content)
				if readToolResultSourceTruncated(result.Content) {
					summary.SourceReadTruncated = true
				}
			}
			summary.LargestToolResults = appendLargestToolResultSize(summary.LargestToolResults, ToolResultSizeLog{
				ToolName:     result.ToolName,
				Path:         normalizeWorkspaceEventPath(result.Event.Path),
				Bytes:        byteSize,
				ReadContent:  isReadWorkspaceTool(result.ToolName),
				Compacted:    isCompactedToolResultContent(result.Content),
				ContentChars: charSize,
			}, 5)
			if charSize > summary.MaxSegmentChars {
				summary.MaxSegmentChars = charSize
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
	if profile.PayloadHardBudgetBytes > 0 && summary.PayloadBytes > profile.PayloadHardBudgetBytes {
		summary.PayloadBudgetExceeded = true
	}
	for _, action := range actions {
		if strings.HasPrefix(action, "payload_pressure_") || strings.HasPrefix(action, "budget_tool_result_pruned:") {
			summary.PayloadCompactionApplied = true
			summary.ProviderContextCompacted = true
			break
		}
	}
	if summary.ContextWindowTokens <= 0 {
		summary.ContextWindowTokens = defaultContextWindowTokens
	}
	summary.ContextWatermarkPercent = contextWatermarkPercent(summary.EstimatedTokens, summary.ContextWindowTokens)
	return summary
}

func readToolResultSourceTruncated(raw string) bool {
	var payload struct {
		Truncated bool `json:"truncated"`
		Items     []struct {
			Truncated bool `json:"truncated"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return false
	}
	if payload.Truncated {
		return true
	}
	for _, item := range payload.Items {
		if item.Truncated {
			return true
		}
	}
	return false
}

func appendLargestToolResultSize(items []ToolResultSizeLog, item ToolResultSizeLog, limit int) []ToolResultSizeLog {
	if item.Bytes <= 0 {
		return items
	}
	items = append(items, item)
	sort.Slice(items, func(i, j int) bool {
		return items[i].Bytes > items[j].Bytes
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func readToolResultContentBytes(raw string) int {
	var payload struct {
		Content         string `json:"content"`
		NumberedContent string `json:"numbered_content"`
		Items           []struct {
			Content         string `json:"content"`
			NumberedContent string `json:"numbered_content"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return 0
	}
	total := len([]byte(payload.Content)) + len([]byte(payload.NumberedContent))
	for _, item := range payload.Items {
		total += len([]byte(item.Content)) + len([]byte(item.NumberedContent))
	}
	return total
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
