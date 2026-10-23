package ai

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAcceptancePatchSchemaRequiresStructuredCall(t *testing.T) {
	for name, tools := range map[string]any{
		"openai":    openAIToolsForLoadedToolsets(map[string]bool{"workspace": true}, false),
		"anthropic": anthropicToolsForLoadedToolsets(map[string]bool{"workspace": true}, false),
	} {
		t.Run(name, func(t *testing.T) {
			raw, _ := json.Marshal(tools)
			if strings.Contains(string(raw), "assistant text is also supported") {
				t.Error("outgoing schema advertises execution of display-only prose")
			}
			if !strings.Contains(string(raw), "Assistant text and code blocks do not execute file changes") {
				t.Error("outgoing schema must state the structured editing boundary")
			}
		})
	}
}

func TestAcceptanceRejectedCommandNeverClaimsExecution(t *testing.T) {
	t.Setenv("PERSEUS_COMMAND_EXECUTION", "disabled")
	env := newTestAIEnv(t)
	var observed map[string]any
	r := &agentRuntime{service: env.service, opts: RunOptions{UserID: testUserID, UserRole: "admin",
		Event: func(kind, level string, data map[string]any) {
			if kind == "tool_call_failed" {
				observed = data
			}
		}}, loadedToolsets: map[string]bool{"workspace": true}}
	_, isError, _, _, err := r.processOneToolCall(t.Context(), agentToolCall{
		ID: "blocked", Name: "run_workspace_command", ArgumentsJSON: `{"command":"echo SHOULD_NOT_RUN","cwd":"."}`,
	})
	if err != nil || !isError || len(r.events) != 1 {
		t.Fatalf("rejection missing: %v %#v", err, r.events)
	}
	if r.events[0].Label != "工具未执行" || observed["label"] != "工具未执行" {
		t.Fatalf("rejected command claims execution: %#v / %#v", r.events[0], observed)
	}
	var count int
	if err := env.db.QueryRow("SELECT count(*) FROM tool_calls").Scan(&count); err != nil || count != 0 {
		t.Fatalf("rejected command reached executor ledger: %d %v", count, err)
	}
	_, budget := r.toolBudgetExhaustedResult(agentToolCall{ID: "budget", Name: "run_workspace_command"}, 1)
	if budget.Label != "工具未执行" {
		t.Fatalf("budget-skipped command claims execution: %#v", budget)
	}
}

func TestAcceptanceExecutorFailureUsesFailureLabel(t *testing.T) {
	env := newTestAIEnv(t)
	var observed map[string]any
	_, event, err := env.service.executeAndRecordTool(t.Context(), "conv", "msg", "read_workspace_text",
		`{"path":"missing.md"}`, "", RunOptions{UserID: testUserID, UserRole: "admin", Event: func(kind, level string, data map[string]any) {
			if kind == "tool_call_failed" {
				observed = data
			}
		}}, providerRuntimeProfile{})
	if err != nil || event.Status != "failed" || event.Label != "工具执行失败" || observed["label"] != event.Label {
		t.Fatalf("execution failure claims completion: %#v %v %#v", event, err, observed)
	}
}
