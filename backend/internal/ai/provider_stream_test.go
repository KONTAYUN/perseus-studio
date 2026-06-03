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

func TestProviderPatchFilteredEmitterHidesPatchBlockOnly(t *testing.T) {
	var deltas []string
	emitter := newProviderPatchFilteredEmitter(newToolGatedStreamDeltaEmitter(providerStreamParseOptions{
		EmitText: func(text string) {
			deltas = append(deltas, text)
		},
	}))

	for _, piece := range []string{
		"我会创建文件。\n",
		"*** Begin",
		" Patch\n*** Add File: draft.md\n+secret\n*** End Patch",
		"\n已经处理。",
	} {
		emitter.Add(piece)
	}
	emitter.Flush()

	if got := strings.Join(deltas, ""); got != "我会创建文件。\n\n已经处理。" {
		t.Fatalf("unexpected streamed text: %q deltas=%#v", got, deltas)
	}
	if strings.Contains(emitter.StreamedContent(), "*** Begin Patch") || strings.Contains(emitter.StreamedContent(), "secret") {
		t.Fatalf("patch content leaked through filter: %q", emitter.StreamedContent())
	}
}

func TestProviderPatchFilteredEmitterSuppressesPatchOnlyWhitespace(t *testing.T) {
	var deltas []string
	emitter := newProviderPatchFilteredEmitter(newToolGatedStreamDeltaEmitter(providerStreamParseOptions{
		EmitText: func(text string) {
			deltas = append(deltas, text)
		},
	}))

	for _, piece := range []string{
		"\n",
		"*** Begin Patch\n*** Add File: draft.md\n+secret\n",
		"*** End Patch",
	} {
		emitter.Add(piece)
	}
	emitter.Flush()

	if got := strings.Join(deltas, ""); got != "" {
		t.Fatalf("patch-only whitespace should not stream, got %q deltas=%#v", got, deltas)
	}
	if got := emitter.StreamedContent(); got != "" {
		t.Fatalf("patch-only whitespace should not be recorded as public content, got %q", got)
	}
}

func TestProviderPatchFilteredEmitterSuppressesPatchFenceOnlyWrapper(t *testing.T) {
	var deltas []string
	emitter := newProviderPatchFilteredEmitter(newToolGatedStreamDeltaEmitter(providerStreamParseOptions{
		EmitText: func(text string) {
			deltas = append(deltas, text)
		},
	}))

	for _, piece := range []string{
		"```patch\n",
		"*** Begin Patch\n*** Add File: draft.md\n+secret\n*** End Patch",
		"\n```",
	} {
		emitter.Add(piece)
	}
	emitter.Flush()

	if got := strings.Join(deltas, ""); got != "" {
		t.Fatalf("fenced patch-only wrapper should not stream, got %q deltas=%#v", got, deltas)
	}
}

func TestProviderPatchFilteredEmitterStreamsRealTextAroundFencedPatch(t *testing.T) {
	var deltas []string
	emitter := newProviderPatchFilteredEmitter(newToolGatedStreamDeltaEmitter(providerStreamParseOptions{
		EmitText: func(text string) {
			deltas = append(deltas, text)
		},
	}))

	for _, piece := range []string{
		"我会创建文件。\n\n```patch\n*** Begin",
		" Patch\n*** Add File: draft.md\n+secret\n*** End Patch\n",
		"```\n已经处理。",
	} {
		emitter.Add(piece)
	}
	emitter.Flush()

	if got := strings.Join(deltas, ""); got != "我会创建文件。\n已经处理。" {
		t.Fatalf("unexpected streamed text: %q deltas=%#v", got, deltas)
	}
}

func TestProviderPatchFilteredEmitterSuppressesRepeatedEndPatchTail(t *testing.T) {
	var deltas []string
	emitter := newProviderPatchFilteredEmitter(newToolGatedStreamDeltaEmitter(providerStreamParseOptions{
		EmitText: func(text string) {
			deltas = append(deltas, text)
		},
	}))

	for _, piece := range []string{
		"*** Begin Patch\n*** Add File: draft.md\n+secret\n",
		"*** End Patch",
		"\n*** End Patch\n我会核对文件是否落盘。",
	} {
		emitter.Add(piece)
	}
	emitter.Flush()

	got := strings.Join(deltas, "")
	if got != "我会核对文件是否落盘。" {
		t.Fatalf("unexpected streamed text: %q deltas=%#v", got, deltas)
	}
	if strings.Contains(got, "*** End Patch") || strings.Contains(got, "secret") {
		t.Fatalf("patch marker leaked through filter: %q", got)
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

func TestParseOpenAIStreamStreamsAttemptCompletionResultFromToolArguments(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"done_1","type":"function","function":{"name":"attempt_completion","arguments":"{\"result\":\"第一段"}}]}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"继续，第二段\\n第三段\",\"summary\":\"完成\"}"}}]},"finish_reason":"tool_calls"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	var deltas []string
	turn, err := parseOpenAIStreamWithOptions(strings.NewReader(stream), providerStreamParseOptions{
		EmitCompletionResultDelta: func(text string) {
			deltas = append(deltas, text)
		},
	})
	if err != nil {
		t.Fatalf("parseOpenAIStream returned error: %v", err)
	}
	if !turn.CompletionResultStreamed {
		t.Fatalf("expected completion result streamed")
	}
	if strings.Join(deltas, "") != "第一段继续，第二段\n第三段" {
		t.Fatalf("unexpected completion deltas: %#v", deltas)
	}
	if len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Name != "attempt_completion" {
		t.Fatalf("unexpected tool calls: %#v", turn.ToolCalls)
	}
}

