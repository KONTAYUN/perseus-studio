package ai

import (
	"encoding/json"
	"strings"
)

type openAIFunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (f openAIFunctionCall) ArgumentsJSON() string {
	raw := strings.TrimSpace(string(f.Arguments))
	if len(raw) == 0 || raw == "null" {
		return "{}"
	}
	if raw[0] == '{' || raw[0] == '[' {
		return raw
	}
	var s string
	if err := json.Unmarshal(f.Arguments, &s); err == nil {
		return s
	}
	return raw
}

type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAIFunctionCall `json:"function"`
}

type openAIResponse struct {
	Error   any            `json:"error"`
	Choices []openAIChoice `json:"choices"`
}

type openAIChoice struct {
	Message      openAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type openAIMessage struct {
	Role         string              `json:"role"`
	Content      any                 `json:"content"`
	ToolCalls    []openAIToolCall    `json:"tool_calls"`
	FunctionCall *openAIFunctionCall `json:"function_call"`
}

func (m openAIMessage) toRuntimeMessage() map[string]any {
	item := map[string]any{"role": "assistant"}
	if m.Content != nil {
		item["content"] = m.Content
	}
	if len(m.ToolCalls) > 0 {
		item["tool_calls"] = m.ToolCalls
	}
	if m.FunctionCall != nil && m.FunctionCall.Name != "" {
		item["function_call"] = m.FunctionCall
	}
	return item
}

type anthropicResponse struct {
	Error      any              `json:"error"`
	Content    []map[string]any `json:"content"`
	StopReason string           `json:"stop_reason"`
}

type anthropicToolUse struct {
	ID    string
	Name  string
	Input map[string]any
}

func (r anthropicResponse) text() string {
	var parts []string
	for _, item := range r.Content {
		if item["type"] == "text" {
			if text, ok := item["text"].(string); ok && text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func (r anthropicResponse) toolUses() []anthropicToolUse {
	var items []anthropicToolUse
	for _, block := range r.Content {
		if block["type"] != "tool_use" {
			continue
		}
		id, _ := block["id"].(string)
		name, _ := block["name"].(string)
		input, _ := block["input"].(map[string]any)
		items = append(items, anthropicToolUse{ID: id, Name: name, Input: input})
	}
	return items
}
