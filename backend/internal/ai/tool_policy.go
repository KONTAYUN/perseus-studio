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
	if entry, ok := toolManifestEntry(name); ok {
		return entry.Group
	}
	return ""
}
