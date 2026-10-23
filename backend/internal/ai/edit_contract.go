package ai

// WorkspacePatchExecutionRule is shared by tool schemas and background prompts.
// Patch syntax is data in a tool argument, never an assistant-text action.
const WorkspacePatchExecutionRule = "Call the structured apply_workspace_patch tool with the complete *** Begin Patch / *** End Patch block in its patch argument. Assistant text and code blocks do not execute file changes."
