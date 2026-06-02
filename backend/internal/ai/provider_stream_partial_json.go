package ai

import (
	"encoding/json"
	"strings"
)

type partialJSONFieldStreamer struct {
	last string
}

func (s *partialJSONFieldStreamer) EmitNewDelta(toolName, partialJSON string, emit func(string)) {
	if s == nil || emit == nil || strings.TrimSpace(toolName) != "attempt_completion" {
		return
	}
	value, ok := partialJSONStringFieldValue(partialJSON, "result")
	if !ok || len(value) <= len(s.last) || !strings.HasPrefix(value, s.last) {
		return
	}
	if firstInternalCompletionMarker(value) != "" {
		return
	}
	delta := value[len(s.last):]
	s.last = value
	if delta != "" {
		emit(delta)
	}
}

func emitStreamedCompletionResultFromToolCalls(calls []agentToolCall, emit func(string)) bool {
	if emit == nil {
		return false
	}
	for _, call := range calls {
		if strings.TrimSpace(call.Name) != "attempt_completion" {
			continue
		}
		var attempt completionAttempt
		if err := json.Unmarshal([]byte(call.ArgumentsJSON), &attempt); err != nil {
			return false
		}
		content := strings.TrimSpace(attempt.Result)
		if strings.TrimSpace(attempt.Notes) != "" {
			content += "\n\n" + strings.TrimSpace(attempt.Notes)
		}
		if content == "" {
			return false
		}
		if firstInternalCompletionMarker(content) != "" {
			return false
		}
		for _, chunk := range splitRunes(content, 256) {
			emit(chunk)
		}
		return true
	}
	return false
}

func splitRunes(text string, maxRunes int) []string {
	if text == "" {
		return nil
	}
	if maxRunes <= 0 {
		maxRunes = 256
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return []string{text}
	}
	chunks := make([]string, 0, (len(runes)/maxRunes)+1)
	for start := 0; start < len(runes); start += maxRunes {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks
}

func partialJSONStringFieldValue(src, field string) (string, bool) {
	keyStart, ok := partialJSONFieldKeyEnd(src, field)
	if !ok {
		return "", false
	}
	colon := partialJSONNextNonSpace(src, keyStart)
	if colon < 0 || src[colon] != ':' {
		return "", false
	}
	valueStart := partialJSONNextNonSpace(src, colon+1)
	if valueStart < 0 || src[valueStart] != '"' {
		return "", false
	}
	return partialJSONStringValue(src, valueStart+1)
}

func partialJSONFieldKeyEnd(src, field string) (int, bool) {
	for index := 0; index < len(src); index++ {
		if src[index] != '"' {
			continue
		}
		value, end, ok := partialJSONStringToken(src, index+1)
		if !ok {
			return 0, false
		}
		next := partialJSONNextNonSpace(src, end)
		if value == field && next >= 0 && src[next] == ':' {
			return end, true
		}
		index = end
	}
	return 0, false
}

func partialJSONStringToken(src string, start int) (string, int, bool) {
	var value strings.Builder
	for index := start; index < len(src); index++ {
		switch src[index] {
		case '\\':
			decoded, next, ok := partialJSONDecodeEscape(src, index+1)
			if !ok {
				return "", index, false
			}
			value.WriteString(decoded)
			index = next - 1
		case '"':
			return value.String(), index + 1, true
		default:
			value.WriteByte(src[index])
		}
	}
	return "", len(src), false
}

func partialJSONStringValue(src string, start int) (string, bool) {
	var value strings.Builder
	for index := start; index < len(src); index++ {
		switch src[index] {
		case '\\':
			decoded, next, ok := partialJSONDecodeEscape(src, index+1)
			if !ok {
				return value.String(), true
			}
			value.WriteString(decoded)
			index = next - 1
		case '"':
			return value.String(), true
		default:
			value.WriteByte(src[index])
		}
	}
	return value.String(), true
}

func partialJSONNextNonSpace(src string, start int) int {
	for index := start; index < len(src); index++ {
		switch src[index] {
		case ' ', '\n', '\r', '\t':
			continue
		default:
			return index
		}
	}
	return -1
}

func partialJSONDecodeEscape(src string, start int) (string, int, bool) {
	if start >= len(src) {
		return "", start, false
	}
	switch src[start] {
	case '"', '\\', '/':
		return string(src[start]), start + 1, true
	case 'b':
		return "\b", start + 1, true
	case 'f':
		return "\f", start + 1, true
	case 'n':
		return "\n", start + 1, true
	case 'r':
		return "\r", start + 1, true
	case 't':
		return "\t", start + 1, true
	case 'u':
		return partialJSONDecodeUnicodeEscape(src, start+1)
	default:
		return string(src[start]), start + 1, true
	}
}

func partialJSONDecodeUnicodeEscape(src string, start int) (string, int, bool) {
	if start+4 > len(src) {
		return "", start, false
	}
	value, ok := partialJSONHex4(src[start : start+4])
	if !ok {
		return "", start, false
	}
	next := start + 4
	if value >= 0xD800 && value <= 0xDBFF && next+6 <= len(src) && src[next] == '\\' && src[next+1] == 'u' {
		low, lowOK := partialJSONHex4(src[next+2 : next+6])
		if lowOK && low >= 0xDC00 && low <= 0xDFFF {
			codepoint := 0x10000 + ((value - 0xD800) << 10) + (low - 0xDC00)
			return string(rune(codepoint)), next + 6, true
		}
	}
	return string(rune(value)), next, true
}

func partialJSONHex4(src string) (int, bool) {
	if len(src) != 4 {
		return 0, false
	}
	value := 0
	for index := 0; index < 4; index++ {
		digit := partialJSONHexDigit(src[index])
		if digit < 0 {
			return 0, false
		}
		value = value*16 + digit
	}
	return value, true
}

func partialJSONHexDigit(ch byte) int {
	switch {
	case ch >= '0' && ch <= '9':
		return int(ch - '0')
	case ch >= 'a' && ch <= 'f':
		return int(ch-'a') + 10
	case ch >= 'A' && ch <= 'F':
		return int(ch-'A') + 10
	default:
		return -1
	}
}
