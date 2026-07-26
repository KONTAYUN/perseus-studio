package ai

import (
	"encoding/json"
	"strings"
	"testing"

	"webide-backend/internal/store"
)

func contextTestItem(seq int64, turnID, kind, role string, payload map[string]any) store.AgentTurnItem {
	raw, _ := json.Marshal(payload)
	return store.AgentTurnItem{
		Seq: seq, ID: "item-" + intString(int(seq)), UserID: "user", ConversationID: "conversation",
		TurnID: turnID, Kind: kind, Role: role, Status: "completed",
		Visibility: store.AgentItemVisibilityModel, SourceID: "source-" + intString(int(seq)),
		PayloadJSON: string(raw), Payload: raw,
	}
}

func contextTestProfile(window int) providerRuntimeProfile {
	return providerRuntimeProfile{
		ContextWindowTokens: window, ContextWindowSource: "user", AutoCompactEnabled: true,
		CompactThresholdPercent: 80, CompactReservedTokens: window / 10, PruneToolOutputsEnabled: true,
	}
}

func TestProtocolFeedbackRemainsRuntimeOwnedAcrossProviderProtocols(t *testing.T) {
	rejected := contextTestItem(2, "turn-1", "assistant_message", "assistant", map[string]any{
		"content": "I need more information.",
	})
	rejected.Status = "rejected"
	items := []store.AgentTurnItem{
		contextTestItem(1, "turn-1", "user_message", "user", map[string]any{"content": "Continue the task."}),
		rejected,
		contextTestItem(3, "turn-1", "protocol_feedback", "system", map[string]any{"content": rooStyleNoToolFeedback}),
	}

	openAI := serializeOpenAIContextItems(items, nil, "")
	for _, message := range openAI.OpenAIMessages {
		content, _ := message["content"].(string)
		if strings.Contains(content, "AUTOMATED AGENT PROTOCOL FEEDBACK") && message["role"] != "system" {
			t.Fatalf("OpenAI protocol feedback must stay runtime-owned: %#v", message)
		}
	}
	if !strings.Contains(mustJSON(openAI.OpenAIMessages), "Rejected assistant output") {
		t.Fatalf("rejected assistant output must remain model-visible: %#v", openAI.OpenAIMessages)
	}

	anthropic := serializeAnthropicContextItems(items, nil, "")
	if !strings.Contains(anthropic.AnthropicSystem, "AUTOMATED AGENT PROTOCOL FEEDBACK") ||
		!strings.Contains(anthropic.AnthropicSystem, "Rejected assistant output") {
		t.Fatalf("Anthropic runtime feedback must be carried in system context: %q", anthropic.AnthropicSystem)
	}
	if strings.Contains(mustJSON(anthropic.AnthropicMessages), "AUTOMATED AGENT PROTOCOL FEEDBACK") {
		t.Fatalf("Anthropic protocol feedback must not masquerade as a user message: %#v", anthropic.AnthropicMessages)
	}
}

func TestContextAssemblerKeepsLargeLowTokenConversationIntact(t *testing.T) {
	items := make([]store.AgentTurnItem, 0, 44)
	contents := make([]string, 0, 44)
	for index := 0; index < 44; index++ {
		content := strings.Repeat("对话内容", 125) + "-" + intString(index)
		contents = append(contents, content)
		role := "user"
		kind := "user_message"
		if index%2 == 1 {
			role = "assistant"
			kind = "assistant_message"
		}
		items = append(items, contextTestItem(int64(index+1), "turn-"+intString(index/2), kind, role, map[string]any{"content": content}))
	}

	assembly := (ContextAssembler{}).Assemble(ContextAssemblerInput{
		Protocol: agentProtocolOpenAI, Items: items, Profile: contextTestProfile(256000), CurrentTurnID: "turn-21",
	})
	if assembly.NeedsCompaction || assembly.PrunedResults != 0 {
		t.Fatalf("low-token conversation must stay intact: %#v", assembly.Snapshot.Summary)
	}
	if assembly.Snapshot.Summary.PayloadBeforeBytes <= 48*1024 {
		t.Fatalf("fixture must exceed the removed 48KB limit, got %d", assembly.Snapshot.Summary.PayloadBeforeBytes)
	}
	serialized, _ := json.Marshal(assembly.Snapshot.OpenAIMessages)
	for _, content := range contents {
		if !strings.Contains(string(serialized), content) {
			t.Fatalf("conversation content was lost: %q", content[len(content)-12:])
		}
	}
}

