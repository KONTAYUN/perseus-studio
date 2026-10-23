package store

import (
	"database/sql"
	"errors"
	"testing"
)

func TestPendingSteerConsumesInputOnlyWithDurableEventAndReplaysOnce(t *testing.T) {
	db, runs, _, conversation := runPreparationEnv(t)
	initial, err := runs.PrepareConversationRun(t.Context(), PrepareConversationRunInput{UserID: "u", ConversationID: conversation.ID, ModelProviderID: "model", Content: "initial"})
	if err != nil {
		t.Fatal(err)
	}
	p := PendingMessage{ID: "convert", ConversationID: conversation.ID, Content: "guidance", ModelProviderID: "model"}
	if _, err := runs.SavePendingMessage(t.Context(), "u", p); err != nil {
		t.Fatal(err)
	}
	in := AgentEventInput{UserID: "u", RunID: initial.Run.ID, ConversationID: conversation.ID, Type: "agent_steer_received", PendingMessageID: p.ID, Payload: map[string]any{"content": p.Content, "client_steer_id": "pending:" + p.ID, "pending_message_id": p.ID}}
	if _, err := db.Exec(`CREATE TRIGGER reject_steer BEFORE INSERT ON agent_events BEGIN SELECT RAISE(ABORT,'steer failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := runs.AppendEvent(t.Context(), in); err == nil {
		t.Fatal("fault did not trigger")
	}
	if _, err := runs.GetPendingMessage(t.Context(), "u", p.ID); err != nil {
		t.Fatal("failed steer lost input", err)
	}
	if _, err := db.Exec(`DROP TRIGGER reject_steer`); err != nil {
		t.Fatal(err)
	}
	first, err := runs.AppendEvent(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := runs.AppendEvent(t.Context(), in)
	if err != nil || second.ID != first.ID {
		t.Fatal("duplicate steer", first, second, err)
	}
	if _, err := runs.GetPendingMessage(t.Context(), "u", p.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("steer left a queued duplicate", err)
	}
	if _, err := runs.SavePendingMessage(t.Context(), "u", p); !errors.Is(err, ErrRequestConflict) {
		t.Fatal("converted identity queued again", err)
	}
	if _, err := runs.PendingSteerEvent(t.Context(), "intruder", p.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("steer disclosure", err)
	}
}

func TestPendingSteerRejectsAlteredInputAndTerminalRunWithoutConsuming(t *testing.T) {
	_, runs, _, conversation := runPreparationEnv(t)
	initial, err := runs.PrepareConversationRun(t.Context(), PrepareConversationRunInput{UserID: "u", ConversationID: conversation.ID, ModelProviderID: "model", Content: "initial"})
	if err != nil {
		t.Fatal(err)
	}
	p := PendingMessage{ID: "convert", ConversationID: conversation.ID, Content: "guidance", ModelProviderID: "model"}
	if _, err := runs.SavePendingMessage(t.Context(), "u", p); err != nil {
		t.Fatal(err)
	}
	in := AgentEventInput{UserID: "u", RunID: initial.Run.ID, ConversationID: conversation.ID, Type: "agent_steer_received", PendingMessageID: p.ID, Payload: map[string]any{"content": "altered", "client_steer_id": "pending:" + p.ID, "pending_message_id": p.ID}}
	if _, err := runs.AppendEvent(t.Context(), in); !errors.Is(err, ErrRequestConflict) {
		t.Fatal("altered input accepted", err)
	}
	in.Payload.(map[string]any)["content"] = p.Content
	if _, err := runs.UpdateRun(t.Context(), "u", initial.Run.ID, UpdateAgentRunInput{Status: AgentRunSucceeded}); err != nil {
		t.Fatal(err)
	}
	if _, err := runs.AppendEvent(t.Context(), in); !errors.Is(err, ErrConversationBusy) {
		t.Fatal("steer accepted after completion", err)
	}
	if _, err := runs.GetPendingMessage(t.Context(), "u", p.ID); err != nil {
		t.Fatal("rejected steer lost input", err)
	}
}
