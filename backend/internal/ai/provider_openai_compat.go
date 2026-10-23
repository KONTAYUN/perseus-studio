package ai

import (
	"strings"

	"webide-backend/internal/store"
)

// Message layout is explicit capability configuration, independent of names.
func normalizeOpenAIProviderPayload(provider store.ModelProvider, payload map[string]any) map[string]any {
	if store.NormalizeSystemMessageMode(provider.SystemMessageMode) == "multiple" {
		return payload
	}
	messages, ok := payload["messages"].([]map[string]any)
	if !ok {
		return payload
	}
	var system []string
	others := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		if message["role"] != "system" {
			others = append(others, message)
			continue
		}
		content, ok := message["content"].(string)
		if !ok {
			return payload
		} // Never silently discard an unfamiliar content type.
		if name, _ := message["name"].(string); name != "" {
			content = "[" + name + "]\n" + content
		}
		system = append(system, content)
	}
	if len(system) == 0 {
		return payload
	}
	merged := make([]map[string]any, 0, len(others)+1)
	merged = append(merged, map[string]any{"role": "system", "content": strings.Join(system, "\n\n")})
	merged = append(merged, others...)
	next := make(map[string]any, len(payload))
	for key, value := range payload {
		next[key] = value
	}
	next["messages"] = merged
	return next
}
