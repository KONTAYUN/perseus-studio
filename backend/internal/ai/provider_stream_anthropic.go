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
	Text        strings.Builder
	Thinking    strings.Builder
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
				updateProviderUsageFromAny(&usage, message["usage"])
			}
		case "content_block_start":
			index := intFromAny(raw["index"])
			blockRaw, _ := raw["content_block"].(map[string]any)
			block := &anthropicStreamBlock{}
			block.Type, _ = blockRaw["type"].(string)
			block.ID, _ = blockRaw["id"].(string)
			block.Name, _ = blockRaw["name"].(string)
			if block.Type == "tool_use" {
				deltas.MarkToolObserved()
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
			case "thinking_delta":
				thinking, _ := delta["thinking"].(string)
				if thinking != "" {
					block.Thinking.WriteString(thinking)
				}
			case "signature_delta":
				// Anthropic sends signatures for extended-thinking blocks just before
				// content_block_stop. They are verification metadata, not user text.
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
		turn.Usage = usage
		if hasCompleteToolCalls(turn) {
			return agentModelTurn{}, &partialStreamTurnError{turn: turn, err: err}
		}
		return agentModelTurn{}, err
	}
	turn := buildAnthropicStreamTurn(content.String(), finishReason, blocks)
	turn.Usage = usage
	return turn, nil
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
