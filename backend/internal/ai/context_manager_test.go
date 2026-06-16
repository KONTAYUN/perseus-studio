package ai

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"webide-backend/internal/store"
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
	if !strings.Contains(text, "draft.txt") || !strings.Contains(text, "read_ranges") {
		t.Fatalf("expected clean stale-read reference metadata in provider context: %s", text)
	}
	for _, leaked := range []string{"stale_after_later_mutation", "context_reference", "reference_only"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("provider-visible stale read leaked internal phrase %q in %s", leaked, text)
		}
	}
}

func TestContextManagerTracksBatchReadPathsForCompaction(t *testing.T) {
	transcript := newAgentTranscript()
	transcript.AppendAssistantTurn(agentModelTurn{ToolCalls: []agentToolCall{{
		ID:            "batch_read_1",
		Name:          "read_workspace_files",
		ArgumentsJSON: `{"paths":["a.txt","b.txt"]}`,
	}}})
	transcript.AppendToolResults([]TranscriptToolResult{{
		ToolCallID: "batch_read_1",
		ToolName:   "read_workspace_files",
		Content:    `{"items":[{"path":"a.txt","content":"old a"},{"path":"b.txt","content":"old b"}],"count":2}`,
		Event:      ToolEvent{ToolName: "read_workspace_files", Status: "success"},
	}})
	transcript.AppendAssistantTurn(agentModelTurn{ToolCalls: []agentToolCall{{
		ID:            "save_1",
		Name:          "save_workspace_file",
		ArgumentsJSON: `{"path":"b.txt","content":"new","mode":"overwrite"}`,
	}}})
	transcript.AppendToolResults([]TranscriptToolResult{{
		ToolCallID: "save_1",
		ToolName:   "save_workspace_file",
		Content:    `{"status":"success","path":"b.txt"}`,
		Event:      ToolEvent{ToolName: "save_workspace_file", Status: "success", Path: "b.txt", ChangedPaths: []string{"b.txt"}},
	}})

	snapshot := newAgentContextManager().Build(agentProtocolOpenAI, nil, nil, transcript, providerRuntimeProfile{}, 3)
	if !containsContextAction(snapshot.Summary.CompactionActions, "stale_read_after_mutation:b.txt") {
		t.Fatalf("expected batch stale read compaction action, got %#v", snapshot.Summary.CompactionActions)
	}
	raw, err := json.Marshal(snapshot.OpenAIMessages)
	if err != nil {
		t.Fatalf("marshal messages: %v", err)
	}
	if strings.Contains(string(raw), "old b") {
		t.Fatalf("expected stale batch read content to be compacted")
	}
	if !strings.Contains(string(raw), "old a") {
		t.Fatalf("expected unaffected batch read content to remain available")
	}
	text := string(raw)
	if !strings.Contains(text, "b.txt") || !strings.Contains(text, "read_ranges") {
		t.Fatalf("expected partial-stale batch read metadata in provider context: %s", text)
	}
	for _, leaked := range []string{"stale_after_later_mutation", "context_reference", "reference_only"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("provider-visible batch read leaked internal phrase %q in %s", leaked, text)
		}
	}
}

func TestContextManagerCompactsBatchNumberedContentStaleAfterMutation(t *testing.T) {
	transcript := newAgentTranscript()
	transcript.AppendAssistantTurn(agentModelTurn{ToolCalls: []agentToolCall{{
		ID:            "batch_read_1",
		Name:          "read_workspace_files",
		ArgumentsJSON: `{"paths":["a.txt","b.txt"]}`,
	}}})
	transcript.AppendToolResults([]TranscriptToolResult{{
		ToolCallID: "batch_read_1",
		ToolName:   "read_workspace_files",
		Content: `{"items":[` +
			`{"path":"a.txt","numbered_content":"1: old a"},` +
			`{"path":"b.txt","numbered_content":"1: old b"}` +
			`],"count":2}`,
		Event: ToolEvent{ToolName: "read_workspace_files", Status: "success"},
	}})
	transcript.AppendAssistantTurn(agentModelTurn{ToolCalls: []agentToolCall{{
		ID:            "save_1",
		Name:          "save_workspace_file",
		ArgumentsJSON: `{"path":"b.txt","content":"new","mode":"overwrite"}`,
	}}})
	transcript.AppendToolResults([]TranscriptToolResult{{
		ToolCallID: "save_1",
		ToolName:   "save_workspace_file",
		Content:    `{"status":"success","path":"b.txt"}`,
		Event:      ToolEvent{ToolName: "save_workspace_file", Status: "success", Path: "b.txt", ChangedPaths: []string{"b.txt"}},
	}})

	snapshot := newAgentContextManager().Build(agentProtocolOpenAI, nil, nil, transcript, providerRuntimeProfile{}, 3)
	raw, err := json.Marshal(snapshot.OpenAIMessages)
	if err != nil {
		t.Fatalf("marshal messages: %v", err)
	}
	text := string(raw)
	if strings.Contains(text, "old b") {
		t.Fatalf("expected stale numbered_content to be compacted")
	}
	if !strings.Contains(text, "old a") {
		t.Fatalf("expected unaffected numbered_content to remain")
	}
}

