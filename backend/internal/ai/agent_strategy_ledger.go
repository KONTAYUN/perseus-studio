package ai

import (
	"encoding/json"
	"fmt"
	"strings"
)

func cloneStrategyLedger(value StrategyLedger) StrategyLedger {
	out := value
	out.Failures = make(map[string]StrategyFailure, len(value.Failures))
	for key, failure := range value.Failures {
		out.Failures[key] = failure
	}
	return out
}

func strategyTargetStateHash(argumentsJSON string) string {
	var payload map[string]any
	if json.Unmarshal([]byte(argumentsJSON), &payload) != nil {
		return ""
	}
	for _, key := range []string{"document_version", "doc_hash", "file_hash", "base_hash", "expected_hash"} {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func strategyFailureKey(toolName, argumentsJSON string, errorCode ToolErrorCode) string {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return ""
	}
	return strings.Join([]string{
		toolName,
		mutationOperationHash(toolName, argumentsJSON),
		strategyTargetStateHash(argumentsJSON),
		string(errorCode),
	}, "\x00")
}

func (r *agentRuntime) recordStrategyFailure(toolName, argumentsJSON string, failure *ToolError) {
	if r == nil || failure == nil {
		return
	}
	if failure.Phase == "preflight" || failure.Phase == "protocol" {
		return
	}
	entry, ok := toolManifestEntry(toolName)
	if !ok || !entry.Mutation {
		return
	}
	if r.strategyLedger.Failures == nil {
		r.strategyLedger.Failures = map[string]StrategyFailure{}
	}
	key := strategyFailureKey(toolName, argumentsJSON, failure.Code)
	if key == "" {
		return
	}
	current := r.strategyLedger.Failures[key]
	current.ToolName = toolName
	current.Target = firstNonEmpty(failure.Target, toolTargetSummary(toolName, argumentsJSON))
	current.OperationHash = mutationOperationHash(toolName, argumentsJSON)
	current.ErrorCode = failure.Code
	current.TargetStateHash = strategyTargetStateHash(argumentsJSON)
	current.Count++
	current.LastRound = r.currentRound
	current.PlanRevision = r.strategyLedger.PlanRevision
	current.RecommendedAction = failure.RecommendedAction
	r.strategyLedger.Failures[key] = current
	r.strategyLedger.MutationFailuresWithoutProgress++
	emitToolLifecycleEvent(r.opts, "agent_strategy_failed", "warn", map[string]any{
		"tool_name":                          toolName,
		"target":                             current.Target,
		"error_code":                         current.ErrorCode,
		"strategy_failure_count":             current.Count,
		"mutation_failures_without_progress": r.strategyLedger.MutationFailuresWithoutProgress,
		"plan_revision":                      r.strategyLedger.PlanRevision,
		"recommended_action":                 current.RecommendedAction,
	})
}

func (r *agentRuntime) rejectRepeatedFailedStrategy(call agentToolCall) error {
	if r == nil {
		return nil
	}
	entry, ok := toolManifestEntry(call.Name)
	if !ok || !entry.Mutation {
		return nil
	}
	if r.strategyLedger.MutationFailuresWithoutProgress >= 3 {
		typed := newToolError(
			ToolErrorRepeatedStrategy,
			"three mutation strategies failed without verified target progress; update the task plan before attempting another mutation",
			nil,
		)
		typed.Target = toolTargetSummary(call.Name, executionToolArgumentsJSON(call))
		typed.RecommendedAction = "Call update_task_plan with the failed strategy, the verified facts, and a genuinely different next action."
		typed.Details = map[string]any{
			"mutation_failures_without_progress": r.strategyLedger.MutationFailuresWithoutProgress,
			"plan_revision":                      r.strategyLedger.PlanRevision,
			"required_tool":                      "update_task_plan",
		}
		return typed
	}
	argumentsJSON := executionToolArgumentsJSON(call)
	operationHash := mutationOperationHash(call.Name, argumentsJSON)
	targetStateHash := strategyTargetStateHash(argumentsJSON)
	var failure StrategyFailure
	exists := false
	for _, candidate := range r.strategyLedger.Failures {
		if candidate.ToolName == call.Name &&
			candidate.OperationHash == operationHash &&
			candidate.TargetStateHash == targetStateHash &&
			candidate.PlanRevision == r.strategyLedger.PlanRevision {
			failure = candidate
			exists = true
			break
		}
	}
	if !exists {
		return nil
	}
	typed := newToolError(
		ToolErrorRepeatedStrategy,
		fmt.Sprintf("the same failed strategy for %s cannot be executed again unchanged", firstNonEmpty(failure.Target, call.Name)),
		nil,
	)
	typed.Target = failure.Target
	typed.RecommendedAction = firstNonEmpty(failure.RecommendedAction, "Read the current target and choose a different tool or operation.")
	typed.Details = map[string]any{
		"previous_tool":       failure.ToolName,
		"previous_error_code": failure.ErrorCode,
		"previous_round":      failure.LastRound,
		"operation_hash":      failure.OperationHash,
		"target_state_hash":   failure.TargetStateHash,
	}
	return typed
}

func (r *agentRuntime) recordStrategySuccess(call agentToolCall, event ToolEvent) {
	if r == nil || event.Status != "success" {
		return
	}
	if isReadWorkspaceTool(call.Name) {
		// A successful prerequisite read changes the recovery state even when
		// it does not mutate a file. It makes a previously blocked target
		// eligible for a fresh preflight.
		r.strategyLedger.MutationFailuresWithoutProgress = 0
		r.strategyLedger.Failures = map[string]StrategyFailure{}
		return
	}
	entry, ok := toolManifestEntry(call.Name)
	if !ok || !entry.Mutation {
		return
	}
	if len(event.ChangedPaths) > 0 {
		r.strategyLedger.MutationFailuresWithoutProgress = 0
		r.strategyLedger.Failures = map[string]StrategyFailure{}
	}
}
