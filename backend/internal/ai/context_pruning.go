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
	keepCurrentRead := currentReadWorkingSetToolCalls(transcript, 1)
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
				if keepCurrentRead[result.ToolCallID] {
					continue
				}
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
			result.Content = compactedToolResultNoticeWithReason(result.ToolName, result.Event, "older_tool_result_reference", "reference_only", "Use preserved recent observations when available; call the relevant tool only when exact details are missing from current context.")
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
				result.Content = compactedToolResultNoticeWithReason(result.ToolName, result.Event, "superseded_by_newer_read", "reference_only", "A newer read of this file range exists later in the transcript. Use the newer observation; read this path/range only if exact text is still missing.")
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
				result.Content = compactedToolResultNoticeWithReason(result.ToolName, result.Event, "stale_after_later_mutation", "stale_after_later_mutation", "This file was modified later in the transcript. Read the path again before relying on exact current text.")
			}
		}
	}
	return dedupeStrings(actions)
}

func compactedToolResultNotice(toolName string, event ToolEvent, message string) string {
	return compactedToolResultNoticeWithReason(toolName, event, "older_observation_reference", "reference_only", message)
}

func compactedToolResultNoticeWithReason(toolName string, event ToolEvent, reasonCode, contentState, message string) string {
	payload := map[string]any{
		"status":            "context_reference",
		"tool_name":         toolName,
		"path":              event.Path,
		"changed_paths":     event.ChangedPaths,
		"content_state":     contentState,
		"reason_code":       reasonCode,
		"next_action":       message,
		"exact_text_needed": "call the relevant workspace tool for the specific path and line range",
	}
	if isReadWorkspaceTool(toolName) {
		payload["read_content_state"] = readContentStateForReference(contentState)
	}
	if event.ExecutionOutcome != "" {
		payload["execution_outcome"] = event.ExecutionOutcome
		payload["failure_class"] = event.FailureClass
		payload["exit_code"] = event.ExitCode
		payload["timed_out"] = event.TimedOut
	}
	return mustJSON(payload)
}

func readContentStateForReference(contentState string) string {
	switch contentState {
	case "stale_after_later_mutation":
		return string(ReadContentStateStaleAfterMutation)
	default:
		return string(ReadContentStateMetadataOnly)
	}
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
			result.Content = compactedToolResultNoticeWithReason(result.ToolName, result.Event, "budget_reference", "reference_only", "Use this historical tool metadata only; call the relevant tool only when exact details are missing from current context.")
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

func currentReadWorkingSetToolCalls(transcript *AgentTranscript, limit int) map[string]bool {
	workingSet := map[string]bool{}
	if transcript == nil || limit == 0 {
		return workingSet
	}
	current := currentSuccessfulReadToolCalls(transcript)
	for entryIndex := len(transcript.Entries) - 1; entryIndex >= 0 && (limit < 0 || len(workingSet) < limit); entryIndex-- {
		entry := transcript.Entries[entryIndex]
		if entry.Kind != transcriptEntryTool {
			continue
		}
		for resultIndex := len(entry.ToolResults) - 1; resultIndex >= 0 && (limit < 0 || len(workingSet) < limit); resultIndex-- {
			result := entry.ToolResults[resultIndex]
			if !isReadWorkspaceTool(result.ToolName) || result.IsError || !current[result.ToolCallID] {
				continue
			}
			if readToolResultContentBytes(result.Content) <= 0 {
				continue
			}
			workingSet[result.ToolCallID] = true
		}
	}
	return workingSet
}

func isReadWorkspaceTool(toolName string) bool {
	return toolName == "read_workspace_text" || toolName == "read_workspace_files"
}

func isCompactedToolResultContent(content string) bool {
	var payload struct {
		Status string `json:"status"`
	}
	return json.Unmarshal([]byte(content), &payload) == nil && (payload.Status == "compacted_context" || payload.Status == "context_reference")
}
