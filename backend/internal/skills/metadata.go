package skills

import (
	"regexp"
	"strings"
)

func parseSkillMetadata(content, fallbackName string) (string, string) {
	name := ""
	description := ""
	lines := strings.Split(content, "\n")
	inFrontMatter := len(lines) > 0 && strings.TrimSpace(lines[0]) == "---"
	if inFrontMatter {
		values := parseFrontMatterValues(lines)
		name = values["name"]
		description = values["description"]
	} else {
		for index, line := range lines {
			if index > 40 {
				break
			}
			trimmed := strings.TrimSpace(line)
			lower := strings.ToLower(trimmed)
			if strings.HasPrefix(lower, "name:") && name == "" {
				name = cleanMetaValue(trimmed[5:])
			}
			if strings.HasPrefix(lower, "description:") && description == "" {
				description = cleanMetaValue(trimmed[12:])
			}
		}
	}
	if name == "" {
		re := regexp.MustCompile(`(?m)^#\s+(.+)$`)
		if match := re.FindStringSubmatch(content); len(match) == 2 {
			name = strings.TrimSpace(match[1])
		}
	}
	if name == "" {
		name = fallbackName
	}
	return truncatePlain(name, 80), truncatePlain(description, 240)
}

func parseFrontMatterValues(lines []string) map[string]string {
	values := map[string]string{}
	for index := 1; index < len(lines); index++ {
		line := strings.TrimRight(lines[index], "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			break
		}
		key, rawValue, ok := splitYAMLKeyValue(trimmed)
		if !ok {
			continue
		}
		key = strings.ToLower(key)
		if key != "name" && key != "description" {
			continue
		}
		if rawValue == ">" || rawValue == ">-" || rawValue == ">+" || rawValue == "|" || rawValue == "|-" || rawValue == "|+" {
			block, nextIndex := collectYAMLBlock(lines, index+1, strings.HasPrefix(rawValue, ">"))
			values[key] = block
			index = nextIndex - 1
			continue
		}
		values[key] = cleanMetaValue(rawValue)
	}
	return values
}

func splitYAMLKeyValue(line string) (string, string, bool) {
	index := strings.Index(line, ":")
	if index <= 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:index])
	value := strings.TrimSpace(line[index+1:])
	if key == "" {
		return "", "", false
	}
	return key, value, true
}

func collectYAMLBlock(lines []string, start int, folded bool) (string, int) {
	var parts []string
	for index := start; index < len(lines); index++ {
		line := strings.TrimRight(lines[index], "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" || isTopLevelYAMLKey(line) {
			return cleanYAMLBlock(parts, folded), index
		}
		if strings.TrimSpace(line) == "" {
			if !folded {
				parts = append(parts, "")
			}
			continue
		}
		parts = append(parts, strings.TrimSpace(line))
	}
	return cleanYAMLBlock(parts, folded), len(lines)
}

func isTopLevelYAMLKey(line string) bool {
	if line == "" || line[0] == ' ' || line[0] == '\t' {
		return false
	}
	_, _, ok := splitYAMLKeyValue(strings.TrimSpace(line))
	return ok
}

func cleanYAMLBlock(parts []string, folded bool) string {
	if folded {
		return strings.TrimSpace(strings.Join(parts, " "))
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func cleanMetaValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	return strings.TrimSpace(value)
}
