package ai

func openAIWorkspaceTools() []map[string]any {
	var tools []map[string]any
	for _, name := range providerVisibleToolNamesForToolset("workspace", "admin") {
		if tool := openAIWorkspaceToolByName(name); tool != nil {
			tools = append(tools, tool)
		}
	}
	return tools
}

func anthropicWorkspaceTools() []map[string]any {
	var tools []map[string]any
	for _, name := range providerVisibleToolNamesForToolset("workspace", "admin") {
		if tool := anthropicWorkspaceToolByName(name); tool != nil {
			tools = append(tools, tool)
		}
	}
	return tools
}

func openAIWorkspaceToolByName(name string) map[string]any {
	switch name {
	case "find_workspace_files":
		return openAIFunctionTool(name, "Find workspace files or folders by name/path query or glob pattern. Use first when the exact path is unknown.", workspaceFindParams())
	case "search_workspace_text":
		return openAIFunctionTool(name, "Search text inside workspace files and return file paths, line numbers, and context. Use for finding symbols, phrases, or likely edit locations.", workspaceSearchTextParams())
	case "inspect_workspace":
		return openAIFunctionTool(name, "Inspect workspace structure by listing files and folders. Prefer find_workspace_files or search_workspace_text for targeted discovery; use this for folder overview.", workspaceListParams())
	case "read_workspace_text":
		return openAIFunctionTool(name, "Read a supported plain-text file from the current user's workspace, optionally by line range. .html notes are automatically redirected to read_workspace_note_view mode=index; do not read HTML notes by line range.", workspaceReadParams())
	case "read_workspace_note_view":
		return openAIFunctionTool(name, "Read an HTML note as a model-friendly view. Use mode=index to locate blocks/sections before exact HTML work; then mode=block or mode=section with view=raw. Prefer structure/text/format for ordinary content understanding.", workspaceNoteViewParams())
	case "read_workspace_files":
		return openAIFunctionTool(name, "Read multiple supported plain-text files from the current user's workspace in one call. .html notes are returned as note indexes, not line-range text.", workspaceReadFilesParams())
	case "edit_workspace_note_blocks":
		return openAIFunctionTool(name, "Atomically edit an existing HTML note. Use the smallest operation in this order: replace_text, replace_inner_html, single-block replace_html, then delete_block_range for contiguous removal. Use the full document_version from read_workspace_note_view. The batch is sanitized, verified, and written as one history version.", workspaceNoteBlockEditParams())
	case "create_workspace_note":
		return openAIFunctionTool(name, "Create a new .html note from an HTML fragment. The target must not exist and the backend sanitizes the fragment.", workspaceNoteCreateParams())
	case "manage_workspace_path":
		return openAIFunctionTool(name, "Perform a validated workspace path operation: mkdir, rename, move, copy, or trash. Use copy for deterministic file duplication without asking the model to rewrite whole files.", workspaceManageParams())
	case "organize_workspace_matches":
		return openAIFunctionTool(name, "Deterministically organize workspace items whose path/name contains a match text into a target folder. The backend creates the target folder if needed, moves matching files by default, and verifies the result. Use for requests like sorting episodes/files into one folder.", workspaceOrganizeParams())
	case "apply_workspace_patch":
		return openAIFunctionTool(name, "Apply one complete workspace patch to create, update, delete, or move text files. Use this as the primary file content editing action. The backend parses, verifies, and applies the patch atomically.", workspacePatchParams())
	default:
		return nil
	}
}

func anthropicWorkspaceToolByName(name string) map[string]any {
	switch name {
	case "find_workspace_files":
		return anthropicTool(name, "Find workspace files or folders by name/path query or glob pattern. Use first when the exact path is unknown.", workspaceFindParams())
	case "search_workspace_text":
		return anthropicTool(name, "Search text inside workspace files and return file paths, line numbers, and context. Use for finding symbols, phrases, or likely edit locations.", workspaceSearchTextParams())
	case "inspect_workspace":
		return anthropicTool(name, "Inspect workspace structure by listing files and folders. Prefer find_workspace_files or search_workspace_text for targeted discovery; use this for folder overview.", workspaceListParams())
	case "read_workspace_text":
		return anthropicTool(name, "Read a supported plain-text file from the current user's workspace, optionally by line range. .html notes are automatically redirected to read_workspace_note_view mode=index; do not read HTML notes by line range.", workspaceReadParams())
	case "read_workspace_note_view":
		return anthropicTool(name, "Read an HTML note as a model-friendly view. Use mode=index to locate blocks/sections before exact HTML work; then mode=block or mode=section with view=raw. Prefer structure/text/format for ordinary content understanding.", workspaceNoteViewParams())
	case "read_workspace_files":
		return anthropicTool(name, "Read multiple supported plain-text files from the current user's workspace in one call. .html notes are returned as note indexes, not line-range text.", workspaceReadFilesParams())
	case "edit_workspace_note_blocks":
		return anthropicTool(name, "Atomically edit an existing HTML note. Use the smallest operation in this order: replace_text, replace_inner_html, single-block replace_html, then delete_block_range for contiguous removal. Use the full document_version from read_workspace_note_view. The batch is sanitized, verified, and written as one history version.", workspaceNoteBlockEditParams())
	case "create_workspace_note":
		return anthropicTool(name, "Create a new .html note from an HTML fragment. The target must not exist and the backend sanitizes the fragment.", workspaceNoteCreateParams())
	case "manage_workspace_path":
		return anthropicTool(name, "Perform a validated workspace path operation: mkdir, rename, move, copy, or trash. Use copy for deterministic file duplication without asking the model to rewrite whole files.", workspaceManageParams())
	case "organize_workspace_matches":
		return anthropicTool(name, "Deterministically organize workspace items whose path/name contains a match text into a target folder. The backend creates the target folder if needed, moves matching files by default, and verifies the result. Use for requests like sorting episodes/files into one folder.", workspaceOrganizeParams())
	case "apply_workspace_patch":
		return anthropicTool(name, "Apply one complete workspace patch to create, update, delete, or move text files. Use this as the primary file content editing action. The backend parses, verifies, and applies the patch atomically.", workspacePatchParams())
	default:
		return nil
	}
}

