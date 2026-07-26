package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type htmlMutationPreconditionTarget struct {
	Path       string
	RequireRaw bool
	Operation  string
}

func (r *agentRuntime) validateHTMLNoteMutationPrecondition(ctx context.Context, call agentToolCall) string {
	if r == nil || (!r.codexParityKernelActive() && !r.kernelV3Active()) || !isWorkspaceMutationTool(call.Name) {
		return ""
	}
	targets, errText := htmlMutationPreconditionTargets(call)
	if errText != "" {
		return errText
	}
	for _, target := range targets {
		path := normalizeWorkspaceEventPath(target.Path)
		if !isWorkspaceHTMLPath(path) {
			continue
		}
		if target.RequireRaw {
			if ok, reason := r.hasCurrentHTMLNoteRawRead(ctx, path); !ok {
				return fmt.Sprintf(
					"html note mutation rejected for %q: %s. Read the note with read_workspace_note_view mode=index, then read the target block or section with view=raw and the current doc_hash before applying a local patch.",
					path,
					reason,
				)
			}
		}
	}
	return ""
}

func htmlMutationPreconditionTargets(call agentToolCall) ([]htmlMutationPreconditionTarget, string) {
	switch call.Name {
	case "apply_workspace_patch":
		var args struct {
			Patch string `json:"patch"`
		}
		if err := json.Unmarshal([]byte(executionToolArgumentsJSON(call)), &args); err != nil {
			return nil, "tool arguments must be valid JSON: " + err.Error()
		}
		patchText := strings.TrimSpace(args.Patch)
		if patchText == "" {
			return nil, "patch is required"
		}
		patch, err := parseWorkspacePatch(patchText)
		if err != nil {
			return nil, err.Error()
		}
		targets := make([]htmlMutationPreconditionTarget, 0, len(patch.Operations))
		for _, op := range patch.Operations {
			if op.Kind != workspacePatchUpdate || len(op.Hunks) == 0 {
				continue
			}
			targets = append(targets, htmlMutationPreconditionTarget{
				Path:       op.Path,
				RequireRaw: true,
				Operation:  "patch_update",
			})
		}
		return targets, ""
	case "edit_workspace_text", "replace_workspace_lines":
		path := singlePathFromToolArguments(executionToolArgumentsJSON(call))
		if isWorkspaceHTMLPath(path) {
			return []htmlMutationPreconditionTarget{{Path: path, RequireRaw: true, Operation: call.Name}}, ""
		}
	case "save_workspace_file":
		var args struct {
			Path string `json:"path"`
			Mode string `json:"mode"`
		}
		if err := json.Unmarshal([]byte(executionToolArgumentsJSON(call)), &args); err != nil {
			return nil, "tool arguments must be valid JSON: " + err.Error()
		}
		if strings.EqualFold(strings.TrimSpace(args.Mode), "overwrite") && isWorkspaceHTMLPath(args.Path) {
			return []htmlMutationPreconditionTarget{{Path: args.Path, RequireRaw: true, Operation: "overwrite"}}, ""
		}
	case "write_workspace_chunk":
		var args struct {
			Path string `json:"path"`
			Mode string `json:"mode"`
		}
		if err := json.Unmarshal([]byte(executionToolArgumentsJSON(call)), &args); err != nil {
			return nil, "tool arguments must be valid JSON: " + err.Error()
		}
		mode := strings.ToLower(strings.TrimSpace(args.Mode))
		if isWorkspaceHTMLPath(args.Path) && mode != "" && mode != "create" {
			return []htmlMutationPreconditionTarget{{Path: args.Path, RequireRaw: true, Operation: mode}}, ""
		}
	}
	return nil, ""
}

func singlePathFromToolArguments(argumentsJSON string) string {
	var args struct {
		Path string `json:"path"`
	}
	if json.Unmarshal([]byte(argumentsJSON), &args) != nil {
		return ""
	}
	return args.Path
}

func (r *agentRuntime) hasCurrentHTMLNoteRawRead(ctx context.Context, path string) (bool, string) {
	if r == nil || r.service == nil || r.service.factory == nil {
		return false, "agent runtime is not ready"
	}
	files, err := r.service.factory.Workspace(r.opts.UserID)
	if err != nil {
		return false, err.Error()
	}
	read, err := files.ReadFile(path)
	if err != nil {
		return false, "the target note is not readable"
	}
	currentHash := noteDocHash(read.Content)
	r.ensureKernelState()
	if len(r.kernel.ReadTargets) == 0 {
		return false, "no current note block has been read in this turn"
	}
	for _, target := range r.kernel.ReadTargets {
		if target.Status != "success" {
			continue
		}
		if normalizeWorkspaceEventPath(target.Path) != path {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(target.DocHash), currentHash) {
			continue
		}
		if isRawHTMLNoteReadTarget(target) {
			return true, ""
		}
	}
	return false, "no raw block or section read matches the current doc_hash"
}

func isRawHTMLNoteReadTarget(target AgentReadTarget) bool {
	if strings.TrimSpace(target.DocHash) == "" {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(target.View), "raw") {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(target.Mode))
	return mode == "block" || mode == "section" || mode == "full"
}
