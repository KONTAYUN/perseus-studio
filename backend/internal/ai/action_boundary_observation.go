package ai

import (
	"encoding/json"
	"strings"
)

type ActionBoundaryObservation struct {
	Status               string              `json:"status"`
	Reason               string              `json:"reason"`
	ReasonCode           string              `json:"reason_code,omitempty"`
	RejectedAction       string              `json:"rejected_action,omitempty"`
	AllowedNextActions   []string            `json:"allowed_next_actions,omitempty"`
	RequiredNextAction   string              `json:"required_next_action,omitempty"`
	TargetPaths          []string            `json:"target_paths,omitempty"`
	VerifiedChangedPaths []string            `json:"verified_changed_paths,omitempty"`
	LastFailure          *runtimeToolFailure `json:"last_failure,omitempty"`
	Message              string              `json:"message"`
}

type WorkspaceEditActionState struct {
	WorkspaceLoaded        bool     `json:"workspace_loaded"`
	HasVerifiedMutation    bool     `json:"has_verified_mutation"`
	VerifiedChangedPaths   []string `json:"verified_changed_paths,omitempty"`
	LastFailureRecoverable bool     `json:"last_failure_recoverable,omitempty"`
	LastFailureTool        string   `json:"last_failure_tool,omitempty"`
}

func (r *agentRuntime) actionRejectedObservation(reason, rejectedAction string, targetPaths []string) string {
	observation := ActionBoundaryObservation{
		Status:               "action_rejected",
		Reason:               strings.TrimSpace(reason),
		ReasonCode:           actionRejectedReasonCode(reason, rejectedAction),
		RejectedAction:       strings.TrimSpace(rejectedAction),
		AllowedNextActions:   r.allowedNextActionsForBoundary(),
		RequiredNextAction:   r.requiredNextActionForRejectedBoundary(rejectedAction, targetPaths),
		TargetPaths:          normalizedProviderPaths(targetPaths),
		VerifiedChangedPaths: r.verifiedWorkspaceChangedPaths(),
		Message:              "The previous response did not execute an accepted action. Continue with one of the allowed next actions. Do not claim completion until an action succeeds and terminal completion is accepted.",
	}
	if r != nil && r.lastToolFailed {
		failure := r.lastToolFailure
		observation.LastFailure = &failure
	}
	payload := map[string]any{
		"observation":           observation,
		"workspace_edit_state":  r.workspaceEditActionState(),
		"model_action_manifest": r.currentModelActionManifest(),
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

func isActionBoundaryObservation(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" {
		return false
	}
	var payload struct {
		Observation struct {
			Status string `json:"status"`
		} `json:"observation"`
	}
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return false
	}
	return payload.Observation.Status == "action_rejected"
}

func (r *agentRuntime) requiredNextActionForRejectedBoundary(rejectedAction string, targetPaths []string) string {
	if r == nil {
		return ""
	}
	if len(normalizedProviderPaths(targetPaths)) > 0 {
		return "apply_workspace_patch"
	}
	if r.lastToolFailed && r.lastToolFailure.Recoverable && isWorkspaceMutationTool(r.lastToolFailure.ToolName) {
		return "apply_workspace_patch"
	}
	if r.lastToolFailed && r.lastToolFailure.Recoverable && r.lastToolFailure.ToolName == "apply_workspace_patch" {
		return "apply_workspace_patch"
	}
	return ""
}

func (r *agentRuntime) allowedNextActionsForBoundary() []string {
	if r == nil {
		return nil
	}
	actions := []string{}
	manifest := r.currentModelActionManifest()
	for _, action := range manifest.JSONTools {
		if strings.TrimSpace(action.Name) != "" {
			actions = append(actions, action.Name)
		}
	}
	for _, action := range manifest.TextActions {
		if strings.TrimSpace(action.Name) != "" {
			actions = append(actions, action.Name)
		}
	}
	for _, action := range manifest.TerminalActions {
		if strings.TrimSpace(action.Name) != "" {
			actions = append(actions, action.Name)
		}
	}
	return dedupeStrings(actions)
}

func actionRejectedReasonCode(reason, rejectedAction string) string {
	action := strings.TrimSpace(rejectedAction)
	switch action {
	case "attempt_completion":
		return "terminal_action_rejected"
	case "assistant_text":
		return "content_without_action"
	case "empty_response":
		return "empty_response"
	default:
		if action != "" {
			return "action_rejected"
		}
	}
	return "action_boundary_rejected"
}

func (r *agentRuntime) workspaceEditActionState() WorkspaceEditActionState {
	state := WorkspaceEditActionState{}
	if r == nil {
		return state
	}
	state.WorkspaceLoaded = normalizeModelActionLoadedToolsets(r.loadedToolsets)["workspace"]
	state.HasVerifiedMutation = r.hasSuccessfulWorkspaceMutation()
	state.VerifiedChangedPaths = r.verifiedWorkspaceChangedPaths()
	if r.lastToolFailed {
		state.LastFailureRecoverable = r.lastToolFailure.Recoverable
		state.LastFailureTool = r.lastToolFailure.ToolName
	}
	return state
}

func (r *agentRuntime) verifiedWorkspaceChangedPaths() []string {
	if r == nil {
		return nil
	}
	paths := map[string]bool{}
	for _, event := range r.events {
		if event.Status != "success" || !isWorkspaceMutationTool(event.ToolName) {
			continue
		}
		for _, path := range workspaceEventPaths(event) {
			if strings.TrimSpace(path) != "" {
				paths[path] = true
			}
		}
	}
	return sortedBoolMapKeys(paths)
}
