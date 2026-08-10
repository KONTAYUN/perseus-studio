package ai

import (
	"io"
	"strings"
	"testing"
)

func TestParseOpenAIStreamAccumulatesTextAndToolCalls(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"hello "}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"world"}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"save_workspace_file","arguments":"{\"path\""}}]}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"a.txt\",\"content\":\"x\",\"mode\":\"create\"}"}}]},"finish_reason":"tool_calls"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	var deltas []string
	turn, err := parseOpenAIStream(strings.NewReader(stream), func(text string) {
		deltas = append(deltas, text)
	})
	if err != nil {
		t.Fatalf("parseOpenAIStream returned error: %v", err)
	}
	if turn.Content != "hello world" {
		t.Fatalf("unexpected content: %q", turn.Content)
	}
	if strings.Join(deltas, "") != "hello world" {
		t.Fatalf("unexpected deltas: %#v", deltas)
	}
	if len(turn.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %#v", turn.ToolCalls)
	}
	call := turn.ToolCalls[0]
	if call.ID != "call_1" || call.Name != "save_workspace_file" {
		t.Fatalf("unexpected tool call: %#v", call)
	}
	if call.ArgumentsJSON != `{"path":"a.txt","content":"x","mode":"create"}` {
		t.Fatalf("unexpected args: %q", call.ArgumentsJSON)
	}
	if !turn.Streamed {
		t.Fatalf("expected streamed turn")
	}
}

func TestParseOpenAIStreamDeduplicatesExactSSEReplay(t *testing.T) {
	first := `{"id":"resp_1","choices":[{"delta":{"content":"first "}}]}`
	second := `{"id":"resp_1","choices":[{"delta":{"content":"second"},"finish_reason":"stop"}]}`
	stream := strings.Join([]string{
		"id: event_1", "data: " + first, "",
		"id: event_1", "data: " + first, "",
		"id: event_1", "data: " + second, "",
		"data: [DONE]", "",
	}, "\n")
	turn, err := parseOpenAIStream(strings.NewReader(stream), nil)
	if err != nil {
		t.Fatalf("parseOpenAIStream returned error: %v", err)
	}
	if turn.Content != "first second" {
		t.Fatalf("unexpected content after replay dedupe: %q", turn.Content)
	}
	if turn.StreamDuplicateEvents != 1 {
		t.Fatalf("expected one duplicate event, got %d", turn.StreamDuplicateEvents)
	}
	if turn.ProviderResponseID != "resp_1" || turn.StreamLastEventID != "event_1" {
		t.Fatalf("unexpected stream identity: response=%q event=%q", turn.ProviderResponseID, turn.StreamLastEventID)
	}
}

func TestToolArgumentStreamAssemblerDeltaKeepsIdenticalFragments(t *testing.T) {
	assembler := toolArgumentStreamAssembler{mode: "delta"}
	if _, _, _, err := assembler.append("x"); err != nil {
		t.Fatalf("first delta returned error: %v", err)
	}
	assembled, relation, replayed, err := assembler.append("x")
	if err != nil {
		t.Fatalf("second delta returned error: %v", err)
	}
	if assembled != "xx" || relation != "delta" || replayed {
		t.Fatalf("identical delta fragment was treated as replay: value=%q relation=%q replayed=%v", assembled, relation, replayed)
	}
}

func TestToolArgumentStreamAssemblerUsesExplicitSnapshotMode(t *testing.T) {
	assembler := toolArgumentStreamAssembler{mode: "snapshot"}
	if _, relation, _, err := assembler.append(`{"path":"a"}`); err != nil || relation != "initial" {
		t.Fatalf("unexpected initial snapshot: relation=%q err=%v", relation, err)
	}
	assembled, relation, replayed, err := assembler.append(`{"path":"ab"}`)
	if err != nil || assembled != `{"path":"ab"}` || relation != "snapshot" || replayed {
		t.Fatalf("explicit snapshot was not accepted: value=%q relation=%q replayed=%v err=%v", assembled, relation, replayed, err)
	}
	assembled, relation, replayed, err = assembler.append(`{"path":"ab"}`)
	if err != nil || assembled != `{"path":"ab"}` || relation != "replay" || !replayed {
		t.Fatalf("identical snapshot was not treated as replay: value=%q relation=%q replayed=%v err=%v", assembled, relation, replayed, err)
	}
}

