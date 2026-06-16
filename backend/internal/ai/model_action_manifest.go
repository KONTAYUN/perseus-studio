package ai

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

func injectModelActionManifest(protocol agentProtocol, snapshot ContextSnapshot, manifest ModelActionManifest) ContextSnapshot {
	manifest = manifestWithWorkspaceState(manifest, snapshot)
	content := modelActionManifestAnchor(manifest)
	if strings.TrimSpace(content) == "" {
		return snapshot
	}
	switch protocol {
	case agentProtocolAnthropic:
		if strings.TrimSpace(snapshot.AnthropicSystem) == "" {
			snapshot.AnthropicSystem = content
		} else {
			snapshot.AnthropicSystem = strings.TrimSpace(snapshot.AnthropicSystem) + "\n\n" + content
		}
	default:
		anchor := map[string]any{
			"role":    "system",
			"content": content,
			"name":    "perseus_action_manifest",
		}
		if len(snapshot.OpenAIMessages) == 0 {
			snapshot.OpenAIMessages = []map[string]any{anchor}
		} else {
			insertAt := 1
			if insertAt > len(snapshot.OpenAIMessages) {
				insertAt = len(snapshot.OpenAIMessages)
			}
			next := append([]map[string]any{}, snapshot.OpenAIMessages[:insertAt]...)
			next = append(next, anchor)
			next = append(next, snapshot.OpenAIMessages[insertAt:]...)
			snapshot.OpenAIMessages = next
		}
	}
	return snapshot
}

func manifestWithWorkspaceState(manifest ModelActionManifest, snapshot ContextSnapshot) ModelActionManifest {
	workingSet := currentReadWorkingSetFromTranscript(snapshot.Transcript, 4)
	if !workingSet.Available && (manifest.WorkspaceState == nil || !manifest.WorkspaceState.RecentWorkspaceReads.Available) {
		return manifest
	}
	state := ModelWorkspaceState{}
	if manifest.WorkspaceState != nil {
		state = *manifest.WorkspaceState
	}
	if workingSet.Available {
		state.CurrentReadWorkingSet = workingSet
	}
	manifest.WorkspaceState = &state
	return manifest
}

func modelActionManifestAnchor(manifest ModelActionManifest) string {
	raw, err := json.Marshal(manifest)
	if err != nil {
		return ""
	}
	return actionManifestAnchorPrefix + string(raw)
}

func modelActionManifestDigest(manifest ModelActionManifest) string {
	raw, _ := json.Marshal(manifest)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}

func modelTextActionNames(manifest ModelActionManifest) []string {
	if len(manifest.TextActions) == 0 {
		return nil
	}
	out := make([]string, 0, len(manifest.TextActions))
	for _, action := range manifest.TextActions {
		if strings.TrimSpace(action.Name) != "" {
			out = append(out, action.Name)
		}
	}
	return out
}

func modelJSONToolActionNames(manifest ModelActionManifest) []string {
	if len(manifest.JSONTools) == 0 {
		return nil
	}
	out := make([]string, 0, len(manifest.JSONTools))
	for _, action := range manifest.JSONTools {
		if strings.TrimSpace(action.Name) != "" {
			out = append(out, action.Name)
		}
	}
	return out
}

func modelTerminalActionNames(manifest ModelActionManifest) []string {
	if len(manifest.TerminalActions) == 0 {
		return nil
	}
	out := make([]string, 0, len(manifest.TerminalActions))
	for _, action := range manifest.TerminalActions {
		if strings.TrimSpace(action.Name) != "" {
			out = append(out, action.Name)
		}
	}
	return out
}
