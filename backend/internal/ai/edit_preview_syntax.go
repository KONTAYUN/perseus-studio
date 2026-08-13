package ai

import (
	"strconv"
	"strings"
)

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// topLevelJSONStringFieldComplete recognizes JSON syntax only. It never repairs
// an unfinished string, so streamed path fragments cannot become file targets.
func topLevelJSONStringFieldComplete(raw, field string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw[0] != '{' {
		return false
	}
	for index := 1; index < len(raw); {
		index = skipJSONWhitespaceAndCommas(raw, index)
		if index >= len(raw) || raw[index] == '}' || raw[index] != '"' {
			return false
		}
		keyEnd, ok := scanJSONStringEnd(raw, index)
		if !ok {
			return false
		}
		key, err := strconv.Unquote(raw[index:keyEnd])
		if err != nil {
			return false
		}
		index = skipJSONWhitespace(raw, keyEnd)
		if index >= len(raw) || raw[index] != ':' {
			return false
		}
		index = skipJSONWhitespace(raw, index+1)
		if key == field {
			if index >= len(raw) || raw[index] != '"' {
				return false
			}
			_, ok = scanJSONStringEnd(raw, index)
			return ok
		}
		valueEnd, ok := scanJSONValueEnd(raw, index)
		if !ok {
			return false
		}
		index = valueEnd
	}
	return false
}

func skipJSONWhitespace(raw string, index int) int {
	for index < len(raw) && strings.ContainsRune(" \t\r\n", rune(raw[index])) {
		index++
	}
	return index
}

func skipJSONWhitespaceAndCommas(raw string, index int) int {
	for index < len(raw) {
		if raw[index] == ',' || strings.ContainsRune(" \t\r\n", rune(raw[index])) {
			index++
			continue
		}
		break
	}
	return index
}

func scanJSONStringEnd(raw string, start int) (int, bool) {
	if start >= len(raw) || raw[start] != '"' {
		return start, false
	}
	escaped := false
	for index := start + 1; index < len(raw); index++ {
		if escaped {
			escaped = false
			continue
		}
		switch raw[index] {
		case '\\':
			escaped = true
		case '"':
			return index + 1, true
		}
	}
	return len(raw), false
}

func scanJSONValueEnd(raw string, start int) (int, bool) {
	if start >= len(raw) {
		return start, false
	}
	if raw[start] == '"' {
		return scanJSONStringEnd(raw, start)
	}
	if raw[start] == '{' || raw[start] == '[' {
		stack := []byte{raw[start]}
		for index := start + 1; index < len(raw); index++ {
			if raw[index] == '"' {
				end, ok := scanJSONStringEnd(raw, index)
				if !ok {
					return len(raw), false
				}
				index = end - 1
				continue
			}
			switch raw[index] {
			case '{', '[':
				stack = append(stack, raw[index])
			case '}', ']':
				stack = stack[:len(stack)-1]
				if len(stack) == 0 {
					return index + 1, true
				}
			}
		}
		return len(raw), false
	}
	for index := start; index < len(raw); index++ {
		if raw[index] == ',' || raw[index] == '}' {
			return index, index > start
		}
	}
	return len(raw), false
}

func completePatchPreviewInput(patchText string, fieldComplete bool) (string, bool) {
	if fieldComplete {
		return patchText, false
	}
	lastNewline := strings.LastIndex(patchText, "\n")
	if lastNewline < 0 {
		return "", isIncompletePatchTargetLine(patchText)
	}
	complete := patchText[:lastNewline+1]
	partialLine := strings.TrimSpace(patchText[lastNewline+1:])
	return complete, isIncompletePatchTargetLine(partialLine)
}

func isIncompletePatchTargetLine(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	for _, prefix := range []string{
		"*** Add File: ",
		"*** Update File: ",
		"*** Delete File: ",
		"*** Move to: ",
	} {
		if strings.HasPrefix(prefix, line) || strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}
