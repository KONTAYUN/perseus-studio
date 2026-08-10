package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"webide-backend/internal/store"
)

func TestTurnEngineRejectsPlainTextAndCompletesThroughExplicitAction(t *testing.T) {
	env := newTurnEngineTestAIEnv(t)
	ctx := context.Background()
	var requestCount atomic.Int32
	var streamed strings.Builder
	protocolRetrySeen := false
	var firstRequestSummary map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid provider payload: %v", err)
		}
		switch requestCount.Add(1) {
		case 1:
			assertOpenAIToolChoiceAuto(t, payload)
			writeOpenAIStreamTextAndToolCall(t, w, "I will inspect first.", "inspect_1", "inspect_workspace", `{"path":"","max_depth":1}`)
		case 2:
			assertOpenAIToolChoiceAuto(t, payload)
			writeOpenAIStreamText(t, w, "Inspection is complete.")
		case 3:
			assertOpenAIToolChoiceAuto(t, payload)
			assertProtocolFeedbackIsSystemOwned(t, payload)
			writeOpenAIToolCall(t, w, "complete_1", "attempt_completion", `{"result":"Inspection is complete."}`)
		default:
			t.Fatalf("unexpected provider request %d", requestCount.Load())
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "inspect the workspace")
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{
		UserID: testUserID,
		Event: func(eventType, _ string, payload map[string]any) {
			switch eventType {
			case "model_delta":
				if text, _ := payload["text"].(string); text != "" {
					streamed.WriteString(text)
				}
			case "agent_protocol_retry":
				protocolRetrySeen = true
			case "model_request_summary":
				if firstRequestSummary == nil {
					firstRequestSummary = payload
				}
			}
		},
	})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if requestCount.Load() != 3 || reply.Content != "Inspection is complete." {
		t.Fatalf("unexpected reply requests=%d reply=%q", requestCount.Load(), reply.Content)
	}
	if reply.TerminalState != AgentTerminationCompleted {
		t.Fatalf("expected explicit completion terminal state, got %q", reply.TerminalState)
	}
	if !protocolRetrySeen {
		t.Fatal("expected a protocol retry event for the rejected plain response")
	}
	if got := streamed.String(); got != "I will inspect first." {
		t.Fatalf("only tool-associated commentary should reach the message stream, got %q", got)
	}
	if firstRequestSummary == nil {
		t.Fatal("expected model_request_summary event")
	}
	if got, _ := firstRequestSummary["tools_count"].(int); got == 0 {
		t.Fatalf("first request summary must report the tools sent to the provider: %#v", firstRequestSummary)
	}
	if got, _ := firstRequestSummary["has_tools"].(bool); !got {
		t.Fatalf("first request summary must report has_tools=true: %#v", firstRequestSummary)
	}
	toolNames, _ := firstRequestSummary["tool_names"].([]string)
	if !containsString(toolNames, "read_workspace_note_view") || !containsString(toolNames, requestUserInputToolName) || !containsString(toolNames, requestUserChoiceToolName) {
		t.Fatalf("first request summary must report the actual provider tools: %#v", firstRequestSummary)
	}
	loadedToolsets, _ := firstRequestSummary["loaded_toolsets"].([]string)
	if !containsString(loadedToolsets, "workspace") {
		t.Fatalf("first request summary must report the loaded workspace toolset: %#v", firstRequestSummary)
	}
	if got, _ := firstRequestSummary["tool_choice"].(string); got != "auto" {
		t.Fatalf("first request summary tool_choice=%q, want auto", got)
	}
	if got, _ := firstRequestSummary["prompt_policy_version"].(string); got != agentActionPolicyVersion {
		t.Fatalf("first request summary prompt_policy_version=%q, want %q", got, agentActionPolicyVersion)
	}
	if got, _ := firstRequestSummary["cumulative_estimated_input_tokens"].(int); got <= 0 {
		t.Fatalf("first request summary must report cumulative input tokens: %#v", firstRequestSummary)
	}
	items, err := env.conversations.ListAgentTurnItems(ctx, testUserID, conversation.ID)
	if err != nil {
		t.Fatalf("ListAgentTurnItems returned error: %v", err)
	}
	var rejectedText, protocolFeedback bool
	for _, item := range items {
		switch {
		case item.Kind == "assistant_message" && item.Status == "rejected" && strings.Contains(contextItemContent(item), "Inspection is complete"):
			rejectedText = true
		case item.Kind == "protocol_feedback" && strings.Contains(contextItemContent(item), "attempt_completion"):
			protocolFeedback = true
		}
	}
	if !rejectedText || !protocolFeedback {
		t.Fatalf("expected rejected assistant and protocol feedback items, got %#v", items)
	}
}

