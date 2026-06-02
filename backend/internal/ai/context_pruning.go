package ai

import (
	"encoding/json"
	"strings"
)

func (m *agentContextManager) compactTranscript(transcript *AgentTranscript, profile providerRuntimeProfile) (*AgentTranscript, []string) {
	compacted := transcript.Clone()
	if compacted == nil {
		return newAgentTranscript(), nil
	}
	var actions []string
	actions = append(actions, compactSupersededReadResults(compacted)...)
	actions = append(actions, compactReadsStaleAfterMutation(compacted)...)
	actions = append(actions, compactToolResultsForBudget(compacted, profile)...)
	return compacted, actions
}

func (m *agentContextManager) compactTranscriptForPayloadPressure(transcript *AgentTranscript, profile providerRuntimeProfile, currentBytes int) []string {
	if transcript == nil || profile.PayloadSoftBudgetBytes <= 0 || currentBytes <= profile.PayloadSoftBudgetBytes {
		return nil
	}
	target := profile.PayloadSoftBudgetBytes
	if profile.PayloadHardBudgetBytes > 0 && currentBytes > profile.PayloadHardBudgetBytes {
		target = profile.PayloadSoftBudgetBytes * 85 / 100
	}
	current := currentBytes
	var actions []string
	recentEntryFloor := len(transcript.Entries) - 4
	if recentEntryFloor < 0 {
		recentEntryFloor = 0
	}
	for entryIndex := 0; entryIndex < len(transcript.Entries) && current > target; entryIndex++ {
		entry := &transcript.Entries[entryIndex]
		if entry.Kind != transcriptEntryTool {
			continue
		}
		for resultIndex := range entry.ToolResults {
			result := &entry.ToolResults[resultIndex]
			if result.IsError || isCompactedToolResultContent(result.Content) {
				continue
			}
			if isReadWorkspaceTool(result.ToolName) {
				before := len([]byte(result.Content))
				if compactReadResultForPayload(result) {
					after := len([]byte(result.Content))
					current -= before - after
					action := "payload_pressure_read_compacted:" + result.ToolName
					if path := normalizeWorkspaceEventPath(result.Event.Path); path != "" {
						action += ":" + path
					}
					actions = append(actions, action)
				}
				if current <= target {
					break
				}
				continue
			}
			if entryIndex >= recentEntryFloor {
				continue
			}
			before := len([]byte(result.Content))
			result.Content = compactedToolResultNotice(result.ToolName, result.Event, "Older successful tool output was compacted because the provider request payload was too large. Re-run or re-read only if exact details are needed.")
			after := len([]byte(result.Content))
			current -= before - after
			action := "payload_pressure_tool_compacted:" + result.ToolName
			if path := normalizeWorkspaceEventPath(result.Event.Path); path != "" {
				action += ":" + path
			}
			actions = append(actions, action)
			if current <= target {
				break
			}
		}
	}
	return dedupeStrings(actions)
}

