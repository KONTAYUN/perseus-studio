package store

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
)

func TestAnswerUserChoiceResumesSameRunAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "followup-answer.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	conversations := NewConversationStore(db)
	if err := conversations.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	runs := NewAgentRunStore(db)
	if err := runs.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	conversation, err := conversations.CreateConversation(ctx, CreateConversationInput{UserID: "user-a", SelectedModelProviderID: "provider-a"})
	if err != nil {
		t.Fatal(err)
	}
	userMessage, err := conversations.AppendMessage(ctx, "user-a", conversation.ID, "user", "inspect the server", `[]`)
	if err != nil {
		t.Fatal(err)
	}
	questionAttachments := `[{"tool_call_id":"call-followup","tool_name":"request_user_choice","status":"waiting_for_user","question":"Run it?","options":["Yes","No"]}]`
	assistantMessage, err := conversations.AppendMessageWithMeta(ctx, MessageInput{
		UserID: "user-a", ConversationID: conversation.ID, Role: "assistant", Content: "Run it?",
		AttachmentsJSON: questionAttachments, Status: "final", MetaJSON: `{"run_id":"run-followup"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := runs.CreateRun(ctx, CreateAgentRunInput{
		ID: "run-followup", UserID: "user-a", ConversationID: conversation.ID,
		UserMessageID: userMessage.ID, ModelProviderID: "provider-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runs.UpdateRun(ctx, "user-a", run.ID, UpdateAgentRunInput{
		Status: AgentRunWaiting, Stage: "waiting_for_user", AssistantMessageID: assistantMessage.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := conversations.StartToolCall(ctx, ToolCallInput{
		ID: "call-followup", UserID: "user-a", RunID: run.ID, ConversationID: conversation.ID,
		MessageID: userMessage.ID, ToolName: "request_user_choice", ArgumentsJSON: `{"question":"Run it?","options":["Yes","No"]}`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := conversations.AppendAgentTurnItem(ctx, AgentTurnItemInput{
		UserID: "user-a", ConversationID: conversation.ID, RunID: run.ID, TurnID: "turn-followup",
		Kind: "tool_call", Role: "assistant", Status: "started", Visibility: AgentItemVisibilityModel,
		SourceID: "call-followup", PayloadJSON: `{"tool_call_id":"call-followup","tool_name":"request_user_choice"}`,
	}); err != nil {
		t.Fatal(err)
	}

	answeredAttachments := `[{"tool_call_id":"call-followup","tool_name":"request_user_choice","status":"answered","question":"Run it?","options":["Yes","No"],"answer_message_id":"message-answer"}]`
	input := AgentFollowupAnswerInput{
		UserID: "user-a", RunID: run.ID, ToolCallID: "call-followup", AnswerID: "answer-1", Answer: "Yes",
		AnswerMessageID: "message-answer", AssistantMessageID: assistantMessage.ID,
		AssistantAttachmentsJSON: answeredAttachments, TurnID: "turn-followup",
		ToolResultPayloadJSON: `{"tool_call_id":"call-followup","tool_name":"request_user_choice","content":"<user_message>\nYes\n</user_message>","status":"answered"}`,
		CheckpointJSON:        `{"current_round":4}`, MessageMetaJSON: `{"kind":"user_response","run_id":"run-followup"}`,
	}
	result, err := runs.AnswerFollowupQuestion(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.AlreadyApplied || result.Run.ID != run.ID || result.Run.Status != AgentRunQueued || result.UserMessage.Content != "Yes" {
		t.Fatalf("unexpected answer result: %#v", result)
	}
	toolCall, err := conversations.GetToolCall(ctx, "user-a", "call-followup")
	if err != nil || toolCall.Status != "success" || toolCall.ResultJSON == "" {
		t.Fatalf("tool call was not completed: call=%#v err=%v", toolCall, err)
	}
	storedQuestion, err := conversations.GetMessage(ctx, "user-a", assistantMessage.ID)
	if err != nil {
		t.Fatal(err)
	}
	var attachments []map[string]any
	if err := json.Unmarshal(storedQuestion.Attachments, &attachments); err != nil || attachments[0]["status"] != "answered" {
		t.Fatalf("question attachment was not persisted as answered: %s err=%v", storedQuestion.Attachments, err)
	}
	items, err := conversations.ListAgentTurnItems(ctx, "user-a", conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	foundToolResult := false
	for _, item := range items {
		if item.Kind == "tool_result" && item.SourceID == run.ID+":tool_result:call-followup" && item.TurnID == "turn-followup" {
			foundToolResult = true
		}
	}
	if !foundToolResult {
		t.Fatalf("answer tool result was not appended to the original turn: %#v", items)
	}
	checkpoint, err := runs.LatestCheckpoint(ctx, "user-a", run.ID)
	if err != nil || checkpoint.StateJSON != input.CheckpointJSON {
		t.Fatalf("answer checkpoint was not saved: checkpoint=%#v err=%v", checkpoint, err)
	}

	replayed, err := runs.AnswerFollowupQuestion(ctx, input)
	if err != nil || !replayed.AlreadyApplied || replayed.UserMessage.ID != result.UserMessage.ID {
		t.Fatalf("same answer_id must be idempotent: result=%#v err=%v", replayed, err)
	}
	conflicting := input
	conflicting.AnswerID = "answer-2"
	conflicting.AnswerMessageID = "message-answer-2"
	_, err = runs.AnswerFollowupQuestion(ctx, conflicting)
	var conflict *AgentFollowupAnswerConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("different second answer must conflict, got %v", err)
	}
}
