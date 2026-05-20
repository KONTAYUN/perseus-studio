package ai

func openAICompletionTool() map[string]any {
	return openAIFunctionTool("attempt_completion", "Finish the current agent task only after required tool work has actually completed and been verified. Use this instead of ending a workspace-changing task with plain text.", completionParams())
}

func anthropicCompletionTool() map[string]any {
	return anthropicTool("attempt_completion", "Finish the current agent task only after required tool work has actually completed and been verified. Use this instead of ending a workspace-changing task with plain text.", completionParams())
}

func completionParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"summary"},
		"properties": map[string]any{
			"summary":       map[string]any{"type": "string", "description": "Concise user-facing summary of what was completed."},
			"changed_paths": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Workspace paths changed, created, moved, or deleted during this task. Include at least one verified path for every successful workspace mutation."},
			"failed_items":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Items that could not be completed, if any."},
			"notes":         map[string]any{"type": "string", "description": "Optional short caveats or follow-up notes."},
		},
	}
}