func compactSupersededReadResults(transcript *AgentTranscript) []string {
	var newerSpans []readResultSpan
	var actions []string
	for entryIndex := len(transcript.Entries) - 1; entryIndex >= 0; entryIndex-- {
		entry := &transcript.Entries[entryIndex]
		if entry.Kind != transcriptEntryTool {
			continue
		}
		for resultIndex := len(entry.ToolResults) - 1; resultIndex >= 0; resultIndex-- {
			result := &entry.ToolResults[resultIndex]
			if !isReadWorkspaceTool(result.ToolName) || result.IsError {
				continue
			}
			spans := readResultSpans(result)
			if len(spans) == 0 {
				continue
			}
			allSuperseded := true
			for _, span := range spans {
				if !readSpanSupersededByAny(span, newerSpans) {
					allSuperseded = false
				}
			}
			if allSuperseded {
				result.Content = compactedToolResultNotice(result.ToolName, result.Event, "A newer read of this file range exists later in the transcript. Re-read this path/range only if exact text is needed again.")
				actions = append(actions, "superseded_read:"+strings.Join(readSpanLabels(spans), ","))
			}
			newerSpans = append(newerSpans, spans...)
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
			if !isReadWorkspaceTool(result.ToolName) || result.IsError {
				continue
			}
			if result.ToolName == "read_workspace_files" {
				stale, stalePaths := compactBatchReadItemsStaleAfterMutation(result, mutatedLater)
				if stale {
					for _, path := range stalePaths {
						actions = append(actions, "stale_read_after_mutation:"+path)
					}
				}
				continue
			}
			paths := readResultPaths(result)
			if len(paths) == 0 {
				continue
			}
			stale := false
			for _, path := range paths {
				if mutatedLater[path] {
					stale = true
					actions = append(actions, "stale_read_after_mutation:"+path)
				}
			}
			if stale {
				result.Content = compactedToolResultNotice(result.ToolName, result.Event, "This file was modified later in the transcript. Re-read this path if exact current text is needed again.")
			}
		}
	}
	return dedupeStrings(actions)
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

func compactToolResultsForBudget(transcript *AgentTranscript, profile providerRuntimeProfile) []string {
	if transcript == nil || !profile.PruneToolOutputsEnabled {
		return nil
	}
	budget := contextBudgetTokens(profile)
	if budget <= 0 {
		return nil
	}
	current := estimateTranscriptTokens(transcript)
	if !profile.AggressiveContextPruning && current < budget {
		return nil
	}
	target := budget - profile.CompactReservedTokens/2
	if target <= 0 || profile.AggressiveContextPruning {
		target = budget * 75 / 100
	}
	if profile.AggressiveContextPruning && current < target {
		target = current * 80 / 100
	}
	if target <= 0 {
		target = budget
	}

	keepCurrentRead := currentSuccessfulReadToolCalls(transcript)
	recentEntryFloor := len(transcript.Entries) - 6
	if recentEntryFloor < 0 {
		recentEntryFloor = 0
	}
	var actions []string
	for entryIndex := 0; entryIndex < len(transcript.Entries) && current > target; entryIndex++ {
		entry := &transcript.Entries[entryIndex]
		if entry.Kind != transcriptEntryTool || entryIndex >= recentEntryFloor {
			continue
		}
		for resultIndex := range entry.ToolResults {
			result := &entry.ToolResults[resultIndex]
			if result.IsError || isCompactedToolResultContent(result.Content) {
				continue
			}
			path := normalizeWorkspaceEventPath(result.Event.Path)
			if isReadWorkspaceTool(result.ToolName) && keepCurrentRead[result.ToolCallID] {
				continue
			}
			before := estimateTextTokens(result.Content)
			result.Content = compactedToolResultNotice(result.ToolName, result.Event, "Older successful tool output was pruned from model context to preserve the context window. Re-run or re-read only if exact details are needed.")
			after := estimateTextTokens(result.Content)
			current -= before - after
			actionKey := result.ToolName
			if path != "" {
				actionKey += ":" + path
			}
			actions = append(actions, "budget_tool_result_pruned:"+actionKey)
			if current <= target {
				break
			}
		}
	}
	return dedupeStrings(actions)
}

func estimateTranscriptTokens(transcript *AgentTranscript) int {
	if transcript == nil {
		return 0
	}
	total := 0
	for _, entry := range transcript.Entries {
		total += estimateTextTokens(entry.Text)
		for _, call := range entry.ToolCalls {
			total += estimateTextTokens(call.Name) + estimateTextTokens(call.ArgumentsJSON) + 4
		}
		for _, result := range entry.ToolResults {
			total += estimateTextTokens(result.ToolName) + estimateTextTokens(result.Content) + 4
		}
	}
	return total
}

func currentSuccessfulReadToolCalls(transcript *AgentTranscript) map[string]bool {
	current := map[string]bool{}
	if transcript == nil {
		return current
	}
	var newerSpans []readResultSpan
	for entryIndex := len(transcript.Entries) - 1; entryIndex >= 0; entryIndex-- {
		entry := transcript.Entries[entryIndex]
		if entry.Kind != transcriptEntryTool {
			continue
		}
		for resultIndex := len(entry.ToolResults) - 1; resultIndex >= 0; resultIndex-- {
			result := entry.ToolResults[resultIndex]
			if !isReadWorkspaceTool(result.ToolName) || result.IsError {
				continue
			}
			spans := readResultSpans(&result)
			if len(spans) == 0 {
				continue
			}
			for _, span := range spans {
				if !readSpanSupersededByAny(span, newerSpans) {
					current[result.ToolCallID] = true
					break
				}
			}
			newerSpans = append(newerSpans, spans...)
		}
	}
	return current
}

func isReadWorkspaceTool(toolName string) bool {
	return toolName == "read_workspace_text" || toolName == "read_workspace_files"
}

func isCompactedToolResultContent(content string) bool {
	var payload struct {
		Status string `json:"status"`
	}
	return json.Unmarshal([]byte(content), &payload) == nil && payload.Status == "compacted_context"
}
