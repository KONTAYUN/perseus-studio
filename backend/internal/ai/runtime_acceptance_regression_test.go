package ai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"webide-backend/internal/store"
)

// These scenarios use local providers and independent temporary workspaces.
func TestRuntimeAcceptanceWriteThenReadLoopStops(t *testing.T) {
	env := newTestAIEnv(t)
	if _, err := env.files.CreateFile(t.Context(), "draft.md", "before\n"); err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			return
		}
		requests++
		switch requests {
		case 1:
			handleOpenAIWorkspaceToolsetLoad(t, w, payload, "load")
		case 2:
			writeOpenAIToolCall(t, w, "write", "apply_workspace_patch", mustJSON(map[string]any{"patch": "*** Begin Patch\n*** Update File: draft.md\n@@\n-before\n+after\n*** End Patch"}))
		default:
			writeOpenAIToolCall(t, w, "read", "read_workspace_text", `{"path":"draft.md"}`)
		}
	}))
	defer server.Close()
	p, c, m, messages := prepareOpenAIConversation(t, t.Context(), env, server.URL, "修改 draft.md 后核验")
	stopped := false
	_, err := env.service.GenerateReply(t.Context(), c.ID, m.ID, p.ID, messages, nil, RunOptions{
		UserID: testUserID, MaxProviderRequests: 12,
		Event: func(kind, level string, payload map[string]any) {
			if kind == "agent_loop_guard" && payload["stop"] == true {
				stopped = true
			}
		},
	})
	if err == nil || !strings.Contains(err.Error(), "重复相同工具动作") || !stopped || requests >= 12 {
		t.Fatalf("loop did not stop before budget: requests=%d stopped=%v err=%v", requests, stopped, err)
	}
	file, err := env.files.ReadFile("draft.md")
	if err != nil || file.Content != "after\n" {
		t.Fatalf("successful write not retained: %#v %v", file, err)
	}
	t.Logf("verified write retained; repeated unchanged reads stopped after %d provider requests", requests)
}

func TestRuntimeAcceptanceRecoveryAfterVerifiedWrite(t *testing.T) {
	for _, recoverable := range []bool{true, false} {
		name := "persistent_failure"
		if recoverable {
			name = "transient_failure"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("PERSEUS_PROVIDER_REQUEST_MAX_ATTEMPTS", "1")
			t.Setenv("PERSEUS_AGENT_RECOVERY_BASE_DELAY_MS", "0")
			t.Setenv("PERSEUS_AGENT_MAX_CHECKPOINT_RESUMES", "2")
			env := newTestAIEnv(t)
			if _, err := env.files.CreateFile(t.Context(), "draft.md", "before\n"); err != nil {
				t.Fatal(err)
			}
			requests, recoveryStarted := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer r.Body.Close()
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				var payload map[string]any
				if err := json.Unmarshal(raw, &payload); err != nil {
					t.Error(err)
					return
				}
				requests++
				switch {
				case requests == 1:
					handleOpenAIWorkspaceToolsetLoad(t, w, payload, "load")
				case requests == 2:
					writeOpenAIToolCall(t, w, "write", "apply_workspace_patch", mustJSON(map[string]any{"patch": "*** Begin Patch\n*** Update File: draft.md\n@@\n-before\n+after\n*** End Patch"}))
				case recoverable && openAIProviderMessageContentContainsAll(payload, `"status":"recovery_observation"`):
					writeOpenAIToolCall(t, w, "done", "attempt_completion", `{"outcome":"success","result":"已从断点恢复，draft.md 保存为 after，没有重复修改。","changed_paths":["draft.md"]}`)
				default:
					w.Header().Set("Content-Type", "text/plain")
					w.WriteHeader(http.StatusBadGateway)
					_, _ = w.Write([]byte("injected upstream disconnect after verified write"))
				}
			}))
			defer server.Close()
			p, c, m, messages := prepareOpenAIConversation(t, t.Context(), env, server.URL, "将 draft.md 保存为 after，之后报告结果")
			run, err := env.runs.CreateRun(t.Context(), store.CreateAgentRunInput{UserID: testUserID, ConversationID: c.ID, UserMessageID: m.ID, ModelProviderID: p.ID})
			if err != nil {
				t.Fatal(err)
			}
			reply, err := env.service.GenerateReply(t.Context(), c.ID, m.ID, p.ID, messages, nil, RunOptions{
				UserID: testUserID, RunID: run.ID, MaxProviderRequests: 8,
				Event: func(kind, level string, payload map[string]any) {
					if kind == "agent_recovery_started" {
						recoveryStarted++
					}
				},
			})
			if recoverable && (err != nil || reply.Outcome != "success") {
				t.Fatalf("transient error did not recover: reply=%#v err=%v", reply, err)
			}
			if !recoverable && err == nil {
				t.Fatal("persistent error falsely succeeded")
			}
			if recoveryStarted == 0 || recoveryStarted > 2 || requests > 8 {
				t.Fatalf("recovery/budget mismatch: recoveries=%d requests=%d", recoveryStarted, requests)
			}
			var writes int
			if err := env.db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM tool_calls WHERE conversation_id=? AND tool_name='apply_workspace_patch'`, c.ID).Scan(&writes); err != nil || writes != 1 {
				t.Fatalf("mutation repeated: writes=%d err=%v", writes, err)
			}
			file, readErr := env.files.ReadFile("draft.md")
			if readErr != nil || file.Content != "after\n" {
				t.Fatalf("verified content lost: %#v %v", file, readErr)
			}
			t.Logf("requests=%d recoveries=%d writes=%d recoverable=%v", requests, recoveryStarted, writes, recoverable)
		})
	}
}
