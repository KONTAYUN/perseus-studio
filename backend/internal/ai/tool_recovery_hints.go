package ai

import "strings"

func toolFailureRecoveryHint(toolName, errorText string) string {
	lowerError := strings.ToLower(errorText)
	editProtocol := manifestRecommendedProtocol(toolName)
	if editProtocol == "" && toolName == "apply_workspace_patch" {
		editProtocol = manifestPrimaryEditProtocol()
	}
	patchPhrase := "*** Begin Patch text patch"
	if editProtocol == string(ToolTransportAssistantTextPatch) {
		patchPhrase = "*** Begin Patch text patch"
	}
	switch toolName {
	case "find_workspace_files":
		return "Search returned paths. Use read_workspace_note_view for .html notes, read_workspace_text for ordinary text files, or search_workspace_text for symbols/text."
	case "search_workspace_text":
		return "Search returned matching lines. Read the relevant file/range before editing unless the exact replacement is already fully clear."
	case "read_workspace_note_view", "read_workspace_text", "read_workspace_files":
		if strings.Contains(lowerError, "unsupported file type") {
			return "This workspace tool only reads supported plain-text files. Do not retry the same binary/document path with read_workspace_text; explain that this file type is not readable by the current workspace text tools unless the user provides text content or a supported text export."
		}
		if strings.Contains(lowerError, "not exist") || strings.Contains(lowerError, "no such") || strings.Contains(lowerError, "不存在") {
			return "One or more file paths may be wrong or stale. Inspect or find the workspace paths, then retry reading. Do not claim you read files until the read tool succeeds."
		}
		return "Check the path and retry only after locating the file inside the current user's workspace."
	case "edit_workspace_text":
		if strings.Contains(lowerError, "matched") || strings.Contains(lowerError, "not found") {
			return "Read the current file or target line range again, then submit a corrected " + patchPhrase + " before claiming completion."
		}
		return "Verify the file path and current content before continuing. For new turns, submit file changes as a " + patchPhrase + "."
	case "apply_workspace_patch":
		return "Read the target file or range again, then submit a corrected " + patchPhrase + ". Do not report the edit as complete until the patch succeeds."
	case "replace_workspace_lines":
		return "Read the target file/range again, then submit a corrected " + patchPhrase + ". Do not report the edit as complete until the change succeeds."
	case "save_workspace_file":
		if strings.Contains(lowerError, "not exist") || strings.Contains(lowerError, "no such") || strings.Contains(lowerError, "不存在") {
			return "The target file does not exist. Inspect or find the correct path, then submit the intended file change as a " + patchPhrase + "."
		}
		return "Read the target file again if needed, then submit the intended file change as a " + patchPhrase + ". Do not report success until the change succeeds."
	case "write_workspace_chunk":
		return "This compatibility write must continue in order. For new turns, submit the intended file change as a " + patchPhrase + ". Do not claim completion until the change verifies."
	case "finalize_workspace_write":
		return "Read the file or continue the existing compatibility write sequence before claiming completion. For new turns, submit file changes as a " + patchPhrase + "."
	case "manage_workspace_path":
		return "Verify the target path with inspect_workspace before retrying. If the request is ambiguous or destructive, ask the user to confirm the exact target."
	case "organize_workspace_matches":
		return "Narrow the source_path or match_text and retry only if the user's intent is clear. Do not claim files were organized until verification succeeds."
	case "run_workspace_command":
		if strings.Contains(lowerError, "safety policy") || strings.Contains(lowerError, "rejected") {
			return "The command was blocked by the workspace safety policy. Do not try to bypass it; explain the limitation or choose a safer workspace-scoped command."
		}
		return "Use the error output to adjust the command, keeping cwd inside the workspace. Do not report command success until run_workspace_command succeeds."
	case "perseus_search_web", "web_search":
		return "Search failed. Try a shorter or more neutral query once, or continue without web results and clearly say that live search failed."
	case "web_extract":
		return "Extraction failed. Verify the URL from search results, try another result URL, or continue with the available search summary."
	case "read_skill":
		return "Verify the skill id with list_skills, then retry only for selected or relevant skills."
	case "load_perseus_toolset":
		return "Load only a valid toolset name from the available toolset list."
	default:
		return "Inspect the error, correct the tool arguments, and retry only if the next action is safe and clearly requested."
	}
}
