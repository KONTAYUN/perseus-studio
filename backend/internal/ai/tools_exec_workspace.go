package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"webide-backend/internal/workspace"
)

func (s *Service) execListWorkspaceFiles(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在查看工作区文件...", "", err
	}
	var args struct {
		Path     string `json:"path"`
		MaxDepth int    `json:"max_depth"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在查看工作区文件...", "", fmt.Errorf("invalid tool arguments")
	}
	nodes, limited, err := files.List(args.Path, args.MaxDepth, 200)
	if err != nil {
		return "", "", "正在查看工作区文件...", args.Path, err
	}
	payload := workspaceToolPayload("inspect_workspace", args.Path, map[string]any{
		"path":      args.Path,
		"items":     nodes,
		"limited":   limited,
		"next_hint": "Use find_workspace_files/search_workspace_text for targeted discovery, then read_workspace_text before editing.",
	})
	summary := fmt.Sprintf("listed %d workspace items", countTreeNodes(nodes))
	return mustJSON(payload), summary, "正在查看工作区文件...", args.Path, nil
}

func (s *Service) execFindWorkspaceFiles(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在查找文件...", "", err
	}
	var args struct {
		Query      string `json:"query"`
		Pattern    string `json:"pattern"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在查找文件...", "", fmt.Errorf("invalid tool arguments")
	}
	results, err := files.FindFiles(args.Query, args.Pattern, args.MaxResults)
	if err != nil {
		return "", "", "正在查找文件...", "", err
	}
	payload := workspaceToolPayload("find_workspace_files", "", map[string]any{
		"query":     args.Query,
		"pattern":   args.Pattern,
		"items":     results,
		"count":     len(results),
		"next_hint": "Use read_workspace_text for file contents or search_workspace_text to find text within these paths.",
	})
	summary := fmt.Sprintf("found %d workspace item(s)", len(results))
	return mustJSON(payload), summary, "正在查找文件...", "", nil
}

func (s *Service) execSearchWorkspaceText(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在搜索文件内容...", "", err
	}
	var args struct {
		Query        string   `json:"query"`
		Regex        bool     `json:"regex"`
		ContextLines int      `json:"context_lines"`
		MaxResults   int      `json:"max_results"`
		PathScope    string   `json:"path_scope"`
		IncludeGlobs []string `json:"include_globs"`
		ExcludeGlobs []string `json:"exclude_globs"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在搜索文件内容...", "", fmt.Errorf("invalid tool arguments")
	}
	results, limited, err := files.SearchTextScoped(args.Query, args.Regex, args.MaxResults, args.ContextLines, workspace.TextSearchOptions{
		PathScope:    args.PathScope,
		IncludeGlobs: args.IncludeGlobs,
		ExcludeGlobs: args.ExcludeGlobs,
	})
	if err != nil {
		return "", "", "正在搜索文件内容...", "", err
	}
	payload := workspaceToolPayload("search_workspace_text", "", map[string]any{
		"query":         args.Query,
		"regex":         args.Regex,
		"path_scope":    args.PathScope,
		"include_globs": args.IncludeGlobs,
		"exclude_globs": args.ExcludeGlobs,
		"matches":       results,
		"count":         len(results),
		"limited":       limited,
		"next_hint":     "Read the relevant path/range with read_workspace_text before editing, unless the exact replacement is already known.",
	})
	summary := fmt.Sprintf("found %d text match(es)", len(results))
	return mustJSON(payload), summary, "正在搜索文件内容...", "", nil
}

func (s *Service) execReadWorkspaceFile(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在读取工作区文件...", "", err
	}
	var args struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
		MaxChars  int    `json:"max_chars"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在读取工作区文件...", "", fmt.Errorf("invalid tool arguments")
	}
	if isWorkspaceHTMLPath(args.Path) {
		redirectArgs := mustJSON(map[string]any{
			"path": args.Path,
			"view": "structure",
			"mode": "index",
		})
		result, summary, label, path, err := s.execReadWorkspaceNoteView(ctx, userID, redirectArgs)
		if err != nil {
			return result, summary, label, path, err
		}
		return result, "redirected read_workspace_text .html request to note index: " + summary, label, path, nil
	}
	maxChars := args.MaxChars
	if maxChars <= 0 || maxChars > 50000 {
		maxChars = 20000
	}
	result, err := files.ReadTextFileRange(args.Path, args.StartLine, args.EndLine, maxChars)
	label := "正在读取工作区文件..."
	if err != nil {
		return "", "", label, args.Path, err
	}
	summary := fmt.Sprintf("read %s (%d chars, truncated=%t)", result.Path, len([]rune(result.Content)), result.Truncated)
	payload := workspaceToolPayload("read_workspace_text", result.Path, map[string]any{
		"path":             result.Path,
		"content":          result.Content,
		"numbered_content": result.NumberedContent,
		"truncated":        result.Truncated,
		"start_line":       result.StartLine,
		"end_line":         result.EndLine,
		"total_lines":      result.TotalLines,
		"next_hint":        "Use apply_workspace_patch for ordinary text changes or edit_workspace_note_blocks for HTML note changes.",
	})
	return mustJSON(payload), summary, label, result.Path, nil
}

const maxBatchReadFiles = 30

