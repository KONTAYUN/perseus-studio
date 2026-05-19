package ai

import (
	"encoding/json"
	"sort"
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
			if !isReadWorkspaceTool(result.ToolName) || result.IsError {
				continue
			}
			paths := readResultPaths(result)
			if len(paths) == 0 {
				continue
			}
			allSuperseded := true
			for _, path := range paths {
				if !latestByPath[path] {
					allSuperseded = false
					latestByPath[path] = true
				}
			}
			if allSuperseded {
				result.Content = compactedToolResultNotice(result.ToolName, result.Event, "A newer read of this file exists later in the transcript. Re-read this path only if exact text is needed again.")
				actions = append(actions, "superseded_read:"+strings.Join(paths, ","))
			}
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

	keepLatestRead := latestSuccessfulReadPaths(transcript)
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
			if result.IsError || strings.Contains(result.Content, `"status":"compacted_context"`) {
				continue
			}
			path := normalizeWorkspaceEventPath(result.Event.Path)
			if isReadWorkspaceTool(result.ToolName) && isLatestReadResult(result, keepLatestRead) {
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

func latestSuccessfulReadPaths(transcript *AgentTranscript) map[string]string {
	latest := map[string]string{}
	if transcript == nil {
		return latest
	}
	for _, entry := range transcript.Entries {
		if entry.Kind != transcriptEntryTool {
			continue
		}
		for _, result := range entry.ToolResults {
			if !isReadWorkspaceTool(result.ToolName) || result.IsError {
				continue
			}
			for _, path := range readResultPaths(&result) {
				latest[path] = result.ToolCallID
			}
		}
	}
	return latest
}

func isReadWorkspaceTool(toolName string) bool {
	return toolName == "read_workspace_text" || toolName == "read_workspace_files"
}

func isLatestReadResult(result *TranscriptToolResult, latest map[string]string) bool {
	if result == nil {
		return false
	}
	paths := readResultPaths(result)
	if len(paths) == 0 {
		return false
	}
	for _, path := range paths {
		if latest[path] != result.ToolCallID {
			return false
		}
	}
	return true
}

func readResultPaths(result *TranscriptToolResult) []string {
	if result == nil {
		return nil
	}
	seen := map[string]bool{}
	var paths []string
	add := func(path string) {
		normalized := normalizeWorkspaceEventPath(path)
		if normalized == "" || seen[normalized] {
			return
		}
		seen[normalized] = true
		paths = append(paths, normalized)
	}
	if result.ToolName != "read_workspace_files" {
		for _, path := range workspaceEventPaths(result.Event) {
			add(path)
		}
	}
	var payload struct {
		Path  string   `json:"path"`
		Paths []string `json:"paths"`
		Items []struct {
			Path string `json:"path"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err == nil {
		add(payload.Path)
		for _, path := range payload.Paths {
			add(path)
		}
		for _, item := range payload.Items {
			add(item.Path)
		}
	}
	sort.Strings(paths)
	return paths
}
