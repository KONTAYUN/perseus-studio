package ai

import (
	"encoding/json"
	"fmt"
	pathpkg "path"
	"strings"
)

func (r *agentRuntime) validateWorkspacePathScope(call agentToolCall) string {
	if r == nil {
		return ""
	}
	if r.opts.AllowedWorkspacePaths == nil {
		return ""
	}
	scope := workspacePathScope{allowedPaths: map[string]bool{}}
	for _, path := range r.opts.AllowedWorkspacePaths {
		addScopePath(scope, path)
	}
	candidates, enforce := scopedToolCandidatePaths(call.Name, call.ArgumentsJSON)
	if !enforce {
		switch call.Name {
		case "load_perseus_toolset", "attempt_completion", "ask_followup_question":
			return ""
		default:
			return "This tool cannot enforce the explicit exact-path workspace scope"
		}
	}
	for _, candidate := range candidates {
		if !scope.allows(candidate) {
			return fmt.Sprintf("tool target %q is outside the explicit allowed workspace paths", candidate)
		}
	}

	return ""
}

type workspacePathScope struct {
	allowedPaths map[string]bool
}

func addScopePath(scope workspacePathScope, path string) {
	normalized := normalizeScopePath(path)
	if normalized == "" {
		return
	}
	scope.allowedPaths[normalized] = true
}

func (s workspacePathScope) allows(path string) bool {
	normalized := normalizeScopePath(path)
	if normalized == "" {
		return false
	}
	return s.allowedPaths[normalized]
}

func scopedToolCandidatePaths(toolName, argumentsJSON string) ([]string, bool) {
	extractor, ok := workspaceScopeToolExtractors[toolName]
	if !ok {
		return nil, false
	}
	return extractor(argumentsJSON)
}

var workspaceScopeToolExtractors = map[string]func(string) ([]string, bool){
	"read_workspace_text":      extractSinglePathScopeTarget,
	"apply_workspace_patch":    extractPatchScopeTargets,
	"edit_workspace_text":      extractSinglePathScopeTarget,
	"replace_workspace_lines":  extractSinglePathScopeTarget,
	"read_workspace_files":     extractBatchReadScopeTargets,
	"save_workspace_file":      extractSaveScopeTarget,
	"write_workspace_chunk":    extractSaveScopeTarget,
	"finalize_workspace_write": extractSinglePathScopeTarget,
	"manage_workspace_path":    extractManagePathScopeTarget,
}

func extractSinglePathScopeTarget(argumentsJSON string) ([]string, bool) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return nil, false
	}
	return checkedScopePaths(args.Path)
}

func extractBatchReadScopeTargets(argumentsJSON string) ([]string, bool) {
	var args struct {
		Paths []string `json:"paths"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return nil, false
	}
	return checkedScopePaths(args.Paths...)
}

func extractPatchScopeTargets(argumentsJSON string) ([]string, bool) {
	var args struct {
		Patch string `json:"patch"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return nil, false
	}
	patch, err := parseWorkspacePatch(args.Patch)
	if err != nil {
		return nil, false // Invalid patches are rejected by the executor before any writes.
	}
	var paths []string
	for _, op := range patch.Operations {
		// Explicit scope includes new outputs and both sides of a move.
		paths = append(paths, op.Path)
		if op.MovePath != "" {
			paths = append(paths, op.MovePath)
		}
	}
	return checkedScopePaths(paths...)
}

func extractSaveScopeTarget(argumentsJSON string) ([]string, bool) {
	var args struct {
		Path string `json:"path"`
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return nil, false
	}

	return checkedScopePaths(args.Path)
}

func extractManagePathScopeTarget(argumentsJSON string) ([]string, bool) {
	var args struct {
		Action        string `json:"action"`
		Path          string `json:"path"`
		NewName       string `json:"new_name"`
		NewParentPath string `json:"new_parent_path"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return nil, false
	}
	source := normalizeScopePath(args.Path)
	if source == "" {
		return nil, false
	}
	switch strings.ToLower(strings.TrimSpace(args.Action)) {
	case "rename":
		// Match the workspace leaf-name contract; a destination is not new_path.
		leaf := normalizeScopePath(strings.TrimSpace(args.NewName))
		if leaf == "" || strings.Contains(leaf, "/") {
			return nil, false
		}
		return checkedScopePaths(source, pathpkg.Join(pathpkg.Dir(source), leaf))
	case "move":
		parent := strings.ReplaceAll(args.NewParentPath, "\\", "/")
		if parent != "" && parent != "." {
			parent = normalizeScopePath(parent)
			if parent == "" {
				return nil, false
			}
		}
		return checkedScopePaths(source, pathpkg.Join(parent, pathpkg.Base(source)))
	case "mkdir", "trash":
		return checkedScopePaths(source)
	default:
		return nil, false
	}
}

// Authorization uses filesystem path semantics, not display normalization:
// whitespace in a filename must not collapse into another allowed filename.
func normalizeScopePath(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	if strings.HasPrefix(value, "/") || strings.ContainsAny(value, ":\x00") {
		return ""
	}
	clean := pathpkg.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return ""
	}
	return clean
}

func checkedScopePaths(paths ...string) ([]string, bool) {
	if len(paths) == 0 {
		return nil, false
	}
	result := make([]string, 0, len(paths))
	for _, raw := range paths {
		clean := normalizeScopePath(raw)
		if clean == "" {
			return nil, false
		}
		result = append(result, clean)
	}
	return result, true
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
