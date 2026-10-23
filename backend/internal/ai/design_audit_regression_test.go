package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"webide-backend/internal/failure"
	"webide-backend/internal/store"
	"webide-backend/internal/workspace"
)

// All scenarios use an independent database/workspace and a local provider.
func runAuditScenario(t *testing.T, scenario string) {
	t.Helper()
	env := newTestAIEnv(t)
	if _, err := env.files.CreateTextFile(t.Context(), "draft.md", "before\n", "test"); err != nil {
		t.Fatal(err)
	}
	requests := 0
	example := "仅用于讲解，请勿执行：\n```diff\n*** Begin Patch\n*** Add File: example-only.md\n+demo\n*** End Patch\n```"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests++
		switch scenario {
		case "example":
			writeOpenAIText(t, w, example)
		case "mixed":
			if requests == 1 {
				writeOpenAIStreamTextAndToolCall(t, w, example, "read", "read_workspace_text", `{"path":"draft.md"}`)
			} else {
				writeOpenAIToolCall(t, w, "done", "attempt_completion", `{"result":"apply_workspace_patch 是结构化编辑工具；上面的代码仅作讲解。"}`)
			}
		case "partial":
			if requests == 1 {
				writeOpenAIToolCall(t, w, "read", "read_workspace_text", `{"path":"missing.md"}`)
			} else {
				writeOpenAIToolCall(t, w, "done", "attempt_completion", `{"outcome":"partial","result":"文件尚未修改，apply_workspace_patch 可用于后续编辑。","failed_items":["missing.md"]}`)
			}
		case "read_partial":
			if requests == 1 {
				writeOpenAIToolCall(t, w, "read", "read_workspace_text", `{"path":"draft.md"}`)
			} else {
				writeOpenAIToolCall(t, w, "done", "attempt_completion", `{"outcome":"partial","result":"文件已读取，尚未修改。","failed_items":["未运行测试"]}`)
			}
		}
	}))
	defer server.Close()
	p, c, m, messages := prepareOpenAIConversation(t, t.Context(), env, server.URL, "仅讲解或检查，不修改文件")
	reply, err := env.service.GenerateReply(t.Context(), c.ID, m.ID, p.ID, messages, nil, RunOptions{UserID: testUserID, MaxProviderRequests: 4})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.files.StatPath("example-only.md"); err == nil {
		t.Fatal("prose executed")
	}
	read, err := env.files.ReadFile("draft.md")
	if err != nil || read.Content != "before\n" {
		t.Fatal("changed existing file")
	}
	if scenario == "example" {
		if reply.Content != example || len(reply.ToolEvents) != 0 || requests != 1 {
			t.Fatalf("example not preserved: %#v", reply)
		}
	}
	if scenario == "mixed" {
		if len(reply.ToolEvents) != 1 || reply.ToolEvents[0].ToolName != "read_workspace_text" || requests != 2 {
			t.Fatalf("structured read overwritten: %#v", reply)
		}
	}
	if strings.Contains(scenario, "partial") && reply.Outcome != "partial" {
		t.Fatalf("lost partial outcome: %#v", reply)
	}
}

func TestAuditProsePatchNeverExecutes(t *testing.T) { runAuditScenario(t, "example") }
func TestAuditTypedFailureSurvivesRuntimeAndWording(t *testing.T) {
	r := &agentRuntime{}
	r.recordToolFailure("manage_workspace_path", "{}", fmt.Errorf("new translated message: %w", workspace.ErrTrashManagedOnly))
	if r.lastToolFailure.FailureClass != "workspace_path_policy" {
		t.Fatal(r.lastToolFailure)
	}
	r.recordToolFailure("manage_workspace_path", "{}", failure.New("custom_code", "path already exists", nil))
	if r.lastToolFailure.FailureClass != "custom_code" {
		t.Fatal("message overrode code")
	}
}
func TestAuditCapabilityIgnoresDisplayNameAndURL(t *testing.T) {
	p := store.ModelProvider{Protocol: "openai", ModelID: "alias", Name: "my model"}
	a := runtimeProfileForProvider(p)
	p.Name = "替代 Claude Grok Gemini"
	p.BaseURL = "https://claude.example/v1"
	b := runtimeProfileForProvider(p)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("names changed execution profile")
	}
}
func TestAuditNestedToolArgumentsRejected(t *testing.T) {
	for _, tc := range []struct {
		args   string
		schema map[string]any
	}{
		{`{"paths":["a.md",42]}`, workspaceReadFilesParams()},
		{`{"title":"x","goal":"x","expected_outputs":[{"type":"invalid","description":"x"}]}`, backgroundTaskParams()},
		{`{"title":"x","goal":"x","expected_outputs":[{"type":"file"}]}`, backgroundTaskParams()},
		{`{"title":"x","goal":"x","suggested_steps":[true]}`, backgroundTaskParams()},
	} {
		var args map[string]any
		if err := json.Unmarshal([]byte(tc.args), &args); err != nil {
			t.Fatal(err)
		}
		if reason := validateToolArgumentsAgainstSchemaMap(args, tc.schema); reason == "" {
			t.Fatalf("accepted malformed args %s", tc.args)
		}
	}
}
func TestAuditRetryWaitAndSharedRequestBudget(t *testing.T) {
	policy := ProviderRetryPolicy{MaxAttempts: 3, MaxDelay: 5 * time.Second}
	resp := &http.Response{Header: http.Header{"Retry-After": []string{"60"}}}
	if d, _ := providerRetryDelay(policy, 1, resp); d != time.Minute {
		t.Fatal(d)
	}
	ctx, cancel := withRunRequestBudget(context.Background(), RunOptions{MaxProviderRequests: 2})
	defer cancel()
	if admitProviderRequest(ctx) != nil || admitProviderRequest(ctx) != nil || failure.Code(admitProviderRequest(ctx)) != "run_request_budget_exhausted" {
		t.Fatal("shared budget ignored")
	}
	if parseRetryAfter("1e99", time.Now()) <= 0 {
		t.Fatal("retry overflow")
	}
}
