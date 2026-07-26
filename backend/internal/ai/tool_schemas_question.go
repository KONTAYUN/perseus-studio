package ai

const (
	requestUserInputToolName       = "request_user_input"
	requestUserChoiceToolName      = "request_user_choice"
	legacyFollowupQuestionToolName = "ask_followup_question"
)

func openAIUserInputTool() map[string]any {
	return openAIFunctionTool(requestUserInputToolName, userInputToolDescription, userInputParams())
}

func anthropicUserInputTool() map[string]any {
	return anthropicTool(requestUserInputToolName, userInputToolDescription, userInputParams())
}

func openAIUserChoiceTool() map[string]any {
	return openAIFunctionTool(requestUserChoiceToolName, userChoiceToolDescription, userChoiceParams())
}

func anthropicUserChoiceTool() map[string]any {
	return anthropicTool(requestUserChoiceToolName, userChoiceToolDescription, userChoiceParams())
}

const userInputToolDescription = "Pause the current turn only when progress requires information that only the user can provide, such as external command output, logs, credentials, or an open-ended answer. Do not use this tool for facts available through context or tools, and do not manufacture suggested answers. The question is complete user-visible Markdown. Put commands, terminal input/output, code, and configuration in fenced code blocks with a language name."

const userChoiceToolDescription = "Pause the current turn only for a bounded decision owned by the user, such as choosing between mutually exclusive approaches or authorizing one of a small set of actions. Do not use this tool to collect open-ended input or facts available through context or tools. Provide exactly two or three non-empty, mutually exclusive options. The question is complete user-visible Markdown."

func userInputParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"question"},
		"properties": map[string]any{
			"question": map[string]any{"type": "string", "description": "The complete user-visible Markdown request. Use fenced code blocks with a language name for commands, code, terminal text, and configuration."},
		},
	}
}

func userChoiceParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"question", "options"},
		"properties": map[string]any{
			"question": map[string]any{"type": "string", "description": "The complete user-visible Markdown question describing the bounded decision."},
			"options":  map[string]any{"type": "array", "minItems": 2, "maxItems": 3, "items": map[string]any{"type": "string"}, "description": "Exactly two or three complete, non-empty, mutually exclusive choices."},
		},
	}
}

func isUserRequestTool(name string) bool {
	switch name {
	case requestUserInputToolName, requestUserChoiceToolName, legacyFollowupQuestionToolName:
		return true
	default:
		return false
	}
}

func userRequestMessageKind(name string) string {
	switch name {
	case requestUserInputToolName:
		return "user_input_request"
	case requestUserChoiceToolName:
		return "user_choice_request"
	default:
		return "followup_question"
	}
}
