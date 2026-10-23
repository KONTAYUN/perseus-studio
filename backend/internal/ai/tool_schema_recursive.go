package ai

import (
	"fmt"
	"reflect"
	"sort"
)

// Validate the published subset recursively, with deterministic error paths.
// Input is JSON-decoded data; recursion is bounded independently of tool logic.
func validateSchemaValue(path string, value any, schema map[string]any, depth int) string {
	bad := func(reason string) string { return "invalid tool arguments: " + path + " " + reason }
	if depth > 32 {
		return bad("exceeds maximum nesting depth")
	}
	if expected, _ := schema["type"].(string); expected != "" && !toolArgumentTypeMatches(value, expected) {
		return bad("must be " + expected)
	}
	if allowed := schemaEnumValues(schema); len(allowed) > 0 {
		text, ok := value.(string)
		if !ok || !containsToolArgumentString(allowed, text) {
			return bad(fmt.Sprintf("must be one of %v", allowed))
		}
	}
	if items, ok := value.([]any); ok {
		if n, ok := schemaNumber(schema["minItems"]); ok && float64(len(items)) < n {
			return bad("contains too few items")
		}
		if n, ok := schemaNumber(schema["maxItems"]); ok && float64(len(items)) > n {
			return bad("contains too many items")
		}
		itemSchema, _ := schema["items"].(map[string]any)
		for i, item := range items {
			if reason := validateSchemaValue(fmt.Sprintf("%s[%d]", path, i), item, itemSchema, depth+1); reason != "" {
				return reason
			}
		}
	}
	if object, ok := value.(map[string]any); ok {
		for _, key := range schemaRequiredFields(schema) {
			if item, exists := object[key]; !exists || item == nil {
				return bad("requires " + key)
			}
		}
		properties, _ := schema["properties"].(map[string]any)
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			property, known := properties[key]
			if !known {
				if allowed, exists := schema["additionalProperties"].(bool); exists && !allowed {
					return bad("contains unknown property " + key)
				}
				continue
			}
			child, _ := property.(map[string]any)
			if reason := validateSchemaValue(path+"."+key, object[key], child, depth+1); reason != "" {
				return reason
			}
		}
	}
	if n, numeric := schemaNumber(value); numeric {
		if min, ok := schemaNumber(schema["minimum"]); ok && n < min {
			return bad("is below minimum")
		}
		if max, ok := schemaNumber(schema["maximum"]); ok && n > max {
			return bad("exceeds maximum")
		}
	}
	if text, ok := value.(string); ok {
		length := float64(len([]rune(text)))
		if min, ok := schemaNumber(schema["minLength"]); ok && length < min {
			return bad("is too short")
		}
		if max, ok := schemaNumber(schema["maxLength"]); ok && length > max {
			return bad("is too long")
		}
	}
	return ""
}

func schemaNumber(value any) (float64, bool) {
	if value == nil {
		return 0, false
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Int, reflect.Int64:
		return float64(v.Int()), true
	case reflect.Float64:
		return v.Float(), true
	default:
		return 0, false
	}
}
