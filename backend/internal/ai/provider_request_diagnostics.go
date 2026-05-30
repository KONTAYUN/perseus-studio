package ai

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"webide-backend/internal/store"
)

func annotatePayloadDiagnostics(summary *ProviderRequestSummary, payload map[string]any, tools []map[string]any, provider store.ModelProvider, policy providerReasoningPolicy) {
	if summary == nil {
		return
	}
	summary.PayloadKeys = sortedPayloadKeys(payload)
	summary.PayloadBytes = payloadByteSize(payload)
	summary.MessageShapes = providerMessageShapes(payload["messages"])
	summary.MessageProtocolWarnings = providerMessageProtocolWarnings(payload["messages"], provider.Protocol)
	summary.ToolSchemaDigest = toolSchemaDigest(tools)
	summary.ToolSchemaWarnings = toolSchemaWarnings(tools, provider.Protocol)
	summary.ToolSchemaItems = toolSchemaItems(tools, provider.Protocol)
	summary.UnsupportedOpenAIParams = unsupportedOpenAIParams(provider, payload, policy)
	summary.ReasoningProfileSource = policy.Profile.Source
	summary.ReasoningProfileLevels = append([]string(nil), policy.Profile.Levels...)
}

func payloadByteSize(payload map[string]any) int {
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0
	}
	return len(raw)
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

func shortSchemaDigest(value any) string {
	raw, err := json.Marshal(value)
	if err != nil || len(raw) == 0 {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:6])
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
		name, schema, _ := toolNameSchemaStrict(tool)
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
		} else {
			warnings = append(warnings, schemaCompatibilityWarnings(name+".parameters", object, strings.ToLower(strings.TrimSpace(protocol)))...)
		}
	}
	if len(warnings) == 0 {
		return nil
	}
	sort.Strings(warnings)
	return warnings
}

func toolSchemaItems(tools []map[string]any, protocol string) []ProviderToolLog {
	if len(tools) == 0 {
		return nil
	}
	items := make([]ProviderToolLog, 0, len(tools))
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	for _, tool := range tools {
		name, schema, strict := toolNameSchemaStrict(tool)
		name = strings.TrimSpace(name)
		if name == "" {
			name = "<missing>"
		}
		item := ProviderToolLog{Name: name, Strict: strict, SchemaDigest: shortSchemaDigest(schema)}
		if eager, _ := tool["eager_input_streaming"].(bool); eager {
			item.EagerStreaming = true
		}
		if object, _ := schema.(map[string]any); object != nil {
			if required, ok := stringSliceFromAny(object["required"]); ok {
				sort.Strings(required)
				item.Required = required
			}
			if props, _ := object["properties"].(map[string]any); props != nil {
				item.Properties = sortedMapKeys(props)
			}
			item.UnsupportedPaths = schemaCompatibilityWarnings(name+".parameters", object, protocol)
		} else {
			item.UnsupportedPaths = []string{name + ".parameters is not object"}
		}
		items = append(items, item)
	}
	return items
}

func toolNameSchemaStrict(tool map[string]any) (string, any, bool) {
	if fn, _ := tool["function"].(map[string]any); fn != nil {
		strict, _ := fn["strict"].(bool)
		return stringFromAny(fn["name"]), fn["parameters"], strict
	}
	return stringFromAny(tool["name"]), tool["input_schema"], false
}

func schemaCompatibilityWarnings(path string, schema map[string]any, protocol string) []string {
	var warnings []string
	if protocol == "openai" {
		if _, ok := schema["default"]; ok {
			warnings = append(warnings, "openai_schema_default:"+path+".default")
		}
	}
	if props, _ := schema["properties"].(map[string]any); props != nil {
		for _, name := range sortedMapKeys(props) {
			child, _ := props[name].(map[string]any)
			if child == nil {
				continue
			}
			warnings = append(warnings, schemaCompatibilityWarnings(path+"."+name, child, protocol)...)
		}
	}
	if items, _ := schema["items"].(map[string]any); items != nil {
		warnings = append(warnings, schemaCompatibilityWarnings(path+".items", items, protocol)...)
	}
	return warnings
}

