package ai

import (
	"encoding/json"
	"strings"
)

func toolsForToolset(toolset string) ([]string, bool) {
	return toolsForToolsetRole(toolset, "admin")
}

func toolsForToolsetRole(toolset, role string) ([]string, bool) {
	switch toolset {
	case "workspace", "command", "search", "skills", "task":
		return append([]string{"attempt_completion"}, providerVisibleToolNamesForToolset(toolset, role)...), true
	default:
		return nil, false
	}
}

func markLoadedToolset(loaded map[string]bool, argumentsJSON string, allowTask bool) {
	var args struct {
		Toolset string `json:"toolset"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return
	}
	toolset := strings.TrimSpace(args.Toolset)
	if toolset == "task" && !allowTask {
		return
	}
	if _, ok := toolsForToolset(toolset); ok {
		loaded[toolset] = true
	}
}
