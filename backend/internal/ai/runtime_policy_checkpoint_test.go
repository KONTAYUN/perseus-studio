package ai

import (
	"encoding/json"
	"testing"
)

func TestCheckpointNeverBroadensExplicitScope(t *testing.T) {
	for _, paths := range [][]string{{"a.md"}, {}} {
		r := &agentRuntime{opts: RunOptions{AllowedWorkspacePaths: paths}, transcript: newAgentTranscript(), completedOutcome: "partial"}
		r.loopGuard = AgentLoopGuard{Observations: map[string]string{"read:a.md": "hash"}, RepeatedActionCount: 3}
		raw, err := json.Marshal(r.checkpoint())
		if err != nil {
			t.Fatal(err)
		}
		var cp AgentRuntimeCheckpoint
		if err = json.Unmarshal(raw, &cp); err != nil {
			t.Fatal(err)
		}
		restored := &agentRuntime{}
		restored.restoreCheckpoint(cp)
		if restored.opts.AllowedWorkspacePaths == nil || len(restored.opts.AllowedWorkspacePaths) != len(paths) {
			t.Fatal("scope disappeared on resume")
		}
		if restored.completedOutcome != "partial" || restored.loopGuard.RepeatedActionCount != 3 {
			t.Fatal("lost runtime facts")
		}
		if restored.validateWorkspacePathScope(agentToolCall{Name: "read_workspace_text", ArgumentsJSON: `{"path":"b.md"}`}) == "" {
			t.Fatal("resumed scope broadened")
		}
	}
	if got := intersectWorkspaceScopes([]string{"a.md", "b.md"}, []string{"a.md"}); len(got) != 1 || got[0] != "a.md" {
		t.Fatal(got)
	}
}