func TestContextAssemblerKeepsSingleLongChineseMessage(t *testing.T) {
	content := strings.Repeat("这是一段用户明确粘贴的完整中文内容。", 1200)
	item := contextTestItem(1, "turn-1", "user_message", "user", map[string]any{"content": content})
	assembly := (ContextAssembler{}).Assemble(ContextAssemblerInput{
		Protocol: agentProtocolOpenAI, Items: []store.AgentTurnItem{item}, Profile: contextTestProfile(256000), CurrentTurnID: "turn-1",
	})
	raw, _ := json.Marshal(assembly.Snapshot.OpenAIMessages)
	if !strings.Contains(string(raw), content) {
		t.Fatal("single long message was not preserved verbatim")
	}
}

func TestContextAssemblerKeepsOldLargeToolResultWithoutTokenPressure(t *testing.T) {
	items := []store.AgentTurnItem{
		contextTestItem(1, "turn-1", "user_message", "user", map[string]any{"content": "inspect old output"}),
		contextTestItem(2, "turn-1", "tool_call", "assistant", map[string]any{"tool_call_id": "call-1", "tool_name": "read_workspace_text", "arguments_json": `{"path":"logs.txt"}`}),
		contextTestItem(3, "turn-1", "tool_result", "tool", map[string]any{"tool_call_id": "call-1", "tool_name": "read_workspace_text", "path": "logs.txt", "ledger_id": "ledger-1", "content": strings.Repeat("tool-output-", 22000)}),
	}
	seq := int64(4)
	for turn := 2; turn <= 4; turn++ {
		turnID := "turn-" + intString(turn)
		items = append(items,
			contextTestItem(seq, turnID, "user_message", "user", map[string]any{"content": "user message " + turnID}),
			contextTestItem(seq+1, turnID, "assistant_message", "assistant", map[string]any{"content": "assistant reply " + turnID}),
		)
		seq += 2
	}

	assembly := (ContextAssembler{}).Assemble(ContextAssemblerInput{
		Protocol: agentProtocolOpenAI, Items: items, Profile: contextTestProfile(1000000), CurrentTurnID: "turn-4",
	})
	if assembly.PrunedResults != 0 {
		t.Fatalf("tool results must remain intact below the token threshold, got %d pruned", assembly.PrunedResults)
	}
	if !strings.Contains(items[2].PayloadJSON, "tool-output-") {
		t.Fatal("canonical stored item was mutated while creating the model view")
	}
	for _, item := range assembly.Items {
		if item.Kind == "user_message" || item.Kind == "assistant_message" {
			if strings.TrimSpace(contextItemContent(item)) == "" {
				t.Fatalf("chat message was pruned: %#v", item)
			}
		}
	}
	if result := contextItemContent(assembly.Items[2]); !strings.Contains(result, "tool-output-") {
		t.Fatalf("large tool result was unexpectedly changed: %s", result)
	}
}