func TestToolArgumentStreamAssemblerRejectsImplicitAutoMode(t *testing.T) {
	assembler := toolArgumentStreamAssembler{mode: "auto"}
	if _, _, _, err := assembler.append(`{"path":"a"}`); err == nil {
		t.Fatal("auto mode must not infer a provider protocol")
	}
}

func TestToolArgumentStreamAssemblerRejectsUnexplainedModeSwitch(t *testing.T) {
	assembler := toolArgumentStreamAssembler{mode: "auto"}
	_, _, _, _ = assembler.append(`{"path"`)
	_, _, _, _ = assembler.append(`{"path":"a"}`)
	_, _, _, err := assembler.append(`}`)
	protocolErr, ok := err.(*ProviderProtocolError)
	if !ok || protocolErr.Code != string(ToolErrorProviderArgumentStream) {
		t.Fatalf("expected structured argument stream conflict, got %#v", err)
	}
}

func TestParseOpenAIStreamAssemblesExplicitCumulativeToolArguments(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read_workspace_text","arguments":"{\"path\""}}]}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":\"a.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	turn, err := parseOpenAIStreamWithOptions(strings.NewReader(stream), providerStreamParseOptions{ToolArgumentStreamMode: "snapshot"})
	if err != nil {
		t.Fatalf("cumulative OpenAI stream returned error: %v", err)
	}
	if len(turn.ToolCalls) != 1 || turn.ToolCalls[0].ArgumentsJSON != `{"path":"a.txt"}` {
		t.Fatalf("unexpected assembled tool call: %#v", turn.ToolCalls)
	}
}

func TestParseAnthropicStreamDeduplicatesExactSSEReplay(t *testing.T) {
	delta := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`
	stream := strings.Join([]string{
		"id: event_0", `event: message_start`, `data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":1}}}`, "",
		"id: event_1", `event: content_block_start`, `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`, "",
		"id: event_2", `event: content_block_delta`, "data: " + delta, "",
		"id: event_2", `event: content_block_delta`, "data: " + delta, "",
		"id: event_3", `event: message_stop`, `data: {"type":"message_stop"}`, "",
	}, "\n")
	turn, err := parseAnthropicStream(strings.NewReader(stream), nil)
	if err != nil {
		t.Fatalf("parseAnthropicStream returned error: %v", err)
	}
	if turn.Content != "hello" {
		t.Fatalf("unexpected content after replay dedupe: %q", turn.Content)
	}
	if turn.StreamDuplicateEvents != 1 {
		t.Fatalf("expected one duplicate event, got %d", turn.StreamDuplicateEvents)
	}
	if turn.ProviderResponseID != "msg_1" || turn.StreamLastEventID != "event_3" {
		t.Fatalf("unexpected stream identity: response=%q event=%q", turn.ProviderResponseID, turn.StreamLastEventID)
	}
}

func TestDetectProviderOutputRepetitionDoesNotRewriteContent(t *testing.T) {
	block := "Run this command on the server and return the complete output so I can verify the current deployment state."
	content := block + "\n\n" + block
	repetition := detectProviderOutputRepetition(content)
	if repetition.Count != 1 || len(repetition.FragmentHashes) != 1 {
		t.Fatalf("unexpected repetition diagnostic: %#v", repetition)
	}
	if content != block+"\n\n"+block {
		t.Fatal("repetition diagnostics must not alter provider content")
	}
}

func TestRuntimeAssistantDeltaPreservesChunkWhitespace(t *testing.T) {
	var deltas []string
	runtime := &agentRuntime{
		activeAssistantMessageID: "active-1",
		opts: RunOptions{
			OnAssistantMessageDelta: func(_ string, delta string) {
				deltas = append(deltas, delta)
			},
		},
	}

	runtime.appendActiveAssistantDelta("hello ")
	runtime.appendActiveAssistantDelta("world\n")

	if strings.Join(deltas, "") != "hello world\n" {
		t.Fatalf("expected whitespace-preserving deltas, got %#v", deltas)
	}
}

