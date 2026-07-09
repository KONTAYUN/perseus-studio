package ai

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type readResultSpan struct {
	Path       string
	ToolName   string
	Mode       string
	View       string
	BlockID    string
	SectionID  string
	DocHash    string
	LedgerID   string
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
	add := func(path string, startLine, endLine, totalLines int, truncated bool, content string, meta AgentReadTarget) {
		normalized := normalizeWorkspaceEventPath(path)
		if normalized == "" {
			return
		}
		span := readResultSpan{
			Path:       normalized,
			ToolName:   firstNonEmpty(meta.ToolName, result.ToolName),
			Mode:       meta.Mode,
			View:       meta.View,
			BlockID:    meta.BlockID,
			SectionID:  meta.SectionID,
			DocHash:    meta.DocHash,
			LedgerID:   meta.LedgerID,
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
		Status          string   `json:"status"`
		Path            string   `json:"path"`
		Paths           []string `json:"paths"`
		Content         string   `json:"content"`
		NumberedContent string   `json:"numbered_content"`
		View            string   `json:"view"`
		Mode            string   `json:"mode"`
		BlockID         string   `json:"block_id"`
		SectionID       string   `json:"section_id"`
		DocHash         string   `json:"doc_hash"`
		LedgerID        string   `json:"ledger_id"`
		Truncated       bool     `json:"truncated"`
		StartLine       int      `json:"start_line"`
		EndLine         int      `json:"end_line"`
		TotalLines      int      `json:"total_lines"`
		Items           []struct {
			Status          string `json:"status"`
			Path            string `json:"path"`
			Content         string `json:"content"`
			NumberedContent string `json:"numbered_content"`
			View            string `json:"view"`
			Mode            string `json:"mode"`
			BlockID         string `json:"block_id"`
			SectionID       string `json:"section_id"`
			DocHash         string `json:"doc_hash"`
			LedgerID        string `json:"ledger_id"`
			Truncated       bool   `json:"truncated"`
			StartLine       int    `json:"start_line"`
			EndLine         int    `json:"end_line"`
			TotalLines      int    `json:"total_lines"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err == nil {
		if isContextReferenceStatus(payload.Status) {
			return nil
		}
		payloadMeta := AgentReadTarget{
			ToolName:  result.ToolName,
			View:      payload.View,
			Mode:      payload.Mode,
			BlockID:   payload.BlockID,
			SectionID: payload.SectionID,
			DocHash:   payload.DocHash,
			LedgerID:  payload.LedgerID,
		}
		add(payload.Path, payload.StartLine, payload.EndLine, payload.TotalLines, payload.Truncated, readSpanContent(payload.Content, payload.NumberedContent), payloadMeta)
		for _, item := range payload.Items {
			if isContextReferenceStatus(item.Status) {
				continue
			}
			itemMeta := AgentReadTarget{
				ToolName:  result.ToolName,
				View:      firstNonEmpty(item.View, payload.View),
				Mode:      firstNonEmpty(item.Mode, payload.Mode),
				BlockID:   firstNonEmpty(item.BlockID, payload.BlockID),
				SectionID: firstNonEmpty(item.SectionID, payload.SectionID),
				DocHash:   firstNonEmpty(item.DocHash, payload.DocHash),
				LedgerID:  firstNonEmpty(item.LedgerID, payload.LedgerID),
			}
			add(item.Path, item.StartLine, item.EndLine, item.TotalLines, item.Truncated, readSpanContent(item.Content, item.NumberedContent), itemMeta)
		}
		if len(spans) == 0 {
			for _, path := range payload.Paths {
				add(path, 0, 0, 0, false, "", payloadMeta)
			}
		}
	}
	if len(spans) == 0 && result.ToolName != "read_workspace_files" {
		for _, path := range workspaceEventPaths(result.Event) {
			add(path, 0, 0, 0, false, "", AgentReadTarget{ToolName: result.ToolName})
		}
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].Path != spans[j].Path {
			return spans[i].Path < spans[j].Path
		}
		if spans[i].Mode != spans[j].Mode {
			return spans[i].Mode < spans[j].Mode
		}
		if spans[i].View != spans[j].View {
			return spans[i].View < spans[j].View
		}
		if spans[i].BlockID != spans[j].BlockID {
			return spans[i].BlockID < spans[j].BlockID
		}
		if spans[i].SectionID != spans[j].SectionID {
			return spans[i].SectionID < spans[j].SectionID
		}
		if spans[i].StartLine != spans[j].StartLine {
			return spans[i].StartLine < spans[j].StartLine
		}
		return spans[i].EndLine < spans[j].EndLine
	})
	return spans
}

func readSpanContent(content, numberedContent string) string {
	if content != "" {
		return content
	}
	return numberedContent
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
		item["status"] = "context_reference"
		item["content"] = ""
		item["numbered_content"] = ""
		item["stale_after_mutation"] = true
		item["content_state"] = "stale_after_later_mutation"
		item["next_action"] = "Read this path again before relying on exact current text because it changed later."
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
	payload["model_view"] = "Some batch read items are stale after later file changes; unaffected items remain available."
	raw, err := json.Marshal(payload)
	if err != nil {
		return false, nil
	}
	result.Content = string(raw)
	return true, stalePaths
}

func compactReadResultForPayload(result *TranscriptToolResult) bool {
	if result == nil || !isReadWorkspaceTool(result.ToolName) {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		return false
	}
	if isContextReferenceStatus(stringFromAny(payload["status"])) {
		return false
	}
	changed := false
	if compactReadPayloadObjectForPayload(payload, result.ToolName) {
		changed = true
	}
	if rawItems, ok := payload["items"].([]any); ok {
		for _, rawItem := range rawItems {
			item, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}
			if compactReadPayloadObjectForPayload(item, result.ToolName) {
				changed = true
			}
		}
		payload["items"] = rawItems
	}
	if !changed {
		return false
	}
	payload["provider_context_reference"] = true
	payload["model_view"] = "Read path and line-range metadata remains available. Use preserved recent reads when available; call read_workspace_text/read_workspace_files with the specific path and range only when exact text is missing."
	raw, err := json.Marshal(payload)
	if err != nil {
		return false
	}
	result.Content = string(raw)
	return true
}

func compactReadPayloadObjectForPayload(payload map[string]any, toolName string) bool {
	if payload == nil {
		return false
	}
	content := stringFromAny(payload["content"])
	numbered := stringFromAny(payload["numbered_content"])
	if content == "" && numbered == "" {
		return false
	}
	if content != "" {
		payload["content_chars"] = len([]rune(content))
		payload["content_bytes"] = len([]byte(content))
		payload["content"] = ""
		payload["content_state"] = "reference_only"
	}
	if numbered != "" {
		payload["numbered_content_chars"] = len([]rune(numbered))
		payload["numbered_content_bytes"] = len([]byte(numbered))
		payload["numbered_content"] = ""
		payload["numbered_content_state"] = "reference_only"
	}
	payload["status"] = "context_reference"
	if toolName == "read_workspace_note_view" {
		payload["next_action"] = "Use a preserved recent note block/section when available; call read_workspace_note_view mode=index, then mode=block or mode=section only when exact HTML is missing."
	} else {
		payload["next_action"] = "Use a preserved recent read when available; call read_workspace_text/read_workspace_files with start_line/end_line only when exact text is missing."
	}
	return true
}

func isContextReferenceStatus(status string) bool {
	status = strings.TrimSpace(status)
	return status == "compacted_context" || status == "context_reference"
}

func readSpanKey(span readResultSpan) string {
	if span.ToolName == "read_workspace_note_view" || span.Mode != "" || span.BlockID != "" || span.SectionID != "" || span.DocHash != "" {
		return fmt.Sprintf("%s:%s:%s:%s:%s:%s:%d:%d:%d:%t", span.Path, span.Mode, span.View, span.DocHash, span.BlockID, span.SectionID, span.StartLine, span.EndLine, span.TotalLines, span.Truncated)
	}
	return fmt.Sprintf("%s:%d:%d:%d:%t", span.Path, span.StartLine, span.EndLine, span.TotalLines, span.Truncated)
}

func readSpanLabels(spans []readResultSpan) []string {
	labels := make([]string, 0, len(spans))
	for _, span := range spans {
		if span.HasRange {
			labels = append(labels, fmt.Sprintf("%s:%d-%d", span.Path, span.StartLine, span.EndLine))
			continue
		}
		if span.ToolName == "read_workspace_note_view" || span.Mode != "" || span.BlockID != "" || span.SectionID != "" {
			labels = append(labels, noteReadSpanLabel(span))
			continue
		}
		labels = append(labels, span.Path)
	}
	sort.Strings(labels)
	return labels
}

func currentReadWorkingSetFromTranscript(transcript *AgentTranscript, limit int) CurrentReadWorkingSet {
	set := CurrentReadWorkingSet{}
	if transcript == nil || limit == 0 {
		return set
	}
	current := currentSuccessfulReadToolCalls(transcript)
	seen := map[string]bool{}
	for entryIndex := len(transcript.Entries) - 1; entryIndex >= 0; entryIndex-- {
		entry := transcript.Entries[entryIndex]
		if entry.Kind != transcriptEntryTool {
			continue
		}
		for resultIndex := len(entry.ToolResults) - 1; resultIndex >= 0; resultIndex-- {
			result := entry.ToolResults[resultIndex]
			if !isReadWorkspaceTool(result.ToolName) || result.IsError || !current[result.ToolCallID] {
				continue
			}
			contentAvailable := readToolResultContentBytes(result.Content) > 0
			for _, span := range readResultSpans(&result) {
				if span.Path == "" {
					continue
				}
				key := readSpanKey(span)
				if seen[key] {
					continue
				}
				seen[key] = true
				state := readContentStateAvailableForSpan(span, contentAvailable)
				set.Spans = append(set.Spans, CurrentReadSpanView{
					Path:                            span.Path,
					ToolName:                        span.ToolName,
					Mode:                            span.Mode,
					View:                            span.View,
					BlockID:                         span.BlockID,
					SectionID:                       span.SectionID,
					DocHash:                         span.DocHash,
					LedgerID:                        span.LedgerID,
					StartLine:                       span.StartLine,
					EndLine:                         span.EndLine,
					TotalLines:                      span.TotalLines,
					ReadContentState:                state,
					ContentAvailableInRecentContext: contentAvailable && state == string(ReadContentStateAvailable),
				})
				if limit > 0 && len(set.Spans) >= limit {
					set.Available = len(set.Spans) > 0
					return set
				}
			}
		}
	}
	set.Available = len(set.Spans) > 0
	return set
}

func readContentStateAvailableForSpan(span readResultSpan, contentAvailable bool) string {
	if span.Truncated {
		return string(ReadContentStateSourceTruncated)
	}
	if contentAvailable && strings.TrimSpace(span.Content) != "" {
		return string(ReadContentStateAvailable)
	}
	return string(ReadContentStateMetadataOnly)
}

func currentReadWorkingSetLabels(set CurrentReadWorkingSet) []string {
	if len(set.Spans) == 0 {
		return nil
	}
	labels := make([]string, 0, len(set.Spans))
	for _, span := range set.Spans {
		if span.Path == "" {
			continue
		}
		if span.StartLine > 0 && span.EndLine > 0 {
			labels = append(labels, fmt.Sprintf("%s:%d-%d", span.Path, span.StartLine, span.EndLine))
			continue
		}
		if span.ToolName == "read_workspace_note_view" || span.Mode != "" || span.BlockID != "" || span.SectionID != "" {
			labels = append(labels, noteWorkingSetSpanLabel(span))
			continue
		}
		labels = append(labels, span.Path)
	}
	sort.Strings(labels)
	return labels
}

func noteWorkingSetSpanLabel(span CurrentReadSpanView) string {
	parts := []string{span.Path}
	if span.Mode != "" {
		parts = append(parts, "mode="+span.Mode)
	}
	if span.View != "" {
		parts = append(parts, "view="+span.View)
	}
	if span.BlockID != "" {
		parts = append(parts, "block="+span.BlockID)
	}
	if span.SectionID != "" {
		parts = append(parts, "section="+span.SectionID)
	}
	if span.DocHash != "" {
		parts = append(parts, "doc="+truncatePlainRunes(span.DocHash, 12))
	}
	return strings.Join(nonEmptyStrings(parts...), " ")
}

func currentReadWorkingSetPaths(set CurrentReadWorkingSet) []string {
	if len(set.Spans) == 0 {
		return nil
	}
	seen := map[string]bool{}
	paths := make([]string, 0, len(set.Spans))
	for _, span := range set.Spans {
		if span.Path == "" || seen[span.Path] {
			continue
		}
		seen[span.Path] = true
		paths = append(paths, span.Path)
	}
	sort.Strings(paths)
	return paths
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
	if newer.ToolName == "read_workspace_note_view" || older.ToolName == "read_workspace_note_view" || newer.Mode != "" || older.Mode != "" {
		return noteReadSpanSupersedes(newer, older)
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

func noteReadSpanLabel(span readResultSpan) string {
	parts := []string{span.Path}
	if span.Mode != "" {
		parts = append(parts, "mode="+span.Mode)
	}
	if span.View != "" {
		parts = append(parts, "view="+span.View)
	}
	if span.BlockID != "" {
		parts = append(parts, "block="+span.BlockID)
	}
	if span.SectionID != "" {
		parts = append(parts, "section="+span.SectionID)
	}
	if span.DocHash != "" {
		parts = append(parts, "doc="+truncatePlainRunes(span.DocHash, 12))
	}
	return strings.Join(nonEmptyStrings(parts...), " ")
}

func noteReadSpanSupersedes(newer, older readResultSpan) bool {
	if newer.Path != older.Path {
		return false
	}
	if firstNonEmpty(newer.Mode, "full") != firstNonEmpty(older.Mode, "full") ||
		firstNonEmpty(newer.View, "structure") != firstNonEmpty(older.View, "structure") ||
		newer.DocHash != older.DocHash ||
		newer.BlockID != older.BlockID ||
		newer.SectionID != older.SectionID {
		return false
	}
	if !newer.Truncated {
		return true
	}
	return newer.Content != "" && newer.Content == older.Content
}
