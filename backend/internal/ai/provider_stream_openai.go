package ai

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

type openAIStreamChunk struct {
	Usage   map[string]any `json:"usage"`
	Choices []struct {
		Delta struct {
			Content   any `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

type openAIStreamToolBuilder struct {
	ID               string
	Type             string
	Name             string
	Arguments        strings.Builder
	CompletionResult partialJSONFieldStreamer
}

type partialStreamTurnError struct {
	turn agentModelTurn
	err  error
}

func (e *partialStreamTurnError) Error() string {
	if e == nil || e.err == nil {
		return "stream ended after a partial provider turn"
	}
	return e.err.Error()
}

func (e *partialStreamTurnError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func partialTurnFromStreamError(err error) (agentModelTurn, bool) {
	if item, ok := err.(*partialStreamTurnError); ok && hasCompleteToolCalls(item.turn) {
		return item.turn, true
	}
	return agentModelTurn{}, false
}

type streamDeltaEmitter struct {
	emit        func(string)
	buffer      strings.Builder
	lastFlush   time.Time
	flushEvery  time.Duration
	flushAtSize int
	chunks      int
	bytes       int
}

type providerStreamParseOptions struct {
	EmitText                  func(string)
	EmitCompletionResultDelta func(string)
	RevealTextAfterTool       bool
}

type toolGatedStreamDeltaEmitter struct {
	inner           *streamDeltaEmitter
	revealAfterTool bool
	toolObserved    bool
	buffer          strings.Builder
}

func newStreamDeltaEmitter(emit func(string)) *streamDeltaEmitter {
	return &streamDeltaEmitter{
		emit:        emit,
		lastFlush:   time.Now(),
		flushEvery:  150 * time.Millisecond,
		flushAtSize: 512,
	}
}

func (e *streamDeltaEmitter) Add(text string) {
	if e == nil || e.emit == nil || text == "" {
		return
	}
	e.chunks++
	e.bytes += len(text)
	e.buffer.WriteString(text)
	if e.buffer.Len() >= e.flushAtSize || time.Since(e.lastFlush) >= e.flushEvery {
		e.Flush()
	}
}

func (e *streamDeltaEmitter) Flush() {
	if e == nil || e.emit == nil || e.buffer.Len() == 0 {
		return
	}
	text := e.buffer.String()
	e.buffer.Reset()
	e.lastFlush = time.Now()
	e.emit(text)
}

func parseOpenAIStream(body io.Reader, emitText func(string)) (agentModelTurn, error) {
	return parseOpenAIStreamWithOptions(body, providerStreamParseOptions{EmitText: emitText})
}

func newToolGatedStreamDeltaEmitter(opts providerStreamParseOptions) *toolGatedStreamDeltaEmitter {
	return &toolGatedStreamDeltaEmitter{
		inner:           newStreamDeltaEmitter(opts.EmitText),
		revealAfterTool: opts.RevealTextAfterTool,
	}
}

func (e *toolGatedStreamDeltaEmitter) Add(text string) {
	if e == nil || text == "" {
		return
	}
	if !e.revealAfterTool || e.toolObserved {
		e.inner.Add(text)
		return
	}
	e.buffer.WriteString(text)
}

func (e *toolGatedStreamDeltaEmitter) MarkToolObserved() {
	if e == nil || e.toolObserved {
		return
	}
	e.toolObserved = true
	if e.buffer.Len() == 0 {
		return
	}
	text := e.buffer.String()
	e.buffer.Reset()
	if isToolPreamblePlaceholder(text) {
		return
	}
	e.inner.Add(text)
	e.inner.Flush()
}

func (e *toolGatedStreamDeltaEmitter) Flush() {
	if e == nil {
		return
	}
	if !e.revealAfterTool || e.toolObserved {
		e.inner.Flush()
		return
	}
	e.buffer.Reset()
}

func parseOpenAIStreamWithOptions(body io.Reader, opts providerStreamParseOptions) (agentModelTurn, error) {
	var content strings.Builder
	deltas := newProviderPatchFilteredEmitter(newToolGatedStreamDeltaEmitter(opts))
	completionDeltas := newStreamDeltaEmitter(opts.EmitCompletionResultDelta)
	visibleText := newProviderVisibleTextFilter()
	toolBuilders := map[int]*openAIStreamToolBuilder{}
	finishReason := ""
	usage := ProviderUsage{}
	completionResultStreamed := false
	emitCompletionResult := func(text string) {
		if text == "" || opts.EmitCompletionResultDelta == nil {
			return
		}
		completionResultStreamed = true
		completionDeltas.Add(text)
	}
	err := readSSEEvents(body, func(event sseEvent) error {
		data := strings.TrimSpace(event.Data)
		if data == "" || data == "[DONE]" {
			return nil
		}
		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return fmt.Errorf("invalid OpenAI stream chunk: %w", err)
		}
		updateProviderUsageFromAny(&usage, chunk.Usage)
		for _, choice := range chunk.Choices {
			if choice.FinishReason != "" {
				finishReason = choice.FinishReason
			}
			if text := normalizeOpenAIContent(choice.Delta.Content); text != "" {
				appendProviderVisibleText(&content, deltas, visibleText, text)
			}
			for _, item := range choice.Delta.ToolCalls {
				deltas.MarkToolObserved()
				builder := toolBuilders[item.Index]
				if builder == nil {
					builder = &openAIStreamToolBuilder{}
					toolBuilders[item.Index] = builder
				}
				if item.ID != "" {
					builder.ID = item.ID
				}
				if item.Type != "" {
					builder.Type = item.Type
				}
				if item.Function.Name != "" {
					builder.Name += item.Function.Name
				}
				if item.Function.Arguments != "" {
					builder.Arguments.WriteString(item.Function.Arguments)
				}
				// Cline streams attempt_completion.result while the tool block is
				// still partial; do the same for native tool-call arguments.
				builder.CompletionResult.EmitNewDelta(builder.Name, builder.Arguments.String(), emitCompletionResult)
			}
		}
		return nil
	})
	closeProviderVisibleText(&content, deltas, visibleText)
	deltas.Flush()
	completionDeltas.Flush()
	if err != nil {
		turn := buildOpenAIStreamTurn(content.String(), finishReason, toolBuilders)
		turn.PublicStreamedContent = deltas.StreamedContent()
		turn.CompletionResultStreamed = completionResultStreamed
		turn.Usage = usage
		if hasCompleteToolCalls(turn) {
			return agentModelTurn{}, &partialStreamTurnError{turn: turn, err: err}
		}
		return agentModelTurn{}, err
	}
	turn := buildOpenAIStreamTurn(content.String(), finishReason, toolBuilders)
	turn.PublicStreamedContent = deltas.StreamedContent()
	turn.CompletionResultStreamed = completionResultStreamed
	turn.Usage = usage
	return turn, nil
}

func buildOpenAIStreamTurn(content string, finishReason string, toolBuilders map[int]*openAIStreamToolBuilder) agentModelTurn {
	turn := agentModelTurn{Content: content, FinishReason: finishReason, Streamed: true}
	if len(toolBuilders) > 0 {
		for index := 0; index < len(toolBuilders); index++ {
			builder := toolBuilders[index]
			if builder == nil {
				continue
			}
			id := builder.ID
			if id == "" {
				id = "stream_tool_" + strconv.Itoa(index)
			}
			args := strings.TrimSpace(builder.Arguments.String())
			turn.ToolCalls = append(turn.ToolCalls, agentToolCall{
				ID:            id,
				Name:          builder.Name,
				ArgumentsJSON: args,
			})
			turn.OpenAIMessage.ToolCalls = append(turn.OpenAIMessage.ToolCalls, openAIToolCall{
				ID:   id,
				Type: "function",
				Function: openAIFunctionCall{
					Name:      builder.Name,
					Arguments: json.RawMessage(strconv.Quote(args)),
				},
			})
		}
	}
	turn.OpenAIMessage.Role = "assistant"
	if turn.Content != "" {
		turn.OpenAIMessage.Content = turn.Content
	}
	return turn
}