func TestContextManagerPreservesAdjacentReadRangesForSameFile(t *testing.T) {
	transcript := newAgentTranscript()
	appendReadRange(t, transcript, "read_1", "draft.txt", 240, 278, 400, "range 240-278 must stay")
	appendReadRange(t, transcript, "read_2", "draft.txt", 278, 355, 400, "range 278-355 must stay")

	compacted, actions := newAgentContextManager().compactTranscript(transcript, providerRuntimeProfile{})
	if len(actions) != 0 {
		t.Fatalf("adjacent ranges should not be compacted, got %#v", actions)
	}
	raw, err := json.Marshal(compacted)
	if err != nil {
		t.Fatalf("marshal transcript: %v", err)
	}
	text := string(raw)
	for _, want := range []string{"range 240-278 must stay", "range 278-355 must stay"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q to remain in compacted transcript", want)
		}
	}
}

func TestContextManagerCompactsExactDuplicateReadRange(t *testing.T) {
	transcript := newAgentTranscript()
	appendReadRange(t, transcript, "read_1", "draft.txt", 240, 278, 400, "same range content")
	appendReadRange(t, transcript, "read_2", "draft.txt", 240, 278, 400, "same range content")

	compacted, actions := newAgentContextManager().compactTranscript(transcript, providerRuntimeProfile{})
	if !containsActionPrefix(actions, "superseded_read:draft.txt:240-278") {
		t.Fatalf("expected duplicate range compaction, got %#v", actions)
	}
	older := compacted.Entries[1].ToolResults[0].Content
	newer := compacted.Entries[3].ToolResults[0].Content
	if !strings.Contains(older, "context_reference") {
		t.Fatalf("expected older duplicate read to be compacted, got %s", older)
	}
	if !strings.Contains(newer, "same range content") {
		t.Fatalf("expected newer duplicate read to remain, got %s", newer)
	}
}

func TestContextManagerCompactsContainedReadAfterWiderRead(t *testing.T) {
	transcript := newAgentTranscript()
	appendReadRange(t, transcript, "read_1", "draft.txt", 240, 278, 400, "narrow range content")
	appendReadRange(t, transcript, "read_2", "draft.txt", 240, 355, 400, "wider range content")

	compacted, actions := newAgentContextManager().compactTranscript(transcript, providerRuntimeProfile{})
	if !containsActionPrefix(actions, "superseded_read:draft.txt:240-278") {
		t.Fatalf("expected contained range compaction, got %#v", actions)
	}
	if !strings.Contains(compacted.Entries[1].ToolResults[0].Content, "context_reference") {
		t.Fatalf("expected contained older read to be compacted")
	}
}

func TestContextManagerDoesNotCompactContainedReadAfterTruncatedWiderRead(t *testing.T) {
	transcript := newAgentTranscript()
	appendReadRange(t, transcript, "read_1", "draft.txt", 240, 278, 400, "narrow range content")
	appendReadRangeWithTruncation(t, transcript, "read_2", "draft.txt", 240, 355, 400, "truncated wider range prefix", true)

	compacted, actions := newAgentContextManager().compactTranscript(transcript, providerRuntimeProfile{})
	if len(actions) != 0 {
		t.Fatalf("truncated wider range should not supersede precise earlier range, got %#v", actions)
	}
	if !strings.Contains(compacted.Entries[1].ToolResults[0].Content, "narrow range content") {
		t.Fatalf("expected precise older range to remain when newer wider read is truncated")
	}
}

