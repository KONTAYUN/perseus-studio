package ai

import (
	"encoding/json"
	"fmt"
	"strings"
)

func (r *agentRuntime) validateSelectedReferenceScope(call agentToolCall) string {
	if r == nil {
		return ""
	}
	scope := selectedWorkspaceReferenceScope(r.attachments)
	if !scope.active() {
		return ""
	}
	candidates, enforce := scopedToolCandidatePaths(call.Name, call.ArgumentsJSON)
	if !enforce || len(candidates) == 0 {
		return ""
	}
	for _, candidate := range candidates {
		if call.Name == "write_workspace_text" && r.pathChangedEarlierInRun(candidate) {
			continue
		}
		if !scope.allows(candidate) {
			return fmt.Sprintf("tool target %q is outside the user's selected workspace file references. Treat phrases like “这几集/这些文件/选中的” as the selected references only. If you need to include this extra file or a whole folder/series, call ask_followup_question to confirm before continuing.", candidate)
		}
	}
	return ""
}

func (r *agentRuntime) pathChangedEarlierInRun(path string) bool {
	if r == nil {
		return false
	}
	normalized := normalizeWorkspaceEventPath(path)
	if normalized == "" {
		return false
	}
	for _, event := range r.events {
		if event.Status != "success" || !isWorkspaceMutationTool(event.ToolName) {
			continue
		}
		for _, candidate := range append([]string{event.Path, event.NewPath}, event.ChangedPaths...) {
			if normalizeWorkspaceEventPath(candidate) == normalized {
				return true
			}
		}
	}
	return false
}

type selectedReferenceScope struct {
	allowedPaths map[string]bool
}

func selectedWorkspaceReferenceScope(attachments []AttachmentPayload) selectedReferenceScope {
	scope := selectedReferenceScope{
		allowedPaths: map[string]bool{},
	}
	selectedReferenceAttachmentKinds := map[string]bool{"workspace_file": true}
	for _, attachment := range attachments {
		if !selectedReferenceAttachmentKinds[attachment.Kind] {
			continue
		}
		addScopePath(scope, attachment.Path)
	}
	return scope
}

func (s selectedReferenceScope) active() bool {
	return len(s.allowedPaths) > 0
}

func addScopePath(scope selectedReferenceScope, path string) {
	normalized := normalizeWorkspaceEventPath(path)
	if normalized == "" {
		return
	}
	scope.allowedPaths[normalized] = true
}

func (s selectedReferenceScope) allows(path string) bool {
	normalized := normalizeWorkspaceEventPath(path)
	if normalized == "" {
		return true
	}
	return s.allowedPaths[normalized]
}

func scopedToolCandidatePaths(toolName, argumentsJSON string) ([]string, bool) {
	extractor, ok := selectedScopeToolExtractors[toolName]
	if !ok {
		return nil, false
	}
	return extractor(argumentsJSON)
}

var selectedScopeToolExtractors = map[string]func(string) ([]string, bool){
	"read_workspace_text":        extractSinglePathScopeTarget,
	"edit_workspace_lines":       extractSinglePathScopeTarget,
	"write_workspace_text":       extractSaveScopeTarget,
	"read_workspace_files":       extractBatchReadScopeTargets,
	"manage_workspace_path":      extractManagePathScopeTarget,
	"organize_workspace_matches": extractOrganizeScopeTarget,
}

func extractSinglePathScopeTarget(argumentsJSON string) ([]string, bool) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return nil, false
	}
	return nonEmptyPaths(args.Path), true
}

func extractBatchReadScopeTargets(argumentsJSON string) ([]string, bool) {
	var args struct {
		Paths []string `json:"paths"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return nil, false
	}
	return normalizeCandidatePaths(args.Paths...), true
}

func extractSaveScopeTarget(argumentsJSON string) ([]string, bool) {
	var args struct {
		Path string `json:"path"`
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return nil, false
	}
	if strings.EqualFold(strings.TrimSpace(args.Mode), "create") {
		return nil, false
	}
	return nonEmptyPaths(args.Path), true
}

func extractManagePathScopeTarget(argumentsJSON string) ([]string, bool) {
	var args struct {
		Action string `json:"action"`
		Path   string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return nil, false
	}
	scopeRelevantActions := map[string]bool{"move": true, "rename": true, "trash": true}
	if !scopeRelevantActions[strings.ToLower(strings.TrimSpace(args.Action))] {
		return nil, false
	}
	return nonEmptyPaths(args.Path), true
}

func extractOrganizeScopeTarget(argumentsJSON string) ([]string, bool) {
	var args struct {
		SourcePath string `json:"source_path"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return nil, false
	}
	if strings.TrimSpace(args.SourcePath) == "" {
		return []string{"<workspace-root>"}, true
	}
	return nonEmptyPaths(args.SourcePath), true
}

func nonEmptyPaths(paths ...string) []string {
	return normalizeCandidatePaths(paths...)
}

func normalizeCandidatePaths(paths ...string) []string {
	seen := map[string]bool{}
	next := make([]string, 0, len(paths))
	for _, path := range paths {
		normalized := normalizeWorkspaceEventPath(path)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		next = append(next, normalized)
	}
	return next
}
