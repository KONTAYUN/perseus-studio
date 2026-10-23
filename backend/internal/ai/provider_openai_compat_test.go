package ai

import (
	"reflect"
	"strings"
	"testing"

	"webide-backend/internal/store"
)

func TestGrokKeepsOneSystemMessageAcrossToolRounds(t *testing.T) {
	userContent := []map[string]any{{"type": "text", "text": "edit this"}, {"type": "image_url", "image_url": map[string]any{"url": "https://example.com/test.png"}}}
	messages := []map[string]any{
		{"role": "system", "content": "base instructions"},
		{"role": "system", "content": "available tools"},
		{"role": "user", "content": userContent},
		{"role": "assistant", "tool_calls": []map[string]any{{"id": "call_1", "type": "function"}}},
		{"role": "tool", "tool_call_id": "call_1", "content": "read result"},
		{"role": "system", "name": "perseus_runtime_observation", "content": "edit now"},
	}
	payload := map[string]any{"messages": messages, "stream": true, "tool_choice": "required"}
	for _, model := range []string{"grok-4.3", "grok-4.20-0309-non-reasoning", "x-ai/grok-4.7"} {
		normalized := normalizeOpenAIProviderPayload(store.ModelProvider{ModelID: model}, payload)
		got := normalized["messages"].([]map[string]any)
		if len(got) != 4 || got[0]["role"] != "system" {
			t.Fatalf("unexpected normalized messages: %#v", got)
		}
		for _, text := range []string{"base instructions", "available tools", "perseus_runtime_observation", "edit now"} {
			if !strings.Contains(got[0]["content"].(string), text) {
				t.Fatalf("lost context %q", text)
			}
		}
		if !reflect.DeepEqual(got[1:], messages[2:5]) {
			t.Fatal("changed user content, tool calls, or tool result ordering")
		}
		if normalized["tool_choice"] != "required" || normalized["stream"] != true {
			t.Fatal("changed transport options")
		}
	}
	if len(payload["messages"].([]map[string]any)) != 6 || messages[0]["content"] != "base instructions" {
		t.Fatal("mutated original context snapshot")
	}
}

func TestExplicitMultipleSystemCapabilityPreservesMessages(t *testing.T) {
	messages := []map[string]any{{"role": "system", "content": "one"}, {"role": "system", "content": "two"}}
	payload := map[string]any{"messages": messages}
	got := normalizeOpenAIProviderPayload(store.ModelProvider{ModelID: "alias", SystemMessageMode: "multiple"}, payload)
	if !reflect.DeepEqual(got, payload) {
		t.Fatal("changed another provider's messages")
	}
}
