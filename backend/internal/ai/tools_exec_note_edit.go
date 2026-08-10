package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	nethtml "golang.org/x/net/html"

	"webide-backend/internal/workspace"
)

type noteBlockEdit struct {
	BlockID         string `json:"block_id"`
	EndBlockID      string `json:"end_block_id,omitempty"`
	SegmentID       string `json:"segment_id,omitempty"`
	Operation       string `json:"operation"`
	HTML            string `json:"html,omitempty"`
	Find            string `json:"find,omitempty"`
	Replace         string `json:"replace,omitempty"`
	ExpectedMatches *int   `json:"expected_matches,omitempty"`
}

type noteBlockEditArgs struct {
	Path            string          `json:"path"`
	DocHash         string          `json:"doc_hash"`
	DocumentVersion string          `json:"document_version"`
	Edits           []noteBlockEdit `json:"edits"`
}

func (s *Service) execEditWorkspaceNoteBlocks(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	var args noteBlockEditArgs
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在编辑笔记...", "", newToolError(ToolErrorInvalidArguments, "invalid edit_workspace_note_blocks arguments", err)
	}
	args.Path = strings.TrimSpace(args.Path)
	args.DocHash = strings.TrimSpace(args.DocHash)
	args.DocumentVersion = strings.TrimSpace(args.DocumentVersion)
	expectedVersion := firstNonEmpty(args.DocumentVersion, args.DocHash)
	if args.Path == "" || !strings.HasSuffix(strings.ToLower(args.Path), ".html") {
		return "", "", "正在编辑笔记...", args.Path, newToolError(ToolErrorUnsupportedFileType, "edit_workspace_note_blocks requires an .html note path", nil)
	}
	if expectedVersion == "" || len(args.Edits) == 0 {
		return "", "", "正在编辑笔记...", args.Path, newToolError(ToolErrorInvalidArguments, "path, doc_hash, and at least one edit are required", nil)
	}

	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在编辑笔记...", args.Path, err
	}
	current, err := files.ReadFile(args.Path)
	if err != nil {
		return "", "", "正在编辑笔记...", args.Path, err
	}
	doc := buildNoteBlockDocument(current.Content)
	if !noteDocumentVersionMatches(expectedVersion, doc.DocHash) {
		typed := newToolError(ToolErrorStaleDocument, "note doc_hash is stale; read the note index again before editing", nil)
		typed.Target = args.Path
		typed.Details = map[string]any{"expected_document_version": expectedVersion, "current_document_version": doc.DocHash}
		return "", "", "正在编辑笔记...", args.Path, typed
	}

	nextContent, changedBlocks, err := applyNoteBlockEdits(current.Content, args.Edits)
	if err != nil {
		typed := normalizeToolError(err)
		typed.Phase = "preflight"
		err = enrichNoteEditError(typed, args.Path, doc.DocHash)
		return "", "", "正在编辑笔记...", args.Path, err
	}
	structureReport, err := validateNoteEditStructure(current.Content, nextContent, args.Edits)
	if err != nil {
		return "", "", "正在校验笔记...", args.Path, err
	}
	write, err := files.WriteFile(ctx, args.Path, nextContent, "ai_note_block_edit")
	if err != nil {
		return "", "", "正在编辑笔记...", args.Path, err
	}
	saved, err := files.ReadFile(args.Path)
	if err != nil {
		return "", "", "正在校验笔记...", args.Path, err
	}
	nextDoc := buildNoteBlockDocument(saved.Content)
	if saved.Hash != write.Hash || nextDoc.DocHash != noteDocHash(nextContent) {
		return "", "", "正在校验笔记...", args.Path, newToolError(ToolErrorVerificationFailed, "saved note hashes do not match the validated edit result", nil)
	}
	payload := workspaceToolPayload("edit_workspace_note_blocks", write.Path, map[string]any{
		"operation":         "edit_note_blocks",
		"path":              write.Path,
		"changed_paths":     write.ChangedPaths,
		"changed_blocks":    changedBlocks,
		"doc_hash":          nextDoc.DocHash,
		"document_version":  nextDoc.DocHash,
		"file_hash":         write.Hash,
		"version_id":        write.VersionID,
		"validation_status": "verified",
		"structure_delta":   structureReport,
		"verification":      "saved note hashes and structure match the validated edit",
		"total_blocks":      len(nextDoc.Blocks),
	})
	return mustJSON(payload), fmt.Sprintf("updated %d note block(s)", len(changedBlocks)), "正在编辑笔记...", write.Path, nil
}

