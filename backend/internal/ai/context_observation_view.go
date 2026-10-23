package ai

import (
	"encoding/json"
	"sort"
)

type ProviderObservationView struct {
	ToolName     string                      `json:"tool_name"`
	Status       string                      `json:"status"`
	Path         string                      `json:"path,omitempty"`
	ChangedPaths []string                    `json:"changed_paths,omitempty"`
	Summary      string                      `json:"summary,omitempty"`
	ReadRanges   []ProviderReadReferenceView `json:"read_ranges,omitempty"`
	NextAction   string                      `json:"next_action,omitempty"`
}

type ProviderReadReferenceView struct {
	Path                 string `json:"path"`
	StartLine            int    `json:"start_line,omitempty"`
	EndLine              int    `json:"end_line,omitempty"`
	TotalLines           int    `json:"total_lines,omitempty"`
	Truncated            bool   `json:"truncated,omitempty"`
	ReadContentState     string `json:"read_content_state,omitempty"`
	ContentChars         int    `json:"content_chars,omitempty"`
	ContentBytes         int    `json:"content_bytes,omitempty"`
	NumberedContentChars int    `json:"numbered_content_chars,omitempty"`
	NumberedContentBytes int    `json:"numbered_content_bytes,omitempty"`
}

func providerToolObservationContent(result TranscriptToolResult) string {
	payload, ok := decodedToolResultPayload(result.Content)
	if !ok {
		return result.Content
	}
	if !needsProviderObservationView(payload) {
		if isReadWorkspaceTool(firstNonEmpty(result.ToolName, result.Event.ToolName)) {
			// Checkpoints from before the encoding fix may contain HTML escapes.
			// Canonicalize their JSON representation without changing file strings.
			return mustJSON(payload)
		}
		return result.Content
	}
	if isReadWorkspaceTool(firstNonEmpty(result.ToolName, result.Event.ToolName)) {
		return mustJSON(providerReadObservationPayload(result, payload))
	}
	view := buildProviderObservationView(result, payload)
	return mustJSON(view)
}

func decodedToolResultPayload(content string) (map[string]any, bool) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return nil, false
	}
	return payload, true
}

func needsProviderObservationView(payload map[string]any) bool {
	if payload == nil {
		return false
	}
	if isContextReferenceStatus(stringFromAny(payload["status"])) {
		return true
	}
	if boolFromAny(payload["provider_context_reference"]) {
		return true
	}
	if stringFromAny(payload["content_state"]) != "" || stringFromAny(payload["numbered_content_state"]) != "" {
		return true
	}
	for _, item := range payloadItems(payload) {
		if isContextReferenceStatus(stringFromAny(item["status"])) ||
			stringFromAny(item["content_state"]) != "" ||
			stringFromAny(item["numbered_content_state"]) != "" {
			return true
		}
	}
	return false
}

func buildProviderObservationView(result TranscriptToolResult, payload map[string]any) ProviderObservationView {
	status := stringFromAny(payload["status"])
	if status == "" || isContextReferenceStatus(status) {
		status = result.Event.Status
	}
	if status == "" {
		status = "success"
	}
	view := ProviderObservationView{
		ToolName:     firstNonEmpty(result.ToolName, result.Event.ToolName),
		Status:       status,
		Path:         firstNonEmpty(stringFromAny(payload["path"]), result.Event.Path),
		ChangedPaths: providerObservationChangedPaths(result, payload),
		Summary:      stringFromAny(payload["summary"]),
	}
	if isReadWorkspaceTool(view.ToolName) {
		view.ReadRanges = providerReadReferenceViews(result, payload)
		view.NextAction = "Use preserved numbered_content when available; call read_workspace_text or read_workspace_files with the path and line range only when exact text is missing, truncated, stale, or outside the available range."
		return view
	}
	if view.Summary == "" {
		view.Summary = firstNonEmpty(result.Event.TargetSummary, result.Event.Label)
	}
	if view.Summary == "" {
		view.Summary = view.ToolName + " completed"
	}
	return view
}

