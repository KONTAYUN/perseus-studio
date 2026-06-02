package ai

import "webide-backend/internal/workspace"

func workspaceToolPayload(toolName, path string, values map[string]any) map[string]any {
	payload := map[string]any{
		"ok":          true,
		"status":      "success",
		"tool":        toolName,
		"tool_name":   toolName,
		"status_kind": manifestStatusKind(toolName),
	}
	if path != "" {
		payload["path"] = path
	}
	for key, value := range values {
		payload[key] = value
	}
	if _, ok := payload["paths"]; !ok {
		if changed, ok := payload["changed_paths"]; ok {
			payload["paths"] = changed
		} else if path != "" {
			payload["paths"] = []string{path}
		}
	}
	if _, ok := payload["summary"]; !ok {
		payload["summary"] = toolName + " completed"
	}
	return payload
}

func countTreeNodes(nodes []workspace.TreeNode) int {
	count := 0
	for _, node := range nodes {
		count++
		count += countTreeNodes(node.Children)
	}
	return count
}