// preflightEditWorkspaceNoteBlocks performs the same deterministic parse,
// version, edit, and structure checks as execution without creating a preview,
// history entry, or file change. The runtime calls this before preview start.
func (s *Service) preflightEditWorkspaceNoteBlocks(ctx context.Context, userID, argumentsJSON string) error {
	var args noteBlockEditArgs
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		typed := newToolError(ToolErrorInvalidArguments, "invalid edit_workspace_note_blocks arguments", err)
		typed.Phase = "preflight"
		return typed
	}
	args.Path = strings.TrimSpace(args.Path)
	expectedVersion := strings.TrimSpace(firstNonEmpty(args.DocumentVersion, args.DocHash))
	if args.Path == "" || !strings.HasSuffix(strings.ToLower(args.Path), ".html") {
		typed := newToolError(ToolErrorUnsupportedFileType, "edit_workspace_note_blocks requires an .html note path", nil)
		typed.Phase = "preflight"
		return typed
	}
	if expectedVersion == "" || len(args.Edits) == 0 {
		typed := newToolError(ToolErrorInvalidArguments, "path, document_version, and at least one edit are required", nil)
		typed.Phase = "preflight"
		return typed
	}
	if s == nil || s.factory == nil {
		typed := newToolError(ToolErrorNoteEditObservation, "workspace is not available for note preflight", nil)
		typed.Phase = "preflight"
		return typed
	}
	files, err := s.factory.Workspace(userID)
	if err != nil {
		typed := normalizeToolError(err)
		typed.Phase = "preflight"
		return typed
	}
	current, err := files.ReadFile(args.Path)
	if err != nil {
		typed := normalizeToolError(err)
		typed.Phase = "preflight"
		return typed
	}
	doc := buildNoteBlockDocument(current.Content)
	if !noteDocumentVersionMatches(expectedVersion, doc.DocHash) {
		typed := newToolError(ToolErrorStaleDocument, "note document_version is stale", nil)
		typed.Phase = "preflight"
		typed.Target = args.Path
		typed.Details = map[string]any{"expected_document_version": expectedVersion, "current_document_version": doc.DocHash}
		return typed
	}
	nextContent, _, err := applyNoteBlockEdits(current.Content, args.Edits)
	if err != nil {
		typed := normalizeToolError(err)
		typed.Phase = "preflight"
		return enrichNoteEditError(typed, args.Path, doc.DocHash)
	}
	if _, err := validateNoteEditStructure(current.Content, nextContent, args.Edits); err != nil {
		typed := normalizeToolError(err)
		typed.Phase = "preflight"
		return enrichNoteEditError(typed, args.Path, doc.DocHash)
	}
	return nil
}

func enrichNoteEditError(err error, path, documentVersion string) error {
	typed := normalizeToolError(err)
	if typed.Details == nil {
		typed.Details = map[string]any{}
	}
	typed.Target = path
	typed.Details["current_document_version"] = documentVersion
	typed.Details["recommended_read"] = map[string]any{
		"tool": "read_workspace_note_view",
		"path": path,
		"mode": "block",
		"view": "raw",
	}
	return typed
}