func TestTurnEngineCanExploreRelevantCurrentDocumentBeforeCompleting(t *testing.T) {
	env := newTurnEngineTestAIEnv(t)
	ctx := context.Background()
	if _, err := env.files.CreateFile(ctx, "reference.html", "<p>Deployment region: east-zone</p>"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid provider payload: %v", err)
		}
		requestText := string(raw)
		switch requestCount.Add(1) {
		case 1:
			if !strings.Contains(requestText, "Local Perseus editor environment for this turn") || !strings.Contains(requestText, "- reference.html") {
				t.Fatalf("current editor path missing from first request: %s", requestText)
			}
			if strings.Contains(requestText, "east-zone") {
				t.Fatalf("current editor metadata leaked file body before a read: %s", requestText)
			}
			names := openAIToolNamesFromPayload(t, payload)
			if !names["read_workspace_note_view"] || !names[requestUserInputToolName] || !names[requestUserChoiceToolName] {
				t.Fatalf("workspace exploration and follow-up tools must both remain available: %#v", names)
			}
			writeOpenAIToolCall(t, w, "read_current_1", "read_workspace_note_view", `{"path":"reference.html","view":"text","mode":"full"}`)
		case 2:
			if !strings.Contains(requestText, "east-zone") {
				t.Fatalf("successful document read missing from next model request: %s", requestText)
			}
			writeOpenAIToolCall(t, w, "complete_current_1", "attempt_completion", `{"result":"The configured region is east-zone."}`)
		default:
			t.Fatalf("unexpected provider request %d", requestCount.Load())
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "Use the current document to report the configured region.")
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, []AttachmentPayload{{
		Kind: "current_file", Path: "reference.html", Name: "reference.html", FileType: "html",
	}}, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if requestCount.Load() != 2 || reply.Content != "The configured region is east-zone." {
		t.Fatalf("unexpected current-document flow requests=%d reply=%q", requestCount.Load(), reply.Content)
	}
	for _, event := range reply.RunEvents {
		if isUserRequestTool(event.ToolName) {
			t.Fatalf("current-document flow asked the user instead of using the available read: %#v", reply.RunEvents)
		}
	}
}

func assertProtocolFeedbackIsSystemOwned(t *testing.T, payload map[string]any) {
	t.Helper()
	messages, _ := payload["messages"].([]any)
	found := false
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		content, _ := message["content"].(string)
		if !strings.Contains(content, "AUTOMATED AGENT PROTOCOL FEEDBACK") {
			continue
		}
		found = true
		if message["role"] != "system" {
			t.Fatalf("protocol feedback must be runtime-owned system context, got %#v", message)
		}
	}
	if !found {
		t.Fatalf("expected protocol feedback in provider request: %#v", payload["messages"])
	}
}

func TestTurnEngineRepeatDetectorMatchesCanonicalIdenticalCallRule(t *testing.T) {
	detector := agentTurnRepeatDetector{}
	call := agentToolCall{Name: "read_workspace_text", ArgumentsJSON: `{"path":"draft.txt","start_line":1}`}
	for i := 0; i < agentTurnRepeatLimit; i++ {
		if !detector.allow(call) {
			t.Fatalf("call %d should remain allowed", i+1)
		}
	}
	if detector.allow(call) {
		t.Fatal("fourth consecutive identical call should be blocked")
	}
	if !detector.allow(agentToolCall{Name: "read_workspace_text", ArgumentsJSON: `{"path":"draft.txt","start_line":2}`}) {
		t.Fatal("a changed target must reset the identical-call detector")
	}
}