func (s *Service) execReadWorkspaceFiles(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return "", "", "正在批量读取文件...", "", err
	}
	var args struct {
		Paths           []string `json:"paths"`
		StartLine       int      `json:"start_line"`
		EndLine         int      `json:"end_line"`
		MaxCharsPerFile int      `json:"max_chars_per_file"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在批量读取文件...", "", fmt.Errorf("invalid tool arguments")
	}
	paths := normalizeBatchReadPaths(args.Paths)
	if len(paths) == 0 {
		return "", "", "正在批量读取文件...", "", fmt.Errorf("paths is required")
	}
	limitedByCount := false
	if len(paths) > maxBatchReadFiles {
		paths = paths[:maxBatchReadFiles]
		limitedByCount = true
	}
	maxChars := args.MaxCharsPerFile
	if maxChars <= 0 || maxChars > 50000 {
		maxChars = 20000
	}

	type readItem struct {
		Path            string `json:"path"`
		Content         string `json:"content"`
		NumberedContent string `json:"numbered_content,omitempty"`
		Truncated       bool   `json:"truncated"`
		StartLine       int    `json:"start_line"`
		EndLine         int    `json:"end_line"`
		TotalLines      int    `json:"total_lines"`
	}
	type failedItem struct {
		Path  string `json:"path"`
		Error string `json:"error"`
	}
	type noteIndexItem struct {
		Path           string           `json:"path"`
		RedirectedTool string           `json:"redirected_tool"`
		View           string           `json:"view"`
		Mode           string           `json:"mode"`
		DocHash        string           `json:"doc_hash"`
		Blocks         []map[string]any `json:"blocks"`
		Sections       []map[string]any `json:"sections"`
		TotalBlocks    int              `json:"total_blocks"`
		ReturnedBlocks int              `json:"returned_blocks"`
		NextStartBlock int              `json:"next_start_block,omitempty"`
		HasMoreBlocks  bool             `json:"has_more_blocks"`
		TotalTextChars int              `json:"total_text_chars"`
		TotalRawChars  int              `json:"total_raw_chars"`
		NextAction     string           `json:"next_action"`
	}
	items := make([]readItem, 0, len(paths))
	noteIndexes := make([]noteIndexItem, 0)
	failed := make([]failedItem, 0)
	truncated := limitedByCount
	for _, path := range paths {
		if isWorkspaceHTMLPath(path) {
			read, readErr := files.ReadFile(path)
			if readErr != nil {
				failed = append(failed, failedItem{Path: path, Error: readErr.Error()})
				continue
			}
			doc := buildNoteBlockDocument(read.Content)
			page := noteIndexPage(doc.Blocks, 0, 80)
			if page.HasMore {
				truncated = true
			}
			noteIndexes = append(noteIndexes, noteIndexItem{
				Path:           read.Path,
				RedirectedTool: "read_workspace_note_view",
				View:           "structure",
				Mode:           "index",
				DocHash:        doc.DocHash,
				Blocks:         page.Blocks,
				Sections:       noteIndexPayloadSections(doc.Sections),
				TotalBlocks:    len(doc.Blocks),
				ReturnedBlocks: len(page.Blocks),
				NextStartBlock: page.NextStartBlock,
				HasMoreBlocks:  page.HasMore,
				TotalTextChars: doc.TotalTextRune,
				TotalRawChars:  doc.TotalRawRune,
				NextAction:     "Use read_workspace_note_view with mode=block or mode=section for targeted content. Use view=raw only for exact HTML edits.",
			})
			continue
		}
		result, readErr := files.ReadTextFileRange(path, args.StartLine, args.EndLine, maxChars)
		if readErr != nil {
			failed = append(failed, failedItem{Path: path, Error: readErr.Error()})
			continue
		}
		if result.Truncated {
			truncated = true
		}
		items = append(items, readItem{
			Path:            result.Path,
			Content:         result.Content,
			NumberedContent: result.NumberedContent,
			Truncated:       result.Truncated,
			StartLine:       result.StartLine,
			EndLine:         result.EndLine,
			TotalLines:      result.TotalLines,
		})
	}
	if len(items) == 0 && len(noteIndexes) == 0 {
		return "", "", "正在批量读取文件...", "", fmt.Errorf("all requested files failed to read")
	}
	readPaths := make([]string, 0, len(items))
	for _, item := range items {
		readPaths = append(readPaths, item.Path)
	}
	for _, item := range noteIndexes {
		readPaths = append(readPaths, item.Path)
	}
	payload := workspaceToolPayload("read_workspace_files", "", map[string]any{
		"items":            items,
		"note_indexes":     noteIndexes,
		"failed":           failed,
		"paths":            readPaths,
		"count":            len(items),
		"note_index_count": len(noteIndexes),
		"failed_count":     len(failed),
		"truncated":        truncated,
		"limited_by_count": limitedByCount,
		"start_line":       args.StartLine,
		"end_line":         args.EndLine,
		"next_hint":        "Use read_workspace_note_view mode=block/section and edit_workspace_note_blocks for .html notes. Use apply_workspace_patch for ordinary text files.",
	})
	summary := fmt.Sprintf("read %d workspace file(s), indexed %d html note(s), %d failed", len(items), len(noteIndexes), len(failed))
	return mustJSON(payload), summary, "正在批量读取文件...", strings.Join(readPaths, ","), nil
}

func isWorkspaceHTMLPath(path string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(path)), ".html")
}

func normalizeBatchReadPaths(paths []string) []string {
	seen := map[string]bool{}
	next := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
		path = strings.Trim(path, "/")
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		next = append(next, path)
	}
	return next
}
