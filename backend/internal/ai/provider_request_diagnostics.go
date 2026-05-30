package ai

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"webide-backend/internal/store"
)

func annotatePayloadDiagnostics(summary *ProviderRequestSummary, payload map[string]any, tools []map[string]any, provider store.ModelProvider, policy providerReasoningPolicy) {
	if summary == nil {
		return
	}
	summary.PayloadKeys = sortedPayloadKeys(payload)
	summary.ToolSchemaDigest = toolSchemaDigest(tools)
	summary.ToolSchemaWarnings = toolSchemaWarnings(tools, provider.Protocol)
	summary.UnsupportedOpenAIParams = unsupportedOpenAIParams(provider, payload, policy)
	summary.ReasoningProfileSource = policy.Profile.Source
	summary.ReasoningProfileLevels = append([]string(nil), policy.Profile.Levels...)
}

func sortedPayloadKeys(payload map[string]any) []string {
	if len(payload) == 0 {
		return nil
	}
	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func toolSchemaDigest(tools []map[string]any) string {
	if len(tools) == 0 {
		return ""
	}
	summaries := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		summary := map[string]any{}
		if fn, _ := tool["function"].(map[string]any); fn != nil {
			summary["name"] = strings.TrimSpace(stringFromAny(fn["name"]))
			summary["parameters"] = schemaShapeDigest(fn["parameters"])
		} else {
			summary["name"] = strings.TrimSpace(stringFromAny(tool["name"]))
			summary["parameters"] = schemaShapeDigest(tool["input_schema"])
		}
		summaries = append(summaries, summary)
	}
	raw, _ := json.Marshal(summaries)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}

func schemaShapeDigest(value any) any {
	object, ok := value.(map[string]any)
	if !ok || object == nil {
		return nil
	}
	result := map[string]any{}
	if typ := stringFromAny(object["type"]); typ != "" {
		result["type"] = typ
	}
	if required, ok := stringSliceFromAny(object["required"]); ok && len(required) > 0 {
		sort.Strings(required)
		result["required"] = required
	}
	if props, ok := object["properties"].(map[string]any); ok {
		names := make([]string, 0, len(props))
		for name := range props {
			names = append(names, name)
		}
		sort.Strings(names)
		result["properties"] = names
	}
	return result
}

func toolSchemaWarnings(tools []map[string]any, protocol string) []string {
	var warnings []string
	seen := map[string]bool{}
	for _, tool := range tools {
		name := ""
		var schema any
		if fn, _ := tool["function"].(map[string]any); fn != nil {
			name = stringFromAny(fn["name"])
			schema = fn["parameters"]
		} else {
			name = stringFromAny(tool["name"])
			schema = tool["input_schema"]
		}
		name = strings.TrimSpace(name)
		if name == "" {
			warnings = append(warnings, "tool missing name")
			continue
		}
		if seen[name] {
			warnings = append(warnings, "duplicate tool name: "+name)
		}
		seen[name] = true
		if object, _ := schema.(map[string]any); object == nil || stringFromAny(object["type"]) != "object" {
			warnings = append(warnings, "tool schema is not object: "+name)
		}
	}
	if len(warnings) == 0 {
		return nil
	}
	sort.Strings(warnings)
	return warnings
}

func unsupportedOpenAIParams(provider store.ModelProvider, payload map[string]any, policy providerReasoningPolicy) []string {
	if strings.ToLower(strings.TrimSpace(provider.Protocol)) != "openai" {
		return nil
	}
	var unsupported []string
	if _, ok := payload["reasoning_effort"]; ok {
		source := policy.Profile.Source
		if source == store.ThinkingProfileSourceModelsDev && !store.IsOfficialOpenAIBaseURL(provider.BaseURL) {
			unsupported = append(unsupported, "reasoning_effort_from_official_model_table_on_compatible_provider")
		}
	}
	sort.Strings(unsupported)
	return unsupported
}

func stringFromAny(value any) string {
	text, _ := value.(string)
	return text
}

func stringSliceFromAny(value any) ([]string, bool) {
	switch items := value.(type) {
	case []string:
		return append([]string(nil), items...), true
	case []any:
		result := make([]string, 0, len(items))
		for _, item := range items {
			text, ok := item.(string)
			if !ok {
				return nil, false
			}
			result = append(result, text)
		}
		return result, true
	default:
		return nil, false
	}
}
