package ai

func openAIWorkspaceTools() []map[string]any {
	return []map[string]any{
		openAIFunctionTool("inspect_workspace", "Inspect workspace structure by listing files and folders. Use before reading or changing files when paths are uncertain.", workspaceListParams()),
		openAIFunctionTool("read_workspace_text", "Read a text file from the current user's workspace. Use when file content is needed; do not guess content from file names.", workspacePathParams("path", "Workspace-relative file path.")),
		openAIFunctionTool("save_workspace_file", "Create or overwrite a workspace text file with backend validation. Use mode=create for new files and mode=overwrite for existing files.", workspaceSaveParams()),
		openAIFunctionTool("manage_workspace_path", "Perform a validated workspace path operation: mkdir, rename, move, or trash. Use this instead of primitive file operation tools.", workspaceManageParams()),
		openAIFunctionTool("organize_workspace_matches", "Deterministically organize workspace items whose path/name contains a match text into a target folder. The backend creates the target folder if needed, moves matching files by default, and verifies the result. Use for requests like sorting episodes/files into one folder.", workspaceOrganizeParams()),
	}
}

func anthropicWorkspaceTools() []map[string]any {
	return []map[string]any{
		anthropicTool("inspect_workspace", "Inspect workspace structure by listing files and folders. Use before reading or changing files when paths are uncertain.", workspaceListParams()),
		anthropicTool("read_workspace_text", "Read a text file from the current user's workspace. Use when file content is needed; do not guess content from file names.", workspacePathParams("path", "Workspace-relative file path.")),
		anthropicTool("save_workspace_file", "Create or overwrite a workspace text file with backend validation. Use mode=create for new files and mode=overwrite for existing files.", workspaceSaveParams()),
		anthropicTool("manage_workspace_path", "Perform a validated workspace path operation: mkdir, rename, move, or trash. Use this instead of primitive file operation tools.", workspaceManageParams()),
		anthropicTool("organize_workspace_matches", "Deterministically organize workspace items whose path/name contains a match text into a target folder. The backend creates the target folder if needed, moves matching files by default, and verifies the result. Use for requests like sorting episodes/files into one folder.", workspaceOrganizeParams()),
	}
}

func workspaceOrganizeParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"match_text", "target_folder"},
		"properties": map[string]any{
			"source_path":     map[string]any{"type": "string", "description": "Workspace-relative folder to search. Omit or empty for workspace root."},
			"match_text":      map[string]any{"type": "string", "description": "Text that must appear in the item path or file/folder name, such as an episode number or project name."},
			"target_folder":   map[string]any{"type": "string", "description": "Workspace-relative folder to create/use as destination."},
			"include_folders": map[string]any{"type": "boolean", "description": "Whether folders may also be moved. Default false; files are moved by default."},
			"max_items":       map[string]any{"type": "integer", "description": "Maximum matching items to move. Default 200, maximum 500."},
		},
	}
}

func workspaceListParams() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":      map[string]any{"type": "string", "description": "Workspace-relative folder path. Omit or use empty string for workspace root."},
			"max_depth": map[string]any{"type": "integer", "description": "Maximum folder depth to return. Default 3."},
		},
	}
}

func workspacePathParams(name, description string) map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{name},
		"properties": map[string]any{
			name: map[string]any{"type": "string", "description": description},
		},
	}
}

func workspaceWriteParams(contentDescription string) map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"path", "content"},
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "Workspace-relative file path."},
			"content": map[string]any{"type": "string", "description": contentDescription},
		},
	}
}

func workspaceSaveParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"path", "content", "mode"},
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "Workspace-relative file path."},
			"content": map[string]any{"type": "string", "description": "Full text content to save."},
			"mode": map[string]any{
				"type":        "string",
				"enum":        []string{"create", "overwrite"},
				"description": "Use create only when the file must not already exist. Use overwrite only for an existing file.",
			},
		},
	}
}

func workspaceRenameParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"old_path", "new_name"},
		"properties": map[string]any{
			"old_path": map[string]any{"type": "string", "description": "Existing workspace-relative file or folder path."},
			"new_name": map[string]any{"type": "string", "description": "New file or folder name only, without any parent path."},
		},
	}
}

func workspaceMoveParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"old_path", "new_parent_path"},
		"properties": map[string]any{
			"old_path":        map[string]any{"type": "string", "description": "Existing workspace-relative file or folder path."},
			"new_parent_path": map[string]any{"type": "string", "description": "Workspace-relative destination folder path. Use empty string for workspace root."},
		},
	}
}

func workspaceManageParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"action", "path"},
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"mkdir", "rename", "move", "trash"},
				"description": "Workspace path operation to perform.",
			},
			"path":            map[string]any{"type": "string", "description": "Existing path for rename/move/trash, or folder path to create for mkdir."},
			"new_name":        map[string]any{"type": "string", "description": "Required for rename. New leaf name only, not a parent path."},
			"new_parent_path": map[string]any{"type": "string", "description": "Required for move. Destination folder path, empty string means workspace root."},
		},
	}
}
