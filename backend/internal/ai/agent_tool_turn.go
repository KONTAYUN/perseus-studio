package ai

import (
	"strings"
	"unicode/utf8"
)

const agentToolPreambleMaxRunes = 220

func prepareToolTurnForExecution(turn agentModelTurn) (agentModelTurn, int) {
	if len(turn.ToolCalls) == 0 {
		return turn, 0
	}
	turn.Content = conciseToolPreamble(turn.Content)
	turn.OpenAIMessage.Content = turn.Content
	turn.AnthropicRaw = conciseAnthropicRaw(turn.AnthropicRaw, turn.Content)
	return turn, 0
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