func TestContextAssemblerDoesNotPruneWhitepaperAtLowWindowUsage(t *testing.T) {
	whitepaper := strings.Repeat("verified-whitepaper-fact-", 1500)
	items := []store.AgentTurnItem{
		contextTestItem(1, "turn-1", "user_message", "user", map[string]any{"content": "Read the whitepaper."}),
		contextTestItem(2, "turn-1", "tool_call", "assistant", map[string]any{"tool_call_id": "call-whitepaper", "tool_name": "read_workspace_note_view", "arguments_json": `{"path":"v14.7.html","view":"text"}`}),
		contextTestItem(3, "turn-1", "tool_result", "tool", map[string]any{"tool_call_id": "call-whitepaper", "tool_name": "read_workspace_note_view", "path": "v14.7.html", "doc_hash": "hash-1", "content": whitepaper}),
		contextTestItem(4, "turn-2", "user_message", "user", map[string]any{"content": "Continue from the verified facts."}),
	}
	assembly := (ContextAssembler{}).Assemble(ContextAssemblerInput{
		Protocol: agentProtocolOpenAI, Items: items, Profile: contextTestProfile(128000), CurrentTurnID: "turn-2",
	})
	if assembly.Tokens >= assembly.ThresholdTokens {
		t.Fatalf("fixture unexpectedly reached pressure: tokens=%d threshold=%d", assembly.Tokens, assembly.ThresholdTokens)
	}
	if assembly.PrunedResults != 0 || !strings.Contains(contextItemContent(assembly.Items[2]), "verified-whitepaper-fact-") {
		t.Fatalf("whitepaper result was pruned below the real threshold: %#v", assembly.Snapshot.Summary)
	}
}

func TestContextAssemblerPrunesEligibleResultOnlyUnderTokenPressure(t *testing.T) {
	large := strings.Repeat("historical-output-", 18000)
	items := []store.AgentTurnItem{
		contextTestItem(1, "turn-1", "user_message", "user", map[string]any{"content": "old investigation"}),
		contextTestItem(2, "turn-1", "tool_call", "assistant", map[string]any{"tool_call_id": "call-old", "tool_name": "read_workspace_text", "arguments_json": `{"path":"old.log"}`}),
		contextTestItem(3, "turn-1", "tool_result", "tool", map[string]any{"tool_call_id": "call-old", "tool_name": "read_workspace_text", "path": "old.log", "ledger_id": "ledger-old", "content": large}),
		contextTestItem(4, "turn-2", "workspace_observation", "system", map[string]any{"tool_call_id": "call-current", "tool_name": "read_workspace_note_view", "path": "v14.7.html", "view": "structure", "mode": "full", "doc_hash": "whitepaper-hash"}),
		contextTestItem(5, "turn-2", "tool_call", "assistant", map[string]any{"tool_call_id": "call-current", "tool_name": "read_workspace_note_view", "arguments_json": `{"path":"v14.7.html"}`}),
		contextTestItem(6, "turn-2", "tool_result", "tool", map[string]any{"tool_call_id": "call-current", "tool_name": "read_workspace_note_view", "path": "v14.7.html", "doc_hash": "whitepaper-hash", "content": strings.Repeat("whitepaper-fact-", 9000)}),
		contextTestItem(7, "turn-3", "user_message", "user", map[string]any{"content": strings.Repeat("current-task-", 4000)}),
	}
	assembly := (ContextAssembler{}).Assemble(ContextAssemblerInput{
		Protocol: agentProtocolOpenAI, Items: items, Profile: contextTestProfile(16000), CurrentTurnID: "turn-3",
	})
	if assembly.PrunedResults != 1 {
		t.Fatalf("expected only the unrelated old result to be pruned, got %d", assembly.PrunedResults)
	}
	if !strings.Contains(contextItemContent(assembly.Items[2]), `"model_content_pruned":true`) {
		t.Fatalf("old result did not become a ledger reference: %s", contextItemContent(assembly.Items[2]))
	}
	if !strings.Contains(contextItemContent(assembly.Items[5]), "whitepaper-fact-") {
		t.Fatal("latest task workspace read must be protected")
	}
}

