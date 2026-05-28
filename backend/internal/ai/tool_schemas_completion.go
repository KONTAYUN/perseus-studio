package ai

func openAICompletionTool() map[string]any {
	return openAIFunctionTool("attempt_completion", "Finish the current agent task only after required tool work has actually completed and been verified. The result parameter is the final user-visible answer, following mature agent completion tools such as Cline's attempt_completion.", completionParams())
}

func anthropicCompletionTool() map[string]any {
	return anthropicTool("attempt_completion", "Finish the current agent task only after required tool work has actually completed and been verified. The result parameter is the final user-visible answer, following mature agent completion tools such as Cline's attempt_completion.", completionParams())
}

func completionParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"result"},
		"properties": map[string]any{
			"result":        map[string]any{"type": "string", "description": "Final user-visible answer. Include the actual requested steps, answer, or deliverable here; do not merely summarize that it was prepared."},
			"summary":       map[string]any{"type": "string", "description": "Optional concise metadata summary of what was completed. This is not a substitute for result."},
			"changed_paths": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Workspace paths changed, created, moved, or deleted during this task. Include at least one verified path for every successful workspace mutation."},
			"failed_items":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Items that could not be completed, if any."},
			"notes":         map[string]any{"type": "string", "description": "Optional short caveats or follow-up notes."},
		},
	}
}
