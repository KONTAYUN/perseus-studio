package ai

// Labels describe observed execution state, independent of error-message wording.
func completedToolLabel(event ToolEvent) string {
	if event.Status != "success" {
		return "工具执行失败"
	}
	if event.ToolName == "run_workspace_command" {
		switch event.ExecutionOutcome {
		case "failed":
			return "命令执行失败"
		case "timed_out":
			return "命令执行超时"
		case "canceled":
			return "命令已取消"
		case "observation_incomplete":
			return "命令结果待核对"
		}
	}
	return userFriendlyToolLabel(event.ToolName)
}