func TestParseOpenAIStreamDoesNotPreviewInternalCompletionProtocol(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"done_1","type":"function","function":{"name":"attempt_completion","arguments":"{\"result\":\"需要使用 *** Begin Patch"}}]}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":" / *** End Patch 修改。\"}"}}]},"finish_reason":"tool_calls"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	var deltas []string
	turn, err := parseOpenAIStreamWithOptions(strings.NewReader(stream), providerStreamParseOptions{
		EmitCompletionResultDelta: func(text string) {
			deltas = append(deltas, text)
		},
	})
	if err != nil {
		t.Fatalf("parseOpenAIStream returned error: %v", err)
	}
	if turn.CompletionResultStreamed || len(deltas) != 0 {
		t.Fatalf("internal completion protocol should not stream, streamed=%v deltas=%#v", turn.CompletionResultStreamed, deltas)
	}
}

func TestParseOpenAIStreamKeepsCompletionResultSeparateFromAssistantText(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"我准备完成"}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"done_1","type":"function","function":{"name":"attempt_completion","arguments":"{\"result\":\"最终答复"}}]}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"。"}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"内容\"}"}}]},"finish_reason":"tool_calls"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	var textDeltas []string
	var completionDeltas []string
	turn, err := parseOpenAIStreamWithOptions(strings.NewReader(stream), providerStreamParseOptions{
		EmitText: func(text string) {
			textDeltas = append(textDeltas, text)
		},
		EmitCompletionResultDelta: func(text string) {
			completionDeltas = append(completionDeltas, text)
		},
		RevealTextAfterTool: true,
	})
	if err != nil {
		t.Fatalf("parseOpenAIStream returned error: %v", err)
	}
	if strings.Join(textDeltas, "") != "我准备完成。" {
		t.Fatalf("assistant text stream should stay separate, got %#v", textDeltas)
	}
	if strings.Join(completionDeltas, "") != "最终答复内容" {
		t.Fatalf("completion stream should contain only attempt_completion.result, got %#v", completionDeltas)
	}
	if !turn.CompletionResultStreamed {
		t.Fatal("expected completion result streamed")
	}
}

func TestEmitStreamedCompletionResultFromToolCallsSplitsLargeFallback(t *testing.T) {
	result := strings.Repeat("你", 600)
	var deltas []string
	streamed := emitStreamedCompletionResultFromToolCalls([]agentToolCall{{
		Name:          "attempt_completion",
		ArgumentsJSON: `{"result":"` + result + `"}`,
	}}, func(text string) {
		deltas = append(deltas, text)
	})
	if !streamed {
		t.Fatalf("expected fallback completion result to stream")
	}
	if len(deltas) != 3 {
		t.Fatalf("expected large fallback to split into 3 chunks, got %d: %#v", len(deltas), deltas)
	}
	if strings.Join(deltas, "") != result {
		t.Fatalf("fallback chunks did not preserve content")
	}
}

func TestParseOpenAIStreamDoesNotLeakNonCompletionToolArguments(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"read_1","type":"function","function":{"name":"read_workspace_text","arguments":"{\"result\":\"not public\",\"path\":\"a.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	var deltas []string
	turn, err := parseOpenAIStreamWithOptions(strings.NewReader(stream), providerStreamParseOptions{
		EmitCompletionResultDelta: func(text string) {
			deltas = append(deltas, text)
		},
	})
	if err != nil {
		t.Fatalf("parseOpenAIStream returned error: %v", err)
	}
	if turn.CompletionResultStreamed || len(deltas) != 0 {
		t.Fatalf("non-completion tool arguments leaked: streamed=%v deltas=%#v", turn.CompletionResultStreamed, deltas)
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

func TestParseAnthropicStreamPreservesEmptyToolArgumentsAsInvalid(t *testing.T) {
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
	if turn.ToolCalls[0].ArgumentsJSON != "" {
		t.Fatalf("empty streamed arguments must stay empty for validation, got %q", turn.ToolCalls[0].ArgumentsJSON)
	}
	if !hasIncompleteToolCallArguments(turn) {
		t.Fatalf("expected empty tool arguments to be treated as incomplete provider output")
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
	if !item.LooksLikeLongWrite || item.RecommendedTool != "assistant_text_patch" {
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
	if !item.LooksLikeLargeEdit || item.RecommendedTool != "assistant_text_patch" {
		t.Fatalf("expected line replacement recommendation, got %#v", item)
	}
	if item.OldTextChars == 0 || item.NewTextChars == 0 {
		t.Fatalf("expected old/new text sizes, got %#v", item)
	}
}

func TestParseAnthropicStreamStreamsAttemptCompletionResultFromToolArguments(t *testing.T) {
	stream := strings.Join([]string{
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_done","name":"attempt_completion","input":{}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"result\":\"第一段"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"继续，第二段\",\"summary\":\"完成\"}"}}`,
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
		EmitCompletionResultDelta: func(text string) {
			deltas = append(deltas, text)
		},
	})
	if err != nil {
		t.Fatalf("parseAnthropicStream returned error: %v", err)
	}
	if !turn.CompletionResultStreamed {
		t.Fatalf("expected completion result streamed")
	}
	if strings.Join(deltas, "") != "第一段继续，第二段" {
		t.Fatalf("unexpected completion deltas: %#v", deltas)
	}
	if len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Name != "attempt_completion" {
		t.Fatalf("unexpected tool calls: %#v", turn.ToolCalls)
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
