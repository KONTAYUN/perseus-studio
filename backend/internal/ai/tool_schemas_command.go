package ai

func openAICommandTools() []map[string]any {
	return []map[string]any{openAICommandTool()}
}

func anthropicCommandTools() []map[string]any {
	return []map[string]any{anthropicCommandTool()}
}

func openAICommandTool() map[string]any {
	return openAIFunctionTool("run_workspace_command", "Run a shell command inside the current user's workspace to inspect projects, run tests, build, or execute scripts. Use only when command execution is necessary. Requires administrator and explicit trusted_host deployment opt-in. This is host execution, NOT an OS sandbox. Cwd is workspace-relative but filesystem/network access uses the server account.", commandParams())
}

func anthropicCommandTool() map[string]any {
	return anthropicTool("run_workspace_command", "Run a shell command inside the current user's workspace to inspect projects, run tests, build, or execute scripts. Use only when command execution is necessary. Requires administrator and explicit trusted_host deployment opt-in. This is host execution, NOT an OS sandbox. Cwd is workspace-relative but filesystem/network access uses the server account.", commandParams())
}

func commandParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"command"},
		"properties": map[string]any{
			"expected_exit_codes": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "minItems": 1, "maxItems": 32, "description": "Expected process exit codes; default [0]. rg no matches may use [0,1]. Timeout/cancellation never count as success."},
			"command":             map[string]any{"type": "string", "description": "Command to run inside the workspace."},
			"cwd":                 map[string]any{"type": "string", "description": "Workspace-relative working directory. Omit or use empty string for workspace root."},
			"timeout_sec":         map[string]any{"type": "integer", "description": "Timeout in seconds. Default 120, maximum 600."},
		},
	}
}
