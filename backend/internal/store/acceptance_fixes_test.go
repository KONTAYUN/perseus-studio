package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestAttachmentPayloadAdmissionBranchPrivacyAndDeletion(t *testing.T) {
	db, runs, conversations, conv := runPreparationEnv(t)
	if err := NewTaskStore(db).Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	input := PrepareConversationRunInput{UserID: "u", ConversationID: conv.ID, Content: "附件", AttachmentPayloadsJSON: `[{"Kind":"text","Text":"PRIVATE_BODY"}]`}
	if _, err := db.Exec(`CREATE TRIGGER reject_input BEFORE INSERT ON agent_events BEGIN SELECT RAISE(FAIL,'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := runs.PrepareConversationRun(t.Context(), input); err == nil {
		t.Fatal("expected rollback")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM message_attachment_payloads`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("orphan payload: %d %v", count, err)
	}
	db.Exec(`DROP TRIGGER reject_input`)
	prepared, err := runs.PrepareConversationRun(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := conversations.GetMessage(t.Context(), "u", prepared.Message.ID)
	if err != nil || got.AttachmentPayloadsJSON != input.AttachmentPayloadsJSON {
		t.Fatalf("payload lost: %+v %v", got, err)
	}
	public, err := json.Marshal(got)
	if err != nil || strings.Contains(string(public), "PRIVATE_BODY") {
		t.Fatal("payload exposed", err)
	}
	if _, err := conversations.GetMessage(t.Context(), "other", got.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("ownership %v", err)
	}
	if _, _, err := runs.CompleteRun(t.Context(), "u", prepared.Run.ID, CompleteAgentRunInput{}); err != nil {
		t.Fatal(err)
	}
	branch, messages, err := conversations.BranchConversation(t.Context(), "u", conv.ID, got.ID)
	if err != nil || len(messages) != 1 || messages[0].AttachmentPayloadsJSON != input.AttachmentPayloadsJSON {
		t.Fatalf("branch lost payload: %v", err)
	}
	for _, id := range []string{conv.ID, branch.ID} {
		if err := conversations.DeleteConversation(t.Context(), "u", id); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM message_attachment_payloads`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("payload not cascaded: %d %v", count, err)
	}
}

func TestHandoffAtomicAtEveryEventAndIdempotent(t *testing.T) {
	for _, kind := range []string{"task_handoff_completed", "assistant_message_completed", "run_completed"} {
		t.Run(kind, func(t *testing.T) {
			db, runs, conversations, conv := runPreparationEnv(t)
			tasks := NewTaskStore(db)
			if err := tasks.Migrate(t.Context()); err != nil {
				t.Fatal(err)
			}
			prepared, err := runs.PrepareConversationRun(t.Context(), PrepareConversationRunInput{UserID: "u", ConversationID: conv.ID, Content: "后台"})
			if err != nil {
				t.Fatal(err)
			}
			input := CreateTaskInput{Title: "handoff", PayloadJSON: fmt.Sprintf(`{"source_run_id":%q,"conversation_id":%q}`, prepared.Run.ID, conv.ID)}
			if _, err := db.Exec(fmt.Sprintf(`CREATE TRIGGER reject_handoff BEFORE INSERT ON agent_events WHEN NEW.type='%s' BEGIN SELECT RAISE(FAIL,'injected'); END`, kind)); err != nil {
				t.Fatal(err)
			}
			if _, err := runs.CompleteTaskHandoff(t.Context(), "u", prepared.Run.ID, input); err == nil {
				t.Fatal("expected rollback")
			}
			items, _ := tasks.ListTasks(t.Context(), "u", 10)
			messages, _ := conversations.ListMessages(t.Context(), "u", conv.ID)
			current, _ := runs.GetRun(t.Context(), "u", prepared.Run.ID)
			if len(items) != 0 || len(messages) != 1 || current.Status != AgentRunQueued {
				t.Fatal("partial handoff visible")
			}
			if _, _, err := runs.FailRun(t.Context(), "u", prepared.Run.ID, AgentRunFailed, "failed", UpdateAgentRunInput{}, nil); err != nil {
				t.Fatal(err)
			}
			if _, _, err := runs.ResumeRunWithEvent(t.Context(), "u", prepared.Run.ID); err != nil {
				t.Fatal(err)
			}
			db.Exec(`DROP TRIGGER reject_handoff`)
			first, err := runs.CompleteTaskHandoff(t.Context(), "u", prepared.Run.ID, input)
			if err != nil {
				t.Fatal(err)
			}
			second, err := runs.CompleteTaskHandoff(t.Context(), "u", prepared.Run.ID, input)
			if err != nil {
				t.Fatal(err)
			}
			if first.Task.ID != second.Task.ID || first.Message.ID != second.Message.ID || len(second.Events) != 0 {
				t.Fatal("duplicate handoff")
			}
			items, _ = tasks.ListTasks(t.Context(), "u", 10)
			messages, _ = conversations.ListMessages(t.Context(), "u", conv.ID)
			if len(items) != 1 || len(messages) != 2 {
				t.Fatal("duplicate durable side effects")
			}
		})
	}
}

func TestFailureTerminalAtomicAndUniquePerAttempt(t *testing.T) {
	db, runs, _, conv := runPreparationEnv(t)
	prepared, err := runs.PrepareConversationRun(t.Context(), PrepareConversationRunInput{UserID: "u", ConversationID: conv.ID, Content: "test"})
	if err != nil {
		t.Fatal(err)
	}
	db.Exec(`CREATE TRIGGER reject_failure BEFORE INSERT ON agent_events WHEN NEW.type='run_failed' BEGIN SELECT RAISE(FAIL,'injected'); END`)
	fail := func() (AgentRun, AgentEvent, error) {
		return runs.FailRun(t.Context(), "u", prepared.Run.ID, AgentRunFailed, "failed", UpdateAgentRunInput{}, nil)
	}
	if _, _, err := fail(); err == nil {
		t.Fatal("accepted missing terminal")
	}
	current, _ := runs.GetRun(t.Context(), "u", prepared.Run.ID)
	if current.Status != AgentRunQueued {
		t.Fatal("partial failure commit")
	}
	db.Exec(`DROP TRIGGER reject_failure`)
	for attempt := 0; attempt < 2; attempt++ {
		if _, event, err := fail(); err != nil || event.Type != "run_failed" {
			t.Fatalf("first terminal %v %v", event, err)
		}
		if _, event, err := fail(); err != nil || event.ID != "" {
			t.Fatal("duplicate terminal", err)
		}
		if _, _, err := runs.ResumeRunWithEvent(t.Context(), "u", prepared.Run.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := runs.CompleteRun(t.Context(), "u", prepared.Run.ID, CompleteAgentRunInput{}); err != nil {
		t.Fatal(err)
	}
	current, event, err := fail()
	if err != nil || current.Status != AgentRunSucceeded || event.ID != "" {
		t.Fatal("late failure overwrote success")
	}
	events, _ := runs.ListEvents(t.Context(), "u", prepared.Run.ID)
	count := 0
	for _, event := range events {
		if event.Type == "run_failed" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("failure count %d", count)
	}
}
