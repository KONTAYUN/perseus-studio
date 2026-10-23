package ai

// A successful unrelated read is not evidence that a failed verification
// command now succeeds. Keep the exact command unresolved until it is rerun
// successfully, or let the agent end honestly with partial/blocked.
func (r *agentRuntime) recordCommandOutcome(event ToolEvent) {
	if event.ToolName != "run_workspace_command" || event.Status != "success" || event.ExecutionOutcome == "" {
		return
	}
	key := event.Cwd + "\x00" + event.Command
	if event.ExecutionOutcome == "success" {
		delete(r.unresolvedCommands, key)
		return
	}
	if r.unresolvedCommands == nil {
		r.unresolvedCommands = map[string]string{}
	}
	r.unresolvedCommands[key] = event.ExecutionOutcome
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	out := map[string]string{}
	for k, v := range input {
		out[k] = v
	}
	return out
}
