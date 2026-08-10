package ai

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type anthropicStreamBlock struct {
	ID          string
	Type        string
	Name        string
	Input       map[string]any
	Text        strings.Builder
	Thinking    strings.Builder
	Signature   string
	Data        string
	PartialJSON strings.Builder
}

func parseAnthropicStream(body io.Reader, emitText func(string)) (agentModelTurn, error) {
	return parseAnthropicStreamWithOptions(body, providerStreamParseOptions{EmitText: emitText})
}

func parseAnthropicStreamWithOptions(body io.Reader, opts providerStreamParseOptions) (agentModelTurn, error) {
	var content strings.Builder
	deltas := newToolGatedStreamDeltaEmitter(opts)
	visibleText := newProviderVisibleTextFilter()
	blocks := map[int]*anthropicStreamBlock{}
	finishReason := ""
	usage := ProviderUsage{}
	providerResponseID := ""
	lastEventID := ""
	duplicateEvents := 0
	seenEventIDs := map[string]string{}
	err := readSSEEvents(body, func(event sseEvent) error {
		if event.ID != "" {
			if previous, exists := seenEventIDs[event.ID]; exists && previous == event.Data {
				duplicateEvents++
				return nil
			}
			seenEventIDs[event.ID] = event.Data
			lastEventID = event.ID
		}
		data := strings.TrimSpace(event.Data)
		if data == "" || data == "[DONE]" {
			return nil
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(data), &raw); err != nil {
			return fmt.Errorf("invalid Anthropic stream chunk: %w", err)
		}
		eventType, _ := raw["type"].(string)
		if eventType == "" {
			eventType = event.Event
		}
		switch eventType {
		case "error":
			if rawError, ok := raw["error"]; ok {
				return formatProviderError(0, rawError)
			}
			return fmt.Errorf("provider stream error: %s", providerErrorMessage(raw))
		case "message_start":
			message, _ := raw["message"].(map[string]any)
			if message != nil {
				providerResponseID, _ = message["id"].(string)
				updateProviderUsageFromAny(&usage, message["usage"])
			}
		case "content_block_start":
			index := intFromAny(raw["index"])
			blockRaw, _ := raw["content_block"].(map[string]any)
			block := &anthropicStreamBlock{}
			block.Type, _ = blockRaw["type"].(string)
			block.ID, _ = blockRaw["id"].(string)
			block.Name, _ = blockRaw["name"].(string)
			if input, _ := blockRaw["input"].(map[string]any); input != nil {
				block.Input = cloneMapAny(input)
			}
			block.Signature, _ = blockRaw["signature"].(string)
			block.Data, _ = blockRaw["data"].(string)
			if block.Type == "tool_use" {
				deltas.MarkToolObserved()
				if opts.EmitToolArguments != nil {
					opts.EmitToolArguments(ToolCallArgumentsDelta{
						Index:         index,
						ToolCallID:    block.ID,
						ToolName:      block.Name,
						ArgumentsJSON: partialAnthropicToolArguments(block),
					})
				}
			}
			if text, _ := blockRaw["text"].(string); text != "" {
				visible := appendProviderVisibleText(&content, deltas, visibleText, text)
				block.Text.WriteString(visible)
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
					visible := appendProviderVisibleText(&content, deltas, visibleText, text)
					block.Text.WriteString(visible)
				}
			case "input_json_delta":
				partial, _ := delta["partial_json"].(string)
				block.PartialJSON.WriteString(partial)
				if opts.EmitToolArguments != nil {
					opts.EmitToolArguments(ToolCallArgumentsDelta{
						Index:         index,
						ToolCallID:    block.ID,
						ToolName:      block.Name,
						ArgumentsJSON: partialAnthropicToolArguments(block),
					})
				}
			case "thinking_delta":
				thinking, _ := delta["thinking"].(string)
				if thinking != "" {
					block.Thinking.WriteString(thinking)
				}
			case "signature_delta":
				signature, _ := delta["signature"].(string)
				if signature != "" {
					block.Signature = signature
				}
			case "redacted_thinking_delta":
				data, _ := delta["data"].(string)
				if data != "" {
					block.Data = data
				}
			}
		case "message_delta":
			delta, _ := raw["delta"].(map[string]any)
			finishReason, _ = delta["stop_reason"].(string)
			updateProviderUsageFromAny(&usage, raw["usage"])
		case "message_stop":
			if finishReason == "" {
				finishReason = "stop"
			}
		}
		return nil
	})
	closeProviderVisibleText(&content, deltas, visibleText)
	deltas.Flush()
	if err != nil {
		turn := buildAnthropicStreamTurn(content.String(), finishReason, blocks)
		turn.PublicStreamedContent = deltas.StreamedContent()
		turn.Usage = usage
		turn.ProviderResponseID = providerResponseID
		turn.StreamLastEventID = lastEventID
		turn.StreamDuplicateEvents = duplicateEvents
		if hasCompleteToolCalls(turn) {
			return agentModelTurn{}, &partialStreamTurnError{turn: turn, err: err}
		}
		if providerStreamTurnHasStarted(turn) {
			return turn, err
		}
		return agentModelTurn{}, err
	}
	turn := buildAnthropicStreamTurn(content.String(), finishReason, blocks)
	turn.PublicStreamedContent = deltas.StreamedContent()
	turn.Usage = usage
	turn.ProviderResponseID = providerResponseID
	turn.StreamLastEventID = lastEventID
	turn.StreamDuplicateEvents = duplicateEvents
	return turn, nil
}

func partialAnthropicToolArguments(block *anthropicStreamBlock) string {
	if block == nil {
		return ""
	}
	if partial := block.PartialJSON.String(); partial != "" {
		return partial
	}
	if block.Input != nil {
		raw, _ := json.Marshal(block.Input)
		return string(raw)
	}
	return ""
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
		case "thinking":
			thinking := block.Thinking.String()
			if thinking != "" || block.Signature != "" {
				item := map[string]any{"type": "thinking", "thinking": thinking}
				if block.Signature != "" {
					item["signature"] = block.Signature
				}
				turn.AnthropicRaw = append(turn.AnthropicRaw, item)
			}
		case "redacted_thinking":
			if block.Data != "" {
				turn.AnthropicRaw = append(turn.AnthropicRaw, map[string]any{"type": "redacted_thinking", "data": block.Data})
			}
		case "tool_use":
			args := strings.TrimSpace(block.PartialJSON.String())
			if args == "" {
				if block.Input != nil {
					raw, _ := json.Marshal(block.Input)
					args = string(raw)
				}
				if args == "" {
					args = "{}"
				}
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
