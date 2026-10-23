package ai

import (
	"encoding/json"
	"strings"
)

func (r *agentRuntime) actionSelectionState(targetPaths []string) ActionSelectionState {
	state := ActionSelectionState{}
	if r == nil {
		return state
	}
	state.WorkspaceLoaded = normalizeModelActionLoadedToolsets(r.loadedToolsets)["workspace"]
	state.HasVerifiedMutation = r.hasSuccessfulWorkspaceMutation()
	state.VerifiedChangedPaths = r.verifiedWorkspaceChangedPaths()
	state.NativePatchToolAvailable = providerNativePatchEnabled(r.provider)
	state.DirectoryConditionsSatisfied = dedupeStrings(r.actionProgress.ConditionSatisfiedPaths)
	state.TargetPaths = normalizedProviderPaths(targetPaths)
	if len(state.TargetPaths) == 0 && r.lastToolFailure.Target != "" {
		state.TargetPaths = normalizedProviderPaths([]string{r.lastToolFailure.Target})
	}
	state.RequiresWorkspaceMutation = r.requiresWorkspaceMutationAction(state.TargetPaths)
	if r.lastToolFailed {
		state.LastFailureTool = r.lastToolFailure.ToolName
		state.LastFailureBlocking = r.lastToolFailure.Recoverable && !r.lastFailureConditionSatisfied()
	}
	return state
}

func (r *agentRuntime) requiresWorkspaceMutationAction(targetPaths []string) bool {
	if r == nil {
		return false
	}
	if r.performedMutation || len(r.appliedMutations) > 0 {
		return true
	}
	for _, path := range normalizedProviderPaths(targetPaths) {
		if path != "" {
			return true
		}
	}
	if r.lastToolFailed && r.lastToolFailure.Mutation {
		return true
	}
	return false
}

func (r *agentRuntime) recordConditionSatisfiedFromToolResult(call agentToolCall, event ToolEvent, result string) {
	if r == nil || event.Status == "success" || call.Name != "manage_workspace_path" {
		return
	}
	var payload struct {
		Reason    string `json:"reason"`
		Operation string `json:"operation"`
		Path      string `json:"path"`
		PathType  string `json:"path_type"`
	}
	_ = json.Unmarshal([]byte(result), &payload)
	if payload.Reason != "already_exists" || payload.Operation != "mkdir" || payload.PathType != "folder" {
		return
	}
	r.recordConditionSatisfiedPath(firstNonEmpty(payload.Path, event.Path))
}

func (r *agentRuntime) recordConditionSatisfiedFromToolError(toolName, argumentsJSON string, err error) {
	if r == nil || strings.TrimSpace(toolName) != "manage_workspace_path" {
		return
	}
	if toolFailureClass(toolName, err) != "workspace_path_already_exists" {
		return
	}
	var payload struct {
		Action    string `json:"action"`
		Operation string `json:"operation"`
		Path      string `json:"path"`
		PathType  string `json:"path_type"`
	}
	_ = json.Unmarshal([]byte(argumentsJSON), &payload)
	operation := firstNonEmpty(payload.Operation, payload.Action)
	if strings.TrimSpace(operation) != "mkdir" {
		return
	}
	if payload.PathType != "" && payload.PathType != "folder" {
		return
	}
	r.recordConditionSatisfiedPath(payload.Path)
}

func (r *agentRuntime) recordConditionSatisfiedPath(path string) {
	if r == nil {
		return
	}
	path = normalizeWorkspaceEventPath(path)
	if path == "" {
		return
	}
	r.actionProgress.ConditionSatisfiedPaths = dedupeStrings(append(r.actionProgress.ConditionSatisfiedPaths, path))
	r.actionProgress.LastProgressRound = r.currentRound
}

func (r *agentRuntime) lastFailureConditionSatisfied() bool {
	if r == nil || !r.lastToolFailed || r.lastToolFailure.ToolName != "manage_workspace_path" {
		return false
	}
	target := normalizeWorkspaceEventPath(r.lastToolFailure.Target)
	if target == "" {
		return false
	}
	for _, path := range r.actionProgress.ConditionSatisfiedPaths {
		if normalizeWorkspaceEventPath(path) == target {
			return true
		}
	}
	return false
}

func (r *agentRuntime) narrowedAllowedNextActions(state ActionSelectionState) []string {
	if state.RequiresWorkspaceMutation && !state.HasVerifiedMutation {
		actions := []string{"apply_workspace_patch", "ask_followup_question"}
		if !r.hasAnySuccessfulReadOrSearch() {
			actions = append([]string{"read_workspace_text", "read_workspace_files", "find_workspace_files", "search_workspace_text"}, actions...)
		}
		return dedupeStrings(actions)
	}
	return r.allowedNextActionsForBoundaryUnscoped()
}

func (r *agentRuntime) hasAnySuccessfulReadOrSearch() bool {
	if r == nil {
		return false
	}
	for _, event := range r.events {
		if event.Status != "success" {
			continue
		}
		switch event.ToolName {
		case "read_workspace_text", "read_workspace_files", "find_workspace_files", "search_workspace_text", "inspect_workspace":
			return true
		}
	}
	return false
}

func actionStateBlockedBy(state ActionSelectionState) []string {
	var blocked []string
	if state.LastFailureBlocking && strings.TrimSpace(state.LastFailureTool) != "" {
		blocked = append(blocked, "recoverable_failure:"+state.LastFailureTool)
	}
	if state.RequiresWorkspaceMutation && !state.HasVerifiedMutation {
		blocked = append(blocked, "missing_verified_workspace_mutation")
	}
	return blocked
}
