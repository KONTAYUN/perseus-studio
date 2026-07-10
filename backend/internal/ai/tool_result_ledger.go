package ai

import (
	"encoding/json"
)

const ledgerToolResultMaxRunes = 20000

type LedgerToolResultSnapshot struct {
	ToolName              string           `json:"tool_name,omitempty"`
	Status                string           `json:"status,omitempty"`
	Path                  string           `json:"path,omitempty"`
	Paths                 []string         `json:"paths,omitempty"`
	ChangedPaths          []string         `json:"changed_paths,omitempty"`
	Mode                  string           `json:"mode,omitempty"`
	View                  string           `json:"view,omitempty"`
	BlockID               string           `json:"block_id,omitempty"`
	SectionID             string           `json:"section_id,omitempty"`
	DocHash               string           `json:"doc_hash,omitempty"`
	LedgerID              string           `json:"ledger_id,omitempty"`
	StartLine             int              `json:"start_line,omitempty"`
	EndLine               int              `json:"end_line,omitempty"`
	TotalLines            int              `json:"total_lines,omitempty"`
	Truncated             bool             `json:"truncated,omitempty"`
	SourceReadTruncated   bool             `json:"source_read_truncated,omitempty"`
	ModelViewTruncated    bool             `json:"model_view_truncated,omitempty"`
	LedgerResultTruncated bool             `json:"ledger_result_truncated"`
	OriginalChars         int              `json:"original_chars"`
	OriginalBytes         int              `json:"original_bytes"`
	ContentChars          int              `json:"content_chars,omitempty"`
	NumberedContentChars  int              `json:"numbered_content_chars,omitempty"`
	Items                 []map[string]any `json:"items,omitempty"`
	Blocks                []map[string]any `json:"blocks,omitempty"`
	TotalBlocks           int              `json:"total_blocks,omitempty"`
	ReturnedBlocks        int              `json:"returned_blocks,omitempty"`
	StartBlock            int              `json:"start_block,omitempty"`
	NextStartBlock        int              `json:"next_start_block,omitempty"`
	HasMoreBlocks         bool             `json:"has_more_blocks,omitempty"`
	PreviewHead           string           `json:"preview_head,omitempty"`
	PreviewTail           string           `json:"preview_tail,omitempty"`
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
			"tool_name":               toolName,
			"ledger_result_truncated": true,
			"original_chars":          len([]rune(result)),
			"original_bytes":          len([]byte(result)),
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
	snapshot.Mode = stringFromAny(payload["mode"])
	snapshot.View = stringFromAny(payload["view"])
	snapshot.BlockID = stringFromAny(payload["block_id"])
	snapshot.SectionID = stringFromAny(payload["section_id"])
	snapshot.DocHash = stringFromAny(payload["doc_hash"])
	snapshot.LedgerID = stringFromAny(payload["ledger_id"])
	snapshot.StartLine = intFromAny(payload["start_line"])
	snapshot.EndLine = intFromAny(payload["end_line"])
	snapshot.TotalLines = intFromAny(payload["total_lines"])
	snapshot.Truncated = boolFromAny(payload["truncated"])
	snapshot.SourceReadTruncated = snapshot.Truncated
	snapshot.ModelViewTruncated = boolFromAny(payload["model_view_truncated"])
	snapshot.ContentChars = len([]rune(stringFromAny(payload["content"])))
	snapshot.NumberedContentChars = len([]rune(stringFromAny(payload["numbered_content"])))
	snapshot.TotalBlocks = intFromAny(payload["total_blocks"])
	snapshot.ReturnedBlocks = intFromAny(payload["returned_blocks"])
	snapshot.StartBlock = intFromAny(payload["start_block"])
	snapshot.NextStartBlock = intFromAny(payload["next_start_block"])
	snapshot.HasMoreBlocks = boolFromAny(payload["has_more_blocks"])
	if rawBlocks, ok := payload["blocks"].([]any); ok {
		for _, rawBlock := range rawBlocks {
			block, ok := rawBlock.(map[string]any)
			if !ok {
				continue
			}
			snapshot.Blocks = append(snapshot.Blocks, map[string]any{
				"block_id":     stringFromAny(block["block_id"]),
				"tag":          stringFromAny(block["tag"]),
				"section_id":   stringFromAny(block["section_id"]),
				"heading_path": stringFromAny(block["heading_path"]),
				"text_preview": stringFromAny(block["text_preview"]),
				"text_chars":   intFromAny(block["text_chars"]),
				"raw_chars":    intFromAny(block["raw_chars"]),
				"level":        intFromAny(block["level"]),
				"has_images":   boolFromAny(block["has_images"]),
				"has_tables":   boolFromAny(block["has_tables"]),
				"has_styles":   boolFromAny(block["has_styles"]),
			})
			if len(snapshot.Blocks) >= defaultNoteIndexBlocks {
				break
			}
		}
	}
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