func applyNoteBlockEdits(fragment string, edits []noteBlockEdit) (string, []map[string]any, error) {
	root, err := nethtml.Parse(strings.NewReader("<!doctype html><html><body>" + fragment + "</body></html>"))
	if err != nil {
		return "", nil, newToolError(ToolErrorInvalidArguments, "note HTML could not be parsed", err)
	}
	body := findFirstHTMLNode(root, "body")
	if body == nil {
		return "", nil, newToolError(ToolErrorInvalidArguments, "note HTML body could not be located", nil)
	}
	nodes := collectNoteBlockNodes(body)
	byID := make(map[string]*nethtml.Node, len(nodes))
	for index, node := range nodes {
		byID[fmt.Sprintf("b%04d", index+1)] = node
	}

	preflightErrors := make([]map[string]any, 0)
	addPreflightError := func(code ToolErrorCode, message string, details map[string]any) {
		if len(preflightErrors) >= 20 {
			return
		}
		item := map[string]any{"error_code": code, "message": message}
		for key, value := range details {
			item[key] = value
		}
		preflightErrors = append(preflightErrors, item)
	}
	seen := map[string]bool{}
	for editIndex, edit := range edits {
		id := strings.TrimSpace(edit.BlockID)
		op := strings.TrimSpace(edit.Operation)
		if id == "" || seen[id] {
			if seen[id] {
				addPreflightError(ToolErrorInvalidArguments, "each edit requires a unique block_id", map[string]any{"edit_index": editIndex, "block_id": id, "duplicate_block_id": id})
				continue
			}
			addPreflightError(ToolErrorInvalidArguments, "each edit requires a unique block_id", map[string]any{"edit_index": editIndex, "block_id": id})
			continue
		}
		seen[id] = true
		if byID[id] == nil {
			addPreflightError(ToolErrorStaleDocument, "note block_id is missing; read the note index again", map[string]any{"edit_index": editIndex, "block_id": id})
			continue
		}
		switch op {
		case "replace_html", "replace_inner_html", "insert_before", "insert_after":
			if strings.TrimSpace(edit.HTML) == "" {
				addPreflightError(ToolErrorInvalidArguments, op+" requires non-empty html", map[string]any{"edit_index": editIndex, "block_id": id})
				continue
			}
		case "replace_text":
			expected := 1
			if edit.ExpectedMatches != nil {
				expected = *edit.ExpectedMatches
			}
			if edit.Find == "" || expected < 1 {
				addPreflightError(ToolErrorInvalidArguments, "replace_text requires non-empty find and expected_matches >= 1", map[string]any{"edit_index": editIndex, "block_id": id, "expected_matches": expected})
				continue
			}
			actual := countTextNodeMatchesForSegment(byID[id], id, edit.Find, edit.SegmentID)
			if actual != expected {
				code := ToolErrorVerificationFailed
				message := "replace_text match count did not equal expected_matches"
				segmentIDs := textSegmentsSpanningMatch(byID[id], id, edit.Find)
				if len(segmentIDs) > 1 && strings.TrimSpace(edit.SegmentID) == "" {
					code = ToolErrorTextSpansMultipleNodes
					message = "replace_text target spans multiple HTML text nodes"
				}
				addPreflightError(code, message, map[string]any{
					"edit_index": editIndex, "block_id": id, "expected_matches": expected, "actual_matches": actual,
					"segment_ids": segmentIDs,
					"inner_html":  renderHTMLNode(byID[id]), "recommended_operation": "replace_inner_html",
				})
				continue
			}
		case "delete":
		case "delete_block_range":
			endID := strings.TrimSpace(edit.EndBlockID)
			if endID == "" || byID[endID] == nil {
				addPreflightError(ToolErrorInvalidArguments, "delete_block_range requires a valid end_block_id", map[string]any{"edit_index": editIndex, "block_id": id, "end_block_id": endID})
				continue
			}
			if noteBlockOrdinal(endID) < noteBlockOrdinal(id) {
				addPreflightError(ToolErrorInvalidArguments, "delete_block_range end_block_id must not precede block_id", map[string]any{"edit_index": editIndex, "block_id": id, "end_block_id": endID})
				continue
			}
			for ordinal := noteBlockOrdinal(id) + 1; ordinal <= noteBlockOrdinal(endID); ordinal++ {
				rangeID := fmt.Sprintf("b%04d", ordinal)
				if seen[rangeID] {
					addPreflightError(ToolErrorInvalidArguments, "delete_block_range overlaps another edit", map[string]any{"edit_index": editIndex, "block_id": id, "overlap_block_id": rangeID})
					break
				}
				seen[rangeID] = true
			}
		default:
			addPreflightError(ToolErrorInvalidArguments, "unsupported note block operation: "+op, map[string]any{"edit_index": editIndex, "block_id": id, "operation": op})
		}
	}
	if len(preflightErrors) > 0 {
		code := ToolErrorInvalidArguments
		message := "note edit preflight rejected one or more edits"
		details := map[string]any{"errors": preflightErrors}
		if len(preflightErrors) == 1 {
			if value, ok := preflightErrors[0]["error_code"].(ToolErrorCode); ok {
				code = value
			}
			if value, ok := preflightErrors[0]["message"].(string); ok && value != "" {
				message = value
			}
			details = map[string]any{}
			for key, value := range preflightErrors[0] {
				if key != "error_code" && key != "message" {
					details[key] = value
				}
			}
		}
		typed := newToolError(code, message, nil)
		typed.Phase = "preflight"
		typed.Details = details
		if len(preflightErrors) > 1 {
			typed.Details["errors"] = preflightErrors
		}
		return "", nil, typed
	}

	changed := make([]map[string]any, 0, len(edits))
	for _, edit := range edits {
		id := strings.TrimSpace(edit.BlockID)
		op := strings.TrimSpace(edit.Operation)
		target := byID[id]
		parent := target.Parent
		if parent == nil {
			return "", nil, newToolError(ToolErrorVerificationFailed, "note block is detached from the document", nil)
		}
		switch op {
		case "delete":
			parent.RemoveChild(target)
		case "delete_block_range":
			startIndex := noteBlockOrdinal(id) - 1
			endIndex := noteBlockOrdinal(strings.TrimSpace(edit.EndBlockID)) - 1
			for index := endIndex; index >= startIndex; index-- {
				node := nodes[index]
				nodeParent := node.Parent
				if nodeParent == nil {
					continue
				}
				nodeParent.RemoveChild(node)
				removeEmptyNoteContainer(nodeParent)
			}
		case "replace_text":
			replaceTextNodeMatchesForSegment(target, id, edit.Find, edit.Replace, edit.SegmentID)
		case "replace_inner_html":
			replacement, parseErr := parseSanitizedNoteContainer(edit.HTML, target.Data)
			if parseErr != nil {
				return "", nil, parseErr
			}
			for target.FirstChild != nil {
				target.RemoveChild(target.FirstChild)
			}
			for replacement.FirstChild != nil {
				child := replacement.FirstChild
				replacement.RemoveChild(child)
				target.AppendChild(child)
			}
		default:
			replacements, parseErr := parseSanitizedNoteFragment(edit.HTML)
			if parseErr != nil {
				return "", nil, parseErr
			}
			if op == "insert_after" {
				next := target.NextSibling
				for _, node := range replacements {
					parent.InsertBefore(node, next)
				}
			} else {
				for _, node := range replacements {
					parent.InsertBefore(node, target)
				}
				if op == "replace_html" {
					parent.RemoveChild(target)
				}
			}
		}
		change := map[string]any{"block_id": id, "operation": op}
		if edit.EndBlockID != "" {
			change["end_block_id"] = strings.TrimSpace(edit.EndBlockID)
		}
		changed = append(changed, change)
	}

	var output strings.Builder
	for child := body.FirstChild; child != nil; child = child.NextSibling {
		if err := nethtml.Render(&output, child); err != nil {
			return "", nil, newToolError(ToolErrorVerificationFailed, "edited note could not be rendered", err)
		}
	}
	return workspace.SanitizeHTMLFragment(output.String()), changed, nil
}