func providerReadObservationPayload(result TranscriptToolResult, payload map[string]any) map[string]any {
	next := map[string]any{
		"tool_name": firstNonEmpty(result.ToolName, result.Event.ToolName),
		"status":    providerObservationStatus(result, payload),
	}
	copyStringField(next, payload, "summary")
	copyNumberField(next, payload, "count")
	copyNumberField(next, payload, "failed_count")
	copyNumberField(next, payload, "start_line")
	copyNumberField(next, payload, "end_line")
	copyNumberField(next, payload, "total_lines")
	copyBoolField(next, payload, "truncated")
	if path := firstNonEmpty(stringFromAny(payload["path"]), result.Event.Path); path != "" {
		next["path"] = normalizeWorkspaceEventPath(path)
	}
	if paths := stringSliceFromAnyLoose(payload["paths"]); len(paths) > 0 {
		next["paths"] = normalizedProviderPaths(paths)
	}
	if failed, ok := payload["failed"]; ok {
		next["failed"] = failed
	}
	rawItems := payloadItems(payload)
	if len(rawItems) > 0 {
		items := make([]map[string]any, 0, len(rawItems))
		for _, item := range rawItems {
			items = append(items, providerReadObservationItem(result, item, stringFromAny(payload["path"])))
		}
		next["items"] = items
	} else if providerReadObjectIsReference(payload) {
		next["read_ranges"] = providerReadReferenceViews(result, payload)
	} else {
		copyStringField(next, payload, "content")
		copyStringField(next, payload, "numbered_content")
	}
	next["next_action"] = "Use preserved numbered_content when available; read by path and line range only when exact text is missing, truncated, stale, or outside the available range."
	return next
}

func providerReadObservationItem(result TranscriptToolResult, item map[string]any, fallbackPath string) map[string]any {
	path := normalizeWorkspaceEventPath(firstNonEmpty(stringFromAny(item["path"]), fallbackPath, result.Event.Path))
	if providerReadObjectIsReference(item) {
		view := providerReadReferenceViewFromObject(item, path)
		return map[string]any{
			"path":               view.Path,
			"read_content_state": view.ReadContentState,
			"read_ranges":        []ProviderReadReferenceView{view},
			"next_action":        "Read this path and line range only if exact text is needed and not already available in recent context.",
		}
	}
	next := map[string]any{}
	if path != "" {
		next["path"] = path
	}
	copyStringField(next, item, "content")
	copyStringField(next, item, "numbered_content")
	copyNumberField(next, item, "start_line")
	copyNumberField(next, item, "end_line")
	copyNumberField(next, item, "total_lines")
	copyBoolField(next, item, "truncated")
	if state := firstNonEmpty(stringFromAny(item["read_content_state"]), inferredReadContentState(item)); state != "" {
		next["read_content_state"] = state
	}
	copyBoolField(next, item, "model_view_truncated")
	copyNumberField(next, item, "next_start_line")
	return next
}

func providerReadObjectIsReference(payload map[string]any) bool {
	if payload == nil {
		return false
	}
	return isContextReferenceStatus(stringFromAny(payload["status"])) ||
		stringFromAny(payload["content_state"]) != "" ||
		stringFromAny(payload["numbered_content_state"]) != ""
}

func providerObservationStatus(result TranscriptToolResult, payload map[string]any) string {
	status := stringFromAny(payload["status"])
	if status == "" || isContextReferenceStatus(status) {
		status = result.Event.Status
	}
	if status == "" {
		status = "success"
	}
	return status
}

func providerObservationChangedPaths(result TranscriptToolResult, payload map[string]any) []string {
	seen := map[string]bool{}
	var paths []string
	for _, path := range append(stringSliceFromAnyLoose(payload["changed_paths"]), result.Event.ChangedPaths...) {
		normalized := normalizeWorkspaceEventPath(path)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		paths = append(paths, normalized)
	}
	sort.Strings(paths)
	return paths
}

