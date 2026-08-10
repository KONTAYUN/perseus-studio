package ai

import (
	"encoding/json"
	"fmt"
)

func (s *Service) execReadWorkspaceNoteBatchView(path, fragment, fileHash, view, mode string, blockIDs, sectionIDs []string, docHash string, maxChars int) (string, string, string, string, error) {
	if len(blockIDs)+len(sectionIDs) > 20 {
		return "", "", "正在读取笔记视图...", path, newToolError(ToolErrorInvalidArguments, "block_ids and section_ids accept at most 20 targets per call", nil)
	}
	if mode == "block" && len(blockIDs) == 0 {
		return "", "", "正在读取笔记视图...", path, newToolError(ToolErrorInvalidArguments, "block_ids is required for batch mode=block", nil)
	}
	if mode == "section" && len(sectionIDs) == 0 {
		return "", "", "正在读取笔记视图...", path, newToolError(ToolErrorInvalidArguments, "section_ids is required for batch mode=section", nil)
	}
	targets := blockIDs
	if mode == "section" {
		targets = sectionIDs
	}
	items := make([]any, 0, len(targets))
	covered := make([]string, 0)
	sectionCoverage := make([]string, 0)
	complete := make([]string, 0)
	hashes := map[string]string{}
	truncated := false
	sourceTruncated := false
	modelViewTruncated := false
	for _, target := range targets {
		blockID, sectionID := "", ""
		if mode == "block" {
			blockID = target
		} else {
			sectionID = target
		}
		raw, _, _, _, err := s.execReadWorkspaceNoteBlockView(
			path, fragment, fileHash, view, mode, blockID, sectionID,
			docHash, 0, 0, maxChars,
		)
		if err != nil {
			return "", "", "正在读取笔记视图...", path, err
		}
		var decoded any
		if json.Unmarshal([]byte(raw), &decoded) != nil {
			return "", "", "正在读取笔记视图...", path, fmt.Errorf("failed to encode batch note view")
		}
		items = append(items, decoded)
		var itemPayload map[string]any
		if json.Unmarshal([]byte(raw), &itemPayload) == nil {
			covered = append(covered, stringSliceFromAnyLoose(itemPayload["covered_block_ids"])...)
			complete = append(complete, stringSliceFromAnyLoose(itemPayload["complete_block_ids"])...)
			sectionCoverage = append(sectionCoverage, stringSliceFromAnyLoose(itemPayload["section_ids"])...)
			truncated = truncated || boolFromAny(itemPayload["truncated"])
			sourceTruncated = sourceTruncated || boolFromAny(itemPayload["source_truncated"])
			modelViewTruncated = modelViewTruncated || boolFromAny(itemPayload["model_view_truncated"])
			if sectionID, ok := itemPayload["section_id"].(string); ok && sectionID != "" {
				sectionCoverage = append(sectionCoverage, sectionID)
			}
			if itemHashes, ok := itemPayload["block_hashes"].(map[string]any); ok {
				for id, value := range itemHashes {
					if hash, ok := value.(string); ok {
						hashes[id] = hash
					}
				}
			}
		}
	}
	payload := workspaceToolPayload("read_workspace_note_view", path, map[string]any{
		"path":                 path,
		"view":                 view,
		"mode":                 mode,
		"doc_hash":             noteDocHash(fragment),
		"document_version":     noteDocHash(fragment),
		"items":                items,
		"covered_block_ids":    sortedUniqueStrings(covered),
		"complete_block_ids":   sortedUniqueStrings(complete),
		"section_ids":          sortedUniqueStrings(sectionCoverage),
		"block_hashes":         hashes,
		"returned_targets":     len(items),
		"truncated":            truncated,
		"source_truncated":     sourceTruncated || truncated,
		"model_view_truncated": modelViewTruncated,
	})
	summary := fmt.Sprintf("read %d %s note targets for %s", len(items), mode, path)
	return mustJSON(payload), summary, "正在读取笔记视图...", path, nil
}
