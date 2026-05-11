package ai

func openAITools() []map[string]any {
	return openAIToolsForLoadedToolsets(nil, true)
}

func openAIToolsForLoadedToolsets(loaded map[string]bool, includeLoader bool) []map[string]any {
	tools := []map[string]any{openAICompletionTool()}
	if includeLoader {
		tools = append(tools, openAIToolsetLoaderTool())
	}
	if loaded["workspace"] {
		tools = append(tools, openAIWorkspaceTools()...)
	}
	if loaded["command"] {
		tools = append(tools, openAICommandTools()...)
	}
	if loaded["search"] {
		tools = append(tools, openAIWebTools()...)
	}
	if loaded["skills"] {
		tools = append(tools, openAISkillTools()...)
	}
	if loaded["task"] {
		tools = append(tools, openAIBackgroundTaskTool())
	}
	return tools
}

func anthropicTools() []map[string]any {
	return anthropicToolsForLoadedToolsets(nil, true)
}

func anthropicToolsForLoadedToolsets(loaded map[string]bool, includeLoader bool) []map[string]any {
	tools := []map[string]any{anthropicCompletionTool()}
	if includeLoader {
		tools = append(tools, anthropicToolsetLoaderTool())
	}
	if loaded["workspace"] {
		tools = append(tools, anthropicWorkspaceTools()...)
	}
	if loaded["command"] {
		tools = append(tools, anthropicCommandTools()...)
	}
	if loaded["search"] {
		tools = append(tools, anthropicWebTools()...)
	}
	if loaded["skills"] {
		tools = append(tools, anthropicSkillTools()...)
	}
	if loaded["task"] {
		tools = append(tools, anthropicBackgroundTaskTool())
	}
	return tools
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
