package ai

import (
	"strings"
	"unicode/utf8"
)

const agentToolPreambleMaxRunes = 220

// prepareToolTurnForExecution keeps the first tool call and returns the calls it
// removed, so the runtime can report them to the model instead of dropping them.
func prepareToolTurnForExecution(turn agentModelTurn) (agentModelTurn, []agentToolCall) {
	if len(turn.ToolCalls) == 0 {
		return turn, nil
	}
	turn.Content = conciseToolPreamble(turn.Content)
	turn.OpenAIMessage.Content = turn.Content
	turn.AnthropicRaw = conciseAnthropicRaw(turn.AnthropicRaw, turn.Content)
	return keepFirstToolCall(turn)
}

func conciseToolPreamble(content string) string {
	content = cleanToolPreamble(content)
	if content == "" {
		return ""
	}
	preamble := firstParagraph(content)
	preamble = strings.Join(strings.Fields(preamble), " ")
	if preamble == "" {
		return ""
	}
	if utf8.RuneCountInString(preamble) <= agentToolPreambleMaxRunes {
		return preamble
	}
	if sentence := firstSentenceWithin(preamble, agentToolPreambleMaxRunes); sentence != "" {
		return sentence
	}
	runes := []rune(preamble)
	return strings.TrimSpace(string(runes[:agentToolPreambleMaxRunes])) + "..."
}

func firstParagraph(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	lines := strings.Split(content, "\n")
	paragraph := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if len(paragraph) > 0 {
				break
			}
			continue
		}
		paragraph = append(paragraph, line)
	}
	return strings.Join(paragraph, " ")
}

func firstSentenceWithin(content string, maxRunes int) string {
	runes := []rune(content)
	for index, item := range runes {
		if !strings.ContainsRune("。！？!?；;", item) {
			continue
		}
		if index+1 <= maxRunes {
			return strings.TrimSpace(string(runes[:index+1]))
		}
		return ""
	}
	return ""
}

func keepFirstToolCall(turn agentModelTurn) (agentModelTurn, []agentToolCall) {
	if len(turn.ToolCalls) <= 1 {
		return turn, nil
	}
	first := turn.ToolCalls[0]
	skipped := append([]agentToolCall(nil), turn.ToolCalls[1:]...)
	turn.ToolCalls = []agentToolCall{first}
	if len(turn.OpenAIMessage.ToolCalls) > 1 {
		turn.OpenAIMessage.ToolCalls = firstOpenAIToolCall(turn.OpenAIMessage.ToolCalls, first)
	}
	turn.AnthropicRaw = firstAnthropicToolUse(turn.AnthropicRaw, first)
	return turn, skipped
}

func firstOpenAIToolCall(items []openAIToolCall, first agentToolCall) []openAIToolCall {
	for _, item := range items {
		if item.ID == first.ID || item.Function.Name == first.Name {
			return []openAIToolCall{item}
		}
	}
	if len(items) > 0 {
		return []openAIToolCall{items[0]}
	}
	return nil
}

func conciseAnthropicRaw(items []map[string]any, preamble string) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	next := make([]map[string]any, 0, len(items))
	if strings.TrimSpace(preamble) != "" {
		next = append(next, map[string]any{"type": "text", "text": preamble})
	}
	for _, item := range items {
		if item["type"] == "text" {
			continue
		}
		next = append(next, item)
	}
	return next
}

func firstAnthropicToolUse(items []map[string]any, first agentToolCall) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	next := make([]map[string]any, 0, len(items))
	keptTool := false
	for _, item := range items {
		if item["type"] != "tool_use" {
			next = append(next, item)
			continue
		}
		if keptTool {
			continue
		}
		if item["id"] == first.ID || item["name"] == first.Name {
			next = append(next, item)
			keptTool = true
		}
	}
	if keptTool {
		return next
	}
	for _, item := range items {
		if item["type"] == "tool_use" {
			next = append(next, item)
			return next
		}
	}
	return next
}
