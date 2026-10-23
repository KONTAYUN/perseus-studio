package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"webide-backend/internal/failure"
	"webide-backend/internal/store"
)

func TestRunBudgetCountsActualHTTPRequestsAcrossTurns(t *testing.T) {
	env := newTestAIEnv(t)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		writeOpenAIToolCall(t, w, "read", "inspect_workspace", `{"path":""}`)
	}))
	defer server.Close()
	p, c, m, messages := prepareOpenAIConversation(t, t.Context(), env, server.URL, "inspect then continue")
	_, err := env.service.GenerateReply(t.Context(), c.ID, m.ID, p.ID, messages, nil, RunOptions{UserID: testUserID, MaxProviderRequests: 1})
	if failure.Code(err) != "run_request_budget_exhausted" || requests != 1 {
		t.Fatalf("budget reset across turns: %d %v", requests, err)
	}
}

func TestRetryAfterBeyondDeadlineDoesNotRetryEarly(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", "http://example.invalid", nil)
	requests := 0
	var attempt int64
	_, err := runSDKAttempt(req, func(*http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"60"}}}, nil
	}, nil, &attempt, ProviderRetryPolicy{MaxAttempts: 3, MaxDelay: time.Millisecond}, nil)
	if failure.Code(err) != "provider_retry_wait_exceeds_run_budget" || requests != 1 {
		t.Fatalf("retried before server wait: %d %v", requests, err)
	}
}

func TestCommandOutcomeIsIndependentOfInvocationSuccess(t *testing.T) {
	t.Setenv("PERSEUS_COMMAND_EXECUTION", "trusted_host")
	env := newTestAIEnv(t)
	for _, expected := range []bool{false, true} {
		args := map[string]any{"command": "exit 23", "timeout_sec": 5}
		if expected {
			args["expected_exit_codes"] = []int{23}
		}
		result, event, err := env.service.executeAndRecordTool(t.Context(), "conv", "msg", "run_workspace_command", mustJSON(args), "", RunOptions{UserID: testUserID, UserRole: "admin"}, providerRuntimeProfile{})
		want := "failed"
		if expected {
			want = "success"
		}
		if err != nil || event.Status != "success" || event.ExecutionOutcome != want {
			t.Fatalf("conflated process/invocation: %#v %v %s", event, err, result)
		}
		wantLabel := "命令执行失败"
		if expected {
			wantLabel = "已运行命令"
		}
		if event.Label != wantLabel {
			t.Fatalf("command label contradicts observed exit outcome: %#v", event)
		}
		var payload map[string]any
		if json.Unmarshal([]byte(result), &payload) != nil || payload["exit_code"] != float64(23) {
			t.Fatal("lost process result")
		}
	}
	_, _, _, _, err := env.service.executeTool(t.Context(), "run_workspace_command", `{"command":"echo test"}`, RunOptions{UserID: testUserID, UserRole: "user"})
	if failure.Code(err) != "command_disabled" {
		t.Fatal("non-admin bypassed trusted host policy")
	}
}

func TestRuntimeBlocksSuccessAfterUnexpectedCommandExit(t *testing.T) {
	t.Setenv("PERSEUS_COMMAND_EXECUTION", "trusted_host")
	env := newTestAIEnv(t)
	r := &agentRuntime{service: env.service, opts: RunOptions{UserID: testUserID, UserRole: "admin"}, loadedToolsets: allAgentToolsets(false), transcript: newAgentTranscript()}
	_, isError, _, _, err := r.processOneToolCall(t.Context(), agentToolCall{Name: "run_workspace_command", ArgumentsJSON: `{"command":"exit 23","timeout_sec":5}`})
	if err != nil || !isError || !r.lastToolFailed {
		t.Fatalf("process failure not propagated: %v %v %#v", err, isError, r.lastToolFailure)
	}
	r.toolCalls = 1
	if _, _, _, _, err := r.processOneToolCall(t.Context(), agentToolCall{Name: "inspect_workspace", ArgumentsJSON: `{"path":""}`}); err != nil {
		t.Fatal(err)
	}
	if r.completionRejection(completionAttempt{Outcome: "success", Result: "all tests passed"}) == "" {
		t.Fatal("unresolved command failure allowed success")
	}
	if r.completionRejection(completionAttempt{Outcome: "partial", Result: "command failed"}) != "" {
		t.Fatal("honest partial rejected")
	}
}

func TestProgressDigestRecognizesContentAndIgnoresTimestamps(t *testing.T) {
	r := &agentRuntime{events: []ToolEvent{{ToolName: "apply_workspace_patch", Status: "success", ChangedPaths: []string{"a.md"}}}}
	call := agentToolCall{Name: "read_workspace_text", ArgumentsJSON: `{"path":"a.md"}`}
	for i := 0; i < 4; i++ {
		r.recordLoopGuardAction(call, false, ToolEvent{Status: "success", Path: "a.md"}, `{"content":"same"}`)
	}
	if _, triggered, stop := r.loopGuardTriggered(); !triggered || !stop {
		t.Fatal("earlier mutation disabled loop guard")
	}
	r = &agentRuntime{}
	for _, hash := range []string{"a", "b", "c", "d", "e"} {
		r.recordLoopGuardAction(agentToolCall{Name: "apply_workspace_patch"}, false, ToolEvent{Status: "success", Path: "a.md", Hash: hash}, "{}")
	}
	if _, _, stop := r.loopGuardTriggered(); stop {
		t.Fatal("real content changes treated as a loop")
	}
	a := semanticToolResultDigest(ToolEvent{}, `{"stdout":"same","started_at":"one"}`)
	b := semanticToolResultDigest(ToolEvent{}, `{"stdout":"same","started_at":"two"}`)
	if a != b {
		t.Fatal("timestamp counted as progress")
	}
}

func TestExplicitSystemCapabilityPersistsAcrossProviderRename(t *testing.T) {
	env := newTestAIEnv(t)
	input := store.ModelProviderInput{Name: "arbitrary", Protocol: "openai", ModelID: "alias", BaseURL: "http://example.invalid", APIKey: "test", Enabled: true, SystemMessageMode: "multiple"}
	p, err := env.configs.CreateModelProvider(t.Context(), testUserID, input)
	if err != nil {
		t.Fatal(err)
	}
	input.Name = "Grok Claude Gemini"
	input.APIKey = ""
	p, err = env.configs.UpdateModelProvider(t.Context(), testUserID, p.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	if p.SystemMessageMode != "multiple" {
		t.Fatal("lost explicit capability")
	}
	input.SystemMessageMode = "guess_by_name"
	if _, err = env.configs.UpdateModelProvider(t.Context(), testUserID, p.ID, input); err == nil {
		t.Fatal("accepted invalid capability")
	}
}
