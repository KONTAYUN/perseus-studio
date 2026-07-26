package api

import (
	"encoding/json"
	"testing"

	"webide-backend/internal/ai"
)

func TestAnsweredFollowupAttachmentsPreservesMarkdownAndMarksQuestionAnswered(t *testing.T) {
	raw := `[{"tool_call_id":"call-1","tool_name":"ask_followup_question","status":"waiting_for_user","question":"Run this?\n\n` + "```bash\\nssh server\\n```" + `","options":["Run it"],"allow_freeform":false}]`
	updated, err := answeredUserRequestAttachments(raw, "call-1", "ask_followup_question", "message-answer")
	if err != nil {
		t.Fatal(err)
	}
	var attachments []map[string]any
	if err := json.Unmarshal([]byte(updated), &attachments); err != nil {
		t.Fatal(err)
	}
	item := attachments[0]
	if item["status"] != "answered" || item["answer_message_id"] != "message-answer" {
		t.Fatalf("question was not marked answered: %#v", item)
	}
	if _, exists := item["allow_freeform"]; exists {
		t.Fatalf("legacy allow_freeform must not remain in new records: %#v", item)
	}
	if item["question"] != "Run this?\n\n```bash\nssh server\n```" {
		t.Fatalf("question Markdown changed: %q", item["question"])
	}
}

func TestAnsweredUserRequestAttachmentsSupportsNewRequestTypes(t *testing.T) {
	for _, toolName := range []string{"request_user_input", "request_user_choice"} {
		raw := `[{"tool_call_id":"call-1","tool_name":"` + toolName + `","status":"waiting_for_user","question":"Continue?","options":[]}]`
		updated, err := answeredUserRequestAttachments(raw, "call-1", toolName, "message-answer")
		if err != nil {
			t.Fatalf("%s: %v", toolName, err)
		}
		var attachments []map[string]any
		if err := json.Unmarshal([]byte(updated), &attachments); err != nil {
			t.Fatal(err)
		}
		if attachments[0]["status"] != "answered" || attachments[0]["answer_message_id"] != "message-answer" {
			t.Fatalf("%s request was not marked answered: %#v", toolName, attachments[0])
		}
	}
}

func TestAssistantPresentationKindDistinguishesInputChoiceAndLegacy(t *testing.T) {
	cases := []struct {
		toolName string
		want     string
	}{
		{"request_user_input", "user_input_request"},
		{"request_user_choice", "user_choice_request"},
		{"ask_followup_question", "followup_question"},
	}
	for _, tc := range cases {
		reply := ai.Reply{
			TerminalState:      ai.AgentTerminationWaitingForUser,
			PresentationEvents: []ai.ToolEvent{{ToolName: tc.toolName}},
		}
		if got := assistantPresentationKind(reply); got != tc.want {
			t.Fatalf("tool %s kind=%q want %q", tc.toolName, got, tc.want)
		}
	}
}