func workspaceToolInputSchemaByName(name string) map[string]any {
	switch name {
	case "find_workspace_files":
		return workspaceFindParams()
	case "search_workspace_text":
		return workspaceSearchTextParams()
	case "inspect_workspace":
		return workspaceListParams()
	case "read_workspace_text":
		return workspaceReadParams()
	case "read_workspace_note_view":
		return workspaceNoteViewParams()
	case "read_workspace_files":
		return workspaceReadFilesParams()
	case "edit_workspace_note_blocks":
		return workspaceNoteBlockEditParams()
	case "create_workspace_note":
		return workspaceNoteCreateParams()
	case "manage_workspace_path":
		return workspaceManageParams()
	case "organize_workspace_matches":
		return workspaceOrganizeParams()
	case "apply_workspace_patch":
		return workspacePatchParams()
	case "edit_workspace_text":
		return workspaceEditParams()
	case "replace_workspace_lines":
		return workspaceReplaceLinesParams()
	case "save_workspace_file":
		return workspaceSaveParams()
	case "write_workspace_chunk":
		return workspaceChunkWriteParams()
	case "finalize_workspace_write":
		return workspaceFinalizeWriteParams()
	default:
		return nil
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
			"path_scope":    map[string]any{"type": "string", "description": "Optional workspace-relative file or folder path to limit the search."},
			"include_globs": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional glob patterns to include, such as *.md or scripts/*.txt."},
			"exclude_globs": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional glob patterns to exclude."},
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
			"path":       map[string]any{"type": "string", "description": "Workspace-relative supported plain-text file path. .html notes are redirected to read_workspace_note_view mode=index; binary/document files such as .docx are not supported."},
			"start_line": map[string]any{"type": "integer", "description": "Optional 1-based first line to read."},
			"end_line":   map[string]any{"type": "integer", "description": "Optional 1-based last line to read."},
			"max_chars":  map[string]any{"type": "integer", "description": "Maximum characters to return. Default 20000, maximum 50000."},
		},
	}
}

func workspaceNoteViewParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"path"},
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Workspace-relative .html note path.",
			},
			"view": map[string]any{
				"type":        "string",
				"enum":        []string{"text", "structure", "format", "raw"},
				"description": "text returns plain body text; structure returns headings/paragraphs/lists/tables/images; format returns lightweight formatting hints; raw returns HTML for precise edits. Default structure.",
			},
			"mode": map[string]any{
				"type":        "string",
				"enum":        []string{"full", "index", "section", "block"},
				"description": "full preserves the legacy whole-view behavior. index returns block/section metadata without large content. section reads one section_id. block reads one block_id. Default full.",
			},
			"block_id": map[string]any{
				"type":        "string",
				"description": "Required when mode=block. Use a block_id returned by mode=index.",
			},
			"block_ids": map[string]any{
				"type":        "array",
				"maxItems":    20,
				"items":       map[string]any{"type": "string"},
				"description": "Optional batch of block ids for mode=block. Use this instead of multiple independent calls when several blocks are needed.",
			},
			"section_id": map[string]any{
				"type":        "string",
				"description": "Required when mode=section. Use a section_id returned by mode=index.",
			},
			"section_ids": map[string]any{
				"type":        "array",
				"maxItems":    20,
				"items":       map[string]any{"type": "string"},
				"description": "Optional batch of section ids for mode=section.",
			},
			"doc_hash": map[string]any{
				"type":        "string",
				"description": "Legacy version field retained for old runs. Prefer document_version.",
			},
			"document_version": map[string]any{
				"type":        "string",
				"description": "Full document_version from mode=index. If the note changed, stale ids are rejected and the note must be indexed again.",
			},
			"start_block": map[string]any{
				"type":        "integer",
				"description": "Optional 1-based first block to return for mode=index pagination. Use next_start_block from a prior index result.",
			},
			"max_blocks": map[string]any{
				"type":        "integer",
				"description": "Maximum blocks to return for mode=index. Default 80, maximum 500.",
			},
			"max_chars": map[string]any{
				"type":        "integer",
				"description": "Maximum characters to return for mode=block or mode=section. Default 20000, maximum 50000.",
			},
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
				"description": "Workspace-relative supported plain-text file paths to read. .html notes are returned as read_workspace_note_view mode=index summaries; binary/document files such as .docx are not supported.",
				"items":       map[string]any{"type": "string"},
			},
			"start_line":         map[string]any{"type": "integer", "description": "Optional 1-based first line to read from each file."},
			"end_line":           map[string]any{"type": "integer", "description": "Optional 1-based last line to read from each file."},
			"max_chars_per_file": map[string]any{"type": "integer", "description": "Maximum characters to return per file. Default 20000, maximum 50000."},
		},
	}
}

func workspaceNoteBlockEditParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"path", "edits"},
		"properties": map[string]any{
			"path":             map[string]any{"type": "string", "description": "Workspace-relative existing .html note path."},
			"doc_hash":         map[string]any{"type": "string", "description": "Legacy version field retained for old runs."},
			"document_version": map[string]any{"type": "string", "description": "Current full document_version returned by read_workspace_note_view."},
			"edits": map[string]any{
				"type":        "array",
				"minItems":    1,
				"description": "Atomic block edits. Every block_id may appear only once.",
				"items": map[string]any{
					"type":     "object",
					"required": []string{"block_id", "operation"},
					"properties": map[string]any{
						"block_id":   map[string]any{"type": "string", "description": "Block id from the current note index."},
						"segment_id": map[string]any{"type": "string", "description": "Optional segment_id from a raw block read. Use it to constrain replace_text to one HTML text node."},
						"operation": map[string]any{
							"type": "string",
							"enum": []string{"replace_text", "replace_inner_html", "replace_html", "insert_before", "insert_after", "delete", "delete_block_range"},
						},
						"end_block_id":     map[string]any{"type": "string", "description": "Required for delete_block_range. Inclusive final block id from the same document version."},
						"html":             map[string]any{"type": "string", "description": "Required for replace_inner_html, replace_html, insert_before, and insert_after. Must be an HTML fragment."},
						"find":             map[string]any{"type": "string", "description": "Required for replace_text. Exact text to find inside one text node of the selected block; cross-node phrases require replace_inner_html."},
						"replace":          map[string]any{"type": "string", "description": "Replacement text for replace_text. May be empty."},
						"expected_matches": map[string]any{"type": "integer", "minimum": 1, "description": "Exact number of text-node matches required for replace_text. Defaults to 1."},
					},
				},
			},
		},
	}
}

func workspaceNoteCreateParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"path", "html"},
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "Workspace-relative new .html note path. Existing files are never overwritten."},
			"html": map[string]any{"type": "string", "description": "HTML fragment to store as the note body."},
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

func workspacePatchParams() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"patch"},
		"properties": map[string]any{
			"patch": map[string]any{
				"type":        "string",
				"description": "Complete patch beginning with *** Begin Patch and ending with *** End Patch. Pass it only as this structured tool argument; never emit it as ordinary assistant text.",
			},
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
				"description": "Compatibility content for historical save calls. New model turns should use apply_workspace_patch for ordinary text files or edit_workspace_note_blocks for HTML notes.",
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
				"description": "Text to append for compatibility with existing chunked runs. Prefer the structured apply_workspace_patch tool for new ordinary text edits.",
			},
			"chunk_index": map[string]any{
				"type":        "integer",
				"description": "1-based segment number. The first create/overwrite segment must be 1; each append increments it.",
			},
			"mode": map[string]any{
				"type":        "string",
				"enum":        []string{"create", "overwrite", "append"},
				"description": "Use create for the first segment of a new file, overwrite for the first segment of an existing file, and append for each following segment.",
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
				"description": "Workspace-relative text file path previously written with the compatibility write tool.",
			},
			"expected_chunks": map[string]any{
				"type":        "integer",
				"description": "Total number of compatibility write calls expected for this file in the current run.",
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
				"enum":        []string{"mkdir", "rename", "move", "copy", "trash"},
				"description": "Workspace path operation to perform.",
			},
			"path":            map[string]any{"type": "string", "description": "Existing path for rename/move/copy/trash, or folder path to create for mkdir."},
			"new_name":        map[string]any{"type": "string", "description": "Required for rename. New leaf name only, not a parent path."},
			"new_parent_path": map[string]any{"type": "string", "description": "Required for move. Destination folder path, empty string means workspace root."},
			"destination_path": map[string]any{
				"type":        "string",
				"description": "Required for copy. Exact destination file path. The backend refuses to overwrite an existing path.",
			},
		},
	}
}