func TestContextManagerProtectsBatchReadWithCurrentSpanFromBudgetPruning(t *testing.T) {
	transcript := newAgentTranscript()
	transcript.AppendAssistantTurn(agentModelTurn{ToolCalls: []agentToolCall{{
		ID:            "batch_read_1",
		Name:          "read_workspace_files",
		ArgumentsJSON: `{"paths":["a.txt","b.txt"],"start_line":1,"end_line":20}`,
	}}})
	transcript.AppendToolResults([]TranscriptToolResult{{
		ToolCallID: "batch_read_1",
		ToolName:   "read_workspace_files",
		Content: `{"items":[` +
			`{"path":"a.txt","content":"batch a range","start_line":1,"end_line":20,"total_lines":20},` +
			`{"path":"b.txt","content":"batch b range","start_line":1,"end_line":20,"total_lines":20}` +
			`],"count":2}`,
		Event: ToolEvent{ToolName: "read_workspace_files", Status: "success"},
	}})
	appendReadRange(t, transcript, "read_a_later", "a.txt", 1, 20, 20, "newer a range")

	currentReads := currentSuccessfulReadToolCalls(transcript)
	if !currentReads["batch_read_1"] {
		t.Fatalf("batch read should remain current because b.txt has no later superseding read")
	}
	if currentReads["read_a_later"] != true {
		t.Fatalf("latest single-file read should be current")
	}
}

func TestContextManagerPrunesOldToolResultsForBudget(t *testing.T) {
	transcript := newAgentTranscript()
	for index := 0; index < 8; index++ {
		callID := "cmd_" + string(rune('a'+index))
		transcript.AppendAssistantTurn(agentModelTurn{ToolCalls: []agentToolCall{{
			ID:            callID,
			Name:          "run_workspace_command",
			ArgumentsJSON: `{"command":"test"}`,
		}}})
		transcript.AppendToolResults([]TranscriptToolResult{{
			ToolCallID: callID,
			ToolName:   "run_workspace_command",
			Content:    strings.Repeat("large output ", 120),
			Event:      ToolEvent{ToolName: "run_workspace_command", Status: "success", Command: "test"},
		}})
	}

	snapshot := newAgentContextManager().Build(agentProtocolOpenAI, nil, nil, transcript, providerRuntimeProfile{
		ContextWindowTokens:     1200,
		CompactThresholdPercent: 80,
		CompactReservedTokens:   200,
		PruneToolOutputsEnabled: true,
	}, 8)
	if !containsActionPrefix(snapshot.Summary.CompactionActions, "budget_tool_result_pruned:run_workspace_command") {
		t.Fatalf("expected budget pruning action, got %#v", snapshot.Summary.CompactionActions)
	}
	if len(snapshot.OpenAIMessages) == 0 {
		t.Fatal("expected provider messages")
	}
}

