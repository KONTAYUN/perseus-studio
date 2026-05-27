package ai

func openAISkillTools() []map[string]any {
	return openAISkillToolsForRole("admin")
}

func openAISkillToolsForRole(role string) []map[string]any {
	tools := []map[string]any{
		openAIFunctionTool("list_skills", "List enabled local Skills by id, name, and description. Does not return SKILL.md content. Do not call just to read every Skill by default.", emptyObjectParams()),
		openAIFunctionTool("read_skill", "Read the SKILL.md content for one enabled Skill when its instructions are needed or when the user selected/referenced that Skill.", skillIDParams()),
	}
	if normalizeRunUserRole(role) != "admin" {
		return tools
	}
	return append(tools,
		openAIFunctionTool("install_skill_from_attachment", "Install a user-uploaded skill.zip attachment. If multiple SKILL.md candidates are found, return candidates for the user to choose instead of installing all.", installSkillParams()),
		openAIFunctionTool("delete_skill", "Permanently delete an installed Skill record and its install directory. Only use when the user explicitly asks to delete a Skill. This does not affect workspace files.", skillIDParams()),
	)
}

func anthropicSkillTools() []map[string]any {
	return anthropicSkillToolsForRole("admin")
}

func anthropicSkillToolsForRole(role string) []map[string]any {
	tools := []map[string]any{
		anthropicTool("list_skills", "List enabled local Skills by id, name, and description. Does not return SKILL.md content. Do not call just to read every Skill by default.", emptyObjectParams()),
		anthropicTool("read_skill", "Read the SKILL.md content for one enabled Skill when its instructions are needed or when the user selected/referenced that Skill.", skillIDParams()),
	}
	if normalizeRunUserRole(role) != "admin" {
		return tools
	}
	return append(tools,
		anthropicTool("install_skill_from_attachment", "Install a user-uploaded skill.zip attachment. If multiple SKILL.md candidates are found, return candidates for the user to choose instead of installing all.", installSkillParams()),
		anthropicTool("delete_skill", "Permanently delete an installed Skill record and its install directory. Only use when the user explicitly asks to delete a Skill. This does not affect workspace files.", skillIDParams()),
	)
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
