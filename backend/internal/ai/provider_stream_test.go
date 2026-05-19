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
	if turn.FinishReason != "end_turn" {
		t.Fatalf("unexpected finish reason: %q", turn.FinishReason)
	}
	if turn.Usage.InputTokens != 10 || turn.Usage.OutputTokens != 8 {
		t.Fatalf("unexpected usage: %#v", turn.Usage)
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