func TestContextManagerCompactsReadResultsForPayloadPressure(t *testing.T) {
	transcript := newAgentTranscript()
	for index := 0; index < 4; index++ {
		callID := fmt.Sprintf("read_%d", index)
		path := fmt.Sprintf("chapter-%d.txt", index)
		transcript.AppendAssistantTurn(agentModelTurn{ToolCalls: []agentToolCall{{
			ID:            callID,
			Name:          "read_workspace_text",
			ArgumentsJSON: fmt.Sprintf(`{"path":%q,"start_line":1,"end_line":200}`, path),
		}}})
		payload := map[string]any{
			"path":             path,
			"numbered_content": fmt.Sprintf("1: CHAPTER_%d_UNIQUE_MARKER %s", index, strings.Repeat("payload pressure text ", 800)),
			"start_line":       1,
			"end_line":         200,
			"total_lines":      200,
			"truncated":        false,
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal read payload: %v", err)
		}
		transcript.AppendToolResults([]TranscriptToolResult{{
			ToolCallID: callID,
			ToolName:   "read_workspace_text",
			Content:    string(raw),
			Event:      ToolEvent{ToolName: "read_workspace_text", Status: "success", Path: path},
		}})
	}

	snapshot := newAgentContextManager().Build(agentProtocolOpenAI, nil, nil, transcript, providerRuntimeProfile{
		PayloadSoftBudgetBytes: 6000,
		PayloadHardBudgetBytes: 9000,
	}, 4)
	if !containsActionPrefix(snapshot.Summary.CompactionActions, "payload_pressure_read_compacted:read_workspace_text") {
		t.Fatalf("expected payload pressure read compaction, got %#v", snapshot.Summary.CompactionActions)
	}
	if !snapshot.Summary.PayloadCompactionApplied {
		t.Fatalf("expected payload compaction summary flag")
	}
	raw, err := json.Marshal(snapshot.OpenAIMessages)
	if err != nil {
		t.Fatalf("marshal messages: %v", err)
	}
	text := string(raw)
	for _, oldMarker := range []string{"CHAPTER_0_UNIQUE_MARKER", "CHAPTER_1_UNIQUE_MARKER", "CHAPTER_2_UNIQUE_MARKER"} {
		if strings.Contains(text, oldMarker) {
			t.Fatalf("expected old read body %s to be omitted from provider payload", oldMarker)
		}
	}
	if !strings.Contains(text, "CHAPTER_3_UNIQUE_MARKER") {
		t.Fatalf("expected latest working-set read body to remain, got %s", text)
	}
	if !strings.Contains(text, "chapter-0.txt") || !strings.Contains(text, "start_line") || !strings.Contains(text, "read_ranges") {
		t.Fatalf("expected path/range metadata and provider read view to remain, got %s", text)
	}
	if snapshot.Summary.SourceReadTruncated {
		t.Fatalf("source read truncation should remain false when tool result truncated=false")
	}
	if !snapshot.Summary.ProviderContextCompacted {
		t.Fatalf("expected provider context compaction diagnostic")
	}
	for _, leaked := range []string{"compacted_context", "context_reference", "reference_only", "provider_context_reference", "读取被压缩", "被系统压缩", "provider request payload", "Exact read text was omitted"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("provider-visible read reference leaked internal phrase %q in %s", leaked, text)
		}
	}
}

func TestProviderMessageBuilderUsesTypedInternalSummary(t *testing.T) {
	transcript := newAgentTranscript()
	for index := 0; index < 10; index++ {
		appendReadRange(t, transcript, fmt.Sprintf("read_%d", index), fmt.Sprintf("draft-%d.txt", index), 1, 40, 120, strings.Repeat("historical body ", 80))
	}

	snapshot := newAgentContextManager().BuildWithOptions(agentProtocolOpenAI, []store.Message{{Role: "user", Content: "继续。"}}, nil, transcript, providerRuntimeProfile{}, 10, ProviderMessageBuildOptions{
		ForceCompact:   true,
		Reason:         "checkpoint_resume",
		Phase:          "checkpoint_resume",
		TriggerEntries: 2,
		PreserveRecent: 4,
	})
	if !snapshot.Summary.MessageBuilderApplied || snapshot.Summary.ProviderMessagePhase != "checkpoint_resume" {
		t.Fatalf("expected typed builder compaction with checkpoint phase, got %#v", snapshot.Summary)
	}
	raw, err := json.Marshal(snapshot.OpenAIMessages)
	if err != nil {
		t.Fatalf("marshal messages: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"role":"system"`) || !strings.Contains(text, `"name":"perseus_context_summary"`) {
		t.Fatalf("expected OpenAI internal context summary as named system message, got %s", text)
	}
	if !strings.Contains(text, "Earlier task context summary") {
		t.Fatalf("expected task fact summary in provider payload")
	}
	if strings.Contains(text, `"role":"user","content":"Earlier task context summary`) {
		t.Fatalf("internal summary should not look like a real user message: %s", text)
	}
	for _, leaked := range []string{"payload_pressure", "provider_message_builder_compacted", "context_reference", "reference_only", "provider payload", "compacted transcript"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("summary leaked internal phrase %q in %s", leaked, text)
		}
	}
}

func TestCompactedToolResultDetectionUsesStatusField(t *testing.T) {
	if !isCompactedToolResultContent(`{"status":"compacted_context","message":"done"}`) {
		t.Fatal("expected compacted status to be detected")
	}
	if !isCompactedToolResultContent(`{"status":"context_reference","content_state":"reference_only"}`) {
		t.Fatal("expected context reference status to be detected")
	}
	if isCompactedToolResultContent(`{"content":"literal \"status\":\"compacted_context\" text"}`) {
		t.Fatal("did not expect literal content text to be treated as compacted")
	}
}