func TestContextAssemblerTreatsProviderOverflowAsRealPressure(t *testing.T) {
	items := []store.AgentTurnItem{
		contextTestItem(1, "turn-old", "tool_call", "assistant", map[string]any{"tool_call_id": "call-old", "tool_name": "read_workspace_text", "arguments_json": `{"path":"old.log"}`}),
		contextTestItem(2, "turn-old", "tool_result", "tool", map[string]any{"tool_call_id": "call-old", "tool_name": "read_workspace_text", "path": "old.log", "content": strings.Repeat("provider-overflow-output-", 2000)}),
		contextTestItem(3, "turn-current", "user_message", "user", map[string]any{"content": "continue"}),
	}
	assembly := (ContextAssembler{}).Assemble(ContextAssemblerInput{
		Protocol: agentProtocolOpenAI, Items: items, Profile: contextTestProfile(128000),
		CurrentTurnID: "turn-current", Phase: "overflow_recovery", ForcePrune: true,
	})
	if assembly.PrunedResults != 1 || assembly.NeedsCompaction {
		t.Fatalf("explicit provider overflow should prune one eligible old result before semantic compaction: %#v", assembly)
	}
}

func TestContextAssemblerUsesPathOnlyEnvironmentAndSessionWorkingSet(t *testing.T) {
	items := []store.AgentTurnItem{
		contextTestItem(1, "turn-whitepaper", "user_message", "user", map[string]any{"content": "继续白皮书"}),
		contextTestItem(2, "turn-whitepaper", "environment_snapshot", "system", map[string]any{
			"active_file": "v14.7.html", "html_fragment": "<p>must never be injected</p>",
			"panels": []map[string]any{{"active_file_path": "v14.7.html", "open_tabs": []string{"v14.7.html"}}},
		}),
		contextTestItem(3, "turn-whitepaper", "workspace_observation", "system", map[string]any{"path": "v14.7.html", "hash": "hash-whitepaper", "view": "structure", "source": "read_workspace_note_view"}),
		contextTestItem(4, "turn-misc", "user_message", "user", map[string]any{"content": "看看当前文件"}),
		contextTestItem(5, "turn-misc", "environment_snapshot", "system", map[string]any{"active_file": "杂记.html"}),
	}
	assembly := (ContextAssembler{}).Assemble(ContextAssemblerInput{
		Protocol: agentProtocolOpenAI, Items: items, Profile: contextTestProfile(128000), CurrentTurnID: "turn-misc",
	})
	raw, _ := json.Marshal(assembly.Snapshot.OpenAIMessages)
	text := string(raw)
	if strings.Contains(text, "must never be injected") {
		t.Fatal("editor HTML content leaked into model context")
	}
	if !strings.Contains(text, "v14.7.html") || !strings.Contains(text, "杂记.html") {
		t.Fatalf("expected historical task path and current editor path, got %s", text)
	}
	if len(assembly.WorkingSet) != 1 || assembly.WorkingSet[0].Path != "v14.7.html" {
		t.Fatalf("tab switch replaced the task working set: %#v", assembly.WorkingSet)
	}
}

func TestContextAssemblerKeepsTaskPlanAndSourceBoundaries(t *testing.T) {
	items := []store.AgentTurnItem{
		contextTestItem(1, "turn-1", "user_message", "user", map[string]any{"content": "Update the local whitepaper from the server evidence."}),
		contextTestItem(2, "turn-1", "task_plan", "system", map[string]any{"content": `{"goal":"update whitepaper","local_targets":["v14.7.html"],"external_targets":["production server"],"current_step":"inspect server","next_steps":["update local note"]}`}),
		contextTestItem(3, "turn-1", "external_observation", "system", map[string]any{"content": `{"source":"user terminal","verified_facts":["service-a is running"]}`}),
		contextTestItem(4, "turn-1", "file_change", "system", map[string]any{"path": "v14.7.html", "hash": "new-hash", "changed_paths": []string{"v14.7.html"}}),
		contextTestItem(5, "turn-2", "task_plan", "system", map[string]any{"content": `{"goal":"update whitepaper","local_targets":["v14.7.html"],"external_targets":["production server"],"completed_steps":["inspect server"],"current_step":"update local note","next_steps":["verify local note"]}`}),
	}
	for _, protocol := range []agentProtocol{agentProtocolOpenAI, agentProtocolAnthropic} {
		assembly := (ContextAssembler{}).Assemble(ContextAssemblerInput{
			Protocol: protocol, Items: items, Profile: contextTestProfile(128000), CurrentTurnID: "turn-2",
		})
		raw, _ := json.Marshal(map[string]any{
			"system":    assembly.Snapshot.AnthropicSystem,
			"openai":    assembly.Snapshot.OpenAIMessages,
			"anthropic": assembly.Snapshot.AnthropicMessages,
		})
		text := string(raw)
		for _, expected := range []string{"v14.7.html", "production server", "service-a is running", "Verified local Perseus workspace file change", "new-hash"} {
			if !strings.Contains(text, expected) {
				t.Fatalf("protocol %s lost task/source boundary %q: %s", protocol, expected, text)
			}
		}
		if strings.Contains(text, `\"current_step\":\"inspect server\"`) {
			t.Fatalf("protocol %s retained superseded task plan: %s", protocol, text)
		}
	}
}