func providerReadReferenceViews(result TranscriptToolResult, payload map[string]any) []ProviderReadReferenceView {
	var views []ProviderReadReferenceView
	add := func(item map[string]any, fallbackPath string) {
		if item == nil {
			return
		}
		path := normalizeWorkspaceEventPath(firstNonEmpty(stringFromAny(item["path"]), fallbackPath))
		if path == "" {
			return
		}
		views = append(views, providerReadReferenceViewFromObject(item, path))
	}
	fallbackPath := firstNonEmpty(result.Event.Path, stringFromAny(payload["path"]))
	add(payload, fallbackPath)
	for _, item := range payloadItems(payload) {
		add(item, fallbackPath)
	}
	if len(views) == 0 {
		for _, path := range append(stringSliceFromAnyLoose(payload["paths"]), workspaceEventPaths(result.Event)...) {
			add(map[string]any{"path": path}, "")
		}
	}
	sort.SliceStable(views, func(i, j int) bool {
		if views[i].Path != views[j].Path {
			return views[i].Path < views[j].Path
		}
		if views[i].StartLine != views[j].StartLine {
			return views[i].StartLine < views[j].StartLine
		}
		return views[i].EndLine < views[j].EndLine
	})
	return views
}

func providerReadReferenceViewFromObject(item map[string]any, path string) ProviderReadReferenceView {
	return ProviderReadReferenceView{
		Path:                 normalizeWorkspaceEventPath(path),
		StartLine:            intFromAny(item["start_line"]),
		EndLine:              intFromAny(item["end_line"]),
		TotalLines:           intFromAny(item["total_lines"]),
		Truncated:            boolFromAny(item["truncated"]),
		ReadContentState:     firstNonEmpty(stringFromAny(item["read_content_state"]), inferredReadContentState(item), string(ReadContentStateMetadataOnly)),
		ContentChars:         intFromAny(item["content_chars"]),
		ContentBytes:         intFromAny(item["content_bytes"]),
		NumberedContentChars: intFromAny(item["numbered_content_chars"]),
		NumberedContentBytes: intFromAny(item["numbered_content_bytes"]),
	}
}

func inferredReadContentState(item map[string]any) string {
	if item == nil {
		return ""
	}
	if boolFromAny(item["model_view_truncated"]) {
		return string(ReadContentStateModelViewTruncated)
	}
	if boolFromAny(item["truncated"]) {
		return string(ReadContentStateSourceTruncated)
	}
	if stringFromAny(item["content"]) != "" || stringFromAny(item["numbered_content"]) != "" {
		return string(ReadContentStateAvailable)
	}
	if providerReadObjectIsReference(item) {
		return string(ReadContentStateMetadataOnly)
	}
	return ""
}

func payloadItems(payload map[string]any) []map[string]any {
	rawItems, ok := payload["items"].([]any)
	if !ok || len(rawItems) == 0 {
		return nil
	}
	items := make([]map[string]any, 0, len(rawItems))
	for _, raw := range rawItems {
		if item, ok := raw.(map[string]any); ok {
			items = append(items, item)
		}
	}
	return items
}

func stringSliceFromAnyLoose(value any) []string {
	items, ok := stringSliceFromAny(value)
	if !ok {
		return nil
	}
	return items
}

func boolFromAny(value any) bool {
	switch item := value.(type) {
	case bool:
		return item
	default:
		return false
	}
}

func copyStringField(target map[string]any, source map[string]any, key string) {
	if value := stringFromAny(source[key]); value != "" {
		target[key] = value
	}
}

func copyNumberField(target map[string]any, source map[string]any, key string) {
	if value := intFromAny(source[key]); value != 0 {
		target[key] = value
	}
}

func copyBoolField(target map[string]any, source map[string]any, key string) {
	if value, ok := source[key].(bool); ok {
		target[key] = value
	}
}

func normalizedProviderPaths(paths []string) []string {
	seen := map[string]bool{}
	var next []string
	for _, path := range paths {
		normalized := normalizeWorkspaceEventPath(path)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		next = append(next, normalized)
	}
	sort.Strings(next)
	return next
}
