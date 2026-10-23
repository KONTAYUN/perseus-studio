package tasks

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"webide-backend/internal/store"
)

func TestAcceptanceBackgroundPromptsUseStructuredPatchThroughRepair(t *testing.T) {
	requests := 0
	env := newTraceTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		raw, _ := json.Marshal(body)
		if strings.Contains(string(raw), "系统会把它当作 workspace patch 执行") || strings.Contains(string(raw), "patch 块让系统执行") || strings.Contains(string(raw), "assistant text is also supported") {
			t.Error("provider receives a stale prose-execution instruction")
		}
		messages, _ := json.Marshal(body["messages"])
		if !strings.Contains(string(messages), "apply_workspace_patch") || !strings.Contains(string(messages), "Assistant text and code blocks do not execute file changes") {
			t.Error("background/repair message does not specify structured editing")
		}
		switch requests {
		case 1:
			traceToolResponse(w, "", "read_workspace_text", `{"path":"source.md"}`)
		case 2:
			traceToolResponse(w, "", "attempt_completion", `{"result":"读取结束。"}`)
		case 3:
			traceToolResponse(w, "", "apply_workspace_patch", `{"patch":"*** Begin Patch\n*** Add File: report.md\n+真实报告\n*** End Patch"}`)
		case 4:
			traceToolResponse(w, "", "read_workspace_text", `{"path":"report.md"}`)
		default:
			traceToolResponse(w, "", "attempt_completion", `{"result":"报告已保存并回读。"}`)
		}
	})
	env.m.executeAIAgentTask(t.Context(), env.task, "worker")
	task, _ := env.m.store.GetTaskByID(t.Context(), env.task.ID)
	files, _ := env.m.factory.Workspace("owner")
	file, err := files.ReadFile("report.md")
	if task.Status != store.TaskDone || err != nil || file.Content != "真实报告\n" || requests != 5 {
		t.Fatalf("structured repair did not persist: status=%s content=%q requests=%d err=%v", task.Status, file.Content, requests, err)
	}
}

func TestAcceptanceBackgroundFailureKeepsExecutionModelSnapshot(t *testing.T) {
	for _, stage := range []string{"provider_failure", "validation_exhausted", "repair_failure"} {
		t.Run(stage, func(t *testing.T) {
			var env traceTestEnv
			requests := 0
			env = newTraceTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
				requests++
				if stage == "provider_failure" || (stage == "repair_failure" && requests == 2) {
					// Mutate configuration after selection: failure attribution must keep
					// the executed model, rather than looking up the latest default/name.
					_, err := env.db.Exec(`UPDATE model_providers SET name='renamed-after-start',model_id='different-after-start' WHERE id=?`, env.payload.ModelProviderID)
					if err != nil {
						t.Error(err)
					}
					http.Error(w, "fixture unauthorized", http.StatusUnauthorized)
					return
				}
				traceToolResponse(w, "", "attempt_completion", `{"result":"未生成报告。"}`)
			})
			env.m.diagnostics = store.NewDiagnosticStore(env.db)
			if err := env.m.diagnostics.Migrate(t.Context()); err != nil {
				t.Fatal(err)
			}
			env.m.executeAIAgentTask(t.Context(), env.task, "worker")
			logs, err := env.m.diagnostics.List(t.Context(), store.DiagnosticLogQuery{UserID: "owner", Limit: 100})
			if err != nil {
				t.Fatal(err)
			}
			var found bool
			for _, log := range logs {
				if log.Stage != "background_task_failed" {
					continue
				}
				found = true
				if log.ModelProviderID != env.payload.ModelProviderID || log.ModelName != "mock" || log.ModelID != "mock" || log.Protocol != "openai" || log.TaskID != env.task.ID || log.MessageID != env.payload.UserMessageID {
					t.Errorf("missing/wrong execution identity: %#v", log)
				}
			}
			if !found {
				t.Fatal("background failure diagnostic missing")
			}
		})
	}
}
