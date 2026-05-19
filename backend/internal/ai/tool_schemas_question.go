package ai

func openAIQuestionTool() map[string]any {
	return openAIFunctionTool("ask_followup_question", "Ask the user one concise follow-up question when their answer would materially change the result. Use sparingly; if a safe default exists, proceed and mention the assumption instead.", questionParams())
}

func anthropicQuestionTool() map[string]any {
	return anthropicTool("ask_followup_question", "Ask the user one concise follow-up question when their answer would materially change the result. Use sparingly; if a safe default exists, proceed and mention the assumption instead.", questionParams())
}

func questionParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"question"},
		"properties": map[string]any{
			"question":       map[string]any{"type": "string", "description": "One focused user-facing question."},
			"options":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional 2-4 short suggested answers."},
			"allow_freeform": map[string]any{"type": "boolean", "description": "Whether the user may answer freely. Defaults to true."},
			"reason":         map[string]any{"type": "string", "description": "Internal reason for asking. Not shown by default."},
		},
	}
}
