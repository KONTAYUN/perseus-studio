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
			"outcome":       map[string]any{"type": "string", "enum": []string{"success", "partial", "blocked"}, "description": "Structured final outcome. Use partial or blocked for unmet work; do not claim unverified changes."},
			"changed_paths": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional paths; backend validates against actual mutations."},
			"result":        map[string]any{"type": "string", "description": "Final user-visible answer. Include the actual requested answer or completion note. This terminal action cannot create, modify, move, or delete workspace files."},
			"summary":       map[string]any{"type": "string", "description": "Optional concise metadata summary of what was completed. This is not a substitute for result."},
			"failed_items":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Unmet work or unperformed checks, whether due to failure, scope or unavailable capabilities."},
			"notes":         map[string]any{"type": "string", "description": "Optional short caveats or follow-up notes."},
		},
	}
}
