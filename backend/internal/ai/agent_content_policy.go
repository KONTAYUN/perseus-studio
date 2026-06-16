package ai

import "strings"

type ContentOnlyCompletionPolicy struct {
	Accept bool   `json:"accept"`
	Reason string `json:"reason"`
}

func (r *agentRuntime) contentOnlyCompletionPolicy(turn agentModelTurn) ContentOnlyCompletionPolicy {
	if r == nil {
		return ContentOnlyCompletionPolicy{Accept: true, Reason: "chat"}
	}
	if strings.TrimSpace(turn.Content) == "" {
		return ContentOnlyCompletionPolicy{Accept: false, Reason: "empty_content"}
	}
	if !r.actionBoundaryActive() {
		return ContentOnlyCompletionPolicy{Accept: true, Reason: "chat_no_action_boundary"}
	}
	if strings.TrimSpace(r.opts.TaskID) != "" {
		return ContentOnlyCompletionPolicy{Accept: false, Reason: "background_task_requires_terminal_action"}
	}
	if strings.TrimSpace(r.opts.ForceTool) != "" {
		return ContentOnlyCompletionPolicy{Accept: false, Reason: "forced_tool_requires_action"}
	}
	if r.requiresCompletion {
		return ContentOnlyCompletionPolicy{Accept: false, Reason: "terminal_completion_required"}
	}
	if r.performedMutation || len(r.appliedMutations) > 0 {
		return ContentOnlyCompletionPolicy{Accept: false, Reason: "mutation_requires_terminal_completion"}
	}
	if r.lastToolFailed && r.lastToolFailure.Recoverable {
		return ContentOnlyCompletionPolicy{Accept: false, Reason: "recoverable_tool_failure_requires_action"}
	}
	if r.hasSuccessfulReadOnlyWorkspaceEvent() {
		if attachmentsRequireToolLoop(r.attachments) {
			return ContentOnlyCompletionPolicy{Accept: false, Reason: "selected_workspace_reference_requires_terminal_or_action"}
		}
		return ContentOnlyCompletionPolicy{Accept: true, Reason: "read_only_workspace_tool_completed"}
	}
	return ContentOnlyCompletionPolicy{Accept: false, Reason: "action_boundary_requires_action"}
}

func (r *agentRuntime) hasSuccessfulReadOnlyWorkspaceEvent() bool {
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
