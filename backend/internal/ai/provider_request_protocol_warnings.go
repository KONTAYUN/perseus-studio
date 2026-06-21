package ai

import (
	"strconv"
	"strings"
)

func providerMessageProtocolWarnings(value any, protocol string) []string {
	normalized := strings.ToLower(strings.TrimSpace(protocol))
	if normalized == "anthropic" {
		return anthropicProviderMessageProtocolWarnings(value)
	}
	if normalized != "openai" {
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

func anthropicProviderMessageProtocolWarnings(value any) []string {
	items := mapSliceFromAny(value)
	if len(items) == 0 {
		return nil
	}
	var warnings []string
	for index, item := range items {
		if stringFromAny(item["role"]) != "assistant" {
			continue
		}
		toolUseIDs := anthropicMessageToolUseIDs(item)
		if len(toolUseIDs) == 0 {
			continue
		}
		if index+1 >= len(items) || stringFromAny(items[index+1]["role"]) != "user" {
			for _, id := range toolUseIDs {
				warnings = append(warnings, "anthropic_unpaired_tool_use:assistant_index="+strconv.Itoa(index)+":tool_use_id="+id)
			}
			continue
		}
		resultIDs := anthropicMessageToolResultIDs(items[index+1])
		for _, id := range toolUseIDs {
			if !resultIDs[id] {
				warnings = append(warnings, "anthropic_missing_immediate_tool_result:assistant_index="+strconv.Itoa(index)+":tool_use_id="+id)
			}
		}
	}
	return warnings
}

func anthropicMessageToolUseIDs(item map[string]any) []string {
	var ids []string
	for _, part := range mapSliceFromAny(item["content"]) {
		if part["type"] != "tool_use" {
			continue
		}
		if id := strings.TrimSpace(stringFromAny(part["id"])); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func anthropicMessageToolResultIDs(item map[string]any) map[string]bool {
	ids := map[string]bool{}
	for _, part := range mapSliceFromAny(item["content"]) {
		if part["type"] != "tool_result" {
			continue
		}
		if id := strings.TrimSpace(stringFromAny(part["tool_use_id"])); id != "" {
			ids[id] = true
		}
	}
	return ids
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