func noteBlockOrdinal(blockID string) int {
	value := strings.TrimPrefix(strings.TrimSpace(blockID), "b")
	var ordinal int
	_, _ = fmt.Sscanf(value, "%d", &ordinal)
	return ordinal
}

func removeEmptyNoteContainer(node *nethtml.Node) {
	for node != nil && node.Parent != nil {
		tag := strings.ToLower(node.Data)
		if tag != "ul" && tag != "ol" {
			return
		}
		if hasDirectElementChild(node, "li") {
			return
		}
		parent := node.Parent
		parent.RemoveChild(node)
		node = parent
	}
}

func parseSanitizedNoteContainer(innerHTML, tag string) (*nethtml.Node, error) {
	clean := strings.TrimSpace(workspace.SanitizeHTMLFragment(innerHTML))
	if clean == "" {
		return nil, newToolError(ToolErrorInvalidArguments, "inner html became empty after sanitization", nil)
	}
	root, err := nethtml.Parse(strings.NewReader("<!doctype html><html><body><" + tag + ">" + clean + "</" + tag + "></body></html>"))
	if err != nil {
		return nil, newToolError(ToolErrorInvalidArguments, "inner html could not be parsed", err)
	}
	container := findFirstHTMLNode(root, tag)
	if container == nil {
		return nil, newToolError(ToolErrorInvalidArguments, "inner html container could not be parsed", nil)
	}
	return container, nil
}

func countTextNodeMatches(root *nethtml.Node, find string) int {
	return countTextNodeMatchesForSegment(root, "", find, "")
}

func countTextNodeMatchesForSegment(root *nethtml.Node, blockID, find, segmentID string) int {
	count := 0
	for _, segment := range noteTextNodesWithSegments(root, blockID) {
		if segmentID != "" && segment.ID != segmentID {
			continue
		}
		count += strings.Count(segment.Node.Data, find)
	}
	return count
}

func replaceTextNodeMatches(root *nethtml.Node, find, replace string) {
	replaceTextNodeMatchesForSegment(root, "", find, replace, "")
}