func TestParseOpenAIStreamRevealsTextAfterToolWhenRequired(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"我先读取文件。"}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read_workspace_text","arguments":"{\"path\":\"a.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	var deltas []string
	turn, err := parseOpenAIStreamWithOptions(strings.NewReader(stream), providerStreamParseOptions{
		EmitText: func(text string) {
			deltas = append(deltas, text)
		},
		RevealTextAfterTool: true,
	})
	if err != nil {
		t.Fatalf("parseOpenAIStream returned error: %v", err)
	}
	if turn.Content != "我先读取文件。" {
		t.Fatalf("unexpected content: %q", turn.Content)
	}
	if strings.Join(deltas, "") != "我先读取文件。" {
		t.Fatalf("expected gated text to flush after tool call, got %#v", deltas)
	}
	if len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Name != "read_workspace_text" {
		t.Fatalf("unexpected tool calls: %#v", turn.ToolCalls)
	}
}

func TestParseOpenAIStreamSuppressesEllipsisToolPreamble(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"..."}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read_workspace_text","arguments":"{\"path\":\"a.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	var deltas []string
	turn, err := parseOpenAIStreamWithOptions(strings.NewReader(stream), providerStreamParseOptions{
		EmitText: func(text string) {
			deltas = append(deltas, text)
		},
		RevealTextAfterTool: true,
	})
	if err != nil {
		t.Fatalf("parseOpenAIStream returned error: %v", err)
	}
	if turn.Content != "..." {
		t.Fatalf("stream parser should preserve provider content for runtime cleanup, got %q", turn.Content)
	}
	if len(deltas) != 0 {
		t.Fatalf("ellipsis preamble should not be emitted to user, got %#v", deltas)
	}
	prepared, _ := prepareToolTurnForExecution(turn)
	if prepared.Content != "" {
		t.Fatalf("ellipsis preamble should be removed before transcript/commit, got %q", prepared.Content)
	}
}

func TestParseOpenAIStreamDoesNotRevealRequiredTextWithoutTool(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"我稍后会处理。"}}]}`,
		``,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	var deltas []string
	turn, err := parseOpenAIStreamWithOptions(strings.NewReader(stream), providerStreamParseOptions{
		EmitText: func(text string) {
			deltas = append(deltas, text)
		},
		RevealTextAfterTool: true,
	})
	if err != nil {
		t.Fatalf("parseOpenAIStream returned error: %v", err)
	}
	if turn.Content != "我稍后会处理。" {
		t.Fatalf("unexpected content: %q", turn.Content)
	}
	if len(deltas) != 0 {
		t.Fatalf("required no-tool text should stay hidden from the user, got %#v", deltas)
	}
}

func TestParseOpenAIStreamReturnsPartialCompleteToolCallOnTransportError(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"save_workspace_file","arguments":"{\"path\":\"a.txt\",\"content\":\"x\",\"mode\":\"create\"}"}}]}}]}`,
		``,
	}, "\n")
	_, err := parseOpenAIStream(&errAfterReader{data: stream, err: io.ErrUnexpectedEOF}, nil)
	if err == nil {
		t.Fatal("expected stream error")
	}
	turn, ok := partialTurnFromStreamError(err)
	if !ok {
		t.Fatalf("expected recoverable partial tool call, got %T %v", err, err)
	}
	if len(turn.ToolCalls) != 1 {
		t.Fatalf("expected one recovered tool call, got %#v", turn.ToolCalls)
	}
	call := turn.ToolCalls[0]
	if call.ID != "call_1" || call.Name != "save_workspace_file" || call.ArgumentsJSON != `{"path":"a.txt","content":"x","mode":"create"}` {
		t.Fatalf("unexpected recovered tool call: %#v", call)
	}
}

func TestParseOpenAIStreamPreservesEmptyToolArgumentsAsInvalid(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"save_workspace_file"}}]},"finish_reason":"tool_calls"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	turn, err := parseOpenAIStream(strings.NewReader(stream), nil)
	if err != nil {
		t.Fatalf("parseOpenAIStream returned error: %v", err)
	}
	if len(turn.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %#v", turn.ToolCalls)
	}
	if turn.ToolCalls[0].ArgumentsJSON != "" {
		t.Fatalf("empty streamed arguments must stay empty for validation, got %q", turn.ToolCalls[0].ArgumentsJSON)
	}
}

