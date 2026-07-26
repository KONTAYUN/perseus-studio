package ai

func openAITaskPlanTool() map[string]any {
	return openAIFunctionTool(
		"update_task_plan",
		"Update durable task state for multi-step work. Record the goal, local workspace targets, external systems, completed work, current step, next steps, verified facts, and missing information. Do not infer task scope from keywords; use the user's request and verified tool results.",
		taskPlanParams(),
	)
}

func anthropicTaskPlanTool() map[string]any {
	return anthropicTool(
		"update_task_plan",
		"Update durable task state for multi-step work. Record the goal, local workspace targets, external systems, completed work, current step, next steps, verified facts, and missing information. Do not infer task scope from keywords; use the user's request and verified tool results.",
		taskPlanParams(),
	)
}

func taskPlanParams() map[string]any {
	stringList := func(description string) map[string]any {
		return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": description}
	}
	return map[string]any{
		"type":     "object",
		"required": []string{"goal", "current_step", "next_steps"},
		"properties": map[string]any{
			"goal":             map[string]any{"type": "string", "description": "The user's durable task goal."},
			"local_targets":    stringList("Workspace files or folders involved in the task."),
			"external_targets": stringList("External servers, services, websites, or environments involved in the task."),
			"completed_steps":  stringList("Steps verified as completed."),
			"current_step":     map[string]any{"type": "string", "description": "The single step currently being worked on."},
			"next_steps":       stringList("Concrete next steps in order."),
			"verified_facts":   stringList("Facts supported by user evidence or successful tool results."),
			"missing_info":     stringList("Information still needed to continue."),
		},
	}
}
