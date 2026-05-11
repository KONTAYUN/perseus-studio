package ai

func toolsetLoaderParams() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"toolset"},
		"properties": map[string]any{
			"toolset": map[string]any{
				"type":        "string",
				"enum":        []string{"workspace", "command", "search", "skills", "task"},
				"description": "Toolset to load for the next model round.",
			},
		},
	}
}

func openAIToolsetLoaderTool() map[string]any {
	return openAIStrictFunctionTool(
		"load_perseus_toolset",
		"Load one optional Perseus toolset for the next model round before using it. Use workspace for files, command for shell commands, search for web search, skills for Skill access, and task for background tasks.",
		toolsetLoaderParams(),
	)
}

func anthropicToolsetLoaderTool() map[string]any {
	return anthropicTool(
		"load_perseus_toolset",
		"Load one optional Perseus toolset for the next model round before using it. Use workspace for files, command for shell commands, search for web search, skills for Skill access, and task for background tasks.",
		toolsetLoaderParams(),
	)
}
