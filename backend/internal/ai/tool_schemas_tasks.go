package ai

func openAIBackgroundTaskTool() map[string]any {
	return openAIFunctionTool("start_background_task", "Create a background task for long-running, multi-step work. Use for web research plus report writing, long documents, batch edits, refactors, build/test/fix loops, or work the user wants to continue in the background.", backgroundTaskParams())
}

func anthropicBackgroundTaskTool() map[string]any {
	return anthropicTool("start_background_task", "Create a background task for long-running, multi-step work. Use for web research plus report writing, long documents, batch edits, refactors, build/test/fix loops, or work the user wants to continue in the background.", backgroundTaskParams())
}

func backgroundTaskParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"title", "goal", "expected_outputs", "suggested_steps"},
		"properties": map[string]any{
			"title": map[string]any{"type": "string"},
			"goal":  map[string]any{"type": "string"},
			"expected_outputs": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":     "object",
					"required": []string{"type", "description"},
					"properties": map[string]any{
						"type":        map[string]any{"type": "string", "enum": []string{"file", "message", "command_result"}},
						"path":        map[string]any{"type": "string", "description": "Workspace-relative path when type=file."},
						"description": map[string]any{"type": "string"},
					},
				},
			},
			"suggested_steps": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
	}
}
