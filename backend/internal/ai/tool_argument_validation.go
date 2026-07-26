package ai

import (
	"fmt"
	"sort"
	"strings"
)

func validateToolArgumentsAgainstSchema(toolName string, args map[string]any, tools []map[string]any) string {
	schema := toolInputSchema(toolName, tools)
	return validateToolArgumentsAgainstSchemaMap(args, schema)
}

func validateToolArgumentsAgainstSchemaMap(args map[string]any, schema map[string]any) string {
	if len(schema) == 0 {
		return ""
	}
	for _, field := range schemaRequiredFields(schema) {
		value, ok := args[field]
		if !ok || value == nil {
			return fmt.Sprintf("invalid tool arguments: %s is required", field)
		}
	}
	properties, _ := schema["properties"].(map[string]any)
	for field, property := range properties {
		value, ok := args[field]
		if !ok || value == nil {
			continue
		}
		propSchema, _ := property.(map[string]any)
		if errText := validateToolArgumentField(field, value, propSchema); errText != "" {
			return errText
		}
	}
	return ""
}

func toolInputSchema(toolName string, tools []map[string]any) map[string]any {
	for _, tool := range tools {
		if function, _ := tool["function"].(map[string]any); function != nil {
			if function["name"] == toolName {
				schema, _ := function["parameters"].(map[string]any)
				return schema
			}
			continue
		}
		if tool["name"] == toolName {
			schema, _ := tool["input_schema"].(map[string]any)
			return schema
		}
	}
	return nil
}

func toolSchemaAllowsEmptyArguments(schema map[string]any) bool {
	if len(schemaRequiredFields(schema)) > 0 {
		return false
	}
	return true
}

func schemaRequiredFields(schema map[string]any) []string {
	raw, _ := schema["required"].([]string)
	if len(raw) > 0 {
		return raw
	}
	items, _ := schema["required"].([]any)
	fields := make([]string, 0, len(items))
	for _, item := range items {
		if field, ok := item.(string); ok && strings.TrimSpace(field) != "" {
			fields = append(fields, field)
		}
	}
	return fields
}

func validateToolArgumentField(field string, value any, schema map[string]any) string {
	if len(schema) == 0 {
		return ""
	}
	if expected := strings.TrimSpace(fmt.Sprint(schema["type"])); expected != "" {
		if !toolArgumentTypeMatches(value, expected) {
			return fmt.Sprintf("invalid tool arguments: %s must be %s", field, expected)
		}
	}
	if allowed := schemaEnumValues(schema); len(allowed) > 0 {
		text, ok := value.(string)
		if !ok || !containsToolArgumentString(allowed, text) {
			sort.Strings(allowed)
			return fmt.Sprintf("invalid tool arguments: %s must be one of %s", field, strings.Join(allowed, ", "))
		}
	}
	return ""
}

func toolArgumentTypeMatches(value any, expected string) bool {
	switch expected {
	case "string":
		_, ok := value.(string)
		return ok
	case "integer":
		switch item := value.(type) {
		case float64:
			return item == float64(int64(item))
		case int, int64:
			return true
		default:
			return false
		}
	case "number":
		switch value.(type) {
		case float64, int, int64:
			return true
		default:
			return false
		}
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	default:
		return true
	}
}

func containsToolArgumentString(items []string, expected string) bool {
	for _, item := range items {
		if item == expected {
			return true
		}
	}
	return false
}

func schemaEnumValues(schema map[string]any) []string {
	items, ok := schema["enum"].([]any)
	if !ok {
		if direct, ok := schema["enum"].([]string); ok {
			return append([]string(nil), direct...)
		}
		return nil
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			values = append(values, text)
		}
	}
	return values
}

func invalidToolArgumentsError(message string) error {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "invalid tool arguments"
	}
	return newToolError(ToolErrorInvalidArguments, message, nil)
}
