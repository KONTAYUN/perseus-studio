package ai

import "testing"

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
