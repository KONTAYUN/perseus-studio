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

func TestProviderMessageProtocolWarningsDetectsAnthropicToolUseWithoutImmediateResult(t *testing.T) {
	messages := []map[string]any{
		{"role": "user", "content": "请修改文件"},
		{"role": "assistant", "content": []map[string]any{{
			"type": "tool_use",
			"id":   "text_patch_1",
			"name": "apply_workspace_patch",
		}}},
		{"role": "user", "content": []map[string]any{{
			"type": "text",
			"text": "runtime observation inserted in between",
		}}},
	}

	warnings := providerMessageProtocolWarnings(messages, "anthropic")
	if len(warnings) != 1 || !strings.Contains(warnings[0], "anthropic_missing_immediate_tool_result:assistant_index=1:tool_use_id=text_patch_1") {
		t.Fatalf("expected Anthropic tool-result adjacency warning, got %#v", warnings)
	}
}

func TestProviderMessageProtocolWarningsAllowsAnthropicImmediateToolResult(t *testing.T) {
	messages := []map[string]any{
		{"role": "assistant", "content": []map[string]any{{
			"type": "tool_use",
			"id":   "text_patch_1",
			"name": "apply_workspace_patch",
		}}},
		{"role": "user", "content": []map[string]any{{
			"type":        "tool_result",
			"tool_use_id": "text_patch_1",
			"content":     "ok",
		}}},
	}

	if warnings := providerMessageProtocolWarnings(messages, "anthropic"); len(warnings) != 0 {
		t.Fatalf("expected valid Anthropic tool-result adjacency, got %#v", warnings)
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

func TestAnthropicTranscriptMessagesRewritesDuplicateToolUseIDs(t *testing.T) {
	transcript := newAgentTranscript()
	for index, path := range []string{"a.txt", "b.txt"} {
		transcript.AppendAssistantTurn(agentModelTurn{
			AnthropicRaw: []map[string]any{{
				"type":  "tool_use",
				"id":    "text_patch_1",
				"name":  "apply_workspace_patch",
				"input": map[string]any{"transport": "assistant_text_patch"},
			}},
			ToolCalls: []agentToolCall{{
				ID:            "text_patch_1",
				Name:          "apply_workspace_patch",
				ArgumentsJSON: `{"transport":"assistant_text_patch"}`,
			}},
		})
		transcript.AppendToolResults([]TranscriptToolResult{{
			ToolCallID: "text_patch_1",
			ToolName:   "apply_workspace_patch",
			Content:    `{"status":"success"}`,
			Event:      ToolEvent{ToolName: "apply_workspace_patch", Status: "success", Path: path, ChangedPaths: []string{path}},
		}})
		if index == 0 {
			transcript.AppendRuntimeObservation(`{"status":"ok"}`, nil)
		}
	}

	messages := anthropicTranscriptMessages(transcript.Entries)
	var toolUseIDs []string
	var toolResultIDs []string
	for _, message := range messages {
		content, _ := message["content"].([]map[string]any)
		for _, block := range content {
			switch block["type"] {
			case "tool_use":
				toolUseIDs = append(toolUseIDs, stringFromAny(block["id"]))
			case "tool_result":
				toolResultIDs = append(toolResultIDs, stringFromAny(block["tool_use_id"]))
			}
		}
	}
	if len(toolUseIDs) != 2 || len(toolResultIDs) != 2 {
		t.Fatalf("expected two tool pairs, got uses=%#v results=%#v messages=%#v", toolUseIDs, toolResultIDs, messages)
	}
	if toolUseIDs[0] != "text_patch_1" || toolResultIDs[0] != "text_patch_1" {
		t.Fatalf("expected first legacy id to stay paired, got uses=%#v results=%#v", toolUseIDs, toolResultIDs)
	}
	if toolUseIDs[1] == "text_patch_1" || toolUseIDs[1] != toolResultIDs[1] {
		t.Fatalf("expected duplicate legacy id to be rewritten and paired, got uses=%#v results=%#v", toolUseIDs, toolResultIDs)
	}
}
