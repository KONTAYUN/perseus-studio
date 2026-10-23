package ai

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Execution timestamps and identifiers are not semantic task progress.
func semanticToolResultDigest(event ToolEvent, result string) string {
	if event.Hash != "" {
		return event.Hash
	}
	var value map[string]any
	if json.Unmarshal([]byte(result), &value) == nil {
		for _, key := range []string{"started_at", "finished_at", "duration_ms", "tool_call_id", "version_id"} {
			delete(value, key)
		}
		if raw, err := json.Marshal(value); err == nil {
			result = string(raw)
		}
	}
	sum := sha256.Sum256([]byte(result))
	return hex.EncodeToString(sum[:])
}
