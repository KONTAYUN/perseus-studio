package tasks

import (
	"testing"

	"webide-backend/internal/ai"
)

func TestRepairForceToolUsesTypedMissingKind(t *testing.T) {
	if got := repairForceTool(validationResult{MissingKinds: []string{missingOutputFile}}); got != "" {
		t.Fatalf("expected file repair not to force a compatibility write tool, got %q", got)
	}
	if got := repairForceTool(validationResult{Missing: []string{"missing file: draft.txt"}}); got != "" {
		t.Fatalf("expected human-readable missing text not to drive tool choice, got %q", got)
	}
}

func TestFirstChangedFileAcceptsPatchAndCompatibilityWrites(t *testing.T) {
	events := []ai.ToolEvent{
		{ToolName: "apply_workspace_patch", Status: "success", Path: "drafts/long.txt", ChangedPaths: []string{"drafts/long.txt"}},
	}
	if got := firstChangedFile(events); got != "drafts/long.txt" {
		t.Fatalf("expected patch output path, got %q", got)
	}

	events = []ai.ToolEvent{
		{ToolName: "finalize_workspace_write", Status: "success", Path: "drafts/final.txt", ChangedPaths: []string{"drafts/final.txt"}},
	}
	if got := firstChangedFile(events); got != "drafts/final.txt" {
		t.Fatalf("expected finalized write output path, got %q", got)
	}
}
