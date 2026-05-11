package ai

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestContextManagerKeepsDistinctWorkspaceReadResults(t *testing.T) {
	transcript := newAgentTranscript()
	var calls []agentToolCall
	var results []TranscriptToolResult
	for index := 0; index < 6; index++ {
		path := "chapter-" + string(rune('a'+index)) + ".txt"
		callID := "read_" + string(rune('a'+index))
		calls = append(calls, agentToolCall{ID: callID, Name: "read_workspace_text", ArgumentsJSON: `{"path":"` + path + `"}`})
		results = append(results, TranscriptToolResult{
			ToolCallID: callID,
			ToolName:   "read_workspace_text",
			Content:    "full distinct context for " + path + strings.Repeat(" x", 200),
			Event:      ToolEvent{ToolName: "read_workspace_text", Status: "success", Path: path},
		})
	}
	transcript.AppendAssistantTurn(agentModelTurn{ToolCalls: calls})
	transcript.AppendToolResults(results)

	snapshot := newAgentContextManager().Build(agentProtocolOpenAI, nil, nil, transcript, providerRuntimeProfile{MaxRecentMessageRunes: 20}, 2)
	if len(snapshot.Summary.CompactionActions) != 0 {
		t.Fatalf("distinct context files should not be budget-compacted, got %#v", snapshot.Summary.CompactionActions)
	}
	raw, err := json.Marshal(snapshot.OpenAIMessages)
	if err != nil {
		t.Fatalf("marshal messages: %v", err)
	}
	text := string(raw)
	for _, result := range results {
		if !strings.Contains(text, result.Content) {
			t.Fatalf("expected distinct read content to remain in provider context for %s", result.Event.Path)
		}
	}
}

func TestContextManagerCompactsReadStaleAfterMutation(t *testing.T) {
	transcript := newAgentTranscript()
	transcript.AppendAssistantTurn(agentModelTurn{ToolCalls: []agentToolCall{
		{ID: "read_1", Name: "read_workspace_text", ArgumentsJSON: `{"path":"draft.txt"}`},
	}})
	transcript.AppendToolResults([]TranscriptToolResult{{
		ToolCallID: "read_1",
		ToolName:   "read_workspace_text",
		Content:    "old draft content that should not be replayed after a later write",
		Event:      ToolEvent{ToolName: "read_workspace_text", Status: "success", Path: "draft.txt"},
	}})
	transcript.AppendAssistantTurn(agentModelTurn{ToolCalls: []agentToolCall{
		{ID: "save_1", Name: "save_workspace_file", ArgumentsJSON: `{"path":"draft.txt","content":"new","mode":"overwrite"}`},
	}})
	transcript.AppendToolResults([]TranscriptToolResult{{
		ToolCallID: "save_1",
		ToolName:   "save_workspace_file",
		Content:    `{"status":"success","path":"draft.txt"}`,
		Event:      ToolEvent{ToolName: "save_workspace_file", Status: "success", Path: "draft.txt", ChangedPaths: []string{"draft.txt"}},
	}})

	snapshot := newAgentContextManager().Build(agentProtocolOpenAI, nil, nil, transcript, providerRuntimeProfile{}, 3)
	if !containsContextAction(snapshot.Summary.CompactionActions, "stale_read_after_mutation:draft.txt") {
		t.Fatalf("expected stale read compaction action, got %#v", snapshot.Summary.CompactionActions)
	}
	raw, err := json.Marshal(snapshot.OpenAIMessages)
	if err != nil {
		t.Fatalf("marshal messages: %v", err)
	}
	text := string(raw)
	if strings.Contains(text, "old draft content that should not be replayed") {
		t.Fatalf("expected stale pre-mutation read content to be compacted")
	}
	if !strings.Contains(text, "This file was modified later in the transcript") {
		t.Fatalf("expected compacted stale-read notice in provider context")
	}
}

func containsContextAction(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
