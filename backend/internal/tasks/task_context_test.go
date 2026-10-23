package tasks

import (
	"net/http"
	"testing"
)

func TestBackgroundContextKeepsSourceUploadsAndExcludesLaterTurns(t *testing.T) {
	env := newTraceTestEnv(t, func(http.ResponseWriter, *http.Request) {})
	body := `[{"Kind":"text","Text":"source fact"}]`
	if _, err := env.db.Exec(`INSERT INTO message_attachment_payloads(message_id,user_id,payload_json) VALUES(?,?,?)`, env.payload.UserMessageID, "owner", body); err != nil {
		t.Fatal(err)
	}
	if _, err := env.m.conversations.AppendMessage(t.Context(), "owner", env.payload.ConversationID, "user", "LATER_UNRELATED_INPUT", "[]"); err != nil {
		t.Fatal(err)
	}
	messages, err := env.m.taskMessages(t.Context(), "owner", env.payload, "TASK_EXECUTION")
	if err != nil || len(messages) != 2 || messages[0].AttachmentPayloadsJSON != body || messages[1].Content != "TASK_EXECUTION" {
		t.Fatalf("wrong task input: %+v %v", messages, err)
	}
	if _, err := env.m.taskMessages(t.Context(), "other", env.payload, "prompt"); err == nil {
		t.Fatal("foreign source accepted")
	}
}
