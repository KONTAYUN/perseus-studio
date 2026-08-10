package ai

import "testing"

func TestStrategyLedgerRejectsUnchangedFailedMutation(t *testing.T) {
	runtime := &agentRuntime{currentRound: 3}
	args := `{"patch":"*** Begin Patch\n*** Update File: draft.txt\n@@\n-old\n+new\n*** End Patch"}`
	failure := newToolError(ToolErrorPatchApplyFailed, "patch did not match", nil)
	failure.Target = "draft.txt"
	runtime.recordStrategyFailure("apply_workspace_patch", args, failure)

	err := runtime.rejectRepeatedFailedStrategy(agentToolCall{
		Name:          "apply_workspace_patch",
		ArgumentsJSON: args,
	})
	if err == nil {
		t.Fatal("expected unchanged failed strategy to be rejected")
	}
	typed := normalizeToolError(err)
	if typed.Code != ToolErrorRepeatedStrategy {
		t.Fatalf("expected repeated strategy code, got %s", typed.Code)
	}
	if typed.Target != "draft.txt" {
		t.Fatalf("expected target to be preserved, got %q", typed.Target)
	}
}

func TestStrategyLedgerRequiresPlanUpdateAfterThreeMutationFailures(t *testing.T) {
	runtime := &agentRuntime{
		strategyLedger: StrategyLedger{
			MutationFailuresWithoutProgress: 3,
			PlanRevision:                    1,
		},
	}
	err := runtime.rejectRepeatedFailedStrategy(agentToolCall{
		Name:          "create_workspace_note",
		ArgumentsJSON: `{"path":"draft.html","html":"<p>Draft</p>"}`,
	})
	if err == nil {
		t.Fatal("expected plan update requirement")
	}
	typed := normalizeToolError(err)
	if typed.Code != ToolErrorRepeatedStrategy || typed.Details["required_tool"] != "update_task_plan" {
		t.Fatalf("unexpected strategy error: %#v", typed)
	}
}

func TestStrategyLedgerResetsAfterVerifiedMutation(t *testing.T) {
	runtime := &agentRuntime{
		strategyLedger: StrategyLedger{MutationFailuresWithoutProgress: 2},
	}
	runtime.recordStrategySuccess(agentToolCall{Name: "edit_workspace_note_blocks"}, ToolEvent{
		Status:       "success",
		ChangedPaths: []string{"draft.html"},
	})
	if runtime.strategyLedger.MutationFailuresWithoutProgress != 0 {
		t.Fatalf("verified mutation did not reset failure count: %#v", runtime.strategyLedger)
	}
}

func TestPreflightErrorsDoNotConsumeMutationFailureBudget(t *testing.T) {
	runtime := &agentRuntime{}
	err := newToolError(ToolErrorNoteEditObservation, "read target first", nil)
	runtime.recordTypedToolError("edit_workspace_note_blocks", `{"path":"draft.html"}`, err)
	if runtime.strategyLedger.MutationFailuresWithoutProgress != 0 || runtime.consecutiveToolErrors != 0 {
		t.Fatalf("preflight error consumed execution failure budget: %#v consecutive=%d", runtime.strategyLedger, runtime.consecutiveToolErrors)
	}
}

func TestSuccessfulReadClearsBlockedMutationStrategy(t *testing.T) {
	runtime := &agentRuntime{
		strategyLedger: StrategyLedger{
			MutationFailuresWithoutProgress: 2,
			Failures:                        map[string]StrategyFailure{"stale": {Count: 2}},
		},
	}
	runtime.recordStrategySuccess(agentToolCall{Name: "read_workspace_note_view"}, ToolEvent{Status: "success"})
	if runtime.strategyLedger.MutationFailuresWithoutProgress != 0 || len(runtime.strategyLedger.Failures) != 0 {
		t.Fatalf("successful prerequisite read did not clear strategy block: %#v", runtime.strategyLedger)
	}
}
