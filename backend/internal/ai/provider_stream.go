package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

type sseEvent struct {
	Event string
	Data  string
}

func readSSEEvents(body io.Reader, handle func(sseEvent) error) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var eventName string
	var dataLines []string
	flush := func() error {
		if len(dataLines) == 0 {
			eventName = ""
			return nil
		}
		data := strings.Join(dataLines, "\n")
		dataLines = nil
		event := sseEvent{Event: eventName, Data: data}
		eventName = ""
		return handle(event)
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			eventName = value
		case "data":
			dataLines = append(dataLines, value)
		}
	}
	if err := scanner.Err(); err != nil {
		if flushErr := flush(); flushErr != nil {
			return flushErr
		}
		return err
	}
	return flush()
}

type openAIStreamChunk struct {
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
	ID        string
	Type      string
	Name      string
	Arguments strings.Builder
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

func newStreamDeltaEmitter(emit func(string)) *streamDeltaEmitter {
	return &streamDeltaEmitter{
		emit:        emit,
		lastFlush:   time.Now(),
		flushEvery:  150 * time.Millisecond,
		flushAtSize: 512,
	}
}

func (e *streamDeltaEmitter) Add(text string) {
	if e == nil || text == "" {
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
	var content strings.Builder
	deltas := newStreamDeltaEmitter(emitText)
	toolBuilders := map[int]*openAIStreamToolBuilder{}
	finishReason := ""
	err := readSSEEvents(body, func(event sseEvent) error {
		data := strings.TrimSpace(event.Data)
		if data == "" || data == "[DONE]" {
			return nil
		}
		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return fmt.Errorf("invalid OpenAI stream chunk: %w", err)
		}
		for _, choice := range chunk.Choices {
			if choice.FinishReason != "" {
				finishReason = choice.FinishReason
			}
			if text := normalizeOpenAIContent(choice.Delta.Content); text != "" {
				content.WriteString(text)
				deltas.Add(text)
			}
			for _, item := range choice.Delta.ToolCalls {
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
			}
		}
		return nil
	})
	deltas.Flush()
	if err != nil {
		turn := buildOpenAIStreamTurn(content.String(), finishReason, toolBuilders)
		if hasCompleteToolCalls(turn) {
			return agentModelTurn{}, &partialStreamTurnError{turn: turn, err: err}
		}
		return agentModelTurn{}, err
	}
	return buildOpenAIStreamTurn(content.String(), finishReason, toolBuilders), nil
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
			if args == "" {
				args = "{}"
			}
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

type anthropicStreamBlock struct {
	ID          string
	Type        string
	Name        string
	Text        strings.Builder
	PartialJSON strings.Builder
}

func parseAnthropicStream(body io.Reader, emitText func(string)) (agentModelTurn, error) {
	var content strings.Builder
	deltas := newStreamDeltaEmitter(emitText)
	blocks := map[int]*anthropicStreamBlock{}
	finishReason := ""
	err := readSSEEvents(body, func(event sseEvent) error {
		data := strings.TrimSpace(event.Data)
		if data == "" || data == "[DONE]" {
			return nil
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(data), &raw); err != nil {
			return fmt.Errorf("invalid Anthropic stream chunk: %w", err)
		}
		eventType, _ := raw["type"].(string)
		switch eventType {
		case "content_block_start":
			index := intFromAny(raw["index"])
			blockRaw, _ := raw["content_block"].(map[string]any)
			block := &anthropicStreamBlock{}
			block.Type, _ = blockRaw["type"].(string)
			block.ID, _ = blockRaw["id"].(string)
			block.Name, _ = blockRaw["name"].(string)
			if text, _ := blockRaw["text"].(string); text != "" {
				block.Text.WriteString(text)
				content.WriteString(text)
				deltas.Add(text)
			}
			blocks[index] = block
		case "content_block_delta":
			index := intFromAny(raw["index"])
			block := blocks[index]
			if block == nil {
				block = &anthropicStreamBlock{}
				blocks[index] = block
			}
			delta, _ := raw["delta"].(map[string]any)
			deltaType, _ := delta["type"].(string)
			switch deltaType {
			case "text_delta":
				text, _ := delta["text"].(string)
				if text != "" {
					block.Text.WriteString(text)
					content.WriteString(text)
					deltas.Add(text)
				}
			case "input_json_delta":
				partial, _ := delta["partial_json"].(string)
				block.PartialJSON.WriteString(partial)
			}
		case "message_delta":
			delta, _ := raw["delta"].(map[string]any)
			finishReason, _ = delta["stop_reason"].(string)
		case "message_stop":
			if finishReason == "" {
				finishReason = "stop"
			}
		}
		return nil
	})
	deltas.Flush()
	if err != nil {
		turn := buildAnthropicStreamTurn(content.String(), finishReason, blocks)
		if hasCompleteToolCalls(turn) {
			return agentModelTurn{}, &partialStreamTurnError{turn: turn, err: err}
		}
		return agentModelTurn{}, err
	}
	return buildAnthropicStreamTurn(content.String(), finishReason, blocks), nil
}

func buildAnthropicStreamTurn(content string, finishReason string, blocks map[int]*anthropicStreamBlock) agentModelTurn {
	turn := agentModelTurn{Content: content, FinishReason: finishReason, Streamed: true}
	for index := 0; index < len(blocks); index++ {
		block := blocks[index]
		if block == nil {
			continue
		}
		switch block.Type {
		case "text":
			text := block.Text.String()
			if text != "" {
				turn.AnthropicRaw = append(turn.AnthropicRaw, map[string]any{"type": "text", "text": text})
			}
		case "tool_use":
			args := strings.TrimSpace(block.PartialJSON.String())
			if args == "" {
				args = "{}"
			}
			var input map[string]any
			_ = json.Unmarshal([]byte(args), &input)
			if input == nil {
				input = map[string]any{}
			}
			turn.AnthropicRaw = append(turn.AnthropicRaw, map[string]any{"type": "tool_use", "id": block.ID, "name": block.Name, "input": input})
			turn.ToolCalls = append(turn.ToolCalls, agentToolCall{ID: block.ID, Name: block.Name, ArgumentsJSON: args, AnthropicInput: input})
		}
	}
	return turn
}

func hasCompleteToolCalls(turn agentModelTurn) bool {
	for _, call := range turn.ToolCalls {
		if strings.TrimSpace(call.Name) == "" || strings.TrimSpace(call.ArgumentsJSON) == "" {
			continue
		}
		var decoded any
		if err := json.Unmarshal([]byte(call.ArgumentsJSON), &decoded); err == nil {
			return true
		}
	}
	return false
}

func intFromAny(value any) int {
	switch item := value.(type) {
	case float64:
		return int(item)
	case int:
		return item
	default:
		return 0
	}
}

func shouldFallbackAfterStreamError(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	return true
}

func clonePayloadWithStream(payload map[string]any, stream bool) map[string]any {
	next := make(map[string]any, len(payload))
	for key, value := range payload {
		next[key] = value
	}
	next["stream"] = stream
	return next
}