func TestProviderMessageBuilderCompactsLongTranscriptAndPreservesFacts(t *testing.T) {
	transcript := newAgentTranscript()
	for index := 0; index < 24; index++ {
		callID := fmt.Sprintf("read_%d", index)
		path := fmt.Sprintf("draft-%02d.txt", index)
		transcript.AppendAssistantTurn(agentModelTurn{
			Content: fmt.Sprintf("正在查看 %s", path),
			ToolCalls: []agentToolCall{{
				ID:            callID,
				Name:          "read_workspace_text",
				ArgumentsJSON: fmt.Sprintf(`{"path":%q}`, path),
			}},
		})
		transcript.AppendToolResults([]TranscriptToolResult{{
			ToolCallID: callID,
			ToolName:   "read_workspace_text",
			Content:    strings.Repeat("large historical read content for "+path+" ", 80),
			Event:      ToolEvent{ToolName: "read_workspace_text", Status: "success", Path: path},
		}})
	}
	transcript.AppendAssistantTurn(agentModelTurn{ToolCalls: []agentToolCall{{
		ID:            "patch_1",
		Name:          "apply_workspace_patch",
		ArgumentsJSON: `{"transport":"assistant_text_patch"}`,
	}}})
	transcript.AppendToolResults([]TranscriptToolResult{{
		ToolCallID: "patch_1",
		ToolName:   "apply_workspace_patch",
		Content:    `{"status":"success","changed_paths":["final.md"]}`,
		Event:      ToolEvent{ToolName: "apply_workspace_patch", Status: "success", Path: "final.md", ChangedPaths: []string{"final.md"}},
	}})
	messages := []store.Message{
		{Role: "user", Content: "请整理这批文档。"},
		{Role: "assistant", Content: "我会先查看。"},
		{Role: "user", Content: "继续。"},
	}

	snapshot := newAgentContextManager().BuildWithOptions(agentProtocolOpenAI, messages, nil, transcript, providerRuntimeProfile{}, 25, ProviderMessageBuildOptions{
		ForceCompact:   true,
		Reason:         "test_final_reply",
		TargetBytes:    48 * 1024,
		TriggerBytes:   64 * 1024,
		TriggerEntries: 16,
		PreserveRecent: 8,
	})

	if !snapshot.Summary.MessageBuilderApplied {
		t.Fatalf("expected provider message builder to apply")
	}
	if snapshot.Summary.CompactedEntries == 0 || snapshot.Summary.PreservedRecentEntries != 8 {
		t.Fatalf("unexpected compaction counts: compacted=%d preserved=%d", snapshot.Summary.CompactedEntries, snapshot.Summary.PreservedRecentEntries)
	}
	if !containsContextAction(snapshot.Summary.CompactionActions, "provider_message_builder_compacted:test_final_reply:entries=42:preserved=8") {
		t.Fatalf("expected builder compaction action, got %#v", snapshot.Summary.CompactionActions)
	}
	if !containsContextAction(snapshot.Summary.PreservedChangedPaths, "final.md") {
		t.Fatalf("expected changed path to be preserved, got %#v", snapshot.Summary.PreservedChangedPaths)
	}
	raw, err := json.Marshal(snapshot.OpenAIMessages)
	if err != nil {
		t.Fatalf("marshal messages: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "Earlier task context summary") {
		t.Fatalf("expected task fact summary in provider payload")
	}
	if strings.Contains(text, `"role":"user","content":"Earlier task context summary`) {
		t.Fatalf("internal continuation context must not be represented as an ordinary OpenAI user message")
	}
	for _, leaked := range []string{"payload_pressure", "provider_message_builder_compacted", "context_reference", "reference_only", "provider payload", "compacted transcript"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("summary leaked internal phrase %q in %s", leaked, text)
		}
	}
	if strings.Contains(text, strings.Repeat("large historical read content for draft-00.txt ", 10)) {
		t.Fatalf("expected old large read bodies to be folded out")
	}
	if !strings.Contains(text, "final.md") {
		t.Fatalf("expected verified changed path to remain")
	}
	if snapshot.Summary.PayloadAfterBytes <= 0 || snapshot.Summary.PayloadAfterBytes >= snapshot.Summary.PayloadBeforeBytes {
		t.Fatalf("expected payload to shrink, before=%d after=%d", snapshot.Summary.PayloadBeforeBytes, snapshot.Summary.PayloadAfterBytes)
	}
}