func TestContextWorkingSetInvalidatesAndRefreshesExactObservation(t *testing.T) {
	items := []store.AgentTurnItem{
		contextTestItem(1, "turn-1", "workspace_observation", "system", map[string]any{"path": "v14.7.html", "hash": "old"}),
		contextTestItem(2, "turn-2", "file_change", "system", map[string]any{"path": "v14.7.html", "hash": "new", "changed_paths": []string{"v14.7.html"}}),
	}
	working := contextWorkingSet(items)
	if len(working) != 1 || !working[0].Stale {
		t.Fatalf("file change must stale the previous exact read: %#v", working)
	}
	items = append(items, contextTestItem(3, "turn-2", "workspace_observation", "system", map[string]any{"path": "v14.7.html", "hash": "new", "view": "raw"}))
	working = contextWorkingSet(items)
	if len(working) != 1 || working[0].Stale || working[0].Hash != "new" {
		t.Fatalf("reread must refresh the working set: %#v", working)
	}
}

func TestContextAssemblerRepairsMissingToolResultAndDropsOrphan(t *testing.T) {
	items := []store.AgentTurnItem{
		contextTestItem(1, "turn-1", "user_message", "user", map[string]any{"content": "read it"}),
		contextTestItem(2, "turn-1", "tool_call", "assistant", map[string]any{"tool_call_id": "missing", "tool_name": "read_workspace_text", "arguments_json": `{}`}),
		contextTestItem(3, "turn-1", "tool_result", "tool", map[string]any{"tool_call_id": "orphan", "tool_name": "read_workspace_text", "content": "orphan"}),
	}
	assembly := (ContextAssembler{}).Assemble(ContextAssemblerInput{
		Protocol: agentProtocolOpenAI, Items: items, Profile: contextTestProfile(128000), CurrentTurnID: "turn-1",
	})
	raw, _ := json.Marshal(assembly.Snapshot.OpenAIMessages)
	text := string(raw)
	if !strings.Contains(text, "missing") || !strings.Contains(text, "aborted") {
		t.Fatalf("missing tool result was not repaired: %s", text)
	}
	if strings.Contains(text, "orphan") {
		t.Fatalf("orphan tool result entered model context: %s", text)
	}
}

