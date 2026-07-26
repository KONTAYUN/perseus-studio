package ai

import (
	"context"
	"sort"
	"strings"
)

func (r *agentRuntime) callModel(ctx context.Context, round int) (agentModelTurn, error) {
	return r.callModelForTurnEngine(ctx, round)
}

func loadedToolsetCapabilitiesForDiagnostics(loaded map[string]bool) map[string]any {
	if !loaded["workspace"] {
		return nil
	}
	return toolsetCapabilities("workspace")
}

func loadedToolsetNames(loaded map[string]bool) []string {
	names := make([]string, 0, len(loaded))
	for name, ok := range loaded {
		if ok && strings.TrimSpace(name) != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func (r *agentRuntime) currentModelActionManifest() ModelActionManifest {
	if r == nil {
		return modelActionManifest(nil, true, "admin")
	}
	manifest := modelActionManifest(r.loadedToolsets, r.includeToolsetLoader(), r.opts.UserRole)
	if manifest.WorkspaceState == nil {
		manifest.WorkspaceState = &ModelWorkspaceState{}
	}
	manifest.WorkspaceState.ActionState = r.actionSelectionState(nil)
	return manifest
}

func (r *agentRuntime) includeToolsetLoader() bool {
	return r != nil && r.opts.TaskID == ""
}

func (r *agentRuntime) toolChoice() providerToolChoice {
	return providerToolChoice{Mode: providerToolChoiceAuto}
}

func (r *agentRuntime) tools() []map[string]any {
	includeLoader := r.includeToolsetLoader()
	if r.protocol == agentProtocolAnthropic {
		return anthropicToolsForLoadedToolsets(r.loadedToolsets, includeLoader, r.opts.UserRole)
	}
	return openAIToolsForLoadedToolsets(r.loadedToolsets, includeLoader, r.opts.UserRole)
}

func (r *agentRuntime) actionBoundaryActive() bool {
	return r != nil
}

func (r *agentRuntime) executionMode() AgentExecutionMode {
	if r == nil {
		return AgentExecutionModeChat
	}
	if r.checkpointResumeAttempts > 0 {
		return AgentExecutionModeCheckpointResume
	}
	if r.toolCalls > 0 || strings.TrimSpace(r.opts.TaskID) != "" {
		return AgentExecutionModeToolLoop
	}
	if attachmentsIncludeWorkspaceReference(r.attachments) {
		return AgentExecutionModeWorkspaceAction
	}
	return AgentExecutionModeChat
}
