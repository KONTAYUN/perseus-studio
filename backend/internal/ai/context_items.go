package ai

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"webide-backend/internal/store"
)

type contextItemPayload map[string]any

type contextWorkingFile struct {
	Path             string
	Hash             string
	View             string
	Mode             string
	BlockID          string
	SectionID        string
	LedgerID         string
	Source           string
	Stale            bool
	ContentAvailable bool
	LastSeq          int64
	LastTurnID       string
}

func decodeContextItemPayload(item store.AgentTurnItem) contextItemPayload {
	payload := contextItemPayload{}
	_ = json.Unmarshal([]byte(item.PayloadJSON), &payload)
	return payload
}

func contextPayloadString(payload contextItemPayload, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func contextPayloadStrings(payload contextItemPayload, key string) []string {
	return normalizedProviderPaths(stringSliceFromAnyLoose(payload[key]))
}

func contextItemContent(item store.AgentTurnItem) string {
	return contextPayloadString(decodeContextItemPayload(item), "content", "summary", "message")
}

func contextItemToolCallID(item store.AgentTurnItem) string {
	return contextPayloadString(decodeContextItemPayload(item), "tool_call_id")
}

func contextItemToolName(item store.AgentTurnItem) string {
	return contextPayloadString(decodeContextItemPayload(item), "tool_name")
}

func contextItemArgumentsJSON(item store.AgentTurnItem) string {
	payload := decodeContextItemPayload(item)
	if value := contextPayloadString(payload, "arguments_json", "content"); value != "" {
		return value
	}
	if value, ok := payload["arguments"]; ok {
		raw, _ := json.Marshal(value)
		return string(raw)
	}
	return "{}"
}

func activeAgentContextItems(items []store.AgentTurnItem) ([]store.AgentTurnItem, int64) {
	boundary := int64(0)
	latestCompaction := -1
	for index, item := range items {
		if item.Visibility != store.AgentItemVisibilityModel || item.Kind != "compaction" {
			continue
		}
		payload := decodeContextItemPayload(item)
		seq := contextInt64FromAny(payload["replaces_through_seq"])
		if seq >= boundary {
			boundary = seq
			latestCompaction = index
		}
	}
	active := make([]store.AgentTurnItem, 0, len(items))
	if latestCompaction >= 0 {
		active = append(active, items[latestCompaction])
	}
	for _, item := range items {
		if item.Visibility != store.AgentItemVisibilityModel {
			continue
		}
		if item.Kind == "compaction" {
			continue
		}
		if item.Seq <= boundary {
			continue
		}
		active = append(active, item)
	}
	return active, boundary
}

func repairAgentToolPairs(items []store.AgentTurnItem) []store.AgentTurnItem {
	callIndex := map[string]int{}
	resultIDs := map[string]bool{}
	for index, item := range items {
		switch item.Kind {
		case "tool_call":
			if id := contextItemToolCallID(item); id != "" {
				callIndex[id] = index
			}
		case "tool_result", "aborted":
			if id := contextItemToolCallID(item); id != "" {
				resultIDs[id] = true
			}
		}
	}
	paired := make([]store.AgentTurnItem, 0, len(items)+4)
	for index, item := range items {
		if item.Kind == "tool_result" || item.Kind == "aborted" {
			id := contextItemToolCallID(item)
			if _, ok := callIndex[id]; id == "" || !ok {
				continue
			}
		}
		paired = append(paired, item)
		if item.Kind != "tool_call" {
			continue
		}
		id := contextItemToolCallID(item)
		if id == "" || resultIDs[id] {
			continue
		}
		synthetic := store.AgentTurnItem{
			Seq: item.Seq, ID: item.ID + ":aborted", UserID: item.UserID,
			ConversationID: item.ConversationID, RunID: item.RunID, TurnID: item.TurnID,
			Kind: "aborted", Role: "tool", Status: "aborted",
			Visibility: store.AgentItemVisibilityModel, SourceID: item.SourceID + ":aborted",
			PayloadJSON: mustJSON(map[string]any{
				"tool_call_id": id,
				"tool_name":    contextItemToolName(item),
				"content":      `{"status":"aborted","message":"tool call ended without a recorded result"}`,
			}),
		}
		insertAt := len(paired)
		for next := index + 1; next < len(items) && items[next].Kind == "tool_call"; next++ {
			insertAt++
		}
		_ = insertAt
		paired = append(paired, synthetic)
	}
	return paired
}

func contextEditorPaths(items []store.AgentTurnItem) []string {
	var paths []string
	for _, item := range items {
		if item.Kind != "environment_snapshot" {
			continue
		}
		payload := decodeContextItemPayload(item)
		paths = append(paths, contextPayloadString(payload, "active_file"))
		panels, _ := payload["panels"].([]any)
		for _, rawPanel := range panels {
			panel, _ := rawPanel.(map[string]any)
			paths = append(paths, stringFromAny(panel["active_file_path"]))
			paths = append(paths, stringSliceFromAnyLoose(panel["open_tabs"])...)
		}
	}
	return normalizedProviderPaths(paths)
}

func contextWorkingSet(items []store.AgentTurnItem) []contextWorkingFile {
	files := map[string]contextWorkingFile{}
	for _, item := range items {
		payload := decodeContextItemPayload(item)
		switch item.Kind {
		case "workspace_observation":
			path := normalizeWorkspaceEventPath(contextPayloadString(payload, "path", "target"))
			if path == "" {
				continue
			}
			files[path] = contextWorkingFile{
				Path: path, Hash: contextPayloadString(payload, "content_hash", "hash", "doc_hash"),
				View: contextPayloadString(payload, "view"), Mode: contextPayloadString(payload, "mode"),
				BlockID: contextPayloadString(payload, "block_id"), SectionID: contextPayloadString(payload, "section_id"),
				LedgerID: contextPayloadString(payload, "ledger_id"),
				Source:   contextPayloadString(payload, "source", "tool_name"), LastSeq: item.Seq, LastTurnID: item.TurnID,
			}
		case "file_change":
			paths := contextPayloadStrings(payload, "changed_paths")
			if path := normalizeWorkspaceEventPath(contextPayloadString(payload, "path")); path != "" {
				paths = append(paths, path)
			}
			for _, path := range normalizedProviderPaths(paths) {
				known := files[path]
				known.Path = path
				known.Hash = contextPayloadString(payload, "hash")
				known.Stale = true
				known.LastSeq = item.Seq
				known.LastTurnID = item.TurnID
				files[path] = known
			}
		}
	}
	result := make([]contextWorkingFile, 0, len(files))
	for _, item := range files {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].LastSeq == result[j].LastSeq {
			return result[i].Path < result[j].Path
		}
		return result[i].LastSeq > result[j].LastSeq
	})
	return result
}

