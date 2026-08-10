package ai

import (
	"encoding/json"
	"strings"
)

func (r *agentRuntime) handleTaskPlanUpdate(toolCallID, argumentsJSON string) (string, bool) {
	var update taskPlanUpdate
	if err := json.Unmarshal([]byte(argumentsJSON), &update); err != nil {
		return r.toolErrorJSON("update_task_plan", "invalid update_task_plan arguments: "+err.Error()), true
	}
	update = normalizeTaskPlanUpdate(update)
	if update.Goal == "" {
		return r.toolErrorJSON("update_task_plan", "goal is required"), true
	}
	if update.Status != "completed" && update.CurrentStep == "" {
		return r.toolErrorJSON("update_task_plan", "current_step is required while the plan is active"), true
	}
	if update.Status == "completed" && (update.CurrentStep != "" || len(update.NextSteps) > 0 || len(update.MissingInfo) > 0) {
		return r.toolErrorJSON("update_task_plan", "a completed plan cannot have a current step, next steps, or missing information"), true
	}
	r.pendingTaskPlan = &update
	r.activeTaskPlan = &update
	r.strategyLedger.PlanRevision++
	r.strategyLedger.MutationFailuresWithoutProgress = 0
	emitToolLifecycleEvent(r.opts, "task_plan_updated", "info", map[string]any{
		"tool_call_id": toolCallID,
		"goal":         update.Goal, "local_targets": update.LocalTargets,
		"external_targets": update.ExternalTargets, "current_step": update.CurrentStep,
		"next_steps": update.NextSteps,
	})
	return mustJSON(map[string]any{"status": "updated", "task_plan": update}), false
}

func (r *agentRuntime) persistPendingTaskPlan(toolCallID string) {
	if r == nil || r.pendingTaskPlan == nil {
		return
	}
	update := *r.pendingTaskPlan
	r.pendingTaskPlan = nil
	r.appendKernelItem(AgentTurnItem{
		Kind: "task_plan", Phase: "planning", Status: "completed", ToolCallID: toolCallID, ToolName: "update_task_plan", Content: mustJSON(update),
	})
	if len(update.ExternalTargets) == 0 || len(update.VerifiedFacts) == 0 {
		return
	}
	r.appendKernelItem(AgentTurnItem{
		Kind: "external_observation", Phase: "planning", Status: "completed",
		ToolCallID: toolCallID, ToolName: "update_task_plan",
		Content: mustJSON(map[string]any{
			"source": "task_plan", "external_targets": update.ExternalTargets,
			"verified_facts": update.VerifiedFacts,
		}),
	})
}

func normalizeTaskPlanUpdate(update taskPlanUpdate) taskPlanUpdate {
	update.Goal = truncatePlainRunes(strings.TrimSpace(update.Goal), 1200)
	update.Status = strings.ToLower(strings.TrimSpace(update.Status))
	if update.Status == "" {
		update.Status = "in_progress"
	}
	if update.Status != "in_progress" && update.Status != "completed" {
		update.Status = "in_progress"
	}
	update.CurrentStep = truncatePlainRunes(strings.TrimSpace(update.CurrentStep), 600)
	update.LocalTargets = normalizeTaskPlanList(update.LocalTargets, 24, 500)
	update.ExternalTargets = normalizeTaskPlanList(update.ExternalTargets, 24, 500)
	update.CompletedSteps = normalizeTaskPlanList(update.CompletedSteps, 40, 800)
	update.NextSteps = normalizeTaskPlanList(update.NextSteps, 20, 800)
	update.VerifiedFacts = normalizeTaskPlanList(update.VerifiedFacts, 60, 1000)
	update.MissingInfo = normalizeTaskPlanList(update.MissingInfo, 30, 800)
	return update
}

func normalizeTaskPlanList(values []string, limit, maxRunes int) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = truncatePlainRunes(strings.TrimSpace(value), maxRunes)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
		if len(result) >= limit {
			break
		}
	}
	return result
}
