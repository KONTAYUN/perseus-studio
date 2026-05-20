package ai

import (
	"encoding/json"
	"fmt"
	"sort"
)

type readResultSpan struct {
	Path       string
	StartLine  int
	EndLine    int
	TotalLines int
	Truncated  bool
	Content    string
	HasRange   bool
}

func workspaceEventPaths(event ToolEvent) []string {
	seen := map[string]bool{}
	var paths []string
	for _, path := range append([]string{event.Path, event.NewPath}, event.ChangedPaths...) {
		normalized := normalizeWorkspaceEventPath(path)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		paths = append(paths, normalized)
	}
	return paths
}

func readResultPaths(result *TranscriptToolResult) []string {
	spans := readResultSpans(result)
	if len(spans) == 0 {
		return nil
	}
	seen := map[string]bool{}
	paths := make([]string, 0, len(spans))
	for _, span := range spans {
		if span.Path == "" || seen[span.Path] {
			continue
		}
		seen[span.Path] = true
		paths = append(paths, span.Path)
	}
	sort.Strings(paths)
	return paths
}

func readResultSpans(result *TranscriptToolResult) []readResultSpan {
	if result == nil {
		return nil
	}
	seen := map[string]bool{}
	var spans []readResultSpan
	add := func(path string, startLine, endLine, totalLines int, truncated bool, content string) {
		normalized := normalizeWorkspaceEventPath(path)
		if normalized == "" {
			return
		}
		span := readResultSpan{
			Path:       normalized,
			StartLine:  startLine,
			EndLine:    endLine,
			TotalLines: totalLines,
			Truncated:  truncated,
			Content:    content,
			HasRange:   startLine > 0 && endLine > 0,
		}
		key := readSpanKey(span)
		if seen[key] {
			return
		}
		seen[key] = true
		spans = append(spans, span)
	}
	var payload struct {
		Status     string   `json:"status"`
		Path       string   `json:"path"`
		Paths      []string `json:"paths"`
		Content    string   `json:"content"`
		Truncated  bool     `json:"truncated"`
		StartLine  int      `json:"start_line"`
		EndLine    int      `json:"end_line"`
		TotalLines int      `json:"total_lines"`
		Items      []struct {
			Status     string `json:"status"`
			Path       string `json:"path"`
			Content    string `json:"content"`
			Truncated  bool   `json:"truncated"`
			StartLine  int    `json:"start_line"`
			EndLine    int    `json:"end_line"`
			TotalLines int    `json:"total_lines"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err == nil {
		if payload.Status == "compacted_context" {
			return nil
		}
		add(payload.Path, payload.StartLine, payload.EndLine, payload.TotalLines, payload.Truncated, payload.Content)
		for _, item := range payload.Items {
			if item.Status == "compacted_context" {
				continue
			}
			add(item.Path, item.StartLine, item.EndLine, item.TotalLines, item.Truncated, item.Content)
		}
		if len(spans) == 0 {
			for _, path := range payload.Paths {
				add(path, 0, 0, 0, false, "")
			}
		}
	}
	if len(spans) == 0 && result.ToolName != "read_workspace_files" {
		for _, path := range workspaceEventPaths(result.Event) {
			add(path, 0, 0, 0, false, "")
		}
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].Path != spans[j].Path {
			return spans[i].Path < spans[j].Path
		}
		if spans[i].StartLine != spans[j].StartLine {
			return spans[i].StartLine < spans[j].StartLine
		}
		return spans[i].EndLine < spans[j].EndLine
	})
	return spans
}

func compactBatchReadItemsStaleAfterMutation(result *TranscriptToolResult, mutatedLater map[string]bool) (bool, []string) {
	if result == nil || result.ToolName != "read_workspace_files" || len(mutatedLater) == 0 {
		return false, nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		return false, nil
	}
	rawItems, ok := payload["items"].([]any)
	if !ok || len(rawItems) == 0 {
		return false, nil
	}
	seen := map[string]bool{}
	var stalePaths []string
	for _, rawItem := range rawItems {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		path, _ := item["path"].(string)
		normalized := normalizeWorkspaceEventPath(path)
		if normalized == "" || !mutatedLater[normalized] {
			continue
		}
		item["status"] = "compacted_context"
		item["content"] = ""
		item["stale_after_mutation"] = true
		item["message"] = "This file was modified later in the transcript. Re-read this path if exact current text is needed again."
		if !seen[normalized] {
			seen[normalized] = true
			stalePaths = append(stalePaths, normalized)
		}
	}
	if len(stalePaths) == 0 {
		return false, nil
	}
	sort.Strings(stalePaths)
	payload["items"] = rawItems
	payload["stale_paths"] = stalePaths
	payload["model_view"] = "Some batch read items were compacted because those files were modified later; unaffected items remain available."
	raw, err := json.Marshal(payload)
	if err != nil {
		return false, nil
	}
	result.Content = string(raw)
	return true, stalePaths
}

func readSpanKey(span readResultSpan) string {
	return fmt.Sprintf("%s:%d:%d:%d:%t", span.Path, span.StartLine, span.EndLine, span.TotalLines, span.Truncated)
}

func readSpanLabels(spans []readResultSpan) []string {
	labels := make([]string, 0, len(spans))
	for _, span := range spans {
		if span.HasRange {
			labels = append(labels, fmt.Sprintf("%s:%d-%d", span.Path, span.StartLine, span.EndLine))
			continue
		}
		labels = append(labels, span.Path)
	}
	sort.Strings(labels)
	return labels
}

func readSpanSupersededByAny(older readResultSpan, newerSpans []readResultSpan) bool {
	for _, newer := range newerSpans {
		if readSpanSupersedes(newer, older) {
			return true
		}
	}
	return false
}

func readSpanSupersedes(newer, older readResultSpan) bool {
	if newer.Path == "" || newer.Path != older.Path {
		return false
	}
	if !newer.HasRange || !older.HasRange {
		return !newer.HasRange && !older.HasRange
	}
	if newer.StartLine == older.StartLine && newer.EndLine == older.EndLine {
		if !newer.Truncated {
			return true
		}
		return newer.Content != "" && newer.Content == older.Content
	}
	if newer.Truncated {
		return false
	}
	return newer.StartLine <= older.StartLine && newer.EndLine >= older.EndLine
}