func TestProviderMessageBuilderSkipsShortTranscript(t *testing.T) {
	transcript := newAgentTranscript()
	appendReadRange(t, transcript, "read_1", "draft.txt", 1, 20, 20, "short content")

	snapshot := newAgentContextManager().BuildWithOptions(agentProtocolOpenAI, []store.Message{{Role: "user", Content: "看一下。"}}, nil, transcript, providerRuntimeProfile{}, 1, ProviderMessageBuildOptions{
		TriggerBytes:   64 * 1024,
		TriggerEntries: 16,
	})

	if snapshot.Summary.MessageBuilderApplied {
		t.Fatalf("did not expect provider message builder for short transcript")
	}
	raw, err := json.Marshal(snapshot.OpenAIMessages)
	if err != nil {
		t.Fatalf("marshal messages: %v", err)
	}
	if !strings.Contains(string(raw), "short content") {
		t.Fatalf("expected short read content to remain")
	}
}

func TestProviderMessageBuilderPreservesRecentWorkingSetReadInToolLoop(t *testing.T) {
	transcript := newAgentTranscript()
	for index := 0; index < 18; index++ {
		transcript.AppendAssistantTurn(agentModelTurn{ToolCalls: []agentToolCall{{
			ID:            fmt.Sprintf("cmd_%d", index),
			Name:          "run_workspace_command",
			ArgumentsJSON: `{"command":"echo old"}`,
		}}})
		transcript.AppendToolResults([]TranscriptToolResult{{
			ToolCallID: fmt.Sprintf("cmd_%d", index),
			ToolName:   "run_workspace_command",
			Content:    mustJSON(map[string]any{"stdout": strings.Repeat("older command output ", 220)}),
			Event:      ToolEvent{ToolName: "run_workspace_command", Status: "success"},
		}})
	}
	transcript.AppendAssistantTurn(agentModelTurn{ToolCalls: []agentToolCall{{
		ID:            "read_current",
		Name:          "read_workspace_text",
		ArgumentsJSON: `{"path":"draft.txt","start_line":1,"end_line":160}`,
	}}})
	currentRead := "1: RECENT WORKING SET READ MUST REMAIN\n" + strings.Repeat("2: current exact text\n", 360)
	transcript.AppendToolResults([]TranscriptToolResult{{
		ToolCallID: "read_current",
		ToolName:   "read_workspace_text",
		Content: mustJSON(map[string]any{
			"path":             "draft.txt",
			"numbered_content": currentRead,
			"start_line":       1,
			"end_line":         160,
			"total_lines":      160,
			"truncated":        false,
		}),
		Event: ToolEvent{ToolName: "read_workspace_text", Status: "success", Path: "draft.txt"},
	}})

	snapshot := newAgentContextManager().BuildWithOptions(agentProtocolOpenAI, []store.Message{{Role: "user", Content: "继续编辑。"}}, nil, transcript, providerRuntimeProfile{}, 20, ProviderMessageBuildOptions{
		Phase:          "tool_loop",
		Reason:         "tool_loop",
		TriggerBytes:   12 * 1024,
		TargetBytes:    8 * 1024,
		TriggerEntries: 8,
		PreserveRecent: 4,
	})
	raw, err := json.Marshal(snapshot.OpenAIMessages)
	if err != nil {
		t.Fatalf("marshal messages: %v", err)
	}
	text := string(raw)
	if !snapshot.Summary.MessageBuilderApplied {
		t.Fatalf("expected builder compaction to apply")
	}
	if !strings.Contains(text, "RECENT WORKING SET READ MUST REMAIN") {
		t.Fatalf("recent working-set read body was lost: %s", text)
	}
	if !containsContextAction(snapshot.Summary.CurrentReadWorkingSetPaths, "draft.txt") {
		t.Fatalf("expected working-set diagnostics to include draft.txt, got %#v", snapshot.Summary.CurrentReadWorkingSetPaths)
	}
	if !containsContextAction(snapshot.Summary.CurrentReadWorkingSetRanges, "draft.txt:1-160") {
		t.Fatalf("expected working-set range diagnostic, got %#v", snapshot.Summary.CurrentReadWorkingSetRanges)
	}
	injected := injectModelActionManifest(agentProtocolOpenAI, snapshot, modelActionManifest(nil, true, "admin"))
	injectedRaw, err := json.Marshal(injected.OpenAIMessages)
	if err != nil {
		t.Fatalf("marshal injected messages: %v", err)
	}
	injectedText := string(injectedRaw)
	if !strings.Contains(injectedText, "current_read_working_set") || !strings.Contains(injectedText, "content_available_in_recent_context") {
		t.Fatalf("expected action manifest to expose reusable current read working set: %s", injectedText)
	}
	for _, leaked := range []string{"compacted_context", "reference_only", "provider payload", "读取被压缩", "被系统压缩"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("provider-visible context leaked internal phrase %q in %s", leaked, text)
		}
	}
}

