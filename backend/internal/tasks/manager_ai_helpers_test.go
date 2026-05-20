package tasks

import "testing"

func TestRepairForceToolUsesTypedMissingKind(t *testing.T) {
	if got := repairForceTool(validationResult{MissingKinds: []string{missingOutputFile}}); got != "save_workspace_file" {
		t.Fatalf("expected file repair to force save_workspace_file, got %q", got)
	}
	if got := repairForceTool(validationResult{Missing: []string{"missing file: draft.txt"}}); got != "" {
		t.Fatalf("expected human-readable missing text not to drive tool choice, got %q", got)
	}
}
