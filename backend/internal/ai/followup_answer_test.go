package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"webide-backend/internal/store"
)

func TestUserRequestToolSchemasSeparateOpenInputFromChoice(t *testing.T) {
	inputFunction := openAIUserInputTool()["function"].(map[string]any)
	inputSchema := inputFunction["parameters"].(map[string]any)
	inputRequired := inputSchema["required"].([]string)
	inputProperties := inputSchema["properties"].(map[string]any)
	if len(inputRequired) != 1 || inputRequired[0] != "question" {
		t.Fatalf("unexpected input required fields: %#v", inputRequired)
	}
	if _, exists := inputProperties["options"]; exists {
		t.Fatal("open input schema must not expose options")
	}

	choiceFunction := openAIUserChoiceTool()["function"].(map[string]any)
	choiceSchema := choiceFunction["parameters"].(map[string]any)
	choiceProperties := choiceSchema["properties"].(map[string]any)
	options := choiceProperties["options"].(map[string]any)
	if options["minItems"] != 2 || options["maxItems"] != 3 {
		t.Fatalf("unexpected choice limits: %#v", options)
	}
	if anthropicUserInputTool()["description"] != userInputToolDescription || anthropicUserChoiceTool()["description"] != userChoiceToolDescription {
		t.Fatal("OpenAI and Anthropic must share the user request contracts")
	}
}

func TestHandleUserInputAcceptsQuestionWithoutOptions(t *testing.T) {
	runtime := &agentRuntime{}
	result, isError, completed := runtime.handleUserRequest(requestUserInputToolName, "call-input", `{"question":"Paste the output"}`)
	if isError || !completed || !strings.Contains(result, `"tool_name":"request_user_input"`) {
		t.Fatalf("expected open input wait, result=%s isError=%v completed=%v", result, isError, completed)
	}
	if len(runtime.events) != 1 || len(runtime.events[0].Options) != 0 || runtime.events[0].Type != "user_input_requested" {
		t.Fatalf("unexpected input request event: %#v", runtime.events)
	}
}

func TestHandleUserChoiceRequiresTwoOrThreeOptions(t *testing.T) {
	runtime := &agentRuntime{}
	result, isError, completed := runtime.handleUserRequest(requestUserChoiceToolName, "call-choice", `{"question":"Choose","options":["Only one"]}`)
	if !isError || completed || !strings.Contains(result, "2 or 3") {
		t.Fatalf("expected bounded choice protocol error, result=%s isError=%v completed=%v", result, isError, completed)
	}
}

func TestPrepareUserResponsePairsPendingToolResultAndPreservesMarkdown(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	run, err := env.runs.CreateRun(ctx, store.CreateAgentRunInput{
		UserID: testUserID, ConversationID: "conversation-followup", UserMessageID: "message-original", ModelProviderID: "provider-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	transcript := newAgentTranscript()
	transcript.AppendAssistantTurn(agentModelTurn{ToolCalls: []agentToolCall{{
		ID: "call-input", Name: requestUserInputToolName, ArgumentsJSON: `{"question":"Paste output"}`,
	}}})
	checkpoint := AgentRuntimeCheckpoint{
		Transcript: transcript,
		Events: []ToolEvent{{
			ToolCallID: "call-input", ToolName: requestUserInputToolName, Status: "waiting_for_user", Question: "Paste output",
		}},
		Kernel: AgentKernelState{TurnID: "turn-followup"}, CurrentRound: 3,
	}
	raw, _ := json.Marshal(checkpoint)
	if _, err := env.runs.SaveCheckpoint(ctx, testUserID, run.ID, string(raw)); err != nil {
		t.Fatal(err)
	}

	answer := "Use this output:\n\n```text\nready\n```"
	prepared, err := env.service.PrepareFollowupAnswer(ctx, RunOptions{UserID: testUserID, RunID: run.ID}, "call-input", answer, "message-answer")
	if err != nil {
		t.Fatal(err)
	}
	if prepared.TurnID != "turn-followup" || prepared.ToolName != requestUserInputToolName {
		t.Fatalf("answer lost its turn or tool identity: %#v", prepared)
	}
	if !strings.Contains(prepared.ToolResult, answer) || !strings.Contains(prepared.ToolResultPayload, "message-answer") {
		t.Fatalf("answer content was not preserved: result=%q payload=%s", prepared.ToolResult, prepared.ToolResultPayload)
	}
	if _, pending := checkpointPendingUserRequest(prepared.Checkpoint, "call-input"); pending {
		t.Fatal("answered checkpoint still reports a pending user request")
	}
	if len(prepared.Checkpoint.Transcript.Entries) != 2 || len(prepared.Checkpoint.Transcript.Entries[1].ToolResults) != 1 {
		t.Fatalf("tool result was not paired: %#v", prepared.Checkpoint.Transcript.Entries)
	}
}

func TestPrepareUserResponseSupportsLegacyFollowupCheckpoint(t *testing.T) {
	checkpoint := AgentRuntimeCheckpoint{Transcript: newAgentTranscript()}
	checkpoint.Transcript.AppendAssistantTurn(agentModelTurn{ToolCalls: []agentToolCall{{ID: "legacy", Name: legacyFollowupQuestionToolName}}})
	name, pending := checkpointPendingUserRequest(checkpoint, "legacy")
	if !pending || name != legacyFollowupQuestionToolName {
		t.Fatalf("legacy follow-up must remain answerable: name=%q pending=%v", name, pending)
	}
}
