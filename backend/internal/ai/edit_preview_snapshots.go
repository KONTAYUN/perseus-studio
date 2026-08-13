package ai

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"webide-backend/internal/workspace"
)

func (m *editPreviewManager) buildSnapshots(ctx context.Context, toolName, partial string) ([]editPreviewSnapshot, int) {
	if ctx.Err() != nil || m.service == nil || m.userID == "" {
		return nil, 0
	}
	files, err := m.service.factory.Workspace(m.userID)
	if err != nil {
		return nil, 0
	}
	switch toolName {
	case "create_workspace_note":
		if !topLevelJSONStringFieldComplete(partial, "path") {
			return nil, 1
		}
		var args struct {
			Path string `json:"path"`
			HTML string `json:"html"`
		}
		if !decodePartialJSONObject(partial, &args) {
			return nil, 0
		}
		args.Path = strings.TrimSpace(args.Path)
		if args.Path == "" || !strings.HasSuffix(strings.ToLower(args.Path), ".html") {
			return nil, 0
		}
		if _, err := files.ReadFile(args.Path); err == nil {
			return nil, 0
		}
		content := workspace.SanitizeHTMLFragment(args.HTML)
		if strings.TrimSpace(content) == "" {
			return nil, 0
		}
		return []editPreviewSnapshot{{TargetID: "target_0", TargetIndex: 0, Path: args.Path, PathComplete: true, Content: content, Operation: "create", IsNew: true}}, 0
	case "edit_workspace_note_blocks":
		if !topLevelJSONStringFieldComplete(partial, "path") {
			return nil, 1
		}
		var args noteBlockEditArgs
		if !decodePartialJSONObject(partial, &args) {
			return nil, 0
		}
		version := firstNonEmpty(args.DocumentVersion, args.DocHash)
		if args.Path == "" || version == "" || len(args.Edits) == 0 {
			return nil, 0
		}
		read, err := files.ReadFile(args.Path)
		if err != nil {
			return nil, 0
		}
		doc := buildNoteBlockDocument(read.Content)
		if !noteDocumentVersionMatches(version, doc.DocHash) {
			return nil, 0
		}
		content, _, err := applyNoteBlockEdits(read.Content, args.Edits)
		if err != nil {
			return nil, 0
		}
		return []editPreviewSnapshot{{
			TargetID: "target_0", TargetIndex: 0, Path: args.Path, PathComplete: true,
			BaseHash: read.Hash, DocumentVersion: doc.DocHash, Content: content,
			Operation: "edit_note_blocks", Changes: previewNoteBlockChanges(args.Edits),
		}}, 0
	case "apply_workspace_patch":
		var args struct {
			Patch string `json:"patch"`
		}
		if !decodePartialJSONObject(partial, &args) {
			return nil, 0
		}
		patchFieldComplete := topLevelJSONStringFieldComplete(partial, "patch")
		patchText, incompletePath := completePatchPreviewInput(args.Patch, patchFieldComplete)
		if patchText == "" {
			if incompletePath {
				return nil, 1
			}
			return nil, 0
		}
		return buildWorkspacePatchPreviewSnapshots(files, patchText), boolInt(incompletePath)
	default:
		return nil, 0
	}
}

func previewNoteBlockChanges(edits []noteBlockEdit) []map[string]any {
	changes := make([]map[string]any, 0, len(edits))
	for _, edit := range edits {
		change := map[string]any{
			"block_id":  strings.TrimSpace(edit.BlockID),
			"operation": strings.TrimSpace(edit.Operation),
		}
		if strings.TrimSpace(edit.EndBlockID) != "" {
			change["end_block_id"] = strings.TrimSpace(edit.EndBlockID)
		}
		switch strings.TrimSpace(edit.Operation) {
		case "replace_text":
			change["find"] = edit.Find
			change["replace"] = edit.Replace
			if edit.ExpectedMatches != nil {
				change["expected_matches"] = *edit.ExpectedMatches
			}
		case "replace_html", "replace_inner_html", "insert_before", "insert_after":
			change["html"] = workspace.SanitizeHTMLFragment(edit.HTML)
		}
		changes = append(changes, change)
	}
	return changes
}

func isEditPreviewTool(name string) bool {
	switch name {
	case "edit_workspace_note_blocks", "create_workspace_note", "apply_workspace_patch":
		return true
	default:
		return false
	}
}

func detectPreviewFileType(path string) string {
	if strings.HasSuffix(strings.ToLower(path), ".html") {
		return "html"
	}
	return "text"
}

func decodePartialJSONObject(raw string, target any) bool {
	completed, ok := completePartialJSONObject(strings.TrimSpace(raw))
	if !ok {
		return false
	}
	return json.Unmarshal([]byte(completed), target) == nil
}

func completePartialJSONObject(raw string) (string, bool) {
	if raw == "" || raw[0] != '{' {
		return "", false
	}
	stack := make([]byte, 0, 8)
	inString := false
	escaped := false
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{', '[':
			stack = append(stack, ch)
		case '}', ']':
			if len(stack) == 0 {
				return "", false
			}
			stack = stack[:len(stack)-1]
		}
	}
	if escaped {
		raw = strings.TrimSuffix(raw, "\\")
	}
	if inString {
		raw += `"`
	}
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i] == '{' {
			raw += "}"
		} else {
			raw += "]"
		}
	}
	return raw, true
}

func buildWorkspacePatchPreviewSnapshots(files *workspace.Service, patchText string) []editPreviewSnapshot {
	patchText = strings.TrimSpace(patchText)
	if patchText == "" {
		return nil
	}
	if strings.Contains(patchText, "*** Begin Patch") && !strings.Contains(patchText, "*** End Patch") {
		patchText += "\n*** End Patch"
	}
	patch, err := parseWorkspacePatch(patchText)
	if err != nil {
		return nil
	}
	var snapshots []editPreviewSnapshot
	for index, op := range patch.Operations {
		path := strings.TrimSpace(op.Path)
		if path == "" {
			continue
		}
		switch op.Kind {
		case workspacePatchAdd:
			if _, err := files.ReadFile(path); err == nil {
				continue
			}
			snapshots = append(snapshots, editPreviewSnapshot{
				TargetID: "target_" + strconv.Itoa(index), TargetIndex: index,
				Path: path, PathComplete: true, Content: strings.Join(op.Lines, ""), Operation: "add", IsNew: true,
			})
		case workspacePatchUpdate:
			read, err := files.ReadFile(path)
			if err != nil {
				continue
			}
			content, err := applyWorkspacePatchUpdate(read.Content, op.Hunks)
			if err != nil {
				continue
			}
			target := path
			if strings.TrimSpace(op.MovePath) != "" {
				target = strings.TrimSpace(op.MovePath)
			}
			snapshots = append(snapshots, editPreviewSnapshot{
				TargetID: "target_" + strconv.Itoa(index), TargetIndex: index,
				Path: target, PathComplete: true, BaseHash: read.Hash, Content: content, Operation: "update",
			})
		case workspacePatchDelete:
			read, err := files.ReadFile(path)
			if err != nil {
				continue
			}
			snapshots = append(snapshots, editPreviewSnapshot{
				TargetID: "target_" + strconv.Itoa(index), TargetIndex: index,
				Path: path, PathComplete: true, BaseHash: read.Hash, Content: read.Content, Operation: "delete", Deleted: true,
			})
		}
	}
	return snapshots
}
