package tasks

import "testing"

func TestRepairForceToolUsesTypedMissingKind(t *testing.T) {
	if got := repairForceTool(validationResult{MissingKinds: []string{missingOutputFile}}); got != "write_workspace_text" {
		t.Fatalf("expected file repair to force write_workspace_text, got %q", got)
	}
	if got := repairForceTool(validationResult{Missing: []string{"missing file: draft.txt"}}); got != "" {
		t.Fatalf("expected human-readable missing text not to drive tool choice, got %q", got)
	}
}