func TestProviderMessageBuilderMayCompactCurrentReadInForcedRecovery(t *testing.T) {
	transcript := newAgentTranscript()
	for index := 0; index < 4; index++ {
		transcript.AppendAssistantTurn(agentModelTurn{ToolCalls: []agentToolCall{{
			ID:            fmt.Sprintf("cmd_recovery_%d", index),
			Name:          "run_workspace_command",
			ArgumentsJSON: `{"command":"echo old"}`,
		}}})
		transcript.AppendToolResults([]TranscriptToolResult{{
			ToolCallID: fmt.Sprintf("cmd_recovery_%d", index),
			ToolName:   "run_workspace_command",
			Content:    mustJSON(map[string]any{"stdout": strings.Repeat("older recovery output ", 100)}),
			Event:      ToolEvent{ToolName: "run_workspace_command", Status: "success"},
		}})
	}
	appendReadRange(t, transcript, "read_current", "draft.txt", 1, 300, 300, strings.Repeat("forced recovery read ", 1000))

	snapshot := newAgentContextManager().BuildWithOptions(agentProtocolOpenAI, []store.Message{{Role: "user", Content: "继续。"}}, nil, transcript, providerRuntimeProfile{}, 3, ProviderMessageBuildOptions{
		ForceCompact:   true,
		Phase:          "checkpoint_resume",
		Reason:         "checkpoint_resume",
		TargetBytes:    6 * 1024,
		TriggerBytes:   6 * 1024,
		TriggerEntries: 1,
		PreserveRecent: 2,
	})
	raw, err := json.Marshal(snapshot.OpenAIMessages)
	if err != nil {
		t.Fatalf("marshal messages: %v", err)
	}
	text := string(raw)
	if strings.Contains(text, "forced recovery read forced recovery read") {
		t.Fatalf("forced recovery should be allowed to fold an oversized current read: %s", text)
	}
	if !strings.Contains(text, "draft.txt") || !strings.Contains(text, "read_ranges") {
		t.Fatalf("expected read range metadata after forced compaction: %s", text)
	}
}

func TestCurrentFileAttachmentDoesNotForceReadLanguage(t *testing.T) {
	content := buildUserTextWithAttachments("中文写吧", []AttachmentPayload{{
		Kind:     "current_file",
		Path:     "draft.txt",
		Name:     "draft.txt",
		FileType: "text",
	}})
	if strings.Contains(content, "需要内容时必须调用") || strings.Contains(content, "需要内容时请调用") {
		t.Fatalf("current file attachment should not unconditionally force read: %s", content)
	}
	for _, want := range []string{"不代表本轮必须读取", "没有可复用 recent read observation/current_read_working_set"} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected balanced read guidance %q in %s", want, content)
		}
	}
}

