package ai

import (
	"encoding/json"
	"strings"

	"webide-backend/internal/store"
)

const consumedToolArgumentsThreshold = 4096

// demoteConsumedTransportPayloads removes transport-heavy data from the model
// projection after it has been consumed. The persisted Turn Items remain intact.
func demoteConsumedTransportPayloads(items []store.AgentTurnItem) ([]store.AgentTurnItem, int, int) {
	result := append([]store.AgentTurnItem(nil), items...)
	successfulCalls := map[string]bool{}
	latestFileChange := map[string]int64{}
	for _, item := range items {
		switch item.Kind {
		case "tool_result":
			if successfulToolResult(item) {
				successfulCalls[contextItemToolCallID(item)] = true
			}
		case "file_change":
			payload := decodeContextItemPayload(item)
			paths := append(contextPayloadStrings(payload, "changed_paths"), contextPayloadString(payload, "path", "target"))
			for _, path := range normalizedProviderPaths(paths) {
				if item.Seq > latestFileChange[path] {
					latestFileChange[path] = item.Seq
				}
			}
		}
	}

	demoted := 0
	removed := 0
	for index := range result {
		item := result[index]
		payload := decodeContextItemPayload(item)
		switch item.Kind {
		case "tool_call":
			callID := contextItemToolCallID(item)
			arguments := contextItemArgumentsJSON(item)
			if !successfulCalls[callID] || !manifestToolIsMutation(contextItemToolName(item)) || len(arguments) <= consumedToolArgumentsThreshold {
				continue
			}
			summary := compactMutationArguments(arguments)
			payload["arguments_json"] = mustJSON(summary)
			payload["content"] = payload["arguments_json"]
			payload["model_arguments_demoted"] = true
			removed += len(arguments) - len(contextPayloadString(payload, "arguments_json"))
		case "tool_result":
			if !successfulToolResult(item) || !isReadWorkspaceTool(contextItemToolName(item)) {
				continue
			}
			path := normalizeWorkspaceEventPath(contextPayloadString(payload, "path", "target"))
			if path == "" || latestFileChange[path] <= item.Seq {
				continue
			}
			original := contextItemContent(item)
			payload["content"] = compactedToolResultReference(item, payload)
			payload["model_content_consumed"] = true
			payload["stale_due_to_file_change"] = true
			removed += len(original) - len(contextPayloadString(payload, "content"))
		default:
			continue
		}
		raw, _ := json.Marshal(payload)
		result[index].PayloadJSON = string(raw)
		result[index].Payload = raw
		demoted++
	}
	return result, demoted, removed
}

func compactMutationArguments(argumentsJSON string) map[string]any {
	var arguments map[string]any
	_ = json.Unmarshal([]byte(argumentsJSON), &arguments)
	summary := map[string]any{"model_arguments_demoted": true}
	for _, key := range []string{"path", "document_version", "doc_hash", "operation"} {
		if value, ok := arguments[key]; ok {
			summary[key] = value
		}
	}
	if edits, ok := arguments["edits"].([]any); ok {
		summary["edit_count"] = len(edits)
	}
	if patch, ok := arguments["patch"].(string); ok {
		summary["patch_chars"] = len(strings.TrimSpace(patch))
	}
	if html, ok := arguments["html"].(string); ok {
		summary["html_chars"] = len(html)
	}
	return summary
}
