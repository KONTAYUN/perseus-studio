package ai

import "strings"

func toolFailureRecoveryHint(toolName, errorText string) string {
	lowerError := strings.ToLower(errorText)
	switch toolName {
	case "find_workspace_files":
		return "Search returned paths. Use read_workspace_text before relying on file contents, or search_workspace_text for symbols/text."
	case "search_workspace_text":
		return "Search returned matching lines. Read the relevant file/range before editing unless the exact replacement is already fully clear."
	case "read_workspace_text", "read_workspace_files":
		if strings.Contains(lowerError, "unsupported file type") {
			return "This workspace tool only reads supported plain-text files. Do not retry the same binary/document path with read_workspace_text; explain that this file type is not readable by the current workspace text tools unless the user provides text content or a supported text export."
		}
		if strings.Contains(lowerError, "not exist") || strings.Contains(lowerError, "no such") || strings.Contains(lowerError, "不存在") {
			return "One or more file paths may be wrong or stale. Inspect or find the workspace paths, then retry reading. Do not claim you read files until the read tool succeeds."
		}
		return "Check the path and retry only after locating the file inside the current user's workspace."
	case "edit_workspace_text":
		if strings.Contains(lowerError, "matched") || strings.Contains(lowerError, "not found") {
			return "Read the current file or target line range. For scripts, long documents, duplicate headings, or larger rewrites, use replace_workspace_lines with line numbers instead of retrying a large exact old_text."
		}
		return "Verify the file path and current content before retrying. Do not report the edit as complete until edit_workspace_text succeeds."
	case "replace_workspace_lines":
		return "Read the target file/range again, correct start_line/end_line, then retry replace_workspace_lines. Do not report the edit as complete until the line replacement succeeds."
	case "save_workspace_file":
		if strings.Contains(lowerError, "not exist") || strings.Contains(lowerError, "no such") || strings.Contains(lowerError, "不存在") {
			return "The target file does not exist. If the user wanted a new file, retry save_workspace_file with mode=create; otherwise inspect/read the correct existing file before overwriting."
		}
		return "Read the target file again if needed, adjust the save arguments, then retry. Do not report success until save_workspace_file succeeds."
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