func contextWorkingSetText(items []contextWorkingFile) string {
	if len(items) == 0 {
		return ""
	}
	var lines []string
	for _, item := range items {
		state := "metadata only"
		if item.ContentAvailable {
			state = "content available in recent context"
		}
		if item.Stale {
			state = "stale; reread before exact edits"
		}
		detail := []string{state}
		if item.View != "" {
			detail = append(detail, "view="+item.View)
		}
		if item.Mode != "" {
			detail = append(detail, "mode="+item.Mode)
		}
		if item.Hash != "" {
			detail = append(detail, "hash="+item.Hash)
		}
		if item.LedgerID != "" {
			detail = append(detail, "ledger="+item.LedgerID)
		}
		lines = append(lines, fmt.Sprintf("- %s (%s)", item.Path, strings.Join(detail, ", ")))
	}
	return "Task file working set from successful reads, explicit references, and changes:\n" + strings.Join(lines, "\n")
}

func contextWorkingSetWithAvailability(files []contextWorkingFile, items []store.AgentTurnItem) []contextWorkingFile {
	result := append([]contextWorkingFile(nil), files...)
	for index := range result {
		result[index].ContentAvailable = false
		if result[index].Stale {
			continue
		}
		for itemIndex := len(items) - 1; itemIndex >= 0; itemIndex-- {
			item := items[itemIndex]
			if item.Kind != "tool_result" || !successfulToolResult(item) || !isReadWorkspaceTool(contextItemToolName(item)) {
				continue
			}
			payload := decodeContextItemPayload(item)
			if payload["model_content_pruned"] == true {
				continue
			}
			ledgerID := contextPayloadString(payload, "ledger_id")
			path := normalizeWorkspaceEventPath(contextPayloadString(payload, "path", "target"))
			if (result[index].LedgerID != "" && ledgerID == result[index].LedgerID) || (result[index].LedgerID == "" && path == result[index].Path) {
				result[index].ContentAvailable = strings.TrimSpace(contextItemContent(item)) != ""
				break
			}
		}
	}
	return result
}

func contextInt64FromAny(value any) int64 {
	switch item := value.(type) {
	case float64:
		return int64(item)
	case int64:
		return item
	case int:
		return int64(item)
	case json.Number:
		result, _ := item.Int64()
		return result
	}
	return 0
}

func sortedBoolMapKeys(values map[string]bool) []string {
	if len(values) == 0 {
		return nil
	}
	items := make([]string, 0, len(values))
	for value := range values {
		if strings.TrimSpace(value) != "" {
			items = append(items, value)
		}
	}
	sort.Strings(items)
	return items
}

func isReadWorkspaceTool(toolName string) bool {
	switch strings.TrimSpace(toolName) {
	case "read_workspace_note_view", "read_workspace_text", "read_workspace_files":
		return true
	default:
		return false
	}
}

func isCompactedToolResultContent(content string) bool {
	var payload struct {
		Status             string `json:"status"`
		ModelContentPruned bool   `json:"model_content_pruned"`
	}
	return json.Unmarshal([]byte(content), &payload) == nil &&
		(payload.ModelContentPruned || payload.Status == "compacted_context" || payload.Status == "context_reference")
}
