package ai

import "encoding/json"

// The ledger is the durable source of truth. Model-view pruning happens on a
// copy inside ContextAssembler and never alters this stored result.
func ledgerToolResultJSON(toolName, result string) string {
	if result == "" {
		return result
	}
	if json.Valid([]byte(result)) {
		return result
	}
	return mustJSON(map[string]any{"tool_name": toolName, "content": result})
}