func TestOpenAIAndAnthropicSerializeSameSemanticItems(t *testing.T) {
	items := []store.AgentTurnItem{
		contextTestItem(1, "turn-1", "user_message", "user", map[string]any{"content": "inspect"}),
		contextTestItem(2, "turn-1", "tool_call", "assistant", map[string]any{"tool_call_id": "call", "tool_name": "read_workspace_text", "arguments_json": `{"path":"a.txt"}`}),
		contextTestItem(3, "turn-1", "tool_result", "tool", map[string]any{"tool_call_id": "call", "tool_name": "read_workspace_text", "content": `{"content":"hello"}`}),
		contextTestItem(4, "turn-1", "assistant_message", "assistant", map[string]any{"content": "done"}),
	}
	openAI := (ContextAssembler{}).Assemble(ContextAssemblerInput{Protocol: agentProtocolOpenAI, Items: items, Profile: contextTestProfile(128000), CurrentTurnID: "turn-1"})
	anthropic := (ContextAssembler{}).Assemble(ContextAssemblerInput{Protocol: agentProtocolAnthropic, Items: items, Profile: contextTestProfile(128000), CurrentTurnID: "turn-1"})
	openRaw, _ := json.Marshal(openAI.Snapshot.OpenAIMessages)
	anthropicRaw, _ := json.Marshal(anthropic.Snapshot.AnthropicMessages)
	for _, expected := range []string{"inspect", "read_workspace_text", "hello", "done"} {
		if !strings.Contains(string(openRaw), expected) || !strings.Contains(string(anthropicRaw), expected) {
			t.Fatalf("protocol serializers diverged for %q\nopenai=%s\nanthropic=%s", expected, openRaw, anthropicRaw)
		}
	}
}

func TestContextWindowProfileUsesExplicitBuiltinOrDefaultSource(t *testing.T) {
	if tokens, source := providerContextWindowProfile(store.ModelProvider{ModelID: "gpt-4o"}); tokens != 128000 || source != "builtin" {
		t.Fatalf("unexpected built-in profile: %d %s", tokens, source)
	}
	if tokens, source := providerContextWindowProfile(store.ModelProvider{ModelID: "custom-claude-like-name"}); tokens != defaultContextWindowTokens || source != "default" {
		t.Fatalf("unknown model must not be guessed by name: %d %s", tokens, source)
	}
	if tokens, source := providerContextWindowProfile(store.ModelProvider{ModelID: "anything", ContextWindowTokens: 64000}); tokens != 64000 || source != "user" {
		t.Fatalf("explicit context window must win: %d %s", tokens, source)
	}
}

func TestCompactionKeepsCompleteRecentTurnSuffixAfterSummary(t *testing.T) {
	items := []store.AgentTurnItem{
		contextTestItem(1, "turn-1", "user_message", "user", map[string]any{"content": strings.Repeat("old-one", 120)}),
		contextTestItem(2, "turn-1", "assistant_message", "assistant", map[string]any{"content": strings.Repeat("reply-one", 120)}),
		contextTestItem(3, "turn-2", "user_message", "user", map[string]any{"content": strings.Repeat("old-two", 120)}),
		contextTestItem(4, "turn-2", "assistant_message", "assistant", map[string]any{"content": strings.Repeat("reply-two", 120)}),
		contextTestItem(5, "turn-3", "user_message", "user", map[string]any{"content": "recent-three"}),
		contextTestItem(6, "turn-3", "assistant_message", "assistant", map[string]any{"content": "recent-reply-three"}),
		contextTestItem(7, "turn-4", "user_message", "user", map[string]any{"content": "recent-four"}),
		contextTestItem(8, "turn-4", "assistant_message", "assistant", map[string]any{"content": "recent-reply-four"}),
	}
	retainTokens := estimateAgentItemsTokens(items[4:]) + 1
	if boundary := selectAgentCompactionBoundary(items, retainTokens); boundary != 4 {
		t.Fatalf("expected compaction through the second complete turn, got %d", boundary)
	}
	items = append(items, contextTestItem(9, "turn-4", "compaction", "system", map[string]any{
		"summary": "durable old memory", "replaces_through_seq": 4,
	}))
	active, boundary := activeAgentContextItems(items)
	if boundary != 4 || len(active) != 5 {
		t.Fatalf("unexpected active compacted history: boundary=%d items=%#v", boundary, active)
	}
	if active[0].Kind != "compaction" || contextItemContent(active[0]) != "durable old memory" {
		t.Fatalf("compaction summary must precede the recent suffix: %#v", active)
	}
	for _, item := range active[1:] {
		if item.TurnID != "turn-3" && item.TurnID != "turn-4" {
			t.Fatalf("a partial or replaced turn leaked into the recent suffix: %#v", item)
		}
	}
}
