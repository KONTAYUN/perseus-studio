package ai

func openAISkillTools() []map[string]any {
	return openAISkillToolsForRole("admin")
}

func openAISkillToolsForRole(role string) []map[string]any {
	tools := []map[string]any{
		openAISkillToolByName("list_skills"),
		openAISkillToolByName("read_skill"),
	}
	if normalizeRunUserRole(role) != "admin" {
		return tools
	}
	return append(tools,
		openAISkillToolByName("install_skill_from_attachment"),
		openAISkillToolByName("delete_skill"),
	)
}

func anthropicSkillTools() []map[string]any {
	return anthropicSkillToolsForRole("admin")
}

func anthropicSkillToolsForRole(role string) []map[string]any {
	tools := []map[string]any{
		anthropicSkillToolByName("list_skills"),
		anthropicSkillToolByName("read_skill"),
	}
	if normalizeRunUserRole(role) != "admin" {
		return tools
	}
	return append(tools,
		anthropicSkillToolByName("install_skill_from_attachment"),
		anthropicSkillToolByName("delete_skill"),
	)
}

func openAISkillToolByName(name string) map[string]any {
	switch name {
	case "list_skills":
		return openAIFunctionTool(name, "List enabled local Skills by id, name, and description. Does not return SKILL.md content. Do not call just to read every Skill by default.", emptyObjectParams())
	case "read_skill":
		return openAIFunctionTool(name, "Read the SKILL.md content for one enabled Skill when its instructions are needed or when the user selected/referenced that Skill.", skillIDParams())
	case "install_skill_from_attachment":
		return openAIFunctionTool(name, "Install a user-uploaded skill.zip attachment. If multiple SKILL.md candidates are found, return candidates for the user to choose instead of installing all.", installSkillParams())
	case "delete_skill":
		return openAIFunctionTool(name, "Permanently delete an installed Skill record and its install directory. Only use when the user explicitly asks to delete a Skill. This does not affect workspace files.", skillIDParams())
	default:
		return nil
	}
}

func anthropicSkillToolByName(name string) map[string]any {
	switch name {
	case "list_skills":
		return anthropicTool(name, "List enabled local Skills by id, name, and description. Does not return SKILL.md content. Do not call just to read every Skill by default.", emptyObjectParams())
	case "read_skill":
		return anthropicTool(name, "Read the SKILL.md content for one enabled Skill when its instructions are needed or when the user selected/referenced that Skill.", skillIDParams())
	case "install_skill_from_attachment":
		return anthropicTool(name, "Install a user-uploaded skill.zip attachment. If multiple SKILL.md candidates are found, return candidates for the user to choose instead of installing all.", installSkillParams())
	case "delete_skill":
		return anthropicTool(name, "Permanently delete an installed Skill record and its install directory. Only use when the user explicitly asks to delete a Skill. This does not affect workspace files.", skillIDParams())
	default:
		return nil
	}
}

func emptyObjectParams() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func skillIDParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"skill_id"},
		"properties": map[string]any{
			"skill_id": map[string]any{"type": "string"},
		},
	}
}

func installSkillParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"attachment_id"},
		"properties": map[string]any{
			"attachment_id": map[string]any{"type": "string"},
			"skill_id":      map[string]any{"type": "string", "description": "Optional candidate id only after candidates were returned."},
			"enabled":       map[string]any{"type": "boolean"},
		},
	}
}
