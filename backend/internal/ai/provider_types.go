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
	Usage   map[string]any `json:"usage"`
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
	Usage      map[string]any   `json:"usage"`
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

func sanitizeAnthropicVisibleTextBlocks(content []map[string]any) []map[string]any {
	if len(content) == 0 {
		return content
	}
	filter := newProviderVisibleTextFilter()
	next := make([]map[string]any, 0, len(content))
	for _, block := range content {
		if block["type"] != "text" {
			next = append(next, block)
			continue
		}
		text, _ := block["text"].(string)
		visible := filter.Feed(text)
		visible, _, _ = extractWorkspacePatchBlock(visible)
		if strings.TrimSpace(visible) == "" {
			continue
		}
		clean := cloneMapAny(block)
		if clean == nil {
			clean = map[string]any{"type": "text"}
		}
		clean["text"] = visible
		next = append(next, clean)
	}
	if tail := filter.Close(); strings.TrimSpace(tail) != "" {
		tail, _, _ = extractWorkspacePatchBlock(tail)
		if strings.TrimSpace(tail) == "" {
			return next
		}
		next = append(next, map[string]any{"type": "text", "text": tail})
	}
	return next
}
