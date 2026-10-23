package ai

// Local recovery guidance consumes typed identity, never translated error text.
func toolFailureRecoveryHint(toolName string, err error) string {
	code := toolFailureClass(toolName, err)
	if toolName == "apply_workspace_patch" && code == "workspace_patch_apply_failed" {
		return "The patch was captured but exact preflight matching failed. Read the current range and submit a smaller exact *** Begin Patch / *** End Patch block in the apply_workspace_patch patch argument. Decode JSON escapes; numbered_content prefixes are not file text. Every hunk line has a prefix: space for context, - for removal, + for addition. A line '--' removes a literal '-' line. Do not repeat an unchanged rejected patch."
	}
	return recoveryHintForFailure(code)
}
