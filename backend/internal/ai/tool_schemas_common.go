package ai

func openAITools() []map[string]any {
	return openAIToolsForLoadedToolsets(nil, true, "admin")
}

func openAIToolsForLoadedToolsets(loaded map[string]bool, includeLoader bool, roles ...string) []map[string]any {
	role := "admin"
	if len(roles) > 0 {
		role = normalizeRunUserRole(roles[0])
	}
	var tools []map[string]any
	for _, name := range providerVisibleToolNames(loaded, includeLoader, role) {
		if tool := openAIToolByName(name); tool != nil {
			tools = append(tools, tool)
		}
	}
	return tools
}

func anthropicTools() []map[string]any {
	return anthropicToolsForLoadedToolsets(nil, true, "admin")
}

func anthropicToolsForLoadedToolsets(loaded map[string]bool, includeLoader bool, roles ...string) []map[string]any {
	role := "admin"
	if len(roles) > 0 {
		role = normalizeRunUserRole(roles[0])
	}
	var tools []map[string]any
	for _, name := range providerVisibleToolNames(loaded, includeLoader, role) {
		if tool := anthropicToolByName(name); tool != nil {
			tools = append(tools, tool)
		}
	}
	return tools
}

func openAIToolByName(name string) map[string]any {
	switch name {
	case "attempt_completion":
		return openAICompletionTool()
	case requestUserInputToolName:
		return openAIUserInputTool()
	case requestUserChoiceToolName:
		return openAIUserChoiceTool()
	case "update_task_plan":
		return openAITaskPlanTool()
	case "load_perseus_toolset":
		return openAIToolsetLoaderTool()
	case "run_workspace_command":
		return openAICommandTool()
	case "perseus_search_web", "web_extract":
		return openAIWebToolByName(name)
	case "list_skills", "read_skill", "install_skill_from_attachment", "delete_skill":
		return openAISkillToolByName(name)
	case "start_background_task":
		return openAIBackgroundTaskTool()
	default:
		return openAIWorkspaceToolByName(name)
	}
}

func anthropicToolByName(name string) map[string]any {
	switch name {
	case "attempt_completion":
		return anthropicCompletionTool()
	case requestUserInputToolName:
		return anthropicUserInputTool()
	case requestUserChoiceToolName:
		return anthropicUserChoiceTool()
	case "update_task_plan":
		return anthropicTaskPlanTool()
	case "load_perseus_toolset":
		return anthropicToolsetLoaderTool()
	case "run_workspace_command":
		return anthropicCommandTool()
	case "perseus_search_web", "web_extract":
		return anthropicWebToolByName(name)
	case "list_skills", "read_skill", "install_skill_from_attachment", "delete_skill":
		return anthropicSkillToolByName(name)
	case "start_background_task":
		return anthropicBackgroundTaskTool()
	default:
		return anthropicWorkspaceToolByName(name)
	}
}

func openAIFunctionTool(name, description string, parameters map[string]any) map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        name,
			"description": description,
			"parameters":  parameters,
		},
	}
}

func openAIStrictFunctionTool(name, description string, parameters map[string]any) map[string]any {
	tool := openAIFunctionTool(name, description, parameters)
	function, _ := tool["function"].(map[string]any)
	function["strict"] = true
	return tool
}

func anthropicTool(name, description string, inputSchema map[string]any) map[string]any {
	return map[string]any{
		"name":         name,
		"description":  description,
		"input_schema": inputSchema,
	}
}
