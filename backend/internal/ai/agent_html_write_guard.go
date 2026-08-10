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

func (r *agentRuntime) validateHTMLNoteMutationPrecondition(ctx context.Context, call agentToolCall) error {
	if r == nil || !isWorkspaceMutationTool(call.Name) {
		return nil
	}
	if call.Name == "edit_workspace_note_blocks" {
		return r.validateNoteBlockEditObservation(ctx, call)
	}
	targets, errText := htmlMutationPreconditionTargets(call)
	if errText != "" {
		return newToolError(ToolErrorInvalidArguments, errText, nil)
	}
	for _, target := range targets {
		path := normalizeWorkspaceEventPath(target.Path)
		if !isWorkspaceHTMLPath(path) {
			continue
		}
		if target.RequireRaw {
			if ok, reason := r.hasCurrentHTMLNoteRawRead(ctx, path); !ok {
				return newToolError(ToolErrorNoteEditObservation, fmt.Sprintf(
					"html note mutation rejected for %q: %s. Read the note with read_workspace_note_view mode=index, then read the target block or section with view=raw and the current document_version before applying a local patch.",
					path,
					reason,
				), nil)
			}
		}
	}
	return nil
}

func (r *agentRuntime) validateNoteBlockEditObservation(ctx context.Context, call agentToolCall) error {
	var args noteBlockEditArgs
	if err := json.Unmarshal([]byte(executionToolArgumentsJSON(call)), &args); err != nil {
		return newToolError(ToolErrorInvalidArguments, "edit_workspace_note_blocks arguments must be valid JSON", err)
	}
	path := normalizeWorkspaceEventPath(args.Path)
	if !isWorkspaceHTMLPath(path) {
		return nil
	}
	version := firstNonEmpty(strings.TrimSpace(args.DocumentVersion), strings.TrimSpace(args.DocHash))
	if version == "" {
		return newToolError(ToolErrorDocumentVersionRequired, "edit_workspace_note_blocks requires the complete current document_version", nil)
	}
	if len(args.Edits) == 0 {
		return newToolError(ToolErrorInvalidArguments, "edit_workspace_note_blocks requires document_version and edits", nil)
	}
	if r.service == nil || r.service.factory == nil {
		return newToolError(ToolErrorNoteEditObservation, "agent runtime is not ready to validate note observations", nil)
	}
	files, err := r.service.factory.Workspace(r.opts.UserID)
	if err != nil {
		return newToolError(ToolErrorNoteEditObservation, "workspace is not available for note preflight", err)
	}
	read, err := files.ReadFile(path)
	if err != nil {
		return newToolError(ToolErrorPathNotFound, "the target note is not readable", err)
	}
	doc := buildNoteBlockDocument(read.Content)
	if !noteDocumentVersionMatches(version, doc.DocHash) {
		typed := newToolError(ToolErrorStaleDocument, "note document_version is stale", nil)
		typed.Target = path
		typed.Details = map[string]any{"expected_document_version": version, "current_document_version": doc.DocHash}
		return typed
	}
	required := noteEditRequiredBlockIDs(doc, args.Edits)
	missing := make([]string, 0)
	for _, id := range required {
		block, ok := findNoteBlock(doc.Blocks, id)
		if !ok {
			missing = append(missing, id)
			continue
		}
		if !r.hasCurrentHTMLNoteRawReadForBlock(path, doc.DocHash, block) {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	typed := newToolError(ToolErrorNoteEditObservation, "each HTML edit target must be covered by a current raw block or section read", nil)
	typed.Target = path
	typed.Details = map[string]any{
		"document_version":  doc.DocHash,
		"missing_block_ids": missing,
		"required_reads": map[string]any{
			"tool":      "read_workspace_note_view",
			"path":      path,
			"mode":      "block",
			"view":      "raw",
			"block_ids": missing,
		},
	}
	return typed
}

func (r *agentRuntime) validateNoteBlockEditExecutionPreflight(ctx context.Context, call agentToolCall) error {
	if r == nil || call.Name != "edit_workspace_note_blocks" || r.service == nil {
		return nil
	}
	return r.service.preflightEditWorkspaceNoteBlocks(ctx, r.opts.UserID, executionToolArgumentsJSON(call))
}

func noteEditRequiredBlockIDs(doc noteBlockDocument, edits []noteBlockEdit) []string {
	seen := map[string]bool{}
	ids := make([]string, 0, len(edits))
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		ids = append(ids, id)
	}
	for _, edit := range edits {
		start := strings.TrimSpace(edit.BlockID)
		add(start)
		if edit.Operation != "delete_block_range" {
			continue
		}
		end := strings.TrimSpace(edit.EndBlockID)
		startOrdinal, endOrdinal := noteBlockOrdinal(start), noteBlockOrdinal(end)
		if startOrdinal <= 0 || endOrdinal < startOrdinal {
			add(end)
			continue
		}
		for ordinal := startOrdinal; ordinal <= endOrdinal; ordinal++ {
			if ordinal <= len(doc.Blocks) {
				add(doc.Blocks[ordinal-1].ID)
			}
		}
	}
	return ids
}

func (r *agentRuntime) hasCurrentHTMLNoteRawReadForBlock(path, docHash string, block noteBlock) bool {
	if r == nil {
		return false
	}
	r.ensureKernelState()
	for _, target := range r.kernel.ReadTargets {
		if target.Status != "success" || normalizeWorkspaceEventPath(target.Path) != path || !noteDocumentVersionMatches(target.DocHash, docHash) || !isRawHTMLNoteReadTarget(target) || target.SourceTruncated || target.ModelViewTruncated {
			continue
		}
		if containsNoteBlockID(target.CompleteBlockIDs, block.ID) {
			return true
		}
	}
	return false
}

func containsNoteBlockID(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func htmlMutationPreconditionTargets(call agentToolCall) ([]htmlMutationPreconditionTarget, string) {
	switch call.Name {
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
		if !noteDocumentVersionMatches(target.DocHash, currentHash) {
			continue
		}
		if isRawHTMLNoteReadTarget(target) && !target.SourceTruncated && !target.ModelViewTruncated && len(target.CompleteBlockIDs) > 0 {
			return true, ""
		}
	}
	return false, "no raw block or section read matches the current document_version"
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