func TestParseOpenAIStreamRemovesVisibleReasoningTagsAcrossChunks(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"<thin"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"king>\ninternal notes\n</thinking>\n visible"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":" answer"}}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	var deltas []string
	turn, err := parseOpenAIStream(strings.NewReader(stream), func(text string) {
		deltas = append(deltas, text)
	})
	if err != nil {
		t.Fatalf("parseOpenAIStream returned error: %v", err)
	}
	if turn.Content != "visible answer" {
		t.Fatalf("unexpected visible content: %q", turn.Content)
	}
	if strings.Contains(strings.Join(deltas, ""), "internal notes") {
		t.Fatalf("reasoning leaked into deltas: %#v", deltas)
	}
}

func TestParseAnthropicStreamAccumulatesTextAndToolUse(t *testing.T) {
	stream := strings.Join([]string{
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":"hi "}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"there"}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"read_workspace_text","input":{}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":":\"a.txt\"}"}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")
	var deltas []string
	turn, err := parseAnthropicStream(strings.NewReader(stream), func(text string) {
		deltas = append(deltas, text)
	})
	if err != nil {
		t.Fatalf("parseAnthropicStream returned error: %v", err)
	}
	if turn.Content != "hi there" {
		t.Fatalf("unexpected content: %q", turn.Content)
	}
	if strings.Join(deltas, "") != "hi there" {
		t.Fatalf("unexpected deltas: %#v", deltas)
	}
	if len(turn.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %#v", turn.ToolCalls)
	}
	call := turn.ToolCalls[0]
	if call.ID != "toolu_1" || call.Name != "read_workspace_text" || call.ArgumentsJSON != `{"path":"a.txt"}` {
		t.Fatalf("unexpected tool call: %#v", call)
	}
	if len(turn.AnthropicRaw) != 2 {
		t.Fatalf("expected raw text and tool blocks, got %#v", turn.AnthropicRaw)
	}
}

func TestParseAnthropicStreamPreservesEmptyToolArgumentsAsObject(t *testing.T) {
	stream := strings.Join([]string{
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"save_workspace_file","input":{}}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")
	turn, err := parseAnthropicStream(strings.NewReader(stream), nil)
	if err != nil {
		t.Fatalf("parseAnthropicStream returned error: %v", err)
	}
	if len(turn.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %#v", turn.ToolCalls)
	}
	if turn.ToolCalls[0].ArgumentsJSON != "{}" {
		t.Fatalf("empty streamed arguments must become an object for tool validation, got %q", turn.ToolCalls[0].ArgumentsJSON)
	}
	if hasIncompleteToolCallArguments(turn) {
		t.Fatalf("expected empty object arguments to be handled by tool validation, got %#v", turn.ToolCalls[0])
	}
}

func TestParseAnthropicStreamDetectsTruncatedToolArguments(t *testing.T) {
	stream := strings.Join([]string{
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"save_workspace_file","input":{}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"机甲少女/机甲少女11-22集修改版.txt\", \"mode\": \"create\""}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"max_tokens"}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")
	turn, err := parseAnthropicStream(strings.NewReader(stream), nil)
	if err != nil {
		t.Fatalf("parseAnthropicStream returned error: %v", err)
	}
	if len(turn.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %#v", turn.ToolCalls)
	}
	if !hasIncompleteToolCallArguments(turn) {
		t.Fatalf("expected truncated JSON to be detected as incomplete provider output, got %#v", turn.ToolCalls[0])
	}
	var summary ProviderRequestSummary
	summary.StreamToolArgsState = "incomplete_json"
	annotateStreamToolDiagnostics(&summary, turn)
	if summary.StreamFinishReason != "max_tokens" {
		t.Fatalf("expected max_tokens finish reason in diagnostics, got %q", summary.StreamFinishReason)
	}
	if len(summary.StreamToolCalls) != 1 {
		t.Fatalf("expected one stream tool call diagnostic, got %#v", summary.StreamToolCalls)
	}
	item := summary.StreamToolCalls[0]
	if item.Name != "save_workspace_file" || item.ID != "toolu_1" {
		t.Fatalf("unexpected stream tool call diagnostic: %#v", item)
	}
	if item.TargetSummary != "机甲少女/机甲少女11-22集修改版.txt" {
		t.Fatalf("expected partial path extraction, got %#v", item)
	}
	if item.ArgumentsJSONValid || item.ArgumentsJSONError == "" {
		t.Fatalf("expected invalid JSON details, got %#v", item)
	}
	if !item.LooksLikeLongWrite || item.RecommendedTool != "apply_workspace_patch" {
		t.Fatalf("expected long-write recommendation, got %#v", item)
	}
}

func TestStreamToolDiagnosticsRecommendLineReplaceForLargeEdits(t *testing.T) {
	args := `{"path":"script.txt","old_text":"` + strings.Repeat("旧", 2500) + `","new_text":"` + strings.Repeat("新", 2500)
	var summary ProviderRequestSummary
	annotateStreamToolDiagnostics(&summary, agentModelTurn{
		ToolCalls: []agentToolCall{{
			ID:            "toolu_edit",
			Name:          "edit_workspace_text",
			ArgumentsJSON: args,
		}},
	})
	if len(summary.StreamToolCalls) != 1 {
		t.Fatalf("expected one stream tool call diagnostic, got %#v", summary.StreamToolCalls)
	}
	item := summary.StreamToolCalls[0]
	if !item.LooksLikeLargeEdit || item.RecommendedTool != "apply_workspace_patch" {
		t.Fatalf("expected line replacement recommendation, got %#v", item)
	}
	if item.OldTextChars == 0 || item.NewTextChars == 0 {
		t.Fatalf("expected old/new text sizes, got %#v", item)
	}
}

func TestParseAnthropicStreamRevealsTextAfterToolWhenRequired(t *testing.T) {
	stream := strings.Join([]string{
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":"我先检查文件。"}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"read_workspace_text","input":{}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"a.txt\"}"}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")
	var deltas []string
	turn, err := parseAnthropicStreamWithOptions(strings.NewReader(stream), providerStreamParseOptions{
		EmitText: func(text string) {
			deltas = append(deltas, text)
		},
		RevealTextAfterTool: true,
	})
	if err != nil {
		t.Fatalf("parseAnthropicStream returned error: %v", err)
	}
	if turn.Content != "我先检查文件。" {
		t.Fatalf("unexpected content: %q", turn.Content)
	}
	if strings.Join(deltas, "") != "我先检查文件。" {
		t.Fatalf("expected gated text to flush after tool use, got %#v", deltas)
	}
	if len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Name != "read_workspace_text" {
		t.Fatalf("unexpected tool calls: %#v", turn.ToolCalls)
	}
}

func TestParseAnthropicStreamSuppressesEllipsisToolPreamble(t *testing.T) {
	stream := strings.Join([]string{
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":"..."}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"read_workspace_text","input":{}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"a.txt\"}"}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")
	var deltas []string
	turn, err := parseAnthropicStreamWithOptions(strings.NewReader(stream), providerStreamParseOptions{
		EmitText: func(text string) {
			deltas = append(deltas, text)
		},
		RevealTextAfterTool: true,
	})
	if err != nil {
		t.Fatalf("parseAnthropicStream returned error: %v", err)
	}
	if turn.Content != "..." {
		t.Fatalf("stream parser should preserve provider content for runtime cleanup, got %q", turn.Content)
	}
	if len(deltas) != 0 {
		t.Fatalf("ellipsis preamble should not be emitted to user, got %#v", deltas)
	}
	prepared, _ := prepareToolTurnForExecution(turn)
	if prepared.Content != "" || len(prepared.AnthropicRaw) != 1 {
		t.Fatalf("ellipsis preamble should be removed before transcript/commit, got content=%q raw=%#v", prepared.Content, prepared.AnthropicRaw)
	}
}

func TestParseAnthropicStreamDoesNotRevealRequiredTextWithoutTool(t *testing.T) {
	stream := strings.Join([]string{
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":"我稍后会处理。"}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")
	var deltas []string
	turn, err := parseAnthropicStreamWithOptions(strings.NewReader(stream), providerStreamParseOptions{
		EmitText: func(text string) {
			deltas = append(deltas, text)
		},
		RevealTextAfterTool: true,
	})
	if err != nil {
		t.Fatalf("parseAnthropicStream returned error: %v", err)
	}
	if turn.Content != "我稍后会处理。" {
		t.Fatalf("unexpected content: %q", turn.Content)
	}
	if len(deltas) != 0 {
		t.Fatalf("required no-tool text should stay hidden from the user, got %#v", deltas)
	}
}

func TestParseAnthropicStreamRemovesLiteralReasoningTextDeltas(t *testing.T) {
	stream := strings.Join([]string{
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":"<think>"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hidden"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"</think>\n\n公开结果"}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")
	var deltas []string
	turn, err := parseAnthropicStream(strings.NewReader(stream), func(text string) {
		deltas = append(deltas, text)
	})
	if err != nil {
		t.Fatalf("parseAnthropicStream returned error: %v", err)
	}
	if turn.Content != "公开结果" {
		t.Fatalf("unexpected visible content: %q", turn.Content)
	}
	if strings.Contains(strings.Join(deltas, ""), "hidden") {
		t.Fatalf("reasoning leaked into deltas: %#v", deltas)
	}
	if len(turn.AnthropicRaw) != 1 || turn.AnthropicRaw[0]["text"] != "公开结果" {
		t.Fatalf("unexpected anthropic raw blocks: %#v", turn.AnthropicRaw)
	}
}

func TestSanitizeAnthropicVisibleTextBlocksHandlesReasoningAcrossBlocks(t *testing.T) {
	blocks := []map[string]any{
		{"type": "text", "text": "before <thinking>hidden"},
		{"type": "text", "text": " still hidden</thinking> after"},
		{"type": "tool_use", "id": "toolu_1", "name": "read_workspace_text", "input": map[string]any{"path": "a.txt"}},
	}
	got := sanitizeAnthropicVisibleTextBlocks(blocks)
	if len(got) != 3 {
		t.Fatalf("unexpected block count: %#v", got)
	}
	if got[0]["text"] != "before " || got[1]["text"] != " after" {
		t.Fatalf("unexpected sanitized text blocks: %#v", got)
	}
	if got[2]["type"] != "tool_use" {
		t.Fatalf("tool block was not preserved: %#v", got)
	}
}

func TestParseAnthropicStreamReturnsProviderErrorEvent(t *testing.T) {
	stream := strings.Join([]string{
		`event: error`,
		`data: {"type":"error","error":{"type":"new_api_error","message":"No available channel for model demo-thinking"}}`,
		``,
	}, "\n")
	_, err := parseAnthropicStream(strings.NewReader(stream), nil)
	if err == nil {
		t.Fatal("expected provider stream error")
	}
	if !strings.Contains(err.Error(), "No available channel") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseAnthropicStreamIgnoresThinkingDeltasForVisibleText(t *testing.T) {
	stream := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"usage":{"input_tokens":10,"output_tokens":0}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"internal notes"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig"}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":8}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")
	turn, err := parseAnthropicStream(strings.NewReader(stream), nil)
	if err != nil {
		t.Fatalf("parseAnthropicStream returned error: %v", err)
	}
	if turn.Content != "" || len(turn.ToolCalls) != 0 {
		t.Fatalf("expected no visible content/tool calls, got %#v", turn)
	}
	if len(turn.AnthropicRaw) != 1 || turn.AnthropicRaw[0]["type"] != "thinking" || turn.AnthropicRaw[0]["signature"] != "sig" {
		t.Fatalf("expected thinking block preserved for Anthropic transcript, got %#v", turn.AnthropicRaw)
	}
	if turn.FinishReason != "end_turn" {
		t.Fatalf("unexpected finish reason: %q", turn.FinishReason)
	}
	if turn.Usage.InputTokens != 10 || turn.Usage.OutputTokens != 8 {
		t.Fatalf("unexpected usage: %#v", turn.Usage)
	}
}

func TestParseAnthropicStreamPreservesRedactedThinkingBlock(t *testing.T) {
	stream := strings.Join([]string{
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"redacted_thinking","data":"opaque"}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")
	turn, err := parseAnthropicStream(strings.NewReader(stream), nil)
	if err != nil {
		t.Fatalf("parseAnthropicStream returned error: %v", err)
	}
	if len(turn.AnthropicRaw) != 1 || turn.AnthropicRaw[0]["type"] != "redacted_thinking" || turn.AnthropicRaw[0]["data"] != "opaque" {
		t.Fatalf("expected redacted thinking block preserved, got %#v", turn.AnthropicRaw)
	}
}

type errAfterReader struct {
	data string
	err  error
	done bool
}

func (r *errAfterReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, r.err
	}
	if r.data == "" {
		r.done = true
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}
