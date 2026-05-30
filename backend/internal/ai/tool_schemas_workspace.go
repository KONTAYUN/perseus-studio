package ai

func openAIWorkspaceTools() []map[string]any {
	return []map[string]any{
		openAIFunctionTool("find_workspace_files", "Find workspace files or folders by name/path query or glob pattern. Use first when the exact path is unknown.", workspaceFindParams()),
		openAIFunctionTool("search_workspace_text", "Search text inside workspace files and return file paths, line numbers, and context. Use for finding symbols, phrases, or likely edit locations.", workspaceSearchTextParams()),
		openAIFunctionTool("inspect_workspace", "Inspect workspace structure by listing files and folders. Prefer find_workspace_files or search_workspace_text for targeted discovery; use this for folder overview.", workspaceListParams()),
		openAIFunctionTool("read_workspace_text", "Read a supported plain-text file from the current user's workspace, optionally by line range. Do not use for binary/document formats such as .docx; if only an unsupported document is available, say it is not readable by the current text tools.", workspaceReadParams()),
		openAIFunctionTool("read_workspace_files", "Read multiple supported plain-text files from the current user's workspace in one call. Do not use for binary/document formats such as .docx.", workspaceReadFilesParams()),
		openAIFunctionTool("edit_workspace_text", "Edit an existing text file with exact search/replace. Use after reading or searching the file. For new files or full overwrite, use save_workspace_file.", workspaceEditParams()),
		openAIFunctionTool("replace_workspace_lines", "Replace a 1-based inclusive line range in an existing text file. Prefer this for scripts, long documents, duplicate headings, or larger rewrites after reading the target line range.", workspaceReplaceLinesParams()),
		openAIFunctionTool("save_workspace_file", "Create or overwrite a workspace text file with backend validation. Use mode=create for new files and mode=overwrite for existing files.", workspaceSaveParams()),
		openAIFunctionTool("write_workspace_chunk", "Create, overwrite, or append one chunk of text to a workspace file. Use this for long generated documents instead of putting the whole file into save_workspace_file.content. After the last chunk, call finalize_workspace_write.", workspaceChunkWriteParams()),
		openAIFunctionTool("finalize_workspace_write", "Finalize and verify a long chunked workspace write. Use after write_workspace_chunk for long documents, scripts, reports, or generated files before claiming completion.", workspaceFinalizeWriteParams()),
		openAIFunctionTool("manage_workspace_path", "Perform a validated workspace path operation: mkdir, rename, move, or trash. Use this instead of primitive file operation tools.", workspaceManageParams()),
		openAIFunctionTool("organize_workspace_matches", "Deterministically organize workspace items whose path/name contains a match text into a target folder. The backend creates the target folder if needed, moves matching files by default, and verifies the result. Use for requests like sorting episodes/files into one folder.", workspaceOrganizeParams()),
	}
}

func anthropicWorkspaceTools() []map[string]any {
	return []map[string]any{
		anthropicTool("find_workspace_files", "Find workspace files or folders by name/path query or glob pattern. Use first when the exact path is unknown.", workspaceFindParams()),
		anthropicTool("search_workspace_text", "Search text inside workspace files and return file paths, line numbers, and context. Use for finding symbols, phrases, or likely edit locations.", workspaceSearchTextParams()),
		anthropicTool("inspect_workspace", "Inspect workspace structure by listing files and folders. Prefer find_workspace_files or search_workspace_text for targeted discovery; use this for folder overview.", workspaceListParams()),
		anthropicTool("read_workspace_text", "Read a supported plain-text file from the current user's workspace, optionally by line range. Do not use for binary/document formats such as .docx; if only an unsupported document is available, say it is not readable by the current text tools.", workspaceReadParams()),
		anthropicTool("read_workspace_files", "Read multiple supported plain-text files from the current user's workspace in one call. Do not use for binary/document formats such as .docx.", workspaceReadFilesParams()),
		anthropicTool("edit_workspace_text", "Edit an existing text file with exact search/replace. Use after reading or searching the file. For new files or full overwrite, use save_workspace_file.", workspaceEditParams()),
		anthropicTool("replace_workspace_lines", "Replace a 1-based inclusive line range in an existing text file. Prefer this for scripts, long documents, duplicate headings, or larger rewrites after reading the target line range.", workspaceReplaceLinesParams()),
		anthropicTool("save_workspace_file", "Create or overwrite a workspace text file with backend validation. Use mode=create for new files and mode=overwrite for existing files.", workspaceSaveParams()),
		anthropicTool("write_workspace_chunk", "Create, overwrite, or append one chunk of text to a workspace file. Use this for long generated documents instead of putting the whole file into save_workspace_file.content. After the last chunk, call finalize_workspace_write.", workspaceChunkWriteParams()),
		anthropicTool("finalize_workspace_write", "Finalize and verify a long chunked workspace write. Use after write_workspace_chunk for long documents, scripts, reports, or generated files before claiming completion.", workspaceFinalizeWriteParams()),
		anthropicTool("manage_workspace_path", "Perform a validated workspace path operation: mkdir, rename, move, or trash. Use this instead of primitive file operation tools.", workspaceManageParams()),
		anthropicTool("organize_workspace_matches", "Deterministically organize workspace items whose path/name contains a match text into a target folder. The backend creates the target folder if needed, moves matching files by default, and verifies the result. Use for requests like sorting episodes/files into one folder.", workspaceOrganizeParams()),
	}
}