func TestTurnEngineManifestUsesStructuredPatchTool(t *testing.T) {
	runtime := &agentRuntime{
		provider:       store.ModelProvider{Protocol: string(agentProtocolOpenAI)},
		loadedToolsets: map[string]bool{"workspace": true},
		opts:           RunOptions{UserRole: "admin"},
	}
	manifest := runtime.turnEngineActionManifest()
	if !containsModelTerminalAction(manifest, "attempt_completion") {
		t.Fatalf("turn engine must expose attempt_completion: %#v", manifest.TerminalActions)
	}
	if !containsModelJSONTool(manifest, "apply_workspace_patch") {
		t.Fatalf("turn engine must expose apply_workspace_patch as a JSON tool: %#v", manifest.JSONTools)
	}
}

func TestCompletionRejectsPatchEnvelopeAsFinalText(t *testing.T) {
	if !completionResultIsWorkspacePatch("*** Begin Patch\n*** Add File: draft.txt\n+content\n*** End Patch") {
		t.Fatal("a complete patch envelope must be rejected from attempt_completion")
	}
	if completionResultIsWorkspacePatch("I applied the requested update.") {
		t.Fatal("ordinary final Markdown must remain valid")
	}
}

func TestTurnEnginePersistsAbortedResultsForSkippedToolCalls(t *testing.T) {
	calls := []agentToolCall{
		{ID: "call_1", Name: "inspect_workspace", ArgumentsJSON: `{}`},
		{ID: "call_2", Name: "read_workspace_text", ArgumentsJSON: `{"path":"draft.txt"}`},
	}
	runtime := &agentRuntime{
		transcript: newAgentTranscript(),
		kernel:     newAgentKernelState(),
	}
	runtime.transcript.AppendAssistantTurn(agentModelTurn{ToolCalls: calls})
	runtime.appendTurnEngineAbortedToolCalls(calls, "test stop")

	normalized, actions := normalizeTranscriptToolPairs(runtime.transcript)
	if len(actions) != 0 {
		t.Fatalf("explicit aborted results must not need recovery normalization: %#v", actions)
	}
	var results []TranscriptToolResult
	for _, entry := range normalized.Entries {
		results = append(results, entry.ToolResults...)
	}
	if len(results) != 2 {
		t.Fatalf("expected two paired aborted results, got %#v", normalized.Entries)
	}
	for _, result := range results {
		if !result.IsError || result.Event.Status != "aborted" {
			t.Fatalf("expected explicit aborted result, got %#v", result)
		}
	}
}

func TestPresentationEventsExcludeRestoredRunHistory(t *testing.T) {
	runtime := &agentRuntime{transcript: newAgentTranscript()}
	runtime.restoreCheckpoint(AgentRuntimeCheckpoint{
		Transcript: newAgentTranscript(),
		Events: []ToolEvent{{
			ToolCallID: "old-question",
			ToolName:   "ask_followup_question",
			Status:     "answered",
		}},
	})
	runtime.beginPresentationSegment()
	runtime.events = append(runtime.events, ToolEvent{
		ToolCallID: "new-read",
		ToolName:   "read_workspace_note_view",
		Status:     "success",
	})

	runEvents, presentationEvents := runtime.replyEvents()
	if len(runEvents) != 2 || runEvents[0].ToolCallID != "old-question" {
		t.Fatalf("run events must retain restored history: %#v", runEvents)
	}
	if len(presentationEvents) != 1 || presentationEvents[0].ToolCallID != "new-read" {
		t.Fatalf("presentation events must contain only the resumed stage: %#v", presentationEvents)
	}
}
