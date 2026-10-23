package ai

import (
	"encoding/json"
	"strings"
	"testing"

	"webide-backend/internal/failure"
)

func TestExplicitScopeChecksActualPathDestinationsBeforeMutation(t *testing.T) {
	for _, tc := range []struct{ name, args string }{
		{"manage_workspace_path", `{"action":"rename","path":"source.md","new_name":"outside.md"}`},
		{"manage_workspace_path", `{"action":"move","path":"source.md","new_parent_path":"outside"}`},
		{"organize_workspace_matches", `{"match_text":"before","target_folder":"outside"}`},
		{"save_workspace_file", `{"path":" source.md","mode":"create","content":"wrong target"}`},
		{"save_workspace_file", `{"path":"/source.md","mode":"create","content":"wrong target"}`},
	} {
		t.Run(tc.name+tc.args, func(t *testing.T) {
			env := newTestAIEnv(t)
			if _, err := env.files.CreateTextFile(t.Context(), "source.md", "before\n", "test"); err != nil {
				t.Fatal(err)
			}
			if _, err := env.files.Mkdir("outside"); err != nil {
				t.Fatal(err)
			}
			_, _, _, _, err := env.service.executeTool(t.Context(), tc.name, tc.args, RunOptions{UserID: testUserID, AllowedWorkspacePaths: []string{"source.md"}})
			if failure.Code(err) != "workspace_scope_rejected" {
				t.Fatalf("scope bypass: %v", err)
			}
			file, err := env.files.ReadFile("source.md")
			if err != nil || file.Content != "before\n" {
				t.Fatalf("changed source: %#v %v", file, err)
			}
			for _, path := range []string{"outside.md", "outside/source.md", " source.md"} {
				if _, err := env.files.StatPath(path); err == nil {
					t.Fatalf("created forbidden path %s", path)
				}
			}
		})
	}
}

func TestExplicitScopeAllowsNamedFileMoveAndRejectsRecursiveRename(t *testing.T) {
	env := newTestAIEnv(t)
	if _, err := env.files.CreateTextFile(t.Context(), "source.md", "before\n", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := env.files.Mkdir("dest"); err != nil {
		t.Fatal(err)
	}
	opts := RunOptions{UserID: testUserID, AllowedWorkspacePaths: []string{"source.md", "dest/source.md", "dest/renamed.md", "dest", "renamed-dir"}}
	for _, args := range []string{
		`{"action":"move","path":"./source.md","new_parent_path":"dest"}`,
		`{"action":"rename","path":"dest/source.md","new_name":"renamed.md"}`,
	} {
		if _, _, _, _, err := env.service.executeTool(t.Context(), "manage_workspace_path", args, opts); err != nil {
			t.Fatal(err)
		}
	}
	_, _, _, _, err := env.service.executeTool(t.Context(), "manage_workspace_path", `{"action":"rename","path":"dest","new_name":"renamed-dir"}`, opts)
	if failure.Code(err) != "workspace_scope_rejected" {
		t.Fatalf("recursive scope bypass: %v", err)
	}
	file, err := env.files.ReadFile("dest/renamed.md")
	if err != nil || file.Content != "before\n" {
		t.Fatal("lost original file")
	}
}

func TestPatchSemanticMismatchNeverPartiallyWrites(t *testing.T) {
	for _, tc := range []struct{ name, content, old string }{
		{"indent", "if ready:\n    run()\n", "  run()"},
		{"unicode-quote", "say(\"hello\")\n", "say(“hello”)"},
		{"unicode-space", "a b\n", "a\u00a0b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestAIEnv(t)
			for path, content := range map[string]string{"first.md": "before\n", "target.txt": tc.content} {
				if _, err := env.files.CreateTextFile(t.Context(), path, content, "test"); err != nil {
					t.Fatal(err)
				}
			}
			patch := "*** Begin Patch\n*** Update File: first.md\n@@\n-before\n+after\n*** Update File: target.txt\n@@\n-" + tc.old + "\n+changed\n*** End Patch"
			if _, err := applyWorkspacePatch(t.Context(), env.files, patch); err == nil {
				t.Fatal("semantic mismatch accepted")
			}
			for path, want := range map[string]string{"first.md": "before\n", "target.txt": tc.content} {
				file, err := env.files.ReadFile(path)
				if err != nil || file.Content != want {
					t.Fatalf("partial write: %s %v", path, err)
				}
			}
		})
	}
}

func TestCommandFailureFactsSurviveLargeResultAndCheckpoint(t *testing.T) {
	code := 23
	event := ToolEvent{ToolName: "run_workspace_command", Status: "success", ExecutionOutcome: "failed", ExitCode: &code, Command: "verify", Cwd: "project"}
	raw := mustJSON(map[string]any{"stdout": strings.Repeat("x", 50000), "exit_code": code, "execution_outcome": "failed", "observation_complete": false, "failure_class": "command_exit_unexpected"})
	for _, result := range []string{
		ledgerToolResultJSON("run_workspace_command", raw),
		budgetToolResultForModel("run_workspace_command", raw, providerRuntimeProfile{MaxToolResultRunes: 1000, MaxCommandOutputRunes: 200}),
		compactedToolResultNotice("run_workspace_command", event, "historical result"),
	} {
		var payload map[string]any
		if err := json.Unmarshal([]byte(result), &payload); err != nil {
			t.Fatal(err)
		}
		if payload["execution_outcome"] != "failed" || payload["exit_code"] != float64(23) {
			t.Fatal("compaction erased failed command facts")
		}
	}
	r := &agentRuntime{transcript: newAgentTranscript()}
	r.recordCommandOutcome(event)
	data, err := json.Marshal(r.checkpoint())
	if err != nil {
		t.Fatal(err)
	}
	var cp AgentRuntimeCheckpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		t.Fatal(err)
	}
	restored := &agentRuntime{}
	restored.restoreCheckpoint(cp)
	if len(restored.unresolvedCommands) != 1 {
		t.Fatal("checkpoint erased failure")
	}
	event.ExecutionOutcome = "success"
	event.Cwd = "other"
	restored.recordCommandOutcome(event)
	if len(restored.unresolvedCommands) != 1 {
		t.Fatal("unrelated command cleared failure")
	}
	event.Cwd = "project"
	restored.recordCommandOutcome(event)
	if len(restored.unresolvedCommands) != 0 {
		t.Fatal("successful exact rerun did not clear failure")
	}
}

func TestCompletionOutcomeMustAgreeWithFailedItems(t *testing.T) {
	r := &agentRuntime{}
	if r.completionRejection(completionAttempt{Outcome: "success", Result: "done", FailedItems: []string{"test failed"}}) == "" {
		t.Fatal("contradictory success accepted")
	}
	if r.completionRejection(completionAttempt{Outcome: "partial", Result: "done", FailedItems: []string{"test failed"}}) != "" {
		t.Fatal("honest partial rejected")
	}
	if r.completionRejection(completionAttempt{Outcome: "success", Result: "文件尚未修改；其余不变；apply_workspace_patch 只是说明。"}) != "" {
		t.Fatal("display keywords changed completion facts")
	}
}
