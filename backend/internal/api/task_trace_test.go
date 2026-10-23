package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"webide-backend/internal/ai"
	"webide-backend/internal/store"
	"webide-backend/internal/tasks"
)

func TestBackgroundHandoffHasDurableTerminalIdentity(t *testing.T) {
	for _, reject := range []bool{false, true} {
		t.Run(fmt.Sprintf("reject_terminal_%v", reject), func(t *testing.T) {
			env := newAPITestAIEnv(t)
			runs := store.NewAgentRunStore(env.db)
			taskStore := store.NewTaskStore(env.db)
			for _, migrate := range []func(context.Context) error{runs.Migrate, taskStore.Migrate} {
				if err := migrate(t.Context()); err != nil {
					t.Fatal(err)
				}
			}
			env.ai.AttachRunStore(runs)
			conv, err := env.conversations.CreateConversation(t.Context(), store.CreateConversationInput{UserID: env.admin.ID, Title: "handoff"})
			if err != nil {
				t.Fatal(err)
			}
			msg, err := env.conversations.AppendMessage(t.Context(), env.admin.ID, conv.ID, "user", "后台核对", "[]")
			if err != nil {
				t.Fatal(err)
			}
			run, err := runs.CreateRun(t.Context(), store.CreateAgentRunInput{UserID: env.admin.ID, ConversationID: conv.ID, UserMessageID: msg.ID})
			if err != nil {
				t.Fatal(err)
			}
			if reject {
				if _, err := env.db.Exec(`CREATE TRIGGER reject_handoff_terminal BEFORE INSERT ON agent_events WHEN NEW.type='run_completed' BEGIN SELECT RAISE(FAIL,'handoff terminal failure'); END`); err != nil {
					t.Fatal(err)
				}
			}
			s := &Server{ai: env.ai, runs: runs, store: taskStore, configs: env.configs, conversations: env.conversations, broker: tasks.NewBroker(), runBroker: tasks.NewBroker()}
			s.completeConversationRunWithTask(t.Context(), env.admin.ID, "admin", conv.ID, msg.ID, "", msg.Content, "[]", run.ID, &ai.BackgroundTaskRequest{Title: "核对", Goal: "读取资料"})
			current, err := runs.GetRun(t.Context(), env.admin.ID, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			items, err := taskStore.ListTasks(t.Context(), env.admin.ID, 10)
			wantTasks := 1
			if reject {
				wantTasks = 0
			}
			if err != nil || len(items) != wantTasks {
				t.Fatalf("handoff side effect: %d %v", len(items), err)
			}
			events, _ := runs.ListEvents(t.Context(), env.admin.ID, run.ID)
			var completed []store.AgentEvent
			for _, event := range events {
				if event.Type == "run_completed" {
					completed = append(completed, event)
				}
			}
			if reject {
				if current.Status != store.AgentRunFailed || len(completed) != 0 || !strings.Contains(current.ErrorMessage.String, "handoff terminal failure") {
					t.Fatalf("false success on event failure: %#v", current)
				}
				return
			}
			if current.Status != store.AgentRunSucceeded || len(completed) != 1 || completed[0].TaskID != items[0].ID || !strings.Contains(completed[0].PayloadJSON, "background_task_handoff") {
				t.Fatalf("terminal missing: %#v %#v", current, completed)
			}
			messages, _ := env.conversations.ListMessages(t.Context(), env.admin.ID, conv.ID)
			last := messages[len(messages)-1]
			if !strings.Contains(last.MetaJSON, items[0].ID) || !strings.Contains(items[0].PayloadJSON, run.ID) {
				t.Fatal("handoff association lost")
			}
		})
	}
}

func TestTaskEventAPIIsScopedAndCursorBased(t *testing.T) {
	env := newAPITestAIEnv(t)
	taskStore := store.NewTaskStore(env.db)
	if err := taskStore.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	task, err := taskStore.CreateTask(t.Context(), store.CreateTaskInput{ID: "trace", UserID: env.admin.ID, Type: "mock", Title: "trace", PayloadJSON: "{}"})
	if err != nil {
		t.Fatal(err)
	}
	for range 205 {
		if _, err := taskStore.AppendEvent(t.Context(), store.TaskEventInput{UserID: env.admin.ID, TaskID: task.ID, Type: "model_delta", Payload: map[string]any{"text": "可审阅正文"}}); err != nil {
			t.Fatal(err)
		}
	}
	s := &Server{store: taskStore}
	get := func(user, query string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/tasks/trace/events"+query, nil)
		req = req.WithContext(context.WithValue(req.Context(), ctxSessionKey, &SessionInfo{UserID: user}))
		w := httptest.NewRecorder()
		s.handleTaskRoutes(w, req)
		return w
	}
	if w := get(env.user.ID, ""); w.Code != 404 || strings.Contains(w.Body.String(), "可审阅正文") {
		t.Fatalf("foreign trace: %d", w.Code)
	}
	for _, q := range []string{"?after_seq=-1", "?after_seq=oops", "?limit=0", "?limit=1001"} {
		if w := get(env.admin.ID, q); w.Code != 400 {
			t.Fatalf("invalid cursor accepted: %s %d", q, w.Code)
		}
	}
	for _, tc := range []struct {
		query string
		count int
		next  int64
	}{{"", 200, 200}, {"?after_seq=200", 5, 205}, {"?after_seq=205", 0, 205}} {
		w := get(env.admin.ID, tc.query)
		var result struct {
			Items []store.TaskEvent `json:"items"`
			Next  int64             `json:"next_seq"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || len(result.Items) != tc.count || result.Next != tc.next {
			t.Fatalf("cursor %s: %d %s", tc.query, w.Code, w.Body.String())
		}
	}
}
