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
	case "workspace":
		return []string{
			"attempt_completion",
			"find_workspace_files",
			"search_workspace_text",
			"inspect_workspace",
			"read_workspace_text",
			"read_workspace_files",
			"edit_workspace_text",
			"save_workspace_file",
			"manage_workspace_path",
			"organize_workspace_matches",
		}, true
	case "command":
		return []string{"attempt_completion", "run_workspace_command"}, true
	case "search":
		return []string{"attempt_completion", "perseus_search_web", "web_extract"}, true
	case "skills":
		if normalizeRunUserRole(role) != "admin" {
			return []string{"attempt_completion", "list_skills", "read_skill"}, true
		}
		return []string{"attempt_completion", "list_skills", "read_skill", "install_skill_from_attachment", "delete_skill"}, true
	case "task":
		return []string{"attempt_completion", "start_background_task"}, true
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
