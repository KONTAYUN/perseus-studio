package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"webide-backend/internal/ai"
	"webide-backend/internal/store"
	"webide-backend/internal/usersvc"
)

type traceTestEnv struct {
	db      *sql.DB
	m       *Manager
	task    store.Task
	payload AIAgentPayload
}

func newTraceTestEnv(t *testing.T, handler http.HandlerFunc) traceTestEnv {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	configs := store.NewConfigStore(db)
	conversations := store.NewConversationStore(db)
	tasks := store.NewTaskStore(db)
	skills := store.NewSkillStore(db)
	for _, migrate := range []func(context.Context) error{configs.Migrate, conversations.Migrate, tasks.Migrate, skills.Migrate} {
		if err := migrate(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	factory := usersvc.NewFactory(db, filepath.Join(t.TempDir(), "users"), skills)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	provider, err := configs.CreateModelProvider(t.Context(), "owner", store.ModelProviderInput{Name: "mock", Protocol: "openai", BaseURL: server.URL, APIKey: "test", ModelID: "mock", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := conversations.CreateConversation(t.Context(), store.CreateConversationInput{UserID: "owner", Title: "trace", SelectedModelProviderID: provider.ID})
	if err != nil {
		t.Fatal(err)
	}
	message, err := conversations.AppendMessage(t.Context(), "owner", conv.ID, "user", "核对原稿并保存报告", "[]")
	if err != nil {
		t.Fatal(err)
	}
	payload := AIAgentPayload{SourceRunID: "foreground", ConversationID: conv.ID, UserMessageID: message.ID, ModelProviderID: provider.ID, UserRole: "admin", UserMessage: message.Content, ExpectedOutputs: []ai.ExpectedOutput{{Type: "file", Path: "report.md"}}}
	raw, _ := json.Marshal(payload)
	task, err := tasks.CreateTask(t.Context(), store.CreateTaskInput{ID: "task", UserID: "owner", Type: "ai_agent", Title: "test", PayloadJSON: string(raw)})
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(tasks, conversations, nil, ai.NewService(configs, factory, conversations), factory, NewBroker(), Config{})
	files, err := factory.Workspace("owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := files.CreateFile(t.Context(), "source.md", "原稿\n"); err != nil {
		t.Fatal(err)
	}
	return traceTestEnv{db: db, m: m, task: task, payload: payload}
}

func traceToolResponse(w http.ResponseWriter, content, name, args string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": content, "tool_calls": []any{map[string]any{"id": name, "type": "function", "function": map[string]any{"name": name, "arguments": args}}}}, "finish_reason": "tool_calls"}}})
}

func TestBackgroundTracePreservesRepliesErrorsRetryAndPartial(t *testing.T) {
	t.Setenv("PERSEUS_PROVIDER_RETRY_BASE_DELAY_MS", "0")
	var requests atomic.Int32
	env := newTraceTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		switch requests.Add(1) {
		case 1:
			http.Error(w, "temporary upstream failure", http.StatusBadGateway)
		case 2:
			traceToolResponse(w, "先核对缺失的资料。", "read_workspace_text", `{"path":"missing.md"}`)
		case 3:
			traceToolResponse(w, "资料缺失，改读现有原稿。", "read_workspace_text", `{"path":"source.md"}`)
		case 4:
			traceToolResponse(w, "", "attempt_completion", `{"outcome":"partial","result":"现有原稿已核对，缺失部分保留。","failed_items":["未生成报告"]}`)
		default:
			t.Error("honest partial was forced into another repair")
			http.Error(w, "unexpected request", 400)
		}
	})
	env.m.executeAIAgentTask(t.Context(), env.task, "worker")
	task, err := env.m.store.GetTaskByID(t.Context(), env.task.ID)
	if err != nil || task.Status != store.TaskDone {
		t.Fatalf("terminal: %#v %v", task, err)
	}
	var result struct {
		OK                 bool             `json:"ok"`
		Outcome            string           `json:"outcome"`
		RepairRounds       int              `json:"repair_rounds"`
		AssistantMessageID string           `json:"assistant_message_id"`
		Validation         validationResult `json:"validation"`
	}
	if err := json.Unmarshal([]byte(task.ResultJSON.String), &result); err != nil {
		t.Fatal(err)
	}
	if result.OK || result.Outcome != "partial" || result.RepairRounds != 0 || result.Validation.OK || requests.Load() != 4 {
		t.Fatalf("false success or repeated repair: %#v requests=%d", result, requests.Load())
	}
	events, err := env.m.store.ListEventsAfter(t.Context(), "owner", "task", 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	var full strings.Builder
	for _, e := range events {
		counts[e.Type]++
		full.Write(e.Payload)
	}
	for _, kind := range []string{"model_response_observed", "model_message_completed", "model_call_attempt_started", "model_call_retrying", "tool_call_failed", "task_execution_completed", "task_done"} {
		if counts[kind] == 0 {
			t.Errorf("missing %s", kind)
		}
	}
	for _, text := range []string{"先核对缺失的资料。", "资料缺失，改读现有原稿。", "现有原稿已核对，缺失部分保留。"} {
		if !strings.Contains(full.String(), text) {
			t.Errorf("reply lost: %s", text)
		}
	}
	if counts["model_call_attempt_started"] != 4 {
		t.Fatalf("attempts=%d", counts["model_call_attempt_started"])
	}
	var meta string
	if err := env.db.QueryRow(`SELECT meta_json FROM messages WHERE id=?`, result.AssistantMessageID).Scan(&meta); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(meta, `"task_id":"task"`) || !strings.Contains(meta, `"outcome":"partial"`) {
		t.Fatalf("missing final identity/outcome: %s", meta)
	}
}

func TestBackgroundEventFailureStopsNextMutation(t *testing.T) {
	var requests atomic.Int32
	env := newTraceTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		n := requests.Add(1)
		name := "first.md"
		if n > 1 {
			name = "second.md"
		}
		args, _ := json.Marshal(map[string]any{"patch": "*** Begin Patch\n*** Add File: " + name + "\n+saved\n*** End Patch"})
		traceToolResponse(w, "", "apply_workspace_patch", string(args))
	})
	if _, err := env.db.Exec(`CREATE TRIGGER reject_tool_trace BEFORE INSERT ON task_events WHEN NEW.type='tool_call_completed' BEGIN SELECT RAISE(FAIL,'injected task trace failure'); END`); err != nil {
		t.Fatal(err)
	}
	env.m.executeAIAgentTask(t.Context(), env.task, "worker")
	task, _ := env.m.store.GetTaskByID(t.Context(), "task")
	if task.Status != store.TaskFailed || !strings.Contains(task.ErrorMessage.String, "injected task trace failure") {
		t.Fatalf("lost persistence cause: %#v", task)
	}
	files, _ := env.m.factory.Workspace("owner")
	if _, err := files.ReadFile("first.md"); err != nil {
		t.Fatal("first effect missing", err)
	}
	if _, err := files.ReadFile("second.md"); err == nil || requests.Load() != 1 {
		t.Fatalf("execution continued: requests=%d", requests.Load())
	}
}

func TestBackgroundCancellationKeepsAttemptAndTerminal(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	env := newTraceTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); env.m.executeAIAgentTask(ctx, env.task, "worker") }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("provider not called")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cancel did not converge")
	}
	task, _ := env.m.store.GetTaskByID(context.Background(), "task")
	if task.Status != store.TaskCanceled {
		t.Fatalf("cancel status=%s", task.Status)
	}
	events, _ := env.m.store.ListEventsAfter(context.Background(), "owner", "task", 0, 1000)
	counts := map[string]int{}
	for _, e := range events {
		counts[e.Type]++
	}
	if counts["model_call_attempt_started"] != 1 || counts["task_execution_failed"] != 1 || counts["task_canceled"] != 1 {
		t.Fatalf("cancellation trace incomplete: %#v", counts)
	}
}

