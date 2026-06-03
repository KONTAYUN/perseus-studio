package ai

import (
	"encoding/json"
)

const ledgerToolResultMaxRunes = 20000

type LedgerToolResultSnapshot struct {
	ToolName              string         `json:"tool_name,omitempty"`
	Status                string         `json:"status,omitempty"`
	Path                  string         `json:"path,omitempty"`
	Paths                 []string       `json:"paths,omitempty"`
	ChangedPaths          []string       `json:"changed_paths,omitempty"`
	StartLine             int            `json:"start_line,omitempty"`
	EndLine               int            `json:"end_line,omitempty"`
	TotalLines            int            `json:"total_lines,omitempty"`
	Truncated             bool           `json:"truncated,omitempty"`
	SourceReadTruncated   bool           `json:"source_read_truncated,omitempty"`
	ModelViewTruncated    bool           `json:"model_view_truncated,omitempty"`
	LedgerResultTruncated bool           `json:"ledger_result_truncated"`
	OriginalChars         int            `json:"original_chars"`
	OriginalBytes         int            `json:"original_bytes"`
	ContentChars          int            `json:"content_chars,omitempty"`
	NumberedContentChars  int            `json:"numbered_content_chars,omitempty"`
	Items                 []map[string]any `json:"items,omitempty"`
	PreviewHead           string         `json:"preview_head,omitempty"`
	PreviewTail           string         `json:"preview_tail,omitempty"`
}

func ledgerToolResultJSON(toolName, result string) string {
	if result == "" {
		return result
	}
	if len([]rune(result)) <= ledgerToolResultMaxRunes && json.Valid([]byte(result)) {
		return result
	}
	snapshot := buildLedgerToolResultSnapshot(toolName, result)
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return mustJSON(map[string]any{
			"tool_name":                toolName,
			"ledger_result_truncated":  true,
			"original_chars":           len([]rune(result)),
			"original_bytes":           len([]byte(result)),
		})
	}
	if len([]rune(string(raw))) <= ledgerToolResultMaxRunes {
		return string(raw)
	}
	snapshot.PreviewHead, _ = truncatePlainForModel(snapshot.PreviewHead, 1200)
	snapshot.PreviewTail, _ = truncatePlainForModel(snapshot.PreviewTail, 1200)
	raw, err = json.Marshal(snapshot)
	if err != nil {
		return mustJSON(map[string]any{
			"tool_name":               toolName,
			"ledger_result_truncated": true,
			"original_chars":          len([]rune(result)),
			"original_bytes":          len([]byte(result)),
		})
	}
	return string(raw)
}

func buildLedgerToolResultSnapshot(toolName, result string) LedgerToolResultSnapshot {
	head, tail := ledgerHeadTail(result, 2400)
	snapshot := LedgerToolResultSnapshot{
		ToolName:              toolName,
		LedgerResultTruncated: true,
		OriginalChars:         len([]rune(result)),
		OriginalBytes:         len([]byte(result)),
		PreviewHead:           head,
		PreviewTail:           tail,
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		return snapshot
	}
	snapshot.ToolName = firstNonEmpty(toolName, stringFromAny(payload["tool_name"]), stringFromAny(payload["tool"]))
	snapshot.Status = stringFromAny(payload["status"])
	snapshot.Path = normalizeWorkspaceEventPath(stringFromAny(payload["path"]))
	snapshot.Paths = normalizedProviderPaths(stringSliceFromAnyLoose(payload["paths"]))
	snapshot.ChangedPaths = normalizedProviderPaths(stringSliceFromAnyLoose(payload["changed_paths"]))
	snapshot.StartLine = intFromAny(payload["start_line"])
	snapshot.EndLine = intFromAny(payload["end_line"])
	snapshot.TotalLines = intFromAny(payload["total_lines"])
	snapshot.Truncated = boolFromAny(payload["truncated"])
	snapshot.SourceReadTruncated = snapshot.Truncated
	snapshot.ModelViewTruncated = boolFromAny(payload["model_view_truncated"])
	snapshot.ContentChars = len([]rune(stringFromAny(payload["content"])))
	snapshot.NumberedContentChars = len([]rune(stringFromAny(payload["numbered_content"])))
	if rawItems, ok := payload["items"].([]any); ok {
		for _, rawItem := range rawItems {
			item, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}
			next := map[string]any{
				"path":                   normalizeWorkspaceEventPath(stringFromAny(item["path"])),
				"start_line":             intFromAny(item["start_line"]),
				"end_line":               intFromAny(item["end_line"]),
				"total_lines":            intFromAny(item["total_lines"]),
				"truncated":              boolFromAny(item["truncated"]),
				"model_view_truncated":   boolFromAny(item["model_view_truncated"]),
				"content_chars":          len([]rune(stringFromAny(item["content"]))),
				"numbered_content_chars": len([]rune(stringFromAny(item["numbered_content"]))),
			}
			snapshot.Items = append(snapshot.Items, next)
			if boolFromAny(item["truncated"]) {
				snapshot.SourceReadTruncated = true
			}
			if boolFromAny(item["model_view_truncated"]) {
				snapshot.ModelViewTruncated = true
			}
		}
	}
	return snapshot
}

func ledgerHeadTail(value string, maxRunes int) (string, string) {
	runes := []rune(value)
	if maxRunes <= 0 {
		maxRunes = 2000
	}
	if len(runes) <= maxRunes*2 {
		return value, ""
	}
	return string(runes[:maxRunes]), string(runes[len(runes)-maxRunes:])
}