func workspaceFindParams() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":       map[string]any{"type": "string", "description": "Case-insensitive text that should appear in the file/folder name or workspace-relative path."},
			"pattern":     map[string]any{"type": "string", "description": "Optional glob pattern such as *.md or src/*.js. Use query for fuzzy path/name search."},
			"max_results": map[string]any{"type": "integer", "description": "Maximum results to return. Default 80, maximum 200."},
		},
	}
}

func workspaceSearchTextParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"query"},
		"properties": map[string]any{
			"query":         map[string]any{"type": "string", "description": "Text or regex to search for inside workspace text files."},
			"regex":         map[string]any{"type": "boolean", "description": "Treat query as a Go regular expression. Default false."},
			"context_lines": map[string]any{"type": "integer", "description": "Context lines before/after each match. Default 1, maximum 3."},
			"max_results":   map[string]any{"type": "integer", "description": "Maximum matches to return. Default 80, maximum 200."},
		},
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

func workspaceReadParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"path"},
		"properties": map[string]any{
			"path":       map[string]any{"type": "string", "description": "Workspace-relative supported plain-text file path. Binary/document files such as .docx are not supported by this tool."},
			"start_line": map[string]any{"type": "integer", "description": "Optional 1-based first line to read."},
			"end_line":   map[string]any{"type": "integer", "description": "Optional 1-based last line to read."},
			"max_chars":  map[string]any{"type": "integer", "description": "Maximum characters to return. Default 20000, maximum 50000."},
		},
	}
}

func workspaceReadFilesParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"paths"},
		"properties": map[string]any{
			"paths": map[string]any{
				"type":        "array",
				"description": "Workspace-relative supported plain-text file paths to read. Binary/document files such as .docx are not supported by this tool.",
				"items":       map[string]any{"type": "string"},
			},
			"start_line":         map[string]any{"type": "integer", "description": "Optional 1-based first line to read from each file."},
			"end_line":           map[string]any{"type": "integer", "description": "Optional 1-based last line to read from each file."},
			"max_chars_per_file": map[string]any{"type": "integer", "description": "Maximum characters to return per file. Default 20000, maximum 50000."},
		},
	}
}

func workspaceEditParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"path", "old_text", "new_text"},
		"properties": map[string]any{
			"path":        map[string]any{"type": "string", "description": "Workspace-relative existing text file path."},
			"old_text":    map[string]any{"type": "string", "description": "Exact text to replace. Must match the current file content."},
			"new_text":    map[string]any{"type": "string", "description": "Replacement text."},
			"replace_all": map[string]any{"type": "boolean", "description": "Replace every exact match. Default false; if multiple matches exist without this, the tool fails."},
		},
	}
}

func workspaceReplaceLinesParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"path", "start_line", "end_line", "new_text"},
		"properties": map[string]any{
			"path":       map[string]any{"type": "string", "description": "Workspace-relative existing text file path."},
			"start_line": map[string]any{"type": "integer", "description": "1-based first line to replace, inclusive. Read the range first."},
			"end_line":   map[string]any{"type": "integer", "description": "1-based last line to replace, inclusive."},
			"new_text":   map[string]any{"type": "string", "description": "Replacement text for exactly this line range. Include final newline only when the file should have one at that boundary."},
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
			"path": map[string]any{"type": "string", "description": "Workspace-relative file path."},
			"content": map[string]any{
				"type":        "string",
				"description": "Full short text content to save. For longer content, use write_workspace_chunk.",
			},
			"mode": map[string]any{
				"type":        "string",
				"enum":        []string{"create", "overwrite"},
				"description": "Use create only when the file must not already exist. Use overwrite only for an existing file.",
			},
		},
	}
}

func workspaceChunkWriteParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"path", "content", "mode", "chunk_index"},
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "Workspace-relative text file path."},
			"content": map[string]any{
				"type":        "string",
				"description": "One text chunk to write. Keep each chunk under about 4000 characters.",
			},
			"chunk_index": map[string]any{
				"type":        "integer",
				"description": "1-based chunk number. The first create/overwrite chunk must be 1; each append increments it.",
			},
			"mode": map[string]any{
				"type":        "string",
				"enum":        []string{"create", "overwrite", "append"},
				"description": "Use create for the first chunk of a new file, overwrite for the first chunk of an existing file, and append for each following chunk.",
			},
		},
	}
}

func workspaceFinalizeWriteParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"path", "expected_chunks", "expected_lines", "expected_chars"},
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Workspace-relative text file path previously written with write_workspace_chunk.",
			},
			"expected_chunks": map[string]any{
				"type":        "integer",
				"description": "Total number of write_workspace_chunk calls expected for this file in the current run.",
			},
			"expected_lines": map[string]any{
				"type":        "integer",
				"description": "Expected final line count of the file.",
			},
			"expected_chars": map[string]any{
				"type":        "integer",
				"description": "Expected final character count of the file.",
			},
			"final_hash": map[string]any{
				"type":        "string",
				"description": "Optional expected final sha256 hash of the file content.",
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
