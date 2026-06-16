package ai

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"webide-backend/internal/store"
)

const (
	recentWorkspaceReadQueryLimit      = 80
	recentWorkspaceReadMaxItems        = 4
	recentWorkspaceReadInlineMaxRunes  = 12000
	recentWorkspaceReadContentMaxRunes = 12000
)

type RecentWorkspaceReadMemory struct {
	Available bool                      `json:"available"`
	Reads     []RecentWorkspaceReadView `json:"reads,omitempty"`
}

type RecentWorkspaceReadView struct {
	Path                            string `json:"path"`
	StartLine                       int    `json:"start_line,omitempty"`
	EndLine                         int    `json:"end_line,omitempty"`
	TotalLines                      int    `json:"total_lines,omitempty"`
	Truncated                       bool   `json:"truncated,omitempty"`
	ReadContentState                string `json:"read_content_state"`
	ContentAvailableInRecentContext bool   `json:"content_available_in_recent_context"`
	NumberedContent                 string `json:"numbered_content,omitempty"`
	ContentChars                    int    `json:"content_chars,omitempty"`
	NumberedContentChars            int    `json:"numbered_content_chars,omitempty"`
	Hash                            string `json:"hash,omitempty"`
	Source                          string `json:"source,omitempty"`
}

func (s *Service) recentWorkspaceReadMemory(ctx context.Context, userID, conversationID string, attachments []AttachmentPayload) RecentWorkspaceReadMemory {
	if s == nil || s.calls == nil || strings.TrimSpace(userID) == "" || strings.TrimSpace(conversationID) == "" {
		return RecentWorkspaceReadMemory{}
	}
	calls, err := s.calls.ListRecentToolCalls(ctx, userID, conversationID, recentWorkspaceReadQueryLimit)
	if err != nil {
		return RecentWorkspaceReadMemory{}
	}
	targetPaths := workspaceReferencePathsFromAttachments(attachments)
	mutatedLater := map[string]bool{}
	seenReads := map[string]bool{}
	var reads []RecentWorkspaceReadView
	for _, call := range calls {
		if call.Status != "success" {
			continue
		}
		if isWorkspaceMutationTool(call.ToolName) {
			for _, path := range toolCallChangedPaths(call) {
				mutatedLater[path] = true
			}
			continue
		}
		if !isReadWorkspaceTool(call.ToolName) {
			continue
		}
		for _, read := range recentReadViewsFromToolCall(call) {
			if read.Path == "" || mutatedLater[read.Path] || seenReads[read.Path] {
				continue
			}
			if len(targetPaths) > 0 && !targetPaths[read.Path] {
				continue
			}
			seenReads[read.Path] = true
			reads = append(reads, read)
			if len(reads) >= recentWorkspaceReadMaxItems {
				break
			}
		}
		if len(reads) >= recentWorkspaceReadMaxItems {
			break
		}
	}
	sort.SliceStable(reads, func(i, j int) bool { return reads[i].Path < reads[j].Path })
	return RecentWorkspaceReadMemory{Available: len(reads) > 0, Reads: reads}
}

func workspaceReferencePathsFromAttachments(attachments []AttachmentPayload) map[string]bool {
	paths := map[string]bool{}
	add := func(path string) {
		if normalized := normalizeWorkspaceEventPath(path); normalized != "" {
			paths[normalized] = true
		}
	}
	for _, attachment := range attachments {
		switch attachment.Kind {
		case "workspace_file", "current_file":
			add(attachment.Path)
		case "editor_panels_context":
			for _, panel := range attachment.EditorPanels {
				add(panel.ActiveFilePath)
				for _, tab := range panel.OpenTabs {
					add(tab)
				}
			}
		}
	}
	return paths
}

func toolCallChangedPaths(call store.ToolCall) []string {
	seen := map[string]bool{}
	var paths []string
	add := func(path string) {
		normalized := normalizeWorkspaceEventPath(path)
		if normalized == "" || seen[normalized] {
			return
		}
		seen[normalized] = true
		paths = append(paths, normalized)
	}
	for _, path := range stringSliceFromJSON(call.ChangedPathsJSON) {
		add(path)
	}
	var payload map[string]any
	if json.Unmarshal([]byte(call.ResultJSON), &payload) == nil {
		add(stringFromAny(payload["path"]))
		add(stringFromAny(payload["old_path"]))
		add(stringFromAny(payload["new_path"]))
		for _, path := range stringSliceFromAnyLoose(payload["changed_paths"]) {
			add(path)
		}
	}
	sort.Strings(paths)
	return paths
}

func recentReadViewsFromToolCall(call store.ToolCall) []RecentWorkspaceReadView {
	var payload map[string]any
	if err := json.Unmarshal([]byte(call.ResultJSON), &payload); err != nil {
		return nil
	}
	var reads []RecentWorkspaceReadView
	if rawItems := payloadItems(payload); len(rawItems) > 0 {
		for _, item := range rawItems {
			if read := recentReadViewFromPayload(call, item, stringFromAny(payload["path"])); read.Path != "" {
				reads = append(reads, read)
			}
		}
		return reads
	}
	if read := recentReadViewFromPayload(call, payload, ""); read.Path != "" {
		reads = append(reads, read)
	}
	return reads
}

func recentReadViewFromPayload(call store.ToolCall, payload map[string]any, fallbackPath string) RecentWorkspaceReadView {
	path := normalizeWorkspaceEventPath(firstNonEmpty(stringFromAny(payload["path"]), fallbackPath))
	numbered := stringFromAny(payload["numbered_content"])
	content := stringFromAny(payload["content"])
	contentForModel := numbered
	if contentForModel == "" {
		contentForModel = content
	}
	state := firstNonEmpty(stringFromAny(payload["read_content_state"]), inferredReadContentState(payload))
	if state == "" {
		state = string(ReadContentStateMetadataOnly)
	}
	view := RecentWorkspaceReadView{
		Path:                 path,
		StartLine:            intFromAny(payload["start_line"]),
		EndLine:              intFromAny(payload["end_line"]),
		TotalLines:           intFromAny(payload["total_lines"]),
		Truncated:            boolFromAny(payload["truncated"]),
		ReadContentState:     state,
		ContentChars:         len([]rune(content)),
		NumberedContentChars: len([]rune(numbered)),
		Hash:                 call.ContentHash,
		Source:               "tool_calls.result_json",
	}
	if len([]rune(contentForModel)) > 0 && len([]rune(contentForModel)) <= recentWorkspaceReadInlineMaxRunes {
		view.NumberedContent, _ = truncatePlainForModel(contentForModel, recentWorkspaceReadContentMaxRunes)
		view.ContentAvailableInRecentContext = strings.TrimSpace(view.NumberedContent) != ""
		view.ReadContentState = string(ReadContentStateAvailable)
	}
	if view.Path == "" {
		return RecentWorkspaceReadView{}
	}
	return view
}

func stringSliceFromJSON(raw string) []string {
	var values []string
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &values) != nil {
		return nil
	}
	return values
}

func recentWorkspaceReadMemoryPaths(memory RecentWorkspaceReadMemory) []string {
	if !memory.Available {
		return nil
	}
	paths := make([]string, 0, len(memory.Reads))
	for _, read := range memory.Reads {
		if read.Path != "" {
			paths = append(paths, read.Path)
		}
	}
	sort.Strings(paths)
	return paths
}

func recentWorkspaceReadMemoryInlinedChars(memory RecentWorkspaceReadMemory) int {
	total := 0
	for _, read := range memory.Reads {
		total += len([]rune(read.NumberedContent))
	}
	return total
}