func TestBackgroundStreamKeepsTextAndCompletionPreview(t *testing.T) {
	var requests atomic.Int32
	env := newTraceTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		var chunks []map[string]any
		if requests.Add(1) == 1 {
			chunks = []map[string]any{{"content": "流式说明甲"}, {"content": "乙。"},
				{"tool_calls": []any{map[string]any{"index": 0, "id": "read", "type": "function", "function": map[string]any{"name": "read_workspace_text", "arguments": `{"path":"source.md"}`}}}}}
		} else {
			chunks = []map[string]any{{"tool_calls": []any{map[string]any{"index": 0, "id": "done", "type": "function", "function": map[string]any{"name": "attempt_completion", "arguments": `{"outcome":"partial","result":"流式最终说明。","failed_items":["报告未写"]}`}}}}}
		}
		for _, delta := range chunks {
			raw, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": delta}}})
			fmt.Fprintf(w, "data: %s\n\n", raw)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	})
	env.m.executeAIAgentTask(t.Context(), env.task, "worker")
	task, _ := env.m.store.GetTaskByID(t.Context(), "task")
	if task.Status != store.TaskDone {
		t.Fatalf("stream status=%s error=%s", task.Status, task.ErrorMessage.String)
	}
	events, _ := env.m.store.ListEventsAfter(t.Context(), "owner", "task", 0, 1000)
	counts := map[string]int{}
	var body strings.Builder
	for _, event := range events {
		counts[event.Type]++
		body.Write(event.Payload)
	}
	if counts["model_delta"] == 0 || counts["completion_result_delta"] == 0 || !strings.Contains(body.String(), "流式说明甲乙。") || !strings.Contains(body.String(), "流式最终说明。") {
		t.Fatalf("stream trace lost text or preview: %#v", counts)
	}
}

func TestBackgroundRepairRoundsHaveSeparateExecutionIdentities(t *testing.T) {
	var requests atomic.Int32
	env := newTraceTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		switch requests.Add(1) {
		case 1:
			traceToolResponse(w, "读取原稿。", "read_workspace_text", `{"path":"source.md"}`)
		case 2:
			traceToolResponse(w, "", "attempt_completion", `{"outcome":"success","result":"已完成阅读。"}`)
		case 3:
			traceToolResponse(w, "补做缺失报告。", "apply_workspace_patch", `{"patch":"*** Begin Patch\n*** Add File: report.md\n+报告\n*** End Patch"}`)
		default:
			traceToolResponse(w, "", "attempt_completion", `{"outcome":"success","result":"报告已保存。","changed_paths":["report.md"]}`)
		}
	})
	env.m.executeAIAgentTask(t.Context(), env.task, "worker")
	task, _ := env.m.store.GetTaskByID(t.Context(), "task")
	var result struct {
		OK      bool `json:"ok"`
		Repairs int  `json:"repair_rounds"`
	}
	if err := json.Unmarshal([]byte(task.ResultJSON.String), &result); err != nil {
		t.Fatal(err)
	}
	if task.Status != store.TaskDone || !result.OK || result.Repairs != 1 || requests.Load() != 4 {
		t.Fatalf("repair result: %#v requests=%d", result, requests.Load())
	}
	events, _ := env.m.store.ListEventsAfter(t.Context(), "owner", "task", 0, 1000)
	ids := map[string]bool{}
	for _, event := range events {
		if event.Type == "task_execution_started" {
			ids[event.ExecutionID] = true
		}
	}
	if len(ids) != 2 || ids[""] {
		t.Fatalf("repair executions cannot be distinguished: %#v", ids)
	}
}
