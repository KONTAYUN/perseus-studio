package ai

type ToolGroup string

const (
	ToolGroupRead       ToolGroup = "read"
	ToolGroupSearch     ToolGroup = "search"
	ToolGroupEdit       ToolGroup = "edit"
	ToolGroupCommand    ToolGroup = "command"
	ToolGroupWeb        ToolGroup = "web"
	ToolGroupSkill      ToolGroup = "skill"
	ToolGroupTask       ToolGroup = "task"
	ToolGroupCompletion ToolGroup = "completion"
	ToolGroupQuestion   ToolGroup = "question"
)

func toolGroup(name string) ToolGroup {
	switch name {
	case "attempt_completion":
		return ToolGroupCompletion
	case "ask_followup_question":
		return ToolGroupQuestion
	case "find_workspace_files", "search_workspace_text", "inspect_workspace":
		return ToolGroupSearch
	case "read_workspace_text", "read_workspace_files":
		return ToolGroupRead
	case "edit_workspace_lines", "write_workspace_text", "edit_workspace_text", "replace_workspace_lines", "save_workspace_file", "write_workspace_chunk", "manage_workspace_path", "organize_workspace_matches":
		return ToolGroupEdit
	case "run_workspace_command":
		return ToolGroupCommand
	case "perseus_search_web", "web_search", "web_extract":
		return ToolGroupWeb
	case "list_skills", "read_skill", "install_skill_from_attachment", "delete_skill":
		return ToolGroupSkill
	case "start_background_task":
		return ToolGroupTask
	default:
		return ""
	}
}