func TestModelActionManifestOmitsStaleCurrentReadWorkingSetAfterMutation(t *testing.T) {
	transcript := newAgentTranscript()
	appendReadRange(t, transcript, "read_1", "draft.txt", 1, 40, 80, "old exact text")
	transcript.AppendAssistantTurn(agentModelTurn{ToolCalls: []agentToolCall{{
		ID:            "patch_1",
		Name:          "apply_workspace_patch",
		ArgumentsJSON: `{"transport":"assistant_text_patch"}`,
	}}})
	transcript.AppendToolResults([]TranscriptToolResult{{
		ToolCallID: "patch_1",
		ToolName:   "apply_workspace_patch",
		Content:    `{"status":"success","changed_paths":["draft.txt"]}`,
		Event:      ToolEvent{ToolName: "apply_workspace_patch", Status: "success", Path: "draft.txt", ChangedPaths: []string{"draft.txt"}},
	}})

	snapshot := newAgentContextManager().Build(agentProtocolOpenAI, []store.Message{{Role: "user", Content: "继续编辑。"}}, nil, transcript, providerRuntimeProfile{}, 3)
	injected := injectModelActionManifest(agentProtocolOpenAI, snapshot, modelActionManifest(nil, true, "admin"))
	raw, err := json.Marshal(injected.OpenAIMessages)
	if err != nil {
		t.Fatalf("marshal messages: %v", err)
	}
	text := string(raw)
	if strings.Contains(text, `"current_read_working_set"`) {
		t.Fatalf("stale read should not be exposed as reusable working set: %s", text)
	}
	if len(snapshot.Summary.CurrentReadWorkingSetPaths) != 0 || len(snapshot.Summary.CurrentReadWorkingSetRanges) != 0 {
		t.Fatalf("stale read should not appear in working-set diagnostics: %#v %#v", snapshot.Summary.CurrentReadWorkingSetPaths, snapshot.Summary.CurrentReadWorkingSetRanges)
	}
	if strings.Contains(text, "old exact text") {
		t.Fatalf("stale read body should be compacted after mutation: %s", text)
	}
}

func TestAgentRuntimeProviderMessageOptionsForceFinalAndRecovery(t *testing.T) {
	runtime := &agentRuntime{}
	if opts := runtime.providerMessageBuildOptions(); opts.ForceCompact || opts.Reason != "tool_loop" {
		t.Fatalf("expected normal tool loop options, got %#v", opts)
	}
	runtime.performedMutation = true
	if opts := runtime.providerMessageBuildOptions(); opts.ForceCompact || opts.Reason != "tool_loop" {
		t.Fatalf("mutation alone must not force final reply compaction, got %#v", opts)
	}
	runtime.checkpointResumeAttempts = 1
	if opts := runtime.providerMessageBuildOptions(); !opts.ForceCompact || opts.Reason != "checkpoint_resume" {
		t.Fatalf("expected checkpoint force compaction, got %#v", opts)
	}
	runtime.performedMutation = false
	if opts := runtime.providerMessageBuildOptions(); !opts.ForceCompact || opts.Reason != "checkpoint_resume" {
		t.Fatalf("expected checkpoint force compaction, got %#v", opts)
	}
	if opts := runtime.providerMessageBuildOptionsForPhase("final_reply", true); !opts.ForceCompact || opts.Reason != "final_reply" || opts.Phase != "final_reply" {
		t.Fatalf("expected explicit final reply force compaction, got %#v", opts)
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

func containsActionPrefix(items []string, prefix string) bool {
	for _, item := range items {
		if strings.HasPrefix(item, prefix) {
			return true
		}
	}
	return false
}

func appendReadRange(t *testing.T, transcript *AgentTranscript, callID, path string, startLine, endLine, totalLines int, content string) {
	t.Helper()
	appendReadRangeWithTruncation(t, transcript, callID, path, startLine, endLine, totalLines, content, false)
}

func appendReadRangeWithTruncation(t *testing.T, transcript *AgentTranscript, callID, path string, startLine, endLine, totalLines int, content string, truncated bool) {
	t.Helper()
	transcript.AppendAssistantTurn(agentModelTurn{ToolCalls: []agentToolCall{{
		ID:            callID,
		Name:          "read_workspace_text",
		ArgumentsJSON: fmt.Sprintf(`{"path":%q,"start_line":%d,"end_line":%d}`, path, startLine, endLine),
	}}})
	payload := map[string]any{
		"path":        path,
		"content":     content,
		"truncated":   truncated,
		"start_line":  startLine,
		"end_line":    endLine,
		"total_lines": totalLines,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal read payload: %v", err)
	}
	transcript.AppendToolResults([]TranscriptToolResult{{
		ToolCallID: callID,
		ToolName:   "read_workspace_text",
		Content:    string(raw),
		Event: ToolEvent{
			ToolName: "read_workspace_text",
			Status:   "success",
			Path:     path,
		},
	}})
}
