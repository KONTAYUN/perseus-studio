package ai

import (
	"encoding/json"
	"strings"
)

type ToolsetActions struct {
	JSONTools       []string              `json:"json_tools"`
	TextActions     []ModelTextAction     `json:"text_actions,omitempty"`
	TerminalActions []ModelTerminalAction `json:"terminal_actions,omitempty"`
}

func toolsForToolset(toolset string) ([]string, bool) {
	return toolsForToolsetRole(toolset, "admin")
}

func toolsForToolsetRole(toolset, role string) ([]string, bool) {
	actions, ok := actionsForToolsetRole(toolset, role)
	if !ok {
		return nil, false
	}
	return append([]string{}, actions.JSONTools...), true
}

func actionsForToolsetRole(toolset, role string) (ToolsetActions, bool) {
	switch toolset {
	case "workspace", "command", "search", "skills", "task":
		actions := ToolsetActions{
			JSONTools:       providerVisibleToolNamesForToolset(toolset, role),
			TextActions:     modelTextActionsForToolset(toolset, role),
			TerminalActions: modelTerminalActionsForRole(role),
		}
		return actions, true
	default:
		return ToolsetActions{}, false
	}
}

func modelTextActionsForToolset(toolset, role string) []ModelTextAction {
	role = normalizeRunUserRole(role)
	var actions []ModelTextAction
	for _, entry := range toolManifestEntries {
		if entry.Toolset != toolset || !entry.ModelVisible || entry.ActionKind != ModelActionText || !toolVisibleForRole(entry.Name, role) {
			continue
		}
		actions = append(actions, ModelTextAction{
			Name:      entry.Name,
			Toolset:   entry.Toolset,
			Group:     entry.Group,
			Transport: entry.Transport,
			Syntax:    textActionSyntax(entry),
			Primary:   entry.PrimaryEditProtocol,
		})
	}
	return actions
}

func modelTerminalActionsForRole(role string) []ModelTerminalAction {
	role = normalizeRunUserRole(role)
	var actions []ModelTerminalAction
	for _, entry := range toolManifestEntries {
		if !entry.ModelVisible || entry.ActionKind != ModelActionTerminal || !toolVisibleForRole(entry.Name, role) {
			continue
		}
		actions = append(actions, ModelTerminalAction{
			Name:      entry.Name,
			Group:     entry.Group,
			Transport: entry.Transport,
			Terminal:  true,
		})
	}
	return actions
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
