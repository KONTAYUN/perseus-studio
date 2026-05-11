package ai

import (
	"log"
	"strings"
)

func logOpenAIResponseSummary(conversationID string, choice openAIChoice, parsedToolCalls []openAIToolCall) {
	message := choice.Message
	hasToolCalls := len(message.ToolCalls) > 0
	hasFunctionCall := message.FunctionCall != nil && message.FunctionCall.Name != ""
	hasContent := strings.TrimSpace(normalizeOpenAIContent(message.Content)) != ""
	if len(parsedToolCalls) == 0 {
		log.Printf("[ai] openai response conversation=%s finish_reason=%s has_tool_calls=%t tool_calls_count=%d has_function_call=%t function_call_name=%s has_content=%t tool_calls=none", conversationID, choice.FinishReason, hasToolCalls, len(message.ToolCalls), hasFunctionCall, valueOrEmpty(message.FunctionCall), hasContent)
		return
	}
	names := make([]string, 0, len(parsedToolCalls))
	for _, call := range parsedToolCalls {
		names = append(names, call.Function.Name)
	}
	log.Printf("[ai] openai response conversation=%s finish_reason=%s has_tool_calls=%t tool_calls_count=%d has_function_call=%t function_call_name=%s has_content=%t parsed_tool_calls=%s", conversationID, choice.FinishReason, hasToolCalls, len(message.ToolCalls), hasFunctionCall, valueOrEmpty(message.FunctionCall), hasContent, strings.Join(names, ","))
}

func logAnthropicResponseSummary(conversationID string, response anthropicResponse, toolUses []anthropicToolUse) {
	hasText := strings.TrimSpace(response.text()) != ""
	if len(toolUses) == 0 {
		log.Printf("[ai] anthropic response conversation=%s stop_reason=%s has_tool_use=%t tool_use_count=%d has_text=%t tool_calls=none", conversationID, response.StopReason, false, len(toolUses), hasText)
		return
	}
	names := make([]string, 0, len(toolUses))
	for _, call := range toolUses {
		names = append(names, call.Name)
	}
	log.Printf("[ai] anthropic response conversation=%s stop_reason=%s has_tool_use=%t tool_use_count=%d has_text=%t parsed_tool_calls=%s", conversationID, response.StopReason, true, len(toolUses), hasText, strings.Join(names, ","))
}

func valueOrEmpty(call *openAIFunctionCall) string {
	if call == nil {
		return ""
	}
	return call.Name
}
