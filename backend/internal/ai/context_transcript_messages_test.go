package ai

import (
	"strings"
	"testing"
)

func TestProviderMessageProtocolWarningsDetectsUnpairedOpenAIToolCall(t *testing.T) {
	messages := []map[string]any{
		{"role": "user", "content": "请后台执行"},
		{
			"role":    "assistant",
			"content": "我会创建后台任务。",
			"tool_calls": []openAIToolCall{{
				ID:   "call_task_1",
				Type: "function",
				Function: openAIFunctionCall{
					Name:      "start_background_task",
					Arguments: []byte(`"{\"title\":\"任务\"}"`),
				},
			}},
		},
		{"role": "user", "content": "继续"},
	}

	warnings := providerMessageProtocolWarnings(messages, "openai")
	if len(warnings) != 1 || !strings.Contains(warnings[0], "openai_unpaired_tool_call:assistant_index=1:tool_call_id=call_task_1") {
		t.Fatalf("expected unpaired tool-call warning, got %#v", warnings)
	}
}

func TestProviderMessageProtocolWarningsAllowsPendingLastAssistantToolCall(t *testing.T) {
	messages := []map[string]any{
		{"role": "user", "content": "请后台执行"},
		{
			"role": "assistant",
			"tool_calls": []openAIToolCall{{
				ID:       "call_task_1",
				Type:     "function",
				Function: openAIFunctionCall{Name: "start_background_task", Arguments: []byte(`"{}"`)},
			}},
		},
	}

	if warnings := providerMessageProtocolWarnings(messages, "openai"); len(warnings) != 0 {
		t.Fatalf("latest assistant tool call is a response shape, not a next request violation: %#v", warnings)
	}
}

func TestAnthropicTranscriptMessagesCoalescesAdjacentToolResults(t *testing.T) {
	transcript := newAgentTranscript()
	transcript.AppendAssistantTurn(agentModelTurn{ToolCalls: []agentToolCall{
		{ID: "call_1", Name: "read_workspace_text", ArgumentsJSON: `{"path":"a.txt"}`},
		{ID: "call_2", Name: "read_workspace_text", ArgumentsJSON: `{"path":"b.txt"}`},
	}})
	transcript.AppendToolResults([]TranscriptToolResult{{
		ToolCallID: "call_1",
		ToolName:   "read_workspace_text",
		Content:    `{"path":"a.txt","content":"a"}`,
	}})
	transcript.AppendToolResults([]TranscriptToolResult{{
		ToolCallID: "call_2",
		ToolName:   "read_workspace_text",
		Content:    `{"path":"b.txt","content":"b"}`,
	}})

	messages := anthropicTranscriptMessages(transcript.Entries)
	if len(messages) != 2 {
		t.Fatalf("expected assistant plus one coalesced tool-result message, got %#v", messages)
	}
	content, ok := messages[1]["content"].([]map[string]any)
	if !ok {
		t.Fatalf("unexpected tool-result content shape: %#v", messages[1]["content"])
	}
	if len(content) != 2 {
		t.Fatalf("expected two tool results in one user message, got %#v", content)
	}
	if content[0]["tool_use_id"] != "call_1" || content[1]["tool_use_id"] != "call_2" {
		t.Fatalf("unexpected tool result order: %#v", content)
	}
}

func TestAnthropicTranscriptMessagesPreservesThinkingBlocks(t *testing.T) {
	transcript := newAgentTranscript()
	transcript.AppendAssistantTurn(agentModelTurn{
		Content: "我会读取文件。",
		AnthropicRaw: []map[string]any{
			{"type": "thinking", "thinking": "private reasoning", "signature": "sig"},
			{"type": "text", "text": "我会读取文件。"},
			{"type": "tool_use", "id": "call_1", "name": "read_workspace_text", "input": map[string]any{"path": "a.txt"}},
		},
		ToolCalls: []agentToolCall{{ID: "call_1", Name: "read_workspace_text", ArgumentsJSON: `{"path":"a.txt"}`}},
	})

	messages := anthropicTranscriptMessages(transcript.Entries)
	content, ok := messages[0]["content"].([]map[string]any)
	if !ok || len(content) != 3 {
		t.Fatalf("unexpected assistant content: %#v", messages)
	}
	if content[0]["type"] != "thinking" || content[0]["signature"] != "sig" {
		t.Fatalf("expected thinking block preserved, got %#v", content[0])
	}
	for _, block := range content {
		if block["type"] == "text" && strings.Contains(block["text"].(string), "private reasoning") {
			t.Fatalf("thinking leaked into visible text: %#v", content)
		}
	}
}