func replaceTextNodeMatchesForSegment(root *nethtml.Node, blockID, find, replace, segmentID string) {
	for _, segment := range noteTextNodesWithSegments(root, blockID) {
		if segmentID != "" && segment.ID != segmentID {
			continue
		}
		segment.Node.Data = strings.ReplaceAll(segment.Node.Data, find, replace)
	}
}

type noteTextNodeSegment struct {
	ID   string
	Node *nethtml.Node
}

func noteTextNodesWithSegments(root *nethtml.Node, blockID string) []noteTextNodeSegment {
	segments := make([]noteTextNodeSegment, 0)
	index := 0
	var walk func(*nethtml.Node)
	walk = func(node *nethtml.Node) {
		if node == nil {
			return
		}
		if node.Type == nethtml.TextNode {
			index++
			id := ""
			if blockID != "" {
				id = fmt.Sprintf("%s:s%04d", blockID, index)
			}
			segments = append(segments, noteTextNodeSegment{ID: id, Node: node})
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return segments
}

func textSegmentsSpanningMatch(root *nethtml.Node, blockID, find string) []string {
	if strings.TrimSpace(find) == "" {
		return nil
	}
	segments := noteTextNodesWithSegments(root, blockID)
	var visible strings.Builder
	for _, segment := range segments {
		visible.WriteString(segment.Node.Data)
	}
	start := strings.Index(visible.String(), find)
	if start < 0 {
		return nil
	}
	end := start + len(find)
	position := 0
	ids := make([]string, 0)
	for _, segment := range segments {
		next := position + len(segment.Node.Data)
		if start < next && end > position {
			ids = append(ids, segment.ID)
		}
		position = next
	}
	return sortedUniqueStrings(ids)
}

func collectNoteBlockNodes(root *nethtml.Node) []*nethtml.Node {
	var nodes []*nethtml.Node
	var walk func(*nethtml.Node)
	walk = func(node *nethtml.Node) {
		if node == nil {
			return
		}
		if isNoteBlockNode(node) {
			nodes = append(nodes, node)
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		walk(child)
	}
	return nodes
}

func parseSanitizedNoteFragment(fragment string) ([]*nethtml.Node, error) {
	clean := strings.TrimSpace(workspace.SanitizeHTMLFragment(fragment))
	if clean == "" {
		return nil, newToolError(ToolErrorInvalidArguments, "html became empty after sanitization", nil)
	}
	root, err := nethtml.Parse(strings.NewReader("<!doctype html><html><body>" + clean + "</body></html>"))
	if err != nil {
		return nil, newToolError(ToolErrorInvalidArguments, "edit html could not be parsed", err)
	}
	body := findFirstHTMLNode(root, "body")
	if body == nil || body.FirstChild == nil {
		return nil, newToolError(ToolErrorInvalidArguments, "edit html contains no nodes", nil)
	}
	var nodes []*nethtml.Node
	for body.FirstChild != nil {
		node := body.FirstChild
		body.RemoveChild(node)
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func (s *Service) execCreateWorkspaceNote(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	var args struct {
		Path string `json:"path"`
		HTML string `json:"html"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在创建笔记...", "", newToolError(ToolErrorInvalidArguments, "invalid create_workspace_note arguments", err)
	}
	args.Path = strings.TrimSpace(args.Path)
	if args.Path == "" || !strings.HasSuffix(strings.ToLower(args.Path), ".html") {
		return "", "", "正在创建笔记...", args.Path, newToolError(ToolErrorUnsupportedFileType, "create_workspace_note requires an .html path", nil)
	}
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在创建笔记...", args.Path, err
	}
	clean := workspace.SanitizeHTMLFragment(args.HTML)
	write, err := files.CreateFile(ctx, args.Path, clean)
	if err != nil {
		return "", "", "正在创建笔记...", args.Path, err
	}
	doc := buildNoteBlockDocument(clean)
	payload := workspaceToolPayload("create_workspace_note", write.Path, map[string]any{
		"operation":        "create_note",
		"path":             write.Path,
		"changed_paths":    write.ChangedPaths,
		"doc_hash":         doc.DocHash,
		"document_version": doc.DocHash,
		"file_hash":        write.Hash,
		"version_id":       write.VersionID,
		"total_blocks":     len(doc.Blocks),
		"verification":     "created note parsed successfully",
	})
	return mustJSON(payload), "created HTML note", "正在创建笔记...", write.Path, nil
}