func providerMessageShapes(value any) []ProviderMessageLog {
	items := mapSliceFromAny(value)
	if len(items) == 0 {
		return nil
	}
	shapes := make([]ProviderMessageLog, 0, len(items))
	for index, item := range items {
		shape := ProviderMessageLog{
			Index: index,
			Role:  stringFromAny(item["role"]),
		}
		annotateContentShape(&shape, item["content"])
		if calls, _ := item["tool_calls"].([]openAIToolCall); len(calls) > 0 {
			shape.ToolCalls = len(calls)
			for _, call := range calls {
				shape.ToolCallIDs = append(shape.ToolCallIDs, call.ID)
				shape.ToolCallName = append(shape.ToolCallName, call.Function.Name)
			}
		} else if callsAny, _ := item["tool_calls"].([]any); len(callsAny) > 0 {
			shape.ToolCalls = len(callsAny)
			for _, callAny := range callsAny {
				call, _ := callAny.(map[string]any)
				shape.ToolCallIDs = append(shape.ToolCallIDs, stringFromAny(call["id"]))
				if fn, _ := call["function"].(map[string]any); fn != nil {
					shape.ToolCallName = append(shape.ToolCallName, stringFromAny(fn["name"]))
				}
			}
		}
		shape.ToolCallID = stringFromAny(item["tool_call_id"])
		shapes = append(shapes, shape)
	}
	return shapes
}

func providerMessageProtocolWarnings(value any, protocol string) []string {
	if strings.ToLower(strings.TrimSpace(protocol)) != "openai" {
		return nil
	}
	items := mapSliceFromAny(value)
	if len(items) == 0 {
		return nil
	}
	pending := map[string]int{}
	var warnings []string
	for index, item := range items {
		if item == nil {
			continue
		}
		if calls, _ := item["tool_calls"].([]openAIToolCall); len(calls) > 0 {
			for _, call := range calls {
				if strings.TrimSpace(call.ID) != "" {
					pending[call.ID] = index
				}
			}
		} else if callsAny, _ := item["tool_calls"].([]any); len(callsAny) > 0 {
			for _, callAny := range callsAny {
				call, _ := callAny.(map[string]any)
				id := strings.TrimSpace(stringFromAny(call["id"]))
				if id != "" {
					pending[id] = index
				}
			}
		}
		if stringFromAny(item["role"]) == "tool" {
			delete(pending, strings.TrimSpace(stringFromAny(item["tool_call_id"])))
		}
	}
	if len(pending) == 0 {
		return nil
	}
	last := items[len(items)-1]
	if stringFromAny(last["role"]) == "assistant" && messageHasToolCalls(last) {
		return nil
	}
	for _, id := range sortedStringMapKeys(pending) {
		warnings = append(warnings, "openai_unpaired_tool_call:assistant_index="+strconv.Itoa(pending[id])+":tool_call_id="+id)
	}
	return warnings
}

func messageHasToolCalls(item map[string]any) bool {
	if calls, _ := item["tool_calls"].([]openAIToolCall); len(calls) > 0 {
		return true
	}
	if calls, _ := item["tool_calls"].([]any); len(calls) > 0 {
		return true
	}
	return false
}

func mapSliceFromAny(value any) []map[string]any {
	items, ok := value.([]map[string]any)
	if ok {
		return items
	}
	anyItems, ok := value.([]any)
	if !ok {
		return nil
	}
	items = make([]map[string]any, 0, len(anyItems))
	for _, item := range anyItems {
		object, _ := item.(map[string]any)
		items = append(items, object)
	}
	return items
}

func annotateContentShape(shape *ProviderMessageLog, content any) {
	switch value := content.(type) {
	case nil:
		shape.ContentKind = "null"
	case string:
		shape.ContentKind = "string"
		shape.TextChars = len([]rune(value))
	case []map[string]any:
		shape.ContentKind = "parts"
		for _, part := range value {
			shape.PartTypes = append(shape.PartTypes, stringFromAny(part["type"]))
			if text := stringFromAny(part["text"]); text != "" {
				shape.TextChars += len([]rune(text))
			}
		}
	case []any:
		shape.ContentKind = "parts"
		for _, item := range value {
			part, _ := item.(map[string]any)
			shape.PartTypes = append(shape.PartTypes, stringFromAny(part["type"]))
			if text := stringFromAny(part["text"]); text != "" {
				shape.TextChars += len([]rune(text))
			}
		}
	default:
		shape.ContentKind = "other"
	}
}

func sortedMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringMapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
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
