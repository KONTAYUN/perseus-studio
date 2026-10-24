package ai

import "testing"

func TestContentOnlyCompletionAfterReadOnlyTools(t *testing.T) {
	success := func(names ...string) []ToolEvent {
		events := make([]ToolEvent, 0, len(names))
		for _, name := range names {
			events = append(events, ToolEvent{ToolName: name, Status: "success"})
		}
		return events
	}
	cases := []struct {
		name   string
		mutate func(*agentRuntime)
		accept bool
		reason string
	}{
		{"web search answer", func(r *agentRuntime) { r.events = success("load_perseus_toolset", "perseus_search_web") }, true, "read_only_web_tool_completed"},
		{"web extract answer", func(r *agentRuntime) { r.events = success("perseus_search_web", "web_extract") }, true, "read_only_web_tool_completed"},
		{"workspace read answer", func(r *agentRuntime) { r.events = success("read_workspace_text") }, true, "read_only_workspace_tool_completed"},
		{"failed web search", func(r *agentRuntime) {
			r.events = []ToolEvent{{ToolName: "perseus_search_web", Status: "error"}}
		}, false, "action_boundary_requires_action"},
		{"toolset load only", func(r *agentRuntime) { r.events = success("load_perseus_toolset") }, false, "action_boundary_requires_action"},
		{"web search then mutation", func(r *agentRuntime) {
			r.events = success("perseus_search_web")
			r.performedMutation = true
		}, false, "mutation_requires_terminal_completion"},
		{"web search with completion pending", func(r *agentRuntime) {
			r.events = success("perseus_search_web")
			r.requiresCompletion = true
		}, false, "terminal_completion_required"},
		{"web search with selected workspace file", func(r *agentRuntime) {
			r.events = success("perseus_search_web")
			r.attachments = []AttachmentPayload{{Kind: "workspace_file"}}
		}, false, "selected_workspace_reference_requires_terminal_or_action"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &agentRuntime{toolLoopActive: true}
			tc.mutate(r)
			got := r.contentOnlyCompletionPolicy(agentModelTurn{Content: "final answer"})
			if got.Accept != tc.accept || got.Reason != tc.reason {
				t.Fatalf("got accept=%v reason=%q, want accept=%v reason=%q", got.Accept, got.Reason, tc.accept, tc.reason)
			}
		})
	}
}
