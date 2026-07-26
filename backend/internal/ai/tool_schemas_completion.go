package ai

const completionToolDescription = "Finish the current agent task only after required tool work has actually completed and been verified. The result parameter is the complete final user-visible Markdown answer, following mature agent completion tools such as Roo Code's attempt_completion. Keep all final formatting inside result; summary is metadata only."

const completionResultDescription = "Complete final user-visible Markdown answer. Include the actual requested answer or completion note and preserve all required formatting. Executable commands, multi-line code, terminal input or output, and configuration snippets must use fenced code blocks with an appropriate language tag. For example: Run this command:\n\n```bash\nssh user@example.com\n```\nThis terminal action cannot create, modify, move, or delete workspace files."

func openAICompletionTool() map[string]any {
	return openAIFunctionTool("attempt_completion", completionToolDescription, completionParams())
}

func anthropicCompletionTool() map[string]any {
	return anthropicTool("attempt_completion", completionToolDescription, completionParams())
}

func completionParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"result"},
		"properties": map[string]any{
			"result":       map[string]any{"type": "string", "description": completionResultDescription},
			"summary":      map[string]any{"type": "string", "description": "Optional concise internal metadata summary of what was completed. It is not user-visible content and cannot replace or carry formatting omitted from result."},
			"failed_items": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional internal metadata listing items that could not be completed after a real tool/action failure. It is not appended to the user-visible answer."},
			"notes":        map[string]any{"type": "string", "description": "Optional internal metadata for diagnostics. It is not user-visible and cannot carry content or formatting omitted from result."},
		},
	}
}
