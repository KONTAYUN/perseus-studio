package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"webide-backend/internal/store"
	"webide-backend/internal/workspace"
)

func TestGenerateReplyOpenAICreateFileAfterReminder(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}

		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid request payload: %v", err)
		}

		count := requestCount.Add(1)
		assertOpenAISystemPromptSupportsWrite(t, payload)

		switch count {
		case 1:
			assertOnlyCoreToolsLoaded(t, payload)
			writeOpenAIToolCall(t, w, "load_workspace_1", "load_perseus_toolset", `{"toolset":"workspace"}`)
		case 2:
			assertOpenAIWorkspaceToolsLoaded(t, payload)
			assertWorkspaceToolsetResultSupportsPatchEditing(t, lastOpenAIToolContent(t, payload))
			writeOpenAIToolCall(t, w, "structured_patch", "apply_workspace_patch", mustJSON(map[string]any{"patch": "*** Begin Patch\n*** Add File: ai-test.txt\n+hello from ai\n*** End Patch"}))
		case 3:
			writeOpenAIToolCall(t, w, "call_done_1", "attempt_completion", `{"result":"已创建 ai-test.txt。","changed_paths":["ai-test.txt"]}`)
		default:
			t.Fatalf("unexpected request count: %d", count)
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "请创建 ai-test.txt，内容写 hello from ai")
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}

	result, err := env.files.ReadFile("ai-test.txt")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if result.Content != "hello from ai\n" {
		t.Fatalf("unexpected file content: %q", result.Content)
	}
	if !strings.Contains(reply.Content, "ai-test.txt") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	assertToolCallRecord(t, env, "apply_workspace_patch", "success")
	if requestCount.Load() != 3 {
		t.Fatalf("expected toolset load, write, and completion requests, got %d", requestCount.Load())
	}
}

func TestGenerateReplyOpenAIAskFollowupQuestionCompletesRun(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid request payload: %v", err)
		}
		names := openAIToolNamesFromPayload(t, payload)
		if !names["ask_followup_question"] {
			t.Fatalf("ask_followup_question missing from payload: %#v", names)
		}
		if requestCount.Add(1) != 1 {
			t.Fatal("unexpected extra model request after follow-up question")
		}
		writeOpenAIToolCall(t, w, "call_question_1", "ask_followup_question", `{"question":"这篇文章主要面向谁？","options":["普通读者","专业编辑"],"allow_freeform":true}`)
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "帮我润色这篇文章")
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if reply.Content != "这篇文章主要面向谁？" {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	if len(reply.ToolEvents) != 1 || reply.ToolEvents[0].ToolName != "ask_followup_question" || len(reply.ToolEvents[0].Options) != 2 {
		t.Fatalf("unexpected tool events: %#v", reply.ToolEvents)
	}
	assertToolCallRecord(t, env, "ask_followup_question", "success")
}

func TestGenerateReplyOpenAIToolLoopStopsRepeatedInspectLoop(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid request payload: %v", err)
		}

		count := requestCount.Add(1)
		if count == 1 {
			handleOpenAIWorkspaceToolsetLoad(t, w, payload, "load_workspace_1")
			return
		}
		if count <= agentChatMaxRounds+2 {
			writeOpenAIToolCall(t, w, fmt.Sprintf("call_inspect_%d", count), "inspect_workspace", `{"path":"","max_depth":1}`)
			return
		}
		t.Fatalf("loop guard should stop before request %d", count)
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "请连续检查工作区并完成")
	_, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err == nil || !strings.Contains(err.Error(), "重复相同工具动作") {
		t.Fatalf("expected loop guard failure, got %v", err)
	}
	if requestCount.Load() >= int32(agentChatMaxRounds) {
		t.Fatalf("loop guard should stop before chat max rounds, got %d", requestCount.Load())
	}
}

func TestAgentLimitsAreDoubledForChatWorkspaceAndTask(t *testing.T) {
	if agentChatMaxRounds != 48 || agentChatMaxToolCalls != 60 {
		t.Fatalf("unexpected chat limits: rounds=%d tools=%d", agentChatMaxRounds, agentChatMaxToolCalls)
	}
	if agentWorkspaceMaxRounds != 96 || agentWorkspaceMaxToolCalls != 160 {
		t.Fatalf("unexpected workspace limits: rounds=%d tools=%d", agentWorkspaceMaxRounds, agentWorkspaceMaxToolCalls)
	}
	if agentTaskMaxRounds != 240 || agentTaskMaxToolCalls != 240 {
		t.Fatalf("unexpected task limits: rounds=%d tools=%d", agentTaskMaxRounds, agentTaskMaxToolCalls)
	}
}

func TestAgentBudgetExhaustionRequestsNoToolSummaryAndResetsCheckpointBudget(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid request payload: %v", err)
		}
		if _, ok := payload["tools"]; ok {
			t.Fatalf("budget summary request must not include tools: %s", raw)
		}
		if _, ok := payload["tool_choice"]; ok {
			t.Fatalf("budget summary request must not include tool_choice: %s", raw)
		}
		if requestCount.Add(1) != 1 {
			t.Fatal("unexpected extra model request")
		}
		writeJSON(t, w, map[string]any{
			"choices": []map[string]any{{
				"finish_reason": "stop",
				"message": map[string]any{
					"role":    "assistant",
					"content": "已达到上限：已检查工作区，下一步继续整理剩余文件。",
				},
			}},
		})
	}))
	defer server.Close()

	run, err := env.runs.CreateRun(ctx, store.CreateAgentRunInput{
		ID:              "run_budget_test",
		UserID:          testUserID,
		ConversationID:  "conv_budget_test",
		UserMessageID:   "msg_budget_test",
		ModelProviderID: "provider_budget_test",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	provider := store.ModelProvider{
		ID:                "provider_budget_test",
		Name:              "Budget Test",
		Protocol:          "openai",
		SystemMessageMode: "multiple",
		BaseURL:           server.URL,
		APIKey:            "test-key",
		ModelID:           "gpt-test",
		Enabled:           true,
	}
	runtime := &agentRuntime{
		service:          env.service,
		protocol:         agentProtocolOpenAI,
		conversationID:   "conv_budget_test",
		messageID:        "msg_budget_test",
		provider:         provider,
		messages:         []store.Message{{ID: "msg_budget_test", Role: "user", Content: "继续处理"}},
		opts:             RunOptions{UserID: testUserID, RunID: run.ID},
		profile:          runtimeProfileForProvider(provider),
		allowTask:        true,
		loadedToolsets:   allAgentToolsets(true),
		appliedMutations: map[string]ToolEvent{"hash-1": {ToolName: "save_workspace_file", Status: "success", Path: "draft.txt"}},
		toolLoopActive:   true,
		events: []ToolEvent{{
			ToolName:     "save_workspace_file",
			Status:       "success",
			Path:         "draft.txt",
			ChangedPaths: []string{"draft.txt"},
		}},
		transcript:        newAgentTranscript(),
		contextManager:    newAgentContextManager(),
		currentRound:      agentWorkspaceMaxRounds,
		toolCalls:         agentWorkspaceMaxToolCalls,
		performedMutation: true,
	}
	reply, err := runtime.run(ctx)
	if err != nil {
		t.Fatalf("runtime.run returned error: %v", err)
	}
	if reply.IncompleteReason != IncompleteReasonBudgetExhausted || !reply.Recoverable || reply.Budget == nil {
		t.Fatalf("expected recoverable budget reply, got %#v", reply)
	}
	if !strings.Contains(reply.Content, "已达到上限") {
		t.Fatalf("unexpected summary content: %q", reply.Content)
	}
	checkpoint, ok := env.service.loadRunCheckpoint(ctx, RunOptions{UserID: testUserID, RunID: run.ID})
	if !ok {
		t.Fatal("expected budget resume checkpoint")
	}
	if checkpoint.CurrentRound != 0 || checkpoint.ToolCalls != 0 {
		t.Fatalf("expected reset checkpoint budget, got round=%d tools=%d", checkpoint.CurrentRound, checkpoint.ToolCalls)
	}
	if len(checkpoint.AppliedMutations) != 1 {
		t.Fatalf("expected applied mutation ledger to be preserved, got %#v", checkpoint.AppliedMutations)
	}
}

func TestAgentBudgetExhaustionCanTriggerOnToolCallLimit(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid request payload: %v", err)
		}
		if _, ok := payload["tools"]; ok {
			t.Fatalf("budget summary request must not include tools: %s", raw)
		}
		if _, ok := payload["tool_choice"]; ok {
			t.Fatalf("budget summary request must not include tool_choice: %s", raw)
		}
		writeJSON(t, w, map[string]any{
			"choices": []map[string]any{{
				"finish_reason": "stop",
				"message": map[string]any{
					"role":    "assistant",
					"content": "工具预算已用完，下一步继续处理剩余文件。",
				},
			}},
		})
	}))
	defer server.Close()

	run, err := env.runs.CreateRun(ctx, store.CreateAgentRunInput{
		ID:              "run_budget_tools_test",
		UserID:          testUserID,
		ConversationID:  "conv_budget_tools_test",
		UserMessageID:   "msg_budget_tools_test",
		ModelProviderID: "provider_budget_tools_test",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	provider := store.ModelProvider{
		ID:                "provider_budget_tools_test",
		Name:              "Budget Tool Test",
		Protocol:          "openai",
		SystemMessageMode: "multiple",
		BaseURL:           server.URL,
		APIKey:            "test-key",
		ModelID:           "gpt-test",
		Enabled:           true,
	}
	runtime := &agentRuntime{
		service:          env.service,
		protocol:         agentProtocolOpenAI,
		conversationID:   "conv_budget_tools_test",
		messageID:        "msg_budget_tools_test",
		provider:         provider,
		messages:         []store.Message{{ID: "msg_budget_tools_test", Role: "user", Content: "继续处理"}},
		opts:             RunOptions{UserID: testUserID, RunID: run.ID},
		profile:          runtimeProfileForProvider(provider),
		allowTask:        true,
		loadedToolsets:   allAgentToolsets(true),
		appliedMutations: map[string]ToolEvent{},
		toolLoopActive:   true,
		transcript:       newAgentTranscript(),
		contextManager:   newAgentContextManager(),
		currentRound:     1,
		toolCalls:        agentWorkspaceMaxToolCalls,
	}

	reply, err := runtime.run(ctx)
	if err != nil {
		t.Fatalf("runtime.run returned error: %v", err)
	}
	if reply.IncompleteReason != IncompleteReasonBudgetExhausted || reply.Budget == nil {
		t.Fatalf("expected recoverable budget reply, got %#v", reply)
	}
	if reply.Budget.Reason != "max_tool_calls" {
		t.Fatalf("expected max_tool_calls reason, got %q", reply.Budget.Reason)
	}
	if reply.Budget.ToolCalls != agentWorkspaceMaxToolCalls {
		t.Fatalf("expected capped tool count %d, got %d", agentWorkspaceMaxToolCalls, reply.Budget.ToolCalls)
	}
}

func TestProcessToolCallsAddsSyntheticResultsForBudgetSkippedCalls(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	run, err := env.runs.CreateRun(ctx, store.CreateAgentRunInput{
		ID:              "run_budget_pairing_test",
		UserID:          testUserID,
		ConversationID:  "conv_budget_pairing_test",
		UserMessageID:   "msg_budget_pairing_test",
		ModelProviderID: "provider_budget_pairing_test",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	runtime := &agentRuntime{
		service:          env.service,
		protocol:         agentProtocolOpenAI,
		conversationID:   "conv_budget_pairing_test",
		messageID:        "msg_budget_pairing_test",
		opts:             RunOptions{UserID: testUserID, RunID: run.ID},
		profile:          providerRuntimeProfile{},
		allowTask:        true,
		loadedToolsets:   allAgentToolsets(true),
		appliedMutations: map[string]ToolEvent{},
		toolLoopActive:   true,
		transcript:       newAgentTranscript(),
		contextManager:   newAgentContextManager(),
		currentRound:     1,
		toolCalls:        agentWorkspaceMaxToolCalls - 1,
	}
	calls := []agentToolCall{
		{ID: "call_unknown_1", Name: "unknown_tool_one", ArgumentsJSON: `{}`},
		{ID: "", Name: "unknown_tool_two", ArgumentsJSON: `{}`},
		{ID: "call_unknown_1", Name: "unknown_tool_three", ArgumentsJSON: `{}`},
	}
	calls = normalizeAgentToolCallIDs(calls)
	runtime.appendAssistantTurn(agentModelTurn{ToolCalls: calls})

	if _, completed, err := runtime.processToolCalls(ctx, calls); err != nil || completed {
		t.Fatalf("processToolCalls returned completed=%v err=%v", completed, err)
	}
	if len(runtime.transcript.Entries) != 2 {
		t.Fatalf("expected assistant turn plus tool result turn, got %#v", runtime.transcript.Entries)
	}
	results := runtime.transcript.Entries[1].ToolResults
	if len(results) != len(calls) {
		t.Fatalf("expected one tool result per tool call, got %d results for %d calls", len(results), len(calls))
	}
	for index, result := range results {
		if result.ToolCallID != calls[index].ID {
			t.Fatalf("result %d has tool_call_id %q, want %q", index, result.ToolCallID, calls[index].ID)
		}
	}
	if calls[1].ID == "" || calls[1].ID == "call_unknown_1" || calls[2].ID == "call_unknown_1" {
		t.Fatalf("expected empty and duplicate tool call IDs to be normalized, got %#v", calls)
	}
	if !strings.Contains(results[1].Content, IncompleteReasonBudgetExhausted) || !strings.Contains(results[2].Content, IncompleteReasonBudgetExhausted) {
		t.Fatalf("expected skipped calls to receive budget_exhausted tool results: %#v", results)
	}
	snapshot := runtime.contextManager.Build(agentProtocolOpenAI, runtime.messages, nil, runtime.transcript, providerRuntimeProfile{}, runtime.currentRound)
	assistantToolCalls := 0
	toolMessages := 0
	for _, message := range snapshot.OpenAIMessages {
		if calls, ok := message["tool_calls"].([]openAIToolCall); ok {
			assistantToolCalls += len(calls)
		}
		if message["role"] == "tool" {
			toolMessages++
		}
	}
	if assistantToolCalls != len(calls) || toolMessages != len(calls) {
		t.Fatalf("expected provider context to preserve tool call/result pairing, got calls=%d results=%d", assistantToolCalls, toolMessages)
	}
}

func TestAgentAppliesPendingSteerBeforeNextModelTurn(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	var capturedMessages []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid request payload: %v", err)
		}
		if items, ok := payload["messages"].([]any); ok {
			for _, item := range items {
				if message, ok := item.(map[string]any); ok {
					capturedMessages = append(capturedMessages, message)
				}
			}
		}
		writeJSON(t, w, map[string]any{
			"choices": []map[string]any{{
				"finish_reason": "stop",
				"message": map[string]any{
					"role":    "assistant",
					"content": "收到引导，我会只处理用户指定的几集。",
				},
			}},
		})
	}))
	defer server.Close()

	conversation, err := env.conversations.CreateConversation(ctx, store.CreateConversationInput{
		UserID:                  testUserID,
		SelectedModelProviderID: "provider_steer_test",
	})
	if err != nil {
		t.Fatalf("CreateConversation returned error: %v", err)
	}
	userMessage, err := env.conversations.AppendMessage(ctx, testUserID, conversation.ID, "user", "继续处理", "[]")
	if err != nil {
		t.Fatalf("AppendMessage returned error: %v", err)
	}
	messages, err := env.conversations.ListMessages(ctx, testUserID, conversation.ID)
	if err != nil {
		t.Fatalf("ListMessages returned error: %v", err)
	}

	run, err := env.runs.CreateRun(ctx, store.CreateAgentRunInput{
		ID:              "run_steer_test",
		UserID:          testUserID,
		ConversationID:  conversation.ID,
		UserMessageID:   userMessage.ID,
		ModelProviderID: "provider_steer_test",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	if _, err := env.runs.UpdateRun(ctx, testUserID, run.ID, store.UpdateAgentRunInput{Status: store.AgentRunRunning, Stage: "calling_model"}); err != nil {
		t.Fatalf("UpdateRun returned error: %v", err)
	}
	if _, err := env.runs.AppendEvent(ctx, store.AgentEventInput{
		RunID:          run.ID,
		UserID:         testUserID,
		ConversationID: conversation.ID,
		Type:           "agent_steer_received",
		Level:          "info",
		Payload: map[string]any{
			"steer_id":        "steer-test",
			"client_steer_id": "client-steer-test",
			"content":         "不要重写全部，只处理我拖入的几集。",
			"content_preview": "不要重写全部，只处理我拖入的几集。",
		},
	}); err != nil {
		t.Fatalf("AppendEvent returned error: %v", err)
	}

	provider := store.ModelProvider{
		ID:                "provider_steer_test",
		Name:              "Steer Test",
		Protocol:          "openai",
		SystemMessageMode: "multiple",
		BaseURL:           server.URL,
		APIKey:            "test-key",
		ModelID:           "gpt-test",
		Enabled:           true,
	}
	runtime := &agentRuntime{
		service:          env.service,
		protocol:         agentProtocolOpenAI,
		conversationID:   conversation.ID,
		messageID:        userMessage.ID,
		provider:         provider,
		messages:         messages,
		opts:             RunOptions{UserID: testUserID, RunID: run.ID},
		profile:          runtimeProfileForProvider(provider),
		allowTask:        true,
		loadedToolsets:   allAgentToolsets(true),
		appliedMutations: map[string]ToolEvent{},
		transcript:       newAgentTranscript(),
		contextManager:   newAgentContextManager(),
	}
	reply, err := runtime.run(ctx)
	if err != nil {
		t.Fatalf("runtime.run returned error: %v", err)
	}
	if !strings.Contains(reply.Content, "收到引导") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	if !messagesContainContent(capturedMessages, "用户在任务运行中补充了最新引导") ||
		!messagesContainContent(capturedMessages, "不要重写全部，只处理我拖入的几集") {
		t.Fatalf("steer guidance missing from provider messages: %#v", capturedMessages)
	}
	events, err := env.runs.ListEvents(ctx, testUserID, run.ID)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if !agentEventsContain(events, "agent_steer_applied", "steer-test") {
		t.Fatalf("expected agent_steer_applied event, got %#v", events)
	}
	pending, err := pendingSteersForRun(ctx, env.runs, testUserID, run.ID)
	if err != nil {
		t.Fatalf("pendingSteersForRun returned error: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending steer after apply, got %#v", pending)
	}
	storedMessages, err := env.conversations.ListMessages(ctx, testUserID, conversation.ID)
	if err != nil {
		t.Fatalf("ListMessages returned error: %v", err)
	}
	if !messagesContainStoredSteer(storedMessages, "steer-test", "不要重写全部，只处理我拖入的几集。") {
		t.Fatalf("expected applied steer to be stored as a timeline message, got %#v", storedMessages)
	}
	assertStoredSteerTimelineMeta(t, storedMessages, "steer-test", "applied")
}

func TestAgentSkipsCanceledPendingSteer(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid request payload: %v", err)
		}
		if messagesContainContentFromPayload(payload, "已经取消的引导") {
			t.Fatalf("canceled steer leaked into provider request: %s", string(raw))
		}
		writeJSON(t, w, map[string]any{
			"choices": []map[string]any{{
				"finish_reason": "stop",
				"message": map[string]any{
					"role":    "assistant",
					"content": "没有收到新的引导。",
				},
			}},
		})
	}))
	defer server.Close()

	conversation, err := env.conversations.CreateConversation(ctx, store.CreateConversationInput{
		UserID:                  testUserID,
		SelectedModelProviderID: "provider_steer_cancel_test",
	})
	if err != nil {
		t.Fatalf("CreateConversation returned error: %v", err)
	}
	userMessage, err := env.conversations.AppendMessage(ctx, testUserID, conversation.ID, "user", "继续处理", "[]")
	if err != nil {
		t.Fatalf("AppendMessage returned error: %v", err)
	}
	messages, err := env.conversations.ListMessages(ctx, testUserID, conversation.ID)
	if err != nil {
		t.Fatalf("ListMessages returned error: %v", err)
	}

	run, err := env.runs.CreateRun(ctx, store.CreateAgentRunInput{
		ID:              "run_steer_cancel_test",
		UserID:          testUserID,
		ConversationID:  conversation.ID,
		UserMessageID:   userMessage.ID,
		ModelProviderID: "provider_steer_cancel_test",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	if _, err := env.runs.UpdateRun(ctx, testUserID, run.ID, store.UpdateAgentRunInput{Status: store.AgentRunRunning, Stage: "calling_model"}); err != nil {
		t.Fatalf("UpdateRun returned error: %v", err)
	}
	if _, err := env.runs.AppendEvent(ctx, store.AgentEventInput{
		RunID:          run.ID,
		UserID:         testUserID,
		ConversationID: conversation.ID,
		Type:           "agent_steer_received",
		Level:          "info",
		Payload: map[string]any{
			"steer_id":        "steer-canceled",
			"client_steer_id": "client-canceled",
			"content":         "已经取消的引导",
			"content_preview": "已经取消的引导",
		},
	}); err != nil {
		t.Fatalf("AppendEvent received returned error: %v", err)
	}
	if _, err := env.runs.AppendEvent(ctx, store.AgentEventInput{
		RunID:          run.ID,
		UserID:         testUserID,
		ConversationID: conversation.ID,
		Type:           "agent_steer_canceled",
		Level:          "info",
		Payload: map[string]any{
			"client_steer_id": "client-canceled",
			"reason":          "user_deleted_guidance",
		},
	}); err != nil {
		t.Fatalf("AppendEvent canceled returned error: %v", err)
	}

	provider := store.ModelProvider{
		ID:                "provider_steer_cancel_test",
		Name:              "Steer Cancel Test",
		Protocol:          "openai",
		SystemMessageMode: "multiple",
		BaseURL:           server.URL,
		APIKey:            "test-key",
		ModelID:           "gpt-test",
		Enabled:           true,
	}
	runtime := &agentRuntime{
		service:          env.service,
		protocol:         agentProtocolOpenAI,
		conversationID:   conversation.ID,
		messageID:        userMessage.ID,
		provider:         provider,
		messages:         messages,
		opts:             RunOptions{UserID: testUserID, RunID: run.ID},
		profile:          runtimeProfileForProvider(provider),
		allowTask:        true,
		loadedToolsets:   allAgentToolsets(true),
		appliedMutations: map[string]ToolEvent{},
		transcript:       newAgentTranscript(),
		contextManager:   newAgentContextManager(),
	}
	if _, err := runtime.run(ctx); err != nil {
		t.Fatalf("runtime.run returned error: %v", err)
	}
	pending, err := pendingSteersForRun(ctx, env.runs, testUserID, run.ID)
	if err != nil {
		t.Fatalf("pendingSteersForRun returned error: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected canceled steer not to be pending, got %#v", pending)
	}
	storedMessages, err := env.conversations.ListMessages(ctx, testUserID, conversation.ID)
	if err != nil {
		t.Fatalf("ListMessages returned error: %v", err)
	}
	if messagesContainStoredSteer(storedMessages, "steer-canceled", "已经取消的引导") {
		t.Fatalf("canceled steer should not be stored as a timeline message: %#v", storedMessages)
	}
}

func TestGenerateReplyOpenAIToolLoopRequiresToolAndSuppressesInvalidText(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}

		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid request payload: %v", err)
		}

		count := requestCount.Add(1)
		switch count {
		case 1:
			assertOpenAIToolChoiceAuto(t, payload)
			writeOpenAIStreamText(t, w, "好，我先看一下 workspace。")
		case 2:
			assertOpenAIToolChoiceAuto(t, payload)
			assertLastUserActionRejected(t, payload, "action_rejected")
			writeOpenAIToolCall(t, w, "call_inspect_1", "inspect_workspace", `{"path":"","max_depth":1}`)
		case 3:
			assertOpenAIToolChoiceAuto(t, payload)
			writeOpenAIToolCall(t, w, "call_done_1", "attempt_completion", `{"result":"已查看工作区。"}`)
		default:
			t.Fatalf("unexpected request count: %d", count)
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "看看引用文件")
	var streamed strings.Builder
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, []AttachmentPayload{{Kind: "workspace_file", Path: "demo.txt"}}, RunOptions{
		UserID: testUserID,
		Event: func(eventType, level string, payload map[string]any) {
			if eventType == "model_delta" {
				if text, _ := payload["text"].(string); text != "" {
					streamed.WriteString(text)
				}
			}
		},
	})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if requestCount.Load() != 3 {
		t.Fatalf("expected corrective retry plus tool completion, got %d requests", requestCount.Load())
	}
	if strings.Contains(streamed.String(), "好，我先看一下") {
		t.Fatalf("invalid no-tool assistant text was streamed to the user: %q", streamed.String())
	}
	if !strings.Contains(reply.Content, "已查看工作区") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
}

func TestGenerateReplyOpenAIStreamsFinalTextAfterToolResult(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}

		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid request payload: %v", err)
		}

		count := requestCount.Add(1)
		switch count {
		case 1:
			assertOpenAIToolChoiceAuto(t, payload)
			writeOpenAIStreamTextAndToolCall(t, w, "我先查看工作区，再决定下一步。", "call_inspect_1", "inspect_workspace", `{"path":"","max_depth":1}`)
		case 2:
			assertOpenAIToolChoiceAuto(t, payload)
			writeOpenAIToolCall(t, w, "call_done_1", "attempt_completion", `{"result":"已查看工作区。下面是整理后的结论。"}`)
		default:
			t.Fatalf("unexpected request count: %d", count)
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "看看引用文件")
	var streamed strings.Builder
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, []AttachmentPayload{{Kind: "workspace_file", Path: "demo.txt"}}, RunOptions{
		UserID: testUserID,
		Event: func(eventType, level string, payload map[string]any) {
			if eventType == "model_delta" {
				if text, _ := payload["text"].(string); text != "" {
					streamed.WriteString(text)
				}
			}
		},
	})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if requestCount.Load() != 2 {
		t.Fatalf("expected tool execution plus completion, got %d requests", requestCount.Load())
	}
	if !strings.Contains(streamed.String(), "我先查看工作区") {
		t.Fatalf("expected same-turn plan text to stream after tool call appeared, got %q", streamed.String())
	}
	if !strings.Contains(reply.Content, "下面是整理后的结论") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
}

func TestGenerateReplyCurrentFileContextDoesNotForceToolLoop(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}

		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid request payload: %v", err)
		}

		count := requestCount.Add(1)
		if count > 2 {
			t.Fatalf("unexpected request count: %d", count)
		}
		assertOpenAIToolChoiceAuto(t, payload)
		switch count {
		case 1:
			writeOpenAIStreamText(t, w, "收到，你已经登录成功。下一步执行 pwd 确认当前目录。")
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "我已经成功登陆上了")
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, []AttachmentPayload{
		{Kind: "current_file", Path: "v14.7.md"},
		{Kind: "editor_panels_context", EditorPanels: []EditorPanelContext{{PositionLabel: "current", ActiveFilePath: "v14.7.md"}}},
	}, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if requestCount.Load() != 1 {
		t.Fatalf("passive current-file metadata must not force a corrective tool loop, got %d requests", requestCount.Load())
	}
	if !strings.Contains(reply.Content, "下一步") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
}

func TestGenerateReplyOpenAIRequiredToolUsesConcisePreambleBeforeTool(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	longPreamble := "我先读取完整文件，看清当前第103镜前后的全貌。\n\n好，我已经完整看清了。现在的问题是这段反应还是太标准了，我会先写很多分析，然后再继续解释为什么要这么改。"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		writeOpenAIStreamTextAndToolCall(t, w, longPreamble, "call_inspect_1", "inspect_workspace", `{"path":"","max_depth":1}`)
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "看看当前文件")
	runtime := &agentRuntime{
		service:          env.service,
		protocol:         agentProtocolOpenAI,
		conversationID:   conversation.ID,
		messageID:        userMessage.ID,
		provider:         provider,
		messages:         messages,
		opts:             RunOptions{UserID: testUserID},
		profile:          runtimeProfileForProvider(provider),
		allowTask:        true,
		loadedToolsets:   allAgentToolsets(true),
		appliedMutations: map[string]ToolEvent{},
		transcript:       newAgentTranscript(),
		contextManager:   newAgentContextManager(),
	}
	turn, err := runtime.callModel(ctx, 0)
	if err != nil {
		t.Fatalf("callModel returned error: %v", err)
	}
	turn.ToolCalls = normalizeAgentToolCallIDs(turn.ToolCalls)
	turn, skipped := prepareToolTurnForExecution(turn)
	if skipped != 0 {
		t.Fatalf("expected no skipped tools, got %d", skipped)
	}
	if turn.Content != "我先读取完整文件，看清当前第103镜前后的全貌。" {
		t.Fatalf("expected concise first preamble, got %q", turn.Content)
	}
}

func TestGenerateReplyOpenAIStartBackgroundTaskReturnsWithoutSecondProviderTurn(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid request payload: %v", err)
		}
		switch count := requestCount.Add(1); count {
		case 1:
			assertOnlyCoreToolsLoaded(t, payload)
			writeOpenAIStreamTextAndToolCall(t, w,
				"我会先加载后台任务工具。",
				"load_task_1",
				"load_perseus_toolset",
				`{"toolset":"task"}`)
		case 2:
			assertOpenAIToolChoiceAuto(t, payload)
			names := openAIToolNamesFromPayload(t, payload)
			if !names["start_background_task"] {
				t.Fatalf("expected task tool after loading task toolset, got %#v", names)
			}
			writeOpenAIStreamTextAndToolCall(t, w,
				"我会创建后台任务来继续写十集。",
				"call_task_1",
				"start_background_task",
				`{"title":"续写向北方10集","goal":"续写《向北方》10集","expected_outputs":[{"type":"file","path":"向北方/向北方 第3-12集.txt","description":"续写10集剧本"}],"suggested_steps":["读取当前第2集","延续人物和悬疑线","分集写作"]}`)
		default:
			t.Fatalf("unexpected request count: %d with payload %s", count, string(raw))
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "那这个向北方，你帮我写续集，一共写10集，用后台任务的方式进行")
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, []AttachmentPayload{{
		Kind:     "current_file",
		Name:     "向北方 第2集.txt",
		Path:     "向北方/向北方 第2集.txt",
		FileType: "text",
	}}, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if requestCount.Load() != 2 {
		t.Fatalf("expected toolset load plus background task request, got %d", requestCount.Load())
	}
	if reply.BackgroundTask == nil {
		t.Fatalf("expected background task request, got %#v", reply)
	}
	if reply.BackgroundTask.Title != "续写向北方10集" || !strings.Contains(reply.BackgroundTask.Goal, "向北方") {
		t.Fatalf("unexpected background task request: %#v", reply.BackgroundTask)
	}
}

func TestAgentRuntimeExecutesOnlyOneToolPerModelTurn(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	if _, err := env.files.CreateFile(ctx, "a.txt", "before"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		count := requestCount.Add(1)
		switch count {
		case 1:
			writeOpenAIStreamTextAndToolCalls(t, w, "我先逐步处理文件。", []streamToolCall{
				{id: "call_read_1", name: "read_workspace_text", args: `{"path":"a.txt"}`},
				{id: "call_save_1", name: "save_workspace_file", args: `{"path":"a.txt","content":"after","mode":"overwrite"}`},
			})
		case 2:
			writeOpenAIToolCall(t, w, "call_done_1", "attempt_completion", `{"result":"已读取文件。"} `)
		default:
			t.Fatalf("unexpected request count: %d", count)
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "处理文件")
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, []AttachmentPayload{{Kind: "workspace_file", Path: "a.txt"}}, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if !strings.Contains(reply.Content, "已读取文件") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	file, err := env.files.ReadFile("a.txt")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if file.Content != "before" {
		t.Fatalf("second tool call should not have executed in same turn, got file content %q", file.Content)
	}
}

func TestAgentRuntimeSkipsStaleToolTurnWhenSteerArrivesBeforeToolExecution(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	if _, err := env.files.CreateFile(ctx, "a.txt", "before"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}

	firstRequestSeen := make(chan struct{})
	allowFirstResponse := make(chan struct{})
	done := make(chan error, 1)
	var requestCount atomic.Int32
	var capturedSecondRequest []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid request payload: %v", err)
		}
		count := requestCount.Add(1)
		switch count {
		case 1:
			close(firstRequestSeen)
			select {
			case <-allowFirstResponse:
			case <-time.After(5 * time.Second):
				t.Fatalf("timed out waiting to release first response")
			}
			writeOpenAIStreamTextAndToolCall(t, w, "我先直接保存。", "call_save_1", "save_workspace_file", `{"path":"a.txt","content":"stale","mode":"overwrite"}`)
		case 2:
			if items, ok := payload["messages"].([]any); ok {
				for _, item := range items {
					if message, ok := item.(map[string]any); ok {
						capturedSecondRequest = append(capturedSecondRequest, message)
					}
				}
			}
			writeOpenAIToolCall(t, w, "call_done_1", "attempt_completion", `{"result":"已按新引导停止旧保存。"} `)
		default:
			t.Fatalf("unexpected request count: %d", count)
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "处理文件")
	run, err := env.runs.CreateRun(ctx, store.CreateAgentRunInput{
		ID:              "run_stale_steer_test",
		UserID:          testUserID,
		ConversationID:  conversation.ID,
		UserMessageID:   userMessage.ID,
		ModelProviderID: provider.ID,
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	if _, err := env.runs.UpdateRun(ctx, testUserID, run.ID, store.UpdateAgentRunInput{Status: store.AgentRunRunning, Stage: "calling_model"}); err != nil {
		t.Fatalf("UpdateRun returned error: %v", err)
	}
	go func() {
		_, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, []AttachmentPayload{{Kind: "workspace_file", Path: "a.txt"}}, RunOptions{UserID: testUserID, RunID: run.ID})
		done <- err
	}()

	select {
	case <-firstRequestSeen:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for first provider request")
	}
	if _, err := env.runs.AppendEvent(ctx, store.AgentEventInput{
		RunID:          run.ID,
		UserID:         testUserID,
		ConversationID: conversation.ID,
		Type:           "agent_steer_received",
		Level:          "info",
		Payload: map[string]any{
			"steer_id":        "steer-late",
			"client_steer_id": "client-late",
			"content":         "不要保存旧方案，先停下来。",
			"content_preview": "不要保存旧方案，先停下来。",
		},
	}); err != nil {
		t.Fatalf("AppendEvent returned error: %v", err)
	}
	close(allowFirstResponse)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("GenerateReply returned error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for GenerateReply")
	}
	file, err := env.files.ReadFile("a.txt")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if file.Content != "before" {
		t.Fatalf("stale tool call should not have executed, got file content %q", file.Content)
	}
	if !messagesContainContent(capturedSecondRequest, "用户在任务运行中补充了最新引导") ||
		!messagesContainContent(capturedSecondRequest, "不要保存旧方案") {
		t.Fatalf("steer guidance missing from second provider request: %#v", capturedSecondRequest)
	}
}

func TestGenerateReplyOpenAIWriteExistingFile(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	if _, err := env.files.CreateFile(ctx, "ai-test.txt", "before"); err != nil {
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
			t.Fatalf("invalid request payload: %v", err)
		}

		count := requestCount.Add(1)
		switch count {
		case 1:
			handleOpenAIWorkspaceToolsetLoad(t, w, payload, "load_workspace_1")
		case 2:
			assertOpenAIWorkspaceToolsLoaded(t, payload)
			writeOpenAIToolCall(t, w, "structured_patch", "apply_workspace_patch", mustJSON(map[string]any{"patch": "*** Begin Patch\n*** Update File: ai-test.txt\n@@\n-before\n+after\n*** End Patch"}))
		case 3:
			writeOpenAIToolCall(t, w, "call_done_1", "attempt_completion", `{"result":"已修改 ai-test.txt。","changed_paths":["ai-test.txt"]}`)
		default:
			t.Fatalf("unexpected request count: %d", count)
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "请把 ai-test.txt 改成 after")
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}

	result, err := env.files.ReadFile("ai-test.txt")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if result.Content != "after\n" {
		t.Fatalf("unexpected file content: %q", result.Content)
	}
	if !strings.Contains(reply.Content, "ai-test.txt") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	assertToolCallRecord(t, env, "apply_workspace_patch", "success")
}

func TestGenerateReplyOpenAIEditWorkspaceTextEmitsWorkspaceChanged(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	if _, err := env.files.CreateFile(ctx, "ai-test.txt", "before"); err != nil {
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
			t.Fatalf("invalid request payload: %v", err)
		}
		switch requestCount.Add(1) {
		case 1:
			handleOpenAIWorkspaceToolsetLoad(t, w, payload, "load_workspace_1")
		case 2:
			writeOpenAIToolCall(t, w, "call_edit_1", "edit_workspace_text", `{"path":"ai-test.txt","old_text":"before","new_text":"after"}`)
		case 3:
			writeOpenAIToolCall(t, w, "call_done_1", "attempt_completion", `{"result":"已编辑 ai-test.txt。","changed_paths":["ai-test.txt"]}`)
		default:
			t.Fatalf("unexpected request count")
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "请把 ai-test.txt 里的 before 改成 after")
	var changedPayload map[string]any
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{
		UserID: testUserID,
		Event: func(eventType, level string, payload map[string]any) {
			if eventType == "workspace_file_changed" && payload["tool_name"] == "edit_workspace_text" {
				changedPayload = payload
			}
		},
	})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if !strings.Contains(reply.Content, "ai-test.txt") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	result, err := env.files.ReadFile("ai-test.txt")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if result.Content != "after" {
		t.Fatalf("unexpected file content: %q", result.Content)
	}
	if changedPayload == nil || changedPayload["path"] != "ai-test.txt" || changedPayload["hash"] == "" || changedPayload["version_id"] == "" {
		t.Fatalf("expected edit workspace change payload with path/hash/version, got %#v", changedPayload)
	}
	assertToolCallRecord(t, env, "edit_workspace_text", "success")
}

func TestGenerateReplyOpenAIWritePersistsCheckpointWhenFinalReplyTimesOut(t *testing.T) {
	env := newTestAIEnv(t)
	env.service.client.Timeout = 20 * time.Millisecond
	ctx := context.Background()
	if _, err := env.files.CreateFile(ctx, "notes/test.md", "version 1"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		count := requestCount.Add(1)
		switch count {
		case 1:
			writeJSON(t, w, map[string]any{
				"choices": []map[string]any{
					{
						"finish_reason": "tool_calls",
						"message": map[string]any{
							"role": "assistant",
							"tool_calls": []map[string]any{
								{
									"id":   "load_workspace_1",
									"type": "function",
									"function": map[string]any{
										"name":      "load_perseus_toolset",
										"arguments": `{"toolset":"workspace"}`,
									},
								},
							},
						},
					},
				},
			})
		case 2:
			writeJSON(t, w, map[string]any{
				"choices": []map[string]any{
					{
						"finish_reason": "tool_calls",
						"message": map[string]any{
							"role": "assistant",
							"tool_calls": []map[string]any{
								{
									"id":   "call_write_md",
									"type": "function",
									"function": map[string]any{
										"name":      "save_workspace_file",
										"arguments": `{"path":"notes/test.md","content":"version 2","mode":"overwrite"}`,
									},
								},
							},
						},
					},
				},
			})
		default:
			time.Sleep(200 * time.Millisecond)
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "请把 notes/test.md 改成 version 2")
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err == nil {
		t.Fatalf("expected GenerateReply error after final timeout, got reply %q", reply.Content)
	}
	if !isCheckpointRecoverableProviderError(err) {
		t.Fatalf("expected recoverable provider error, got %v", err)
	}
	readResult, err := env.files.ReadFile("notes/test.md")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if readResult.Content != "version 2" {
		t.Fatalf("expected file to be written despite final timeout, got %q", readResult.Content)
	}
	assertToolCallRecord(t, env, "save_workspace_file", "success")
}

func TestGenerateReplyOpenAIReadThenTimeoutDoesNotApplyHeuristicReplacement(t *testing.T) {
	env := newTestAIEnv(t)
	env.service.client.Timeout = 20 * time.Millisecond
	t.Setenv("PERSEUS_AGENT_MAX_CHECKPOINT_RESUMES", "0")
	ctx := context.Background()
	if _, err := env.files.CreateFile(ctx, "docs/book.md", "白皮书版本：14.2\n"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		count := requestCount.Add(1)
		switch count {
		case 1:
			writeJSON(t, w, map[string]any{
				"choices": []map[string]any{
					{
						"finish_reason": "tool_calls",
						"message": map[string]any{
							"role": "assistant",
							"tool_calls": []map[string]any{
								{
									"id":   "load_workspace_1",
									"type": "function",
									"function": map[string]any{
										"name":      "load_perseus_toolset",
										"arguments": `{"toolset":"workspace"}`,
									},
								},
							},
						},
					},
				},
			})
		case 2:
			writeJSON(t, w, map[string]any{
				"choices": []map[string]any{
					{
						"finish_reason": "tool_calls",
						"message": map[string]any{
							"role": "assistant",
							"tool_calls": []map[string]any{
								{
									"id":   "call_read_md",
									"type": "function",
									"function": map[string]any{
										"name":      "read_workspace_text",
										"arguments": `{"path":"docs/book.md"}`,
									},
								},
							},
						},
					},
				},
			})
		default:
			time.Sleep(200 * time.Millisecond)
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "你尝试修改一下工作给里面白皮书的内容，白皮书的版本本来是14.2，内部写错了，写成14.0.7，你帮我修改下。")
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err == nil {
		t.Fatalf("expected provider timeout instead of heuristic mutation fallback, got reply %q", reply.Content)
	}
	readResult, err := env.files.ReadFile("docs/book.md")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if readResult.Content != "白皮书版本：14.2\n" {
		t.Fatalf("unexpected file content: %q", readResult.Content)
	}
	assertToolCallRecord(t, env, "read_workspace_text", "success")
	var savedCount int
	if err := env.db.QueryRow(`SELECT COUNT(*) FROM tool_calls WHERE tool_name = 'save_workspace_file'`).Scan(&savedCount); err != nil {
		t.Fatalf("QueryRow returned error: %v", err)
	}
	if savedCount != 0 {
		t.Fatalf("expected no heuristic save_workspace_file call, got %d", savedCount)
	}
}

func TestGenerateReplyOpenAIMkdirWorkspace(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}

		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid request payload: %v", err)
		}

		count := requestCount.Add(1)
		switch count {
		case 1:
			handleOpenAIWorkspaceToolsetLoad(t, w, payload, "load_workspace_1")
		case 2:
			assertOpenAIWorkspaceToolsLoaded(t, payload)
			writeOpenAIToolCall(t, w, "call_mkdir_1", "manage_workspace_path", `{"action":"mkdir","path":"ai-folder"}`)
		case 3:
			writeOpenAIToolCall(t, w, "call_done_1", "attempt_completion", `{"result":"已创建 ai-folder。","changed_paths":["ai-folder"]}`)
		default:
			t.Fatalf("unexpected request count: %d", count)
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "请创建一个 ai-folder 文件夹")
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}

	nodes, _, err := env.files.List("", 3, 50)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if !hasFolder(nodes, "ai-folder") {
		t.Fatalf("expected ai-folder to exist, got %#v", nodes)
	}
	if !strings.Contains(reply.Content, "ai-folder") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	assertToolCallRecord(t, env, "manage_workspace_path", "success")
}

func TestGenerateReplyOpenAIDeleteAfterReadRequiresMutationTool(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	if _, err := env.files.CreateFile(ctx, "delete-me.txt", "hello"); err != nil {
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
			t.Fatalf("invalid request payload: %v", err)
		}

		count := requestCount.Add(1)
		switch count {
		case 1:
			handleOpenAIWorkspaceToolsetLoad(t, w, payload, "load_workspace_1")
		case 2:
			assertOpenAIWorkspaceToolsLoaded(t, payload)
			writeOpenAIToolCall(t, w, "call_read_1", "read_workspace_text", `{"path":"delete-me.txt"}`)
		case 3:
			writeOpenAIToolCall(t, w, "call_delete_1", "manage_workspace_path", `{"action":"trash","path":"delete-me.txt"}`)
		case 4:
			writeOpenAIToolCall(t, w, "call_done_1", "attempt_completion", `{"result":"已将 delete-me.txt 移入回收站。","changed_paths":["回收站/delete-me.txt"]}`)
		default:
			t.Fatalf("unexpected request count: %d", count)
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "请把 delete-me.txt 删除到回收站")
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}

	if _, err := env.files.ReadFile("delete-me.txt"); err == nil {
		t.Fatalf("expected source file to be moved away")
	}
	trashItems, err := env.files.TrashItems()
	if err != nil {
		t.Fatalf("TrashItems returned error: %v", err)
	}
	if !hasTrashItem(trashItems, workspace.TrashDirName+"/delete-me.txt") {
		t.Fatalf("expected file in trash, got %#v", trashItems)
	}
	if !strings.Contains(reply.Content, "回收站") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	assertToolCallRecord(t, env, "read_workspace_text", "success")
	assertToolCallRecord(t, env, "manage_workspace_path", "success")
	if requestCount.Load() != 4 {
		t.Fatalf("expected toolset load, read, delete, and completion requests, got %d", requestCount.Load())
	}
}

func TestGenerateReplyOpenAIEmptyNoToolRetriesWithoutKeywordGate(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		writeJSON(t, w, map[string]any{
			"choices": []map[string]any{
				{
					"finish_reason": "stop",
					"message": map[string]any{
						"role":    "assistant",
						"content": "",
					},
				},
			},
		})
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "请创建 empty-fail.txt，内容写 hello")
	_, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err == nil {
		t.Fatal("expected empty response error")
	}
	if !strings.Contains(err.Error(), "empty model response") {
		t.Fatalf("unexpected error: %v", err)
	}
	if requestCount.Load() != 3 {
		t.Fatalf("expected corrective loop to try 3 model turns, got %d", requestCount.Load())
	}
}

func TestGenerateReplyOpenAIWorkspaceReferencePlainTextDoesNotFinishEarly(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	if _, err := env.files.CreateFile(ctx, "draft.txt", "hello"); err != nil {
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
			t.Fatalf("invalid request payload: %v", err)
		}

		count := requestCount.Add(1)
		switch count {
		case 1:
			assertOpenAIWorkspaceToolsLoaded(t, payload)
			writeJSON(t, w, map[string]any{
				"choices": []map[string]any{
					{
						"finish_reason": "stop",
						"message": map[string]any{
							"role":    "assistant",
							"content": "我先建文件夹，再逐个写入。",
						},
					},
				},
			})
		case 2:
			assertLastUserActionRejected(t, payload, "action_rejected")
			writeOpenAIToolCall(t, w, "call_read_1", "read_workspace_text", `{"path":"draft.txt"}`)
		case 3:
			writeOpenAIToolCall(t, w, "call_done_1", "attempt_completion", `{"result":"已读取 draft.txt 并确认后续步骤。"} `)
		default:
			t.Fatalf("unexpected request count: %d", count)
		}
	}))
	defer server.Close()

	provider, err := env.configs.CreateModelProvider(ctx, testUserID, store.ModelProviderInput{
		Name:              "Test OpenAI",
		Visibility:        store.ResourceVisibilityPublic,
		Protocol:          "openai",
		SystemMessageMode: "multiple",
		BaseURL:           server.URL,
		APIKey:            "test-key",
		ModelID:           "gpt-test",
		Enabled:           true,
	})
	if err != nil {
		t.Fatalf("CreateModelProvider returned error: %v", err)
	}
	conversation, err := env.conversations.CreateConversation(ctx, store.CreateConversationInput{
		UserID:                  testUserID,
		Title:                   "引用文件",
		SelectedModelProviderID: provider.ID,
	})
	if err != nil {
		t.Fatalf("CreateConversation returned error: %v", err)
	}
	userMessage, err := env.conversations.AppendMessage(ctx, testUserID, conversation.ID, "user", "看看这个文件并继续处理", `[{"id":"ref1","name":"draft.txt","kind":"workspace_file","type":"workspace_file","path":"draft.txt","file_type":"text"}]`)
	if err != nil {
		t.Fatalf("AppendMessage returned error: %v", err)
	}
	messages, err := env.conversations.ListMessages(ctx, testUserID, conversation.ID)
	if err != nil {
		t.Fatalf("ListMessages returned error: %v", err)
	}

	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, []AttachmentPayload{{
		ID:       "ref1",
		Name:     "draft.txt",
		Kind:     "workspace_file",
		Path:     "draft.txt",
		FileType: "text",
	}}, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if !strings.Contains(reply.Content, "draft.txt") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	if requestCount.Load() != 3 {
		t.Fatalf("expected corrective loop to continue with tools, got %d requests", requestCount.Load())
	}
}

func TestGenerateReplyOpenAIReadOnlyCanReportPartial(t *testing.T) {
	runAuditScenario(t, "read_partial")
}

func TestGenerateReplyOpenAIAllowsProtocolExplanationInCompletion(t *testing.T) {
	runAuditScenario(t, "read_partial")
}

func TestGenerateReplySuppressesActionPreamblesAndEmitsFinalSegment(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		count := requestCount.Add(1)
		switch count {
		case 1:
			writeJSON(t, w, map[string]any{
				"choices": []map[string]any{
					{
						"message": map[string]any{
							"role":    "assistant",
							"content": "我先创建文件夹，然后开始写第一章。",
							"tool_calls": []map[string]any{
								{
									"id":   "call_mkdir_1",
									"type": "function",
									"function": map[string]any{
										"name":      "manage_workspace_path",
										"arguments": `{"action":"mkdir","path":"novel-project"}`,
									},
								},
							},
						},
					},
				},
			})
		case 2:
			writeJSON(t, w, map[string]any{
				"choices": []map[string]any{
					{
						"message": map[string]any{
							"role":    "assistant",
							"content": "文件夹已经准备好，我现在写第一章内容。",
							"tool_calls": []map[string]any{
								{
									"id":   "call_write_1",
									"type": "function",
									"function": map[string]any{
										"name":      "save_workspace_file",
										"arguments": `{"path":"novel-project/chapter-1.txt","content":"hello","mode":"create"}`,
									},
								},
							},
						},
					},
				},
			})
		case 3:
			writeOpenAIToolCall(t, w, "done", "attempt_completion", `{"result":"已创建 novel-project 并写入 chapter-1.txt。","changed_paths":["novel-project/chapter-1.txt"]}`)
		default:
			t.Fatalf("unexpected request count: %d", count)
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "创建一个文件夹并写入第一章")
	var currentSegment strings.Builder
	var assistantBodies []string
	var heldPreambles int
	flushSegment := func(finalContent string) {
		content := finalContent
		if strings.TrimSpace(content) == "" {
			content = currentSegment.String()
		}
		content = strings.TrimSpace(content)
		if content != "" {
			assistantBodies = append(assistantBodies, content)
		}
		currentSegment.Reset()
	}
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{
		UserID: testUserID,
		Event: func(eventType, level string, payload map[string]any) {
			switch eventType {
			case "model_delta":
				if text, _ := payload["text"].(string); text != "" {
					currentSegment.WriteString(text)
				}
			case "model_message_completed":
				content, _ := payload["content"].(string)
				flushSegment(content)
			case "assistant_action_preamble_held":
				heldPreambles++
			}
		},
		OnAssistantToolBoundary: func() {
			flushSegment("")
		},
	})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if !strings.Contains(reply.Content, "chapter-1.txt") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	flushSegment("")
	if heldPreambles != 2 {
		t.Fatalf("expected two held action preambles, got %d segments=%#v", heldPreambles, assistantBodies)
	}
	if len(assistantBodies) != 1 {
		t.Fatalf("expected only final assistant segment, got %#v", assistantBodies)
	}
	if strings.Contains(strings.Join(assistantBodies, "\n"), "我先创建文件夹") || strings.Contains(strings.Join(assistantBodies, "\n"), "我现在写第一章内容") {
		t.Fatalf("action preambles should not be committed as visible assistant content, got %#v", assistantBodies)
	}
	if !strings.Contains(assistantBodies[0], "chapter-1.txt") {
		t.Fatalf("expected final segment to include completion summary, got %#v", assistantBodies)
	}
}

func TestGenerateReplyOpenAIPlainEmptyResponseRecordsDiagnostic(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		writeJSON(t, w, map[string]any{
			"choices": []map[string]any{
				{
					"finish_reason": "stop",
					"message": map[string]any{
						"role":    "assistant",
						"content": "",
					},
				},
			},
		})
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "你好")
	_, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err == nil {
		t.Fatal("expected empty response error")
	}
	if DiagnosticIDFromError(err) == "" {
		t.Fatalf("expected diagnostic id on error, got %v", err)
	}
	if requestCount.Load() != 3 {
		t.Fatalf("expected empty response to be retried through corrective loop, got %d requests", requestCount.Load())
	}
	logs, listErr := env.diagnostics.List(ctx, store.DiagnosticLogQuery{UserID: testUserID, Limit: 10})
	if listErr != nil {
		t.Fatalf("diagnostics.List returned error: %v", listErr)
	}
	if len(logs) == 0 || logs[0].Stage != "provider_empty_response" {
		t.Fatalf("expected provider_empty_response diagnostic, got %#v", logs)
	}
}

func TestGenerateReplyOpenAIToolArgumentErrorFeedsBackAndRecovers(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid request payload: %v", err)
		}
		count := requestCount.Add(1)
		switch count {
		case 1:
			handleOpenAIWorkspaceToolsetLoad(t, w, payload, "load_workspace_1")
		case 2:
			writeOpenAIToolCall(t, w, "bad_args", "save_workspace_file", `{"path":"bad.txt","content":"hello","mode":`)
		case 3:
			lastToolContent := lastOpenAIToolContent(t, payload)
			if !strings.Contains(lastToolContent, "tool arguments must be valid JSON") {
				t.Fatalf("expected invalid JSON tool_result feedback, got %s", lastToolContent)
			}
			writeOpenAIToolCall(t, w, "good_args", "save_workspace_file", `{"path":"bad.txt","content":"hello","mode":"create"}`)
		case 4:
			writeOpenAIToolCall(t, w, "done", "attempt_completion", `{"result":"已创建 bad.txt。","changed_paths":["bad.txt"]}`)
		default:
			t.Fatalf("unexpected request count: %d", count)
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "请创建 bad.txt，内容写 hello")
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if !strings.Contains(reply.Content, "bad.txt") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	result, err := env.files.ReadFile("bad.txt")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if result.Content != "hello" {
		t.Fatalf("unexpected file content: %q", result.Content)
	}
}

func TestGenerateReplyOpenAIRepeatedEmptySaveArgumentsStopsLoop(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_, _ = io.ReadAll(r.Body)
		count := requestCount.Add(1)
		writeOpenAIToolCall(t, w, fmt.Sprintf("bad_empty_%d", count), "save_workspace_file", `{}`)
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "请创建 loop.txt，内容写 hello")
	_, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err == nil {
		t.Fatal("expected repeated invalid tool call loop to stop")
	}
	if requestCount.Load() != agentMaxConsecutiveToolErrs {
		t.Fatalf("expected stop after %d invalid tool turns, got %d", agentMaxConsecutiveToolErrs, requestCount.Load())
	}
	if _, readErr := env.files.ReadFile("loop.txt"); readErr == nil {
		t.Fatal("invalid save tool arguments should not create a file")
	}
	logs, listErr := env.diagnostics.List(ctx, store.DiagnosticLogQuery{UserID: testUserID, Limit: 10})
	if listErr != nil {
		t.Fatalf("diagnostics.List returned error: %v", listErr)
	}
	if len(logs) == 0 {
		t.Fatal("expected diagnostic logs for invalid tool arguments")
	}
	foundInvalidToolArgs := false
	for _, item := range logs {
		if item.Stage == "tool_call:save_workspace_file" && strings.Contains(item.RequestSummary, `"failure_class":"tool_invalid_arguments"`) {
			foundInvalidToolArgs = true
			if item.Level != "warn" {
				t.Fatalf("invalid tool arguments should be warn, got %s", item.Level)
			}
		}
	}
	if !foundInvalidToolArgs {
		t.Fatalf("expected tool_invalid_arguments diagnostic, got %#v", logs)
	}
}

func TestGenerateReplyOpenAIDeduplicatesRepeatedMutationToolCall(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid request payload: %v", err)
		}
		count := requestCount.Add(1)
		switch count {
		case 1:
			handleOpenAIWorkspaceToolsetLoad(t, w, payload, "load_workspace_1")
		case 2:
			writeOpenAIToolCall(t, w, "dup_1", "save_workspace_file", `{"path":"dedupe.txt","content":"hello","mode":"create"}`)
		case 3:
			writeOpenAIToolCall(t, w, "dup_2", "save_workspace_file", `{"path":"dedupe.txt","content":"hello","mode":"create"}`)
		case 4:
			lastToolContent := lastOpenAIToolContent(t, payload)
			if !strings.Contains(lastToolContent, "already_applied") {
				t.Fatalf("expected duplicate mutation to return already_applied, got %s", lastToolContent)
			}
			writeOpenAIToolCall(t, w, "done", "attempt_completion", `{"result":"已创建 dedupe.txt。","changed_paths":["dedupe.txt"]}`)
		default:
			t.Fatalf("unexpected request count: %d", count)
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "请创建 dedupe.txt，内容写 hello")
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if !strings.Contains(reply.Content, "dedupe.txt") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	result, err := env.files.ReadFile("dedupe.txt")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if result.Content != "hello" {
		t.Fatalf("unexpected file content: %q", result.Content)
	}
	var recordedWrites int
	if err := env.db.QueryRow(`SELECT COUNT(*) FROM tool_calls WHERE tool_name = 'save_workspace_file' AND status = 'success'`).Scan(&recordedWrites); err != nil {
		t.Fatalf("QueryRow returned error: %v", err)
	}
	if recordedWrites != 1 {
		t.Fatalf("expected only one recorded write, got %d", recordedWrites)
	}
}

func TestGenerateReplyOpenAIFailsWhenModelNeverCompletesAfterMutation(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid request payload: %v", err)
		}
		switch count := requestCount.Add(1); count {
		case 1:
			handleOpenAIWorkspaceToolsetLoad(t, w, payload, "load_workspace_1")
		case 2:
			writeOpenAIToolCall(t, w, "write_1", "save_workspace_file", `{"path":"verified-fallback.txt","content":"done","mode":"create"}`)
			return
		default:
			writeJSON(t, w, map[string]any{
				"choices": []map[string]any{
					{
						"finish_reason": "stop",
						"message": map[string]any{
							"role":    "assistant",
							"content": "",
						},
					},
				},
			})
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "请创建 verified-fallback.txt，内容写 done")
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err == nil {
		t.Fatalf("expected GenerateReply error when model never completes, got reply %q", reply.Content)
	}
	if !strings.Contains(err.Error(), "workspace 任务未执行完成") {
		t.Fatalf("unexpected error: %v", err)
	}
	result, err := env.files.ReadFile("verified-fallback.txt")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if result.Content != "done" {
		t.Fatalf("unexpected file content: %q", result.Content)
	}
	if requestCount.Load() != 5 {
		t.Fatalf("expected toolset load, write round, and three no-tool rounds, got %d", requestCount.Load())
	}
}

func TestGenerateReplyOpenAIFallsBackToNonStreamWhenFinalStreamBreaks(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	t.Setenv("PERSEUS_PROVIDER_RETRY_BASE_DELAY_MS", "0")

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid request payload: %v", err)
		}
		switch requestCount.Add(1) {
		case 1:
			if payload["stream"] != true {
				t.Fatalf("expected first request to stream, got %#v", payload["stream"])
			}
			writeOpenAIToolCall(t, w, "write_1", "save_workspace_file", `{"path":"verified-stream-fallback.txt","content":"done","mode":"create"}`)
		case 2:
			if payload["stream"] != true {
				t.Fatalf("expected final request to stream before fallback, got %#v", payload["stream"])
			}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial final\"}}]}\n\n")
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				panic(http.ErrAbortHandler)
			}
			conn, _, hijackErr := hijacker.Hijack()
			if hijackErr != nil {
				panic(http.ErrAbortHandler)
			}
			_ = conn.Close()
		case 3:
			if payload["stream"] != false {
				t.Fatalf("expected same-turn fallback to disable stream, got %#v", payload["stream"])
			}
			writeOpenAIToolCall(t, w, "done_after_fallback", "attempt_completion", `{"result":"已通过非流式降级完成。","changed_paths":["verified-stream-fallback.txt"]}`)
		default:
			t.Fatalf("unexpected request count: %d", requestCount.Load())
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "请创建 verified-stream-fallback.txt，内容写 done")
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if !strings.Contains(reply.Content, "已通过非流式降级完成") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	if requestCount.Load() != 3 {
		t.Fatalf("expected write, broken final stream, and non-stream fallback, got %d", requestCount.Load())
	}
}

func TestGenerateReplyRecordsDiagnosticForHTTP502(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	t.Setenv("PERSEUS_PROVIDER_RETRY_BASE_DELAY_MS", "0")

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("errorcode:502"))
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "这是不应该进入日志的完整 prompt")
	_, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err == nil {
		t.Fatal("expected GenerateReply error")
	}
	if requestCount.Load() != 3 {
		t.Fatalf("expected provider policy to make 3 attempts, got %d requests", requestCount.Load())
	}

	logs, err := env.diagnostics.List(ctx, store.DiagnosticLogQuery{UserID: testUserID, Limit: 10})
	if err != nil {
		t.Fatalf("diagnostics.List returned error: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 diagnostic log, got %d", len(logs))
	}
	item := logs[0]
	if item.HTTPStatus != http.StatusBadGateway {
		t.Fatalf("expected HTTP 502, got %d", item.HTTPStatus)
	}
	if item.BaseURLHost == "" || item.FinalURLPath != "/chat/completions" {
		t.Fatalf("unexpected url fields: host=%q path=%q", item.BaseURLHost, item.FinalURLPath)
	}
	if strings.Contains(item.ErrorMessage, "完整 prompt") {
		t.Fatalf("diagnostic error leaked prompt: %q", item.ErrorMessage)
	}
	if strings.Contains(item.ResponseSnippet, "test-key") {
		t.Fatalf("diagnostic snippet leaked api key: %q", item.ResponseSnippet)
	}
}

func TestGenerateReplyRetriesTransientHTTP502WithProviderPolicy(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	t.Setenv("PERSEUS_PROVIDER_RETRY_BASE_DELAY_MS", "0")

	var requestCount atomic.Int32
	var bodies []string
	var retryEvents int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		bodies = append(bodies, string(raw))
		if requestCount.Add(1) < 3 {
			w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("errorcode:502"))
			return
		}
		writeOpenAIToolCall(t, w, "done_after_retry", "attempt_completion", `{"result":"已重试后完成。"}`)
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "hello")
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{
		UserID: testUserID,
		Event: func(eventType, level string, payload map[string]any) {
			if eventType == "model_call_retrying" {
				retryEvents++
				if payload["max_attempts"] != 3 {
					t.Fatalf("expected max attempts diagnostic to be 3, got %#v", payload["max_attempts"])
				}
			}
		},
	})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if !strings.Contains(reply.Content, "已重试后完成") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	if requestCount.Load() != 3 {
		t.Fatalf("expected 3 provider policy attempts, got %d requests", requestCount.Load())
	}
	if retryEvents != 2 {
		t.Fatalf("expected 2 retry events, got %d", retryEvents)
	}
	if len(bodies) != 3 {
		t.Fatalf("expected 3 captured request bodies, got %d", len(bodies))
	}
	for index, body := range bodies {
		if !strings.Contains(body, `"messages"`) || !strings.Contains(body, `"stream":true`) {
			t.Fatalf("attempt %d did not replay the provider payload: %s", index+1, body)
		}
	}
}

func TestGenerateReplyDoesNotRetryHTTP400(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	t.Setenv("PERSEUS_PROVIDER_RETRY_BASE_DELAY_MS", "0")

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad schema"}}`))
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "hello")
	_, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err == nil {
		t.Fatal("expected GenerateReply error")
	}
	if requestCount.Load() != 1 {
		t.Fatalf("expected non-retriable 400 to make 1 request, got %d", requestCount.Load())
	}
}

func TestProviderSDKNativeRetriesRemainDisabled(t *testing.T) {
	if providerSDKMaxRetries != 0 {
		t.Fatalf("provider SDK native retries should stay disabled, got %d", providerSDKMaxRetries)
	}
}

func TestGenerateReplyRetriesHTTP429RetryAfterWithProviderPolicy(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	t.Setenv("PERSEUS_PROVIDER_RETRY_BASE_DELAY_MS", "0")

	var requestCount atomic.Int32
	var retryAfterSeen bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestCount.Add(1) == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "0.001")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
			return
		}
		writeOpenAIToolCall(t, w, "done_after_429", "attempt_completion", `{"result":"已按 Retry-After 重试后完成。"}`)
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "hello")
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{
		UserID: testUserID,
		Event: func(eventType, level string, payload map[string]any) {
			if eventType == "model_call_retrying" && numberFromAnyForTest(payload["retry_after_ms"]) > 0 {
				retryAfterSeen = true
			}
		},
	})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if !strings.Contains(reply.Content, "Retry-After") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	if requestCount.Load() != 2 {
		t.Fatalf("expected 2 provider attempts, got %d", requestCount.Load())
	}
	if !retryAfterSeen {
		t.Fatal("expected retry_after_ms in retry event")
	}
}

func numberFromAnyForTest(value any) int64 {
	switch item := value.(type) {
	case int64:
		return item
	case int:
		return int64(item)
	case float64:
		return int64(item)
	default:
		return 0
	}
}

func TestGenerateReplyProviderRetryDiagnostics(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	t.Setenv("PERSEUS_PROVIDER_RETRY_BASE_DELAY_MS", "0")

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("errorcode:502"))
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "hello")
	_, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err == nil {
		t.Fatal("expected GenerateReply error")
	}
	if requestCount.Load() != 3 {
		t.Fatalf("expected request-level stream attempts only, got %d requests", requestCount.Load())
	}
	logs, err := env.diagnostics.List(ctx, store.DiagnosticLogQuery{UserID: testUserID, Limit: 5})
	if err != nil {
		t.Fatalf("diagnostics.List returned error: %v", err)
	}
	if len(logs) == 0 {
		t.Fatal("expected diagnostic log")
	}
	var summary ProviderRequestSummary
	if err := json.Unmarshal([]byte(logs[0].RequestSummary), &summary); err != nil {
		t.Fatalf("invalid request summary: %v\n%s", err, logs[0].RequestSummary)
	}
	if summary.SDKMaxAttempts != 3 || summary.RequestAttempts != 3 || !summary.RetryableError || summary.StreamRetryPhase != "stream" || summary.StreamFallbackUsed || summary.FallbackAttempts != 0 {
		t.Fatalf("unexpected retry diagnostics: %#v", summary)
	}
}

func TestGenerateReplyTransportErrorDoesNotUseStreamFallback(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	t.Setenv("PERSEUS_PROVIDER_RETRY_BASE_DELAY_MS", "0")

	var requestCount atomic.Int32
	var fallbackRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid request payload: %v", err)
		}
		if payload["stream"] == false {
			fallbackRequests.Add(1)
		}
		requestCount.Add(1)
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			panic(http.ErrAbortHandler)
		}
		conn, _, hijackErr := hijacker.Hijack()
		if hijackErr != nil {
			panic(http.ErrAbortHandler)
		}
		_ = conn.Close()
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "hello")
	_, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err == nil {
		t.Fatal("expected GenerateReply error")
	}
	if requestCount.Load() != 3 {
		t.Fatalf("expected request-level retry attempts only, got %d requests", requestCount.Load())
	}
	if fallbackRequests.Load() != 0 {
		t.Fatalf("transport failure before a stream should not use non-stream fallback, got %d fallback requests", fallbackRequests.Load())
	}
}

func TestGenerateReplyCheckpointResumeAfterTransientProviderError(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	t.Setenv("PERSEUS_AGENT_RECOVERY_BASE_DELAY_MS", "0")
	if _, err := env.files.CreateFile(ctx, "draft.txt", "draft body"); err != nil {
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
			t.Fatalf("invalid request payload: %v", err)
		}
		count := requestCount.Add(1)
		if openAIProviderMessageContentContainsAll(payload, `"status":"recovery_observation"`, `"reason_code":"provider_connection_interrupted"`) {
			writeJSON(t, w, map[string]any{
				"choices": []map[string]any{
					{
						"message": map[string]any{
							"role": "assistant",
							"tool_calls": []map[string]any{
								{
									"id":   "done_after_resume",
									"type": "function",
									"function": map[string]any{
										"name":      "attempt_completion",
										"arguments": `{"result":"已从断点继续并完成。","notes":"没有重复读取已成功的文件。"}`,
									},
								},
							},
						},
					},
				},
			})
			return
		}
		if count == 1 {
			writeJSON(t, w, map[string]any{
				"choices": []map[string]any{
					{
						"finish_reason": "tool_calls",
						"message": map[string]any{
							"role": "assistant",
							"tool_calls": []map[string]any{
								{
									"id":   "load_workspace_1",
									"type": "function",
									"function": map[string]any{
										"name":      "load_perseus_toolset",
										"arguments": `{"toolset":"workspace"}`,
									},
								},
							},
						},
					},
				},
			})
			return
		}
		if count == 2 {
			writeJSON(t, w, map[string]any{
				"choices": []map[string]any{
					{
						"message": map[string]any{
							"role": "assistant",
							"tool_calls": []map[string]any{
								{
									"id":   "read_before_failure",
									"type": "function",
									"function": map[string]any{
										"name":      "read_workspace_text",
										"arguments": `{"path":"draft.txt"}`,
									},
								},
							},
						},
					},
				},
			})
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("errorcode:502"))
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "读取 draft 后继续总结")
	run, err := env.runs.CreateRun(ctx, store.CreateAgentRunInput{
		UserID:          testUserID,
		ConversationID:  conversation.ID,
		UserMessageID:   userMessage.ID,
		ModelProviderID: provider.ID,
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	var eventTypes []string
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{
		UserID: testUserID,
		RunID:  run.ID,
		Event: func(eventType, level string, payload map[string]any) {
			eventTypes = append(eventTypes, eventType)
		},
	})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if !strings.Contains(reply.Content, "已从断点继续并完成") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	for _, expected := range []string{"agent_recovery_started", "agent_recovery_retrying", "agent_recovery_completed"} {
		if !containsString(eventTypes, expected) {
			t.Fatalf("expected %s event, got %#v", expected, eventTypes)
		}
	}
	var readCalls int
	if err := env.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tool_calls WHERE user_id = ? AND conversation_id = ? AND tool_name = ?`, testUserID, conversation.ID, "read_workspace_text").Scan(&readCalls); err != nil {
		t.Fatalf("count tool calls returned error: %v", err)
	}
	if readCalls != 1 {
		t.Fatalf("expected exactly one read tool call, got %d", readCalls)
	}
	logs, err := env.diagnostics.List(ctx, store.DiagnosticLogQuery{UserID: testUserID, Limit: 10})
	if err != nil {
		t.Fatalf("diagnostics.List returned error: %v", err)
	}
	if len(logs) == 0 {
		t.Fatal("expected transient provider warning diagnostic")
	}
	for _, item := range logs {
		if item.Level == "error" {
			t.Fatalf("recovered transient failure should not leave error diagnostic: %#v", item)
		}
	}
}

func TestGenerateReplyStreamErrorFallsBackToNonStreamBeforeCheckpointResume(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	t.Setenv("PERSEUS_AGENT_RECOVERY_BASE_DELAY_MS", "0")
	t.Setenv("PERSEUS_PROVIDER_RETRY_BASE_DELAY_MS", "0")
	if _, err := env.files.CreateFile(ctx, "draft.txt", "draft body"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}

	var requestCount atomic.Int32
	var fallbackRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid request payload: %v", err)
		}
		if payload["stream"] == false {
			fallbackRequests.Add(1)
		}
		switch requestCount.Add(1) {
		case 1:
			if payload["stream"] != true {
				t.Fatalf("expected first request to stream, got %#v", payload["stream"])
			}
			writeOpenAIToolCall(t, w, "load_workspace_1", "load_perseus_toolset", `{"toolset":"workspace"}`)
		case 2:
			if payload["stream"] != true {
				t.Fatalf("expected second request to stream, got %#v", payload["stream"])
			}
			writeOpenAIToolCall(t, w, "read_before_failure", "read_workspace_text", `{"path":"draft.txt"}`)
		case 3:
			if payload["stream"] != true {
				t.Fatalf("expected third request to stream before fallback, got %#v", payload["stream"])
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Connection", "close")
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			hj, ok := w.(http.Hijacker)
			if !ok {
				panic(http.ErrAbortHandler)
			}
			conn, _, hijackErr := hj.Hijack()
			if hijackErr != nil {
				panic(http.ErrAbortHandler)
			}
			_ = conn.Close()
		case 4, 5, 6:
			if payload["stream"] != false {
				t.Fatalf("expected non-stream fallback before checkpoint recovery, got %#v", payload["stream"])
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(529)
			_, _ = w.Write([]byte(`{"error":{"message":"fallback overloaded"}}`))
		case 7:
			if payload["stream"] != true {
				t.Fatalf("expected checkpoint resume to rebuild a streaming provider turn, got %#v", payload["stream"])
			}
			if !openAIProviderMessageContentContainsAll(payload, `"status":"recovery_observation"`, `"reason_code":"provider_connection_interrupted"`) {
				t.Fatalf("expected checkpoint recovery observation in request %d: %s", requestCount.Load(), string(raw))
			}
			writeOpenAIToolCall(t, w, "done_after_resume", "attempt_completion", `{"result":"已从断点继续并完成。"}`)
		default:
			t.Fatalf("unexpected request count: %d", requestCount.Load())
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "读取 draft 后继续总结")
	run, err := env.runs.CreateRun(ctx, store.CreateAgentRunInput{
		UserID:          testUserID,
		ConversationID:  conversation.ID,
		UserMessageID:   userMessage.ID,
		ModelProviderID: provider.ID,
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{
		UserID: testUserID,
		RunID:  run.ID,
	})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if !strings.Contains(reply.Content, "已从断点继续并完成") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	if fallbackRequests.Load() != 3 {
		t.Fatalf("expected fallback request-level retries before checkpoint resume, got %d fallback requests", fallbackRequests.Load())
	}
}

func TestReadWorkspaceTextDoesNotEmitWorkspaceChanged(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	if _, err := env.files.CreateFile(ctx, "draft.txt", "draft body"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		switch requestCount.Add(1) {
		case 1:
			writeOpenAIToolCall(t, w, "load_workspace", "load_perseus_toolset", `{"toolset":"workspace"}`)
		case 2:
			writeOpenAIToolCall(t, w, "call_read_1", "read_workspace_text", `{"path":"draft.txt"}`)
		case 3:
			writeOpenAIToolCall(t, w, "call_done_1", "attempt_completion", `{"result":"已读取 draft.txt。"}`)
		default:
			t.Fatalf("unexpected request count")
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "读取 draft.txt")
	var eventTypes []string
	_, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{
		UserID: testUserID,
		Event: func(eventType, level string, payload map[string]any) {
			eventTypes = append(eventTypes, eventType)
		},
	})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if containsString(eventTypes, "workspace_file_changed") {
		t.Fatalf("read-only tool should not emit workspace_file_changed: %#v", eventTypes)
	}
}

func TestGenerateReplyOpenAIStreamParseErrorFallsBackToNonStream(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid request payload: %v", err)
		}

		count := requestCount.Add(1)
		switch count {
		case 1:
			if payload["stream"] != true {
				t.Fatalf("expected first request to stream, got %#v", payload["stream"])
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {bad json}\n\n"))
		case 2:
			if payload["stream"] != false {
				t.Fatalf("expected fallback request to disable stream, got %#v", payload["stream"])
			}
			writeJSON(t, w, map[string]any{
				"choices": []map[string]any{
					{
						"finish_reason": "stop",
						"message": map[string]any{
							"role": "assistant",
							"tool_calls": []map[string]any{{
								"id":   "done_after_fallback",
								"type": "function",
								"function": map[string]any{
									"name":      "attempt_completion",
									"arguments": `{"result":"非流式恢复成功。"}`,
								},
							}},
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected request count: %d", count)
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "你好")
	var eventTypes []string
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{
		UserID: testUserID,
		Event: func(eventType, level string, payload map[string]any) {
			eventTypes = append(eventTypes, eventType)
		},
	})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if requestCount.Load() != 2 {
		t.Fatalf("expected stream request plus non-stream fallback completion, got %d", requestCount.Load())
	}
	if !strings.Contains(reply.Content, "非流式恢复成功") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	if !containsString(eventTypes, "model_stream_recovering") {
		t.Fatalf("expected model_stream_recovering event, got %#v", eventTypes)
	}
}

func TestGenerateReplyAnthropicEmptyStreamFallsBackToNonStream(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid request payload: %v", err)
		}

		count := requestCount.Add(1)
		switch count {
		case 1:
			if payload["stream"] != true {
				t.Fatalf("expected first request to stream, got %#v", payload["stream"])
			}
			w.Header().Set("Content-Type", "text/event-stream")
		case 2:
			if payload["stream"] != false {
				t.Fatalf("expected fallback request to disable stream, got %#v", payload["stream"])
			}
			writeJSON(t, w, map[string]any{
				"content": []map[string]any{
					{"type": "tool_use", "id": "done_after_fallback", "name": "attempt_completion", "input": map[string]any{"result": "非流式恢复成功。"}},
				},
				"stop_reason": "tool_use",
			})
		default:
			t.Fatalf("unexpected request count: %d", count)
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareAnthropicConversation(t, ctx, env, server.URL, "你好")
	var eventTypes []string
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{
		UserID: testUserID,
		Event: func(eventType, level string, payload map[string]any) {
			eventTypes = append(eventTypes, eventType)
		},
	})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if requestCount.Load() != 2 {
		t.Fatalf("expected stream request plus non-stream fallback, got %d", requestCount.Load())
	}
	if !strings.Contains(reply.Content, "非流式恢复成功") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	if !containsString(eventTypes, "model_stream_recovering") {
		t.Fatalf("expected model_stream_recovering event, got %#v", eventTypes)
	}
	logs, err := env.diagnostics.List(ctx, store.DiagnosticLogQuery{UserID: testUserID, Limit: 10})
	if err != nil {
		t.Fatalf("diagnostics.List returned error: %v", err)
	}
	if len(logs) == 0 || logs[0].Stage != "provider_empty_stream_response" {
		t.Fatalf("expected provider_empty_stream_response diagnostic, got %#v", logs)
	}
	if !strings.Contains(logs[0].RequestSummary, `"protocol":"anthropic"`) {
		t.Fatalf("expected request summary in diagnostic, got %q", logs[0].RequestSummary)
	}
}

func TestGenerateReplyAnthropicEmptyStreamRespectsFallbackDisabled(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	t.Setenv("PERSEUS_PROVIDER_STREAM_FALLBACK_ENABLED", "false")

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid request payload: %v", err)
		}
		if payload["stream"] != true {
			t.Fatalf("fallback should be disabled, got stream=%#v", payload["stream"])
		}
		requestCount.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareAnthropicConversation(t, ctx, env, server.URL, "你好")
	_, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err == nil {
		t.Fatal("expected GenerateReply error")
	}
	if requestCount.Load() != 1 {
		t.Fatalf("expected no non-stream fallback request, got %d requests", requestCount.Load())
	}
}

func TestGenerateReplyAnthropicFallsBackToNonStreamWhenFinalStreamBreaks(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	t.Setenv("PERSEUS_PROVIDER_RETRY_BASE_DELAY_MS", "0")

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid request payload: %v", err)
		}
		switch requestCount.Add(1) {
		case 1:
			if payload["stream"] != true {
				t.Fatalf("expected first request to stream, got %#v", payload["stream"])
			}
			writeJSON(t, w, map[string]any{
				"content": []map[string]any{{
					"type": "tool_use",
					"id":   "toolu_write_1",
					"name": "save_workspace_file",
					"input": map[string]any{
						"path":    "anthropic-verified-stream-fallback.txt",
						"content": "done",
						"mode":    "create",
					},
				}},
				"stop_reason": "tool_use",
			})
		case 2:
			if payload["stream"] != true {
				t.Fatalf("expected final request to stream before fallback, got %#v", payload["stream"])
			}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "event: content_block_start\n")
			fmt.Fprint(w, "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"partial final\"}}\n\n")
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				panic(http.ErrAbortHandler)
			}
			conn, _, hijackErr := hijacker.Hijack()
			if hijackErr != nil {
				panic(http.ErrAbortHandler)
			}
			_ = conn.Close()
		case 3:
			if payload["stream"] != false {
				t.Fatalf("expected same-turn fallback to disable stream, got %#v", payload["stream"])
			}
			writeJSON(t, w, map[string]any{
				"content": []map[string]any{{
					"type": "tool_use",
					"id":   "done_after_fallback",
					"name": "attempt_completion",
					"input": map[string]any{
						"result":        "已通过非流式降级完成。",
						"changed_paths": []string{"anthropic-verified-stream-fallback.txt"},
					},
				}},
				"stop_reason": "tool_use",
			})
		default:
			t.Fatalf("unexpected request count: %d", requestCount.Load())
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareAnthropicConversation(t, ctx, env, server.URL, "请创建 anthropic-verified-stream-fallback.txt，内容写 done")
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if !strings.Contains(reply.Content, "已通过非流式降级完成") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	if requestCount.Load() != 3 {
		t.Fatalf("expected write, broken final stream, and non-stream fallback, got %d", requestCount.Load())
	}
}

func TestGenerateReplyAnthropicIncompleteToolArgumentsDiagnosticNamesTool(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: content_block_start\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_truncated_1\",\"name\":\"save_workspace_file\",\"input\":{}}}\n\n")
		fmt.Fprint(w, "event: content_block_delta\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"path\\\":\\\"机甲少女/机甲少女11-22集修改版.txt\\\", \\\"mode\\\": \\\"create\\\", \\\"content\\\":\\\"第一段\"}}\n\n")
		fmt.Fprint(w, "event: message_delta\n")
		fmt.Fprintf(w, "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"max_tokens\"},\"usage\":{\"output_tokens\":%d}}\n\n", anthropicAgentMaxTokens)
		fmt.Fprint(w, "event: message_stop\n")
		fmt.Fprint(w, "data: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareAnthropicConversation(t, ctx, env, server.URL, "请后台生成长文件")
	_, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err == nil {
		t.Fatal("expected incomplete tool arguments error")
	}
	logs, listErr := env.diagnostics.List(ctx, store.DiagnosticLogQuery{UserID: testUserID, Limit: 10})
	if listErr != nil {
		t.Fatalf("diagnostics.List returned error: %v", listErr)
	}
	if len(logs) == 0 || logs[0].Stage != "provider_stream_incomplete_tool_arguments" {
		t.Fatalf("expected incomplete tool diagnostic, got %#v", logs)
	}
	var summary ProviderRequestSummary
	if err := json.Unmarshal([]byte(logs[0].RequestSummary), &summary); err != nil {
		t.Fatalf("request summary should unmarshal: %v\n%s", err, logs[0].RequestSummary)
	}
	if summary.StreamToolArgsState != "incomplete_json" || summary.StreamFinishReason != "max_tokens" {
		t.Fatalf("unexpected stream summary: %#v", summary)
	}
	if summary.StreamUsage.OutputTokens != anthropicAgentMaxTokens {
		t.Fatalf("expected output token usage, got %#v", summary.StreamUsage)
	}
	if len(summary.StreamToolCalls) != 1 {
		t.Fatalf("expected stream tool call details, got %#v", summary.StreamToolCalls)
	}
	call := summary.StreamToolCalls[0]
	if call.Name != "save_workspace_file" || call.ID != "toolu_truncated_1" {
		t.Fatalf("unexpected stream tool call summary: %#v", call)
	}
	if call.TargetSummary != "机甲少女/机甲少女11-22集修改版.txt" {
		t.Fatalf("expected target path in diagnostic, got %#v", call)
	}
	if call.ArgumentsJSONValid || call.ArgumentsJSONError == "" || !call.LooksLikeLongWrite || call.RecommendedTool != "apply_workspace_patch" {
		t.Fatalf("expected invalid long-write details, got %#v", call)
	}
}

func TestGenerateReplyCapturesTextPatchWithoutStreamingPatchToClient(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		switch requestCount.Add(1) {
		case 1:
			writeOpenAIToolCall(t, w, "load_workspace", "load_perseus_toolset", `{"toolset":"workspace"}`)
		case 2:
			writeOpenAIToolCall(t, w, "structured_patch", "apply_workspace_patch", mustJSON(map[string]any{"patch": "*** Begin Patch\n*** Add File: patch-output.md\n+hello\n*** End Patch"}))
		case 3:
			writeOpenAIToolCall(t, w, "done_patch_output", "attempt_completion", `{"result":"已创建 patch-output.md。","changed_paths":["patch-output.md"]}`)
		default:
			t.Fatalf("unexpected request %d", requestCount.Load())
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "创建 patch-output.md")
	var streamed []string
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{
		UserID: testUserID,
		Event: func(eventType, _ string, payload map[string]any) {
			if eventType == "model_delta" {
				if text, _ := payload["text"].(string); text != "" {
					streamed = append(streamed, text)
				}
			}
		},
	})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if strings.Contains(strings.Join(streamed, ""), "*** Begin Patch") {
		t.Fatalf("patch leaked to streamed model deltas: %#v", streamed)
	}
	if !strings.Contains(reply.Content, "patch-output.md") {
		t.Fatalf("unexpected reply: %#v", reply)
	}
	read, err := env.files.ReadFile("patch-output.md")
	if err != nil || read.Content != "hello\n" {
		t.Fatalf("expected patch-created file, content=%q err=%v", read.Content, err)
	}
	if len(reply.ToolEvents) == 0 || reply.ToolEvents[0].ToolName != "load_perseus_toolset" || reply.ToolEvents[1].ToolName != "apply_workspace_patch" {
		t.Fatalf("expected loader and internal patch tool events, got %#v", reply.ToolEvents)
	}
}

func TestGenerateReplyUsesNativePatchToolWhenProviderCapabilityEnabled(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		w.Header().Set("Content-Type", "text/event-stream")
		switch requestCount.Add(1) {
		case 1:
			names := openAIToolNamesFromPayload(t, payload)
			if !names["apply_workspace_patch"] {
				t.Fatalf("native patch capability should expose native patch tool, got %#v", names)
			}
			writeOpenAIToolCall(t, w, "patch_native", "apply_workspace_patch", `{"patch":"*** Begin Patch\n*** Add File: native-patch.md\n+native\n*** End Patch"}`)
		case 2:
			writeOpenAIToolCall(t, w, "done_native", "attempt_completion", `{"result":"已创建 native-patch.md。"}`)
		default:
			t.Fatalf("unexpected request %d", requestCount.Load())
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "创建 native-patch.md")
	nativePatchEnabled := true
	if _, err := env.configs.UpdateModelProvider(ctx, provider.UserID, provider.ID, store.ModelProviderInput{
		Name:                        provider.Name,
		Visibility:                  provider.Visibility,
		Protocol:                    provider.Protocol,
		BaseURL:                     server.URL,
		ModelID:                     provider.ModelID,
		Enabled:                     provider.Enabled,
		IsDefault:                   provider.IsDefault,
		ReasoningCapability:         provider.ReasoningCapability,
		ReasoningCapabilitySource:   provider.ReasoningCapabilitySource,
		ReasoningProfileLevels:      provider.ReasoningProfileLevels,
		ContextWindowTokens:         provider.ContextWindowTokens,
		AutoCompactEnabled:          &provider.AutoCompactEnabled,
		AutoCompactThresholdPercent: provider.AutoCompactThresholdPercent,
		CompactReservedTokens:       provider.CompactReservedTokens,
		PruneToolOutputsEnabled:     &provider.PruneToolOutputsEnabled,
		NativePatchToolEnabled:      &nativePatchEnabled,
	}); err != nil {
		t.Fatalf("UpdateModelProvider returned error: %v", err)
	}

	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if !strings.Contains(reply.Content, "native-patch.md") {
		t.Fatalf("unexpected reply: %#v", reply)
	}
	read, err := env.files.ReadFile("native-patch.md")
	if err != nil || read.Content != "native\n" {
		t.Fatalf("expected native patch-created file, content=%q err=%v", read.Content, err)
	}
}

func TestGenerateReplyAllowsPartialAfterToolFailure(t *testing.T) {
	runAuditScenario(t, "partial")
}

func TestGenerateReplyRecoversPrematureCompletionWithPatchActionObservation(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	if _, err := env.files.MkdirNew("人间烟火系统：开局一个盒饭摊"); err != nil {
		t.Fatalf("MkdirNew returned error: %v", err)
	}

	var requestCount atomic.Int32
	var rejectedPayloads []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		w.Header().Set("Content-Type", "text/event-stream")
		switch requestCount.Add(1) {
		case 1:
			assertOpenAIWorkspaceToolsLoaded(t, payload)
			assertOpenAIActionManifestIncludesPatch(t, payload)
			writeOpenAIToolCall(t, w, "load_workspace", "load_perseus_toolset", `{"toolset":"workspace"}`)
		case 2:
			assertOpenAIActionManifestIncludesPatch(t, payload)
			writeOpenAIToolCall(t, w, "premature_done", "attempt_completion", `{"result":"已创建开局版本对比.md。","changed_paths":["人间烟火系统：开局一个盒饭摊/开局版本对比.md"]}`)
		case 3:
			toolContent := lastOpenAIToolContent(t, payload)
			var observation map[string]any
			if err := json.Unmarshal([]byte(toolContent), &observation); err != nil {
				t.Fatalf("expected typed action rejection JSON, got %v content=%s", err, toolContent)
			}
			rejectedPayloads = append(rejectedPayloads, observation)
			rawObservation, _ := observation["observation"].(map[string]any)
			if rawObservation["status"] != "action_rejected" || rawObservation["required_next_action"] != "apply_workspace_patch" {
				t.Fatalf("expected patch required action observation, got %#v", observation)
			}
			writeOpenAIToolCall(t, w, "structured_patch", "apply_workspace_patch", mustJSON(map[string]any{"patch": "*** Begin Patch\n*** Add File: 人间烟火系统：开局一个盒饭摊/开局版本对比.md\n+# 开局版本对比\n+\n+盒饭摊版本。\n*** End Patch"}))
		case 4:
			writeOpenAIToolCall(t, w, "done", "attempt_completion", `{"result":"已创建开局版本对比.md。"}`)
		default:
			t.Fatalf("unexpected request %d", requestCount.Load())
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "在工作区里面新建文档写进去吧，不要发到对话里面。写道人间烟火系统这个文件夹里面")
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{
		UserID: testUserID,
	})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if requestCount.Load() != 4 {
		t.Fatalf("expected completion rejection, patch, then terminal completion; requests=%d reply=%#v", requestCount.Load(), reply)
	}
	if len(rejectedPayloads) != 1 {
		t.Fatalf("expected one typed rejection observation, got %#v", rejectedPayloads)
	}
	read, err := env.files.ReadFile("人间烟火系统：开局一个盒饭摊/开局版本对比.md")
	if err != nil || !strings.Contains(read.Content, "盒饭摊版本") {
		t.Fatalf("expected patch-created file, content=%q err=%v", read.Content, err)
	}
}

func TestGenerateReplyPreservesStructuredCallAlongsideProsePatch(t *testing.T) {
	runAuditScenario(t, "mixed")
}

func TestGenerateReplyStreamsAttemptCompletionPreviewWithoutDuplicateModelCompletion(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_, _ = io.ReadAll(r.Body)
		switch requestCount.Add(1) {
		case 1:
			writeOpenAIToolCall(t, w, "load_workspace", "load_perseus_toolset", `{"toolset":"workspace"}`)
		case 2:
			writeOpenAIStreamTextAndToolCall(t, w, "我先检查。", "read_1", "read_workspace_text", `{"path":"draft.txt"}`)
		case 3:
			w.Header().Set("Content-Type", "text/event-stream")
			chunks := []map[string]any{
				{"choices": []map[string]any{{"delta": map[string]any{"content": "我准备完成"}}}},
				{"choices": []map[string]any{{"delta": map[string]any{"tool_calls": []map[string]any{{
					"index": 0,
					"id":    "done_1",
					"type":  "function",
					"function": map[string]any{
						"name":      "attempt_completion",
						"arguments": `{"result":"最终答复`,
					},
				}}}}}},
				{"choices": []map[string]any{{"delta": map[string]any{"content": "。"}}}},
				{"choices": []map[string]any{{"delta": map[string]any{"tool_calls": []map[string]any{{
					"index": 0,
					"function": map[string]any{
						"arguments": `内容"}`,
					},
				}}}, "finish_reason": "tool_calls"}}},
			}
			for _, chunk := range chunks {
				raw, err := json.Marshal(chunk)
				if err != nil {
					t.Fatalf("Marshal returned error: %v", err)
				}
				fmt.Fprintf(w, "data: %s\n\n", raw)
			}
			fmt.Fprint(w, "data: [DONE]\n\n")
		default:
			t.Fatalf("unexpected request %d", requestCount.Load())
		}
	}))
	defer server.Close()

	if _, err := env.files.CreateFile(ctx, "draft.txt", "hello\n"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}
	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "读取 draft 后完成")
	var modelDeltas []string
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{
		UserID: testUserID,
		Event: func(eventType, _ string, payload map[string]any) {
			if eventType == "model_delta" {
				if text, _ := payload["text"].(string); text != "" {
					modelDeltas = append(modelDeltas, text)
				}
			}
		},
	})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if reply.Content != "最终答复内容" {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	if got := strings.Join(modelDeltas, ""); got != "我先检查。我准备完成。" {
		t.Fatalf("unexpected ordinary model deltas: %#v", modelDeltas)
	}
	if strings.Contains(strings.Join(modelDeltas, ""), "最终答复") {
		t.Fatalf("attempt_completion.result leaked into ordinary model deltas: %#v", modelDeltas)
	}
}

func TestGenerateReplyRecordsDiagnosticForNonJSON(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "hello")
	_, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err == nil {
		t.Fatal("expected GenerateReply error")
	}

	logs, err := env.diagnostics.List(ctx, store.DiagnosticLogQuery{UserID: testUserID, Limit: 10})
	if err != nil {
		t.Fatalf("diagnostics.List returned error: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 diagnostic log, got %d", len(logs))
	}
	if logs[0].ContentType != "text/plain" {
		t.Fatalf("unexpected content type: %q", logs[0].ContentType)
	}
	if logs[0].ResponseSnippet != "not json" {
		t.Fatalf("unexpected response snippet: %q", logs[0].ResponseSnippet)
	}
}

func TestGenerateReplyRecordsProvider400ToolRequestSummary(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid request payload: %v", err)
		}
		if payload["stream"] != true {
			t.Fatalf("expected stream=true on provider request, got %#v", payload["stream"])
		}
		if payload["tool_choice"] != "auto" {
			t.Fatalf("expected provider-auto tool_choice with local action boundary, got %#v", payload["tool_choice"])
		}
		names := openAIToolNamesFromPayload(t, payload)
		if !names["attempt_completion"] || !names["ask_followup_question"] {
			t.Fatalf("expected workflow tools in payload, got %#v", names)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"schema rejected by provider"}}`))
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "看看引用文件")
	_, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, []AttachmentPayload{{Kind: "workspace_file", Path: "demo.txt"}}, RunOptions{UserID: testUserID})
	if err == nil {
		t.Fatal("expected GenerateReply error")
	}
	logs, err := env.diagnostics.List(ctx, store.DiagnosticLogQuery{UserID: testUserID, Limit: 10})
	if err != nil {
		t.Fatalf("diagnostics.List returned error: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 diagnostic log, got %#v", logs)
	}
	var summary map[string]any
	if err := json.Unmarshal([]byte(logs[0].RequestSummary), &summary); err != nil {
		t.Fatalf("request_summary should be JSON: %v\n%s", err, logs[0].RequestSummary)
	}
	expected := map[string]any{
		"request_kind":           "chat_completions",
		"stream":                 true,
		"tool_choice":            "auto",
		"failure_class":          "provider_bad_request_with_tools",
		"has_tools":              true,
		"action_boundary_active": true,
	}
	for key, value := range expected {
		if summary[key] != value {
			t.Fatalf("expected summary[%s]=%#v, got %#v in %#v", key, value, summary[key], summary)
		}
	}
	if got, _ := summary["tools_count"].(float64); got < 2 {
		t.Fatalf("expected tools_count >= 2, got %#v in %#v", summary["tools_count"], summary)
	}
	if summary["provider_host"] == "" || summary["model_id"] == "" {
		t.Fatalf("expected provider host and model id in summary: %#v", summary)
	}
	toolNames, ok := summary["tool_names"].([]any)
	if !ok || !containsAnyStringValue(toolNames, "attempt_completion") || !containsAnyStringValue(toolNames, "ask_followup_question") {
		t.Fatalf("expected tool_names to include workflow tools, got %#v", summary["tool_names"])
	}
	if strings.Contains(logs[0].RequestSummary, "看看当前文件") || strings.Contains(logs[0].RequestSummary, "test-key") {
		t.Fatalf("request_summary leaked prompt or API key: %s", logs[0].RequestSummary)
	}
}

func TestGenerateReplyProvider400DoesNotGuessRequiredToolChoice(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("invalid request payload: %v", err)
		}
		if payload["tool_choice"] != "auto" {
			t.Fatalf("expected provider-auto tool_choice with local action boundary, got %#v", payload["tool_choice"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"openai_error","type":"bad_response_status_code","code":"bad_response_status_code"}`))
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "看看引用文件")
	_, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, []AttachmentPayload{{Kind: "workspace_file", Path: "demo.txt"}}, RunOptions{UserID: testUserID})
	if err == nil {
		t.Fatal("expected GenerateReply error")
	}
	logs, err := env.diagnostics.List(ctx, store.DiagnosticLogQuery{UserID: testUserID, Limit: 10})
	if err != nil {
		t.Fatalf("diagnostics.List returned error: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 diagnostic log, got %#v", logs)
	}
	var summary map[string]any
	if err := json.Unmarshal([]byte(logs[0].RequestSummary), &summary); err != nil {
		t.Fatalf("request_summary should be JSON: %v\n%s", err, logs[0].RequestSummary)
	}
	if summary["failure_class"] == "provider_rejected_required_tool_choice" {
		t.Fatalf("must not guess required tool_choice rejection from generic 400: %#v", summary)
	}
	if summary["failure_class"] != "provider_bad_request_with_tools" {
		t.Fatalf("expected provider_bad_request_with_tools, got %#v in %#v", summary["failure_class"], summary)
	}
	if _, ok := summary["payload_keys"].([]any); !ok {
		t.Fatalf("expected payload_keys in diagnostic summary: %#v", summary)
	}
	if digest, _ := summary["tool_schema_digest"].(string); digest == "" {
		t.Fatalf("expected tool_schema_digest in diagnostic summary: %#v", summary)
	}
	if bytes, _ := summary["payload_bytes"].(float64); bytes <= 0 {
		t.Fatalf("expected payload_bytes in diagnostic summary: %#v", summary)
	}
	if summary["has_tools"] != true || summary["has_required_tool_choice"] == true || summary["action_boundary_active"] != true {
		t.Fatalf("expected tool presence, provider-auto, and local action boundary diagnostics: %#v", summary)
	}
	if summary["provider_host"] == "" || summary["model_id"] == "" {
		t.Fatalf("expected provider host and model id in diagnostic summary: %#v", summary)
	}
	messageShapes, ok := summary["message_shapes"].([]any)
	if !ok || len(messageShapes) == 0 {
		t.Fatalf("expected message_shapes in diagnostic summary: %#v", summary)
	}
	if warnings, _ := summary["message_protocol_warnings"].([]any); len(warnings) != 0 {
		t.Fatalf("did not expect protocol warnings for a single failed first request, got %#v", warnings)
	}
	toolItems, ok := summary["tool_schema_items"].([]any)
	if !ok || len(toolItems) == 0 {
		t.Fatalf("expected tool_schema_items in diagnostic summary: %#v", summary)
	}
	if warnings, _ := summary["tool_schema_warnings"].([]any); len(warnings) != 0 {
		t.Fatalf("workspace tool schema should be OpenAI-compatible, got warnings %#v in %#v", warnings, summary)
	}
}

func summaryContainsUnsupportedPath(summary map[string]any, expected string) bool {
	if warnings, _ := summary["tool_schema_warnings"].([]any); containsAnyStringValue(warnings, expected) {
		return true
	}
	items, _ := summary["tool_schema_items"].([]any)
	for _, rawItem := range items {
		item, _ := rawItem.(map[string]any)
		paths, _ := item["unsupported_paths"].([]any)
		if containsAnyStringValue(paths, expected) {
			return true
		}
	}
	return false
}

func prepareOpenAIConversation(t *testing.T, ctx context.Context, env *testAIEnv, baseURL, prompt string) (store.ModelProvider, store.Conversation, store.Message, []store.Message) {
	t.Helper()

	provider, err := env.configs.CreateModelProvider(ctx, testUserID, store.ModelProviderInput{
		Name:              "Test OpenAI",
		Visibility:        store.ResourceVisibilityPublic,
		Protocol:          "openai",
		SystemMessageMode: "multiple",
		BaseURL:           baseURL,
		APIKey:            "test-key",
		ModelID:           "test-model",
		Enabled:           true,
		IsDefault:         true,
	})
	if err != nil {
		t.Fatalf("CreateModelProvider returned error: %v", err)
	}

	conversation, err := env.conversations.CreateConversation(ctx, store.CreateConversationInput{
		UserID:                  testUserID,
		Title:                   "测试会话",
		SelectedModelProviderID: provider.ID,
	})
	if err != nil {
		t.Fatalf("CreateConversation returned error: %v", err)
	}

	userMessage, err := env.conversations.AppendMessage(ctx, testUserID, conversation.ID, "user", prompt, "[]")
	if err != nil {
		t.Fatalf("AppendMessage returned error: %v", err)
	}

	messages, err := env.conversations.ListMessages(ctx, testUserID, conversation.ID)
	if err != nil {
		t.Fatalf("ListMessages returned error: %v", err)
	}
	return provider, conversation, userMessage, messages
}

func containsAnyStringValue(items []any, expected string) bool {
	for _, item := range items {
		if text, ok := item.(string); ok && text == expected {
			return true
		}
	}
	return false
}

func prepareAnthropicConversation(t *testing.T, ctx context.Context, env *testAIEnv, baseURL, prompt string) (store.ModelProvider, store.Conversation, store.Message, []store.Message) {
	t.Helper()

	provider, err := env.configs.CreateModelProvider(ctx, testUserID, store.ModelProviderInput{
		Name:       "Test Anthropic",
		Visibility: store.ResourceVisibilityPublic,
		Protocol:   "anthropic",
		BaseURL:    baseURL,
		APIKey:     "test-key",
		ModelID:    "test-model",
		Enabled:    true,
		IsDefault:  true,
	})
	if err != nil {
		t.Fatalf("CreateModelProvider returned error: %v", err)
	}

	conversation, err := env.conversations.CreateConversation(ctx, store.CreateConversationInput{
		UserID:                  testUserID,
		Title:                   "测试会话",
		SelectedModelProviderID: provider.ID,
	})
	if err != nil {
		t.Fatalf("CreateConversation returned error: %v", err)
	}

	userMessage, err := env.conversations.AppendMessage(ctx, testUserID, conversation.ID, "user", prompt, "[]")
	if err != nil {
		t.Fatalf("AppendMessage returned error: %v", err)
	}

	messages, err := env.conversations.ListMessages(ctx, testUserID, conversation.ID)
	if err != nil {
		t.Fatalf("ListMessages returned error: %v", err)
	}
	return provider, conversation, userMessage, messages
}

func assertOpenAIWorkspaceToolsLoaded(t *testing.T, payload map[string]any) {
	t.Helper()

	if payload["parallel_tool_calls"] != false {
		t.Fatalf("expected parallel_tool_calls=false for stable serial tool execution, got %#v", payload["parallel_tool_calls"])
	}

	names := openAIToolNamesFromPayload(t, payload)
	for _, expected := range []string{"inspect_workspace", "find_workspace_files", "search_workspace_text", "read_workspace_text", "read_workspace_files", "manage_workspace_path", "organize_workspace_matches"} {
		if !names[expected] {
			t.Fatalf("tool %q missing from payload: %#v", expected, names)
		}
	}
	for _, unexpected := range []string{"edit_workspace_text", "replace_workspace_lines", "save_workspace_file", "write_workspace_chunk", "finalize_workspace_write"} {
		if names[unexpected] {
			t.Fatalf("old write tool %q should not be model-visible: %#v", unexpected, names)
		}
	}
}

func assertOpenAIActionManifestIncludesPatch(t *testing.T, payload map[string]any) {
	t.Helper()
	rawMessages, ok := payload["messages"].([]any)
	if !ok || len(rawMessages) == 0 {
		t.Fatalf("messages missing from payload")
	}
	var manifestPayload map[string]any
	for _, raw := range rawMessages {
		message, _ := raw.(map[string]any)
		if message["name"] != "perseus_action_manifest" {
			continue
		}
		content, _ := message["content"].(string)
		prefix := "Perseus current action manifest. This is environment-provided action state, not user input. Follow it for the next action.\n"
		if !strings.HasPrefix(content, prefix) {
			t.Fatalf("unexpected action manifest anchor: %s", content)
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(content, prefix)), &manifestPayload); err != nil {
			t.Fatalf("action manifest anchor is not JSON: %v content=%s", err, content)
		}
		break
	}
	if manifestPayload == nil {
		t.Fatalf("action manifest anchor missing from payload messages: %#v", rawMessages)
	}
	textActions, _ := manifestPayload["json_tools"].([]any)
	foundPatch := false
	for _, raw := range textActions {
		action, _ := raw.(map[string]any)
		if action["name"] == "apply_workspace_patch" && action["transport"] == "json" {
			foundPatch = true
		}
	}
	if !foundPatch {
		t.Fatalf("action manifest missing apply_workspace_patch text action: %#v", manifestPayload)
	}
	terminalActions, _ := manifestPayload["terminal_actions"].([]any)
	foundCompletion := false
	for _, raw := range terminalActions {
		action, _ := raw.(map[string]any)
		if action["name"] == "attempt_completion" && action["terminal"] == true {
			foundCompletion = true
		}
	}
	if !foundCompletion {
		t.Fatalf("action manifest missing terminal completion: %#v", manifestPayload)
	}
}

func assertOnlyCoreToolsLoaded(t *testing.T, payload map[string]any) {
	t.Helper()

	names := openAIToolNamesFromPayload(t, payload)
	for _, expected := range []string{"attempt_completion", "ask_followup_question", "load_perseus_toolset", "inspect_workspace", "find_workspace_files", "search_workspace_text", "read_workspace_text", "read_workspace_files", "manage_workspace_path", "organize_workspace_matches"} {
		if !names[expected] {
			t.Fatalf("expected core tool %q in payload, got %#v", expected, names)
		}
	}
	for _, unexpected := range []string{
		"edit_workspace_text",
		"replace_workspace_lines",
		"save_workspace_file",
		"write_workspace_chunk",
		"finalize_workspace_write",
		"run_workspace_command",
		"perseus_search_web",
		"list_skills",
		"start_background_task",
	} {
		if names[unexpected] {
			t.Fatalf("tool %q should require explicit toolset load, got %#v", unexpected, names)
		}
	}
}

func handleOpenAIWorkspaceToolsetLoad(t *testing.T, w http.ResponseWriter, payload map[string]any, id string) {
	t.Helper()
	assertOnlyCoreToolsLoaded(t, payload)
	writeOpenAIToolCall(t, w, id, "load_perseus_toolset", `{"toolset":"workspace"}`)
}

func assertOpenAISystemPromptSupportsWrite(t *testing.T, payload map[string]any) {
	t.Helper()

	rawMessages, ok := payload["messages"].([]any)
	if !ok || len(rawMessages) == 0 {
		t.Fatalf("messages missing from payload")
	}

	first, _ := rawMessages[0].(map[string]any)
	content, _ := first["content"].(string)
	for _, fragment := range []string{
		"标准 Agent Loop",
		"工具工作完成后，必须调用 attempt_completion",
		"attempt_completion 是终止动作，不能创建、修改、移动或删除文件",
		"当前文件上下文和编辑器面板上下文只是 metadata",
		"find_workspace_files 按名称/路径/glob 找文件",
		"改文件优先调用 apply_workspace_patch",
		"任何文件内容变更都使用 *** Begin Patch / *** End Patch patch 块表达",
		"run_workspace_command 在 workspace 内运行命令",
		"不可以永久删除文件",
		"不可以访问 workspace 之外的路径",
	} {
		if !strings.Contains(content, fragment) {
			t.Fatalf("system prompt missing fragment %q: %s", fragment, content)
		}
	}
}

func assertOpenAIToolsetLoaderPresent(t *testing.T, payload map[string]any) {
	t.Helper()
	names := openAIToolNamesFromPayload(t, payload)
	for _, expected := range []string{"attempt_completion", "read_workspace_text", "manage_workspace_path", "run_workspace_command"} {
		if !names[expected] {
			t.Fatalf("expected %q in agent tool payload, got %#v", expected, names)
		}
	}
	for _, unexpected := range []string{"edit_workspace_text", "replace_workspace_lines", "save_workspace_file", "write_workspace_chunk", "finalize_workspace_write"} {
		if names[unexpected] {
			t.Fatalf("old write tool %q should not be registered in payload, got %#v", unexpected, names)
		}
	}
}

func assertLastUserReminder(t *testing.T, payload map[string]any, expected string) {
	t.Helper()

	rawMessages, ok := payload["messages"].([]any)
	if !ok || len(rawMessages) == 0 {
		t.Fatalf("messages missing from payload")
	}

	last, _ := rawMessages[len(rawMessages)-1].(map[string]any)
	role, _ := last["role"].(string)
	content, _ := last["content"].(string)
	if role != "user" || content != expected {
		t.Fatalf("unexpected reminder message: role=%q content=%q", role, content)
	}
}

func assertLastUserReminderContains(t *testing.T, payload map[string]any, expected string) {
	t.Helper()

	rawMessages, ok := payload["messages"].([]any)
	if !ok || len(rawMessages) == 0 {
		t.Fatalf("messages missing from payload")
	}

	last, _ := rawMessages[len(rawMessages)-1].(map[string]any)
	role, _ := last["role"].(string)
	content, _ := last["content"].(string)
	if role != "user" || !strings.Contains(content, expected) {
		t.Fatalf("unexpected reminder message: role=%q content=%q", role, content)
	}
}

func assertLastUserActionRejected(t *testing.T, payload map[string]any, expectedStatus string) {
	t.Helper()
	rawMessages, ok := payload["messages"].([]any)
	if !ok || len(rawMessages) == 0 {
		t.Fatalf("messages missing from payload")
	}
	var last map[string]any
	for index := len(rawMessages) - 1; index >= 0; index-- {
		message, _ := rawMessages[index].(map[string]any)
		name, _ := message["name"].(string)
		role, _ := message["role"].(string)
		if name == "perseus_runtime_observation" || role == "user" {
			last = message
			break
		}
	}
	if last == nil {
		t.Fatalf("runtime action observation missing from payload: %#v", payload["messages"])
	}
	role, _ := last["role"].(string)
	content, _ := last["content"].(string)
	var decoded map[string]any
	if err := json.Unmarshal([]byte(content), &decoded); err != nil {
		t.Fatalf("expected typed action observation JSON, got role=%q content=%q err=%v", role, content, err)
	}
	observation, _ := decoded["observation"].(map[string]any)
	name, _ := last["name"].(string)
	if role != "system" && role != "user" {
		t.Fatalf("unexpected action observation role=%q decoded=%#v", role, decoded)
	}
	if role == "system" && name != "perseus_runtime_observation" {
		t.Fatalf("unexpected action observation system name=%q decoded=%#v", name, decoded)
	}
	if observation["status"] != expectedStatus || decoded["model_action_manifest"] == nil {
		t.Fatalf("unexpected action observation: role=%q decoded=%#v", role, decoded)
	}
}

func assertOpenAIHasActionRejectedReason(t *testing.T, payload map[string]any, expected string) {
	t.Helper()
	content := openAIProviderMessageContentContaining(payload, "action_rejected")
	if content == "" || !strings.Contains(content, expected) {
		t.Fatalf("expected action rejection containing %q, got %q", expected, content)
	}
}

func openAIProviderMessageContentContaining(payload map[string]any, expected string) string {
	rawMessages, ok := payload["messages"].([]any)
	if !ok {
		return ""
	}
	for index := len(rawMessages) - 1; index >= 0; index-- {
		message, _ := rawMessages[index].(map[string]any)
		content, _ := message["content"].(string)
		if strings.Contains(content, expected) {
			return content
		}
	}
	return ""
}

func openAIProviderMessageContentContainsAll(payload map[string]any, expected ...string) bool {
	rawMessages, ok := payload["messages"].([]any)
	if !ok {
		return false
	}
	for _, rawMessage := range rawMessages {
		message, _ := rawMessage.(map[string]any)
		content, _ := message["content"].(string)
		if content == "" {
			continue
		}
		matches := true
		for _, item := range expected {
			if !strings.Contains(content, item) {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func assertToolCallRecord(t *testing.T, env *testAIEnv, toolName, status string) {
	t.Helper()

	var count int
	if err := env.db.QueryRow(`SELECT COUNT(*) FROM tool_calls WHERE tool_name = ? AND status = ?`, toolName, status).Scan(&count); err != nil {
		t.Fatalf("QueryRow returned error: %v", err)
	}
	if count == 0 {
		t.Fatalf("expected tool_calls record for %s/%s", toolName, status)
	}
}

func assertNoToolCallRecord(t *testing.T, env *testAIEnv, toolName string) {
	t.Helper()

	var count int
	if err := env.db.QueryRow(`SELECT COUNT(*) FROM tool_calls WHERE tool_name = ?`, toolName).Scan(&count); err != nil {
		t.Fatalf("QueryRow returned error: %v", err)
	}
	if count != 0 {
		t.Fatalf("did not expect tool_calls record for %s, got %d", toolName, count)
	}
}

func openAIToolNamesFromPayload(t *testing.T, payload map[string]any) map[string]bool {
	t.Helper()

	rawTools, ok := payload["tools"].([]any)
	if !ok {
		t.Fatalf("tools missing from payload: %#v", payload["tools"])
	}

	names := make(map[string]bool)
	for _, item := range rawTools {
		tool, _ := item.(map[string]any)
		function, _ := tool["function"].(map[string]any)
		name, _ := function["name"].(string)
		if name != "" {
			names[name] = true
		}
	}
	return names
}

func assertOpenAIToolChoiceRequired(t *testing.T, payload map[string]any) {
	t.Helper()
	if payload["tool_choice"] != "required" {
		t.Fatalf("expected tool_choice=required, got %#v", payload["tool_choice"])
	}
}

func assertOpenAIToolChoiceAuto(t *testing.T, payload map[string]any) {
	t.Helper()
	if payload["tool_choice"] != "auto" {
		t.Fatalf("expected tool_choice=auto, got %#v", payload["tool_choice"])
	}
}

func writeOpenAIText(t *testing.T, w http.ResponseWriter, content string) {
	t.Helper()
	writeJSON(t, w, map[string]any{
		"choices": []map[string]any{
			{
				"message": map[string]any{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
	})
}

func writeOpenAIStreamText(t *testing.T, w http.ResponseWriter, content string) {
	t.Helper()
	w.Header().Set("Content-Type", "text/event-stream")
	chunks := []map[string]any{
		{"choices": []map[string]any{{"delta": map[string]any{"content": content}}}},
		{"choices": []map[string]any{{"delta": map[string]any{}, "finish_reason": "stop"}}},
	}
	for _, chunk := range chunks {
		raw, err := json.Marshal(chunk)
		if err != nil {
			t.Fatalf("Marshal returned error: %v", err)
		}
		fmt.Fprintf(w, "data: %s\n\n", raw)
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
}

func writeOpenAIStreamTextAndToolCall(t *testing.T, w http.ResponseWriter, content, id, name, arguments string) {
	t.Helper()
	writeOpenAIStreamTextAndToolCalls(t, w, content, []streamToolCall{{id: id, name: name, args: arguments}})
}

type streamToolCall struct {
	id   string
	name string
	args string
}

func writeOpenAIStreamTextAndToolCalls(t *testing.T, w http.ResponseWriter, content string, calls []streamToolCall) {
	t.Helper()
	w.Header().Set("Content-Type", "text/event-stream")
	toolCalls := make([]map[string]any, 0, len(calls))
	for index, call := range calls {
		toolCalls = append(toolCalls, map[string]any{
			"index": index,
			"id":    call.id,
			"type":  "function",
			"function": map[string]any{
				"name":      call.name,
				"arguments": call.args,
			},
		})
	}
	chunks := []map[string]any{
		{"choices": []map[string]any{{"delta": map[string]any{"content": content}}}},
		{"choices": []map[string]any{{"delta": map[string]any{"tool_calls": toolCalls}, "finish_reason": "tool_calls"}}},
	}
	for _, chunk := range chunks {
		raw, err := json.Marshal(chunk)
		if err != nil {
			t.Fatalf("Marshal returned error: %v", err)
		}
		fmt.Fprintf(w, "data: %s\n\n", raw)
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
}

func writeOpenAIToolCall(t *testing.T, w http.ResponseWriter, id, name, arguments string) {
	t.Helper()
	writeJSON(t, w, map[string]any{
		"choices": []map[string]any{
			{
				"message": map[string]any{
					"role": "assistant",
					"tool_calls": []map[string]any{
						{
							"id":   id,
							"type": "function",
							"function": map[string]any{
								"name":      name,
								"arguments": arguments,
							},
						},
					},
				},
			},
		},
	})
}

func lastOpenAIToolContent(t *testing.T, payload map[string]any) string {
	t.Helper()
	rawMessages, ok := payload["messages"].([]any)
	if !ok {
		t.Fatalf("messages missing from payload")
	}
	for index := len(rawMessages) - 1; index >= 0; index-- {
		message, _ := rawMessages[index].(map[string]any)
		if message["role"] != "tool" {
			continue
		}
		content, _ := message["content"].(string)
		return content
	}
	t.Fatalf("tool result message missing from payload: %#v", payload["messages"])
	return ""
}

func assertWorkspaceToolsetResultSupportsPatchEditing(t *testing.T, content string) {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		t.Fatalf("toolset result is not JSON: %v content=%s", err, content)
	}
	if result["loaded_toolset"] != "workspace" {
		t.Fatalf("unexpected loaded toolset result: %#v", result)
	}
	for _, key := range []string{"available_tools", "json_tools"} {
		rawTools, ok := result[key].([]any)
		if !ok {
			t.Fatalf("toolset result missing %s list: %#v", key, result)
		}
		for _, raw := range rawTools {
			if raw == "attempt_completion" || raw == "save_workspace_file" || raw == "write_workspace_chunk" {
				t.Fatalf("workspace text edit protocol must not be exposed as a JSON tool in %s: %#v", key, rawTools)
			}
		}
	}
	textActions, ok := result["json_tools"].([]any)
	if !ok {
		t.Fatalf("toolset result missing text_actions: %#v", result)
	}
	foundPatchAction := false
	for _, raw := range textActions {
		action, _ := raw.(map[string]any)
		if raw == "apply_workspace_patch" || (action["name"] == "apply_workspace_patch" && action["transport"] == "json") {
			foundPatchAction = true
		}
	}
	if !foundPatchAction {
		t.Fatalf("toolset result must expose apply_workspace_patch as text action: %#v", textActions)
	}
	terminalActions, ok := result["terminal_actions"].([]any)
	if !ok || len(terminalActions) == 0 {
		t.Fatalf("toolset result missing terminal_actions: %#v", result)
	}
	foundCompletionAction := false
	for _, raw := range terminalActions {
		action, _ := raw.(map[string]any)
		if action["name"] == "attempt_completion" && action["terminal"] == true {
			foundCompletionAction = true
		}
	}
	if !foundCompletionAction {
		t.Fatalf("terminal_actions must expose attempt_completion: %#v", terminalActions)
	}
	capabilities, ok := result["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("toolset result missing capabilities: %#v", result)
	}
	editing, ok := capabilities["file_content_editing"].(map[string]any)
	if !ok || editing["available"] != true || editing["transport"] != "json" {
		t.Fatalf("toolset result missing JSON editing capability and text fallback: %#v", result)
	}
	if strings.TrimSpace(fmt.Sprint(result["tool_manifest_digest"])) == "" {
		t.Fatalf("toolset result missing manifest digest: %#v", result)
	}
	instruction, _ := result["instruction"].(string)
	for _, fragment := range []string{"apply_workspace_patch", "*** Begin Patch", "*** End Patch", "attempt_completion cannot create or modify files"} {
		if !strings.Contains(instruction, fragment) {
			t.Fatalf("toolset instruction missing %q: %s", fragment, instruction)
		}
	}
}

func hasFolder(nodes []workspace.TreeNode, target string) bool {
	for _, node := range nodes {
		if node.Type == "folder" && node.Path == target {
			return true
		}
		if hasFolder(node.Children, target) {
			return true
		}
	}
	return false
}

func hasFile(nodes []workspace.TreeNode, target string) bool {
	for _, node := range nodes {
		if node.Type == "file" && node.Path == target {
			return true
		}
		if hasFile(node.Children, target) {
			return true
		}
	}
	return false
}

func hasTrashItem(items []workspace.TrashItem, target string) bool {
	for _, item := range items {
		if item.Path == target {
			return true
		}
	}
	return false
}

func writeJSON(t *testing.T, w http.ResponseWriter, payload map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
}

func assertEqual(t *testing.T, actual, expected any) {
	t.Helper()
	if fmt.Sprint(actual) != fmt.Sprint(expected) {
		t.Fatalf("expected %v, got %v", expected, actual)
	}
}

func containsString(items []string, expected string) bool {
	for _, item := range items {
		if item == expected {
			return true
		}
	}
	return false
}

func messagesContainContent(messages []map[string]any, expected string) bool {
	for _, message := range messages {
		if strings.Contains(fmt.Sprint(message["content"]), expected) {
			return true
		}
	}
	return false
}

func messagesContainContentFromPayload(payload map[string]any, expected string) bool {
	items, ok := payload["messages"].([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		message, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if strings.Contains(fmt.Sprint(message["content"]), expected) {
			return true
		}
	}
	return false
}

func messagesContainStoredSteer(messages []store.Message, steerID, content string) bool {
	for _, message := range messages {
		if message.Role != "user" || !strings.Contains(message.Content, content) {
			continue
		}
		var meta map[string]any
		if err := json.Unmarshal([]byte(message.MetaJSON), &meta); err != nil {
			continue
		}
		if fmt.Sprint(meta["kind"]) == "steer" && fmt.Sprint(meta["steer_id"]) == steerID {
			return true
		}
	}
	return false
}

func assertStoredSteerTimelineMeta(t *testing.T, messages []store.Message, steerID, status string) {
	t.Helper()
	for _, message := range messages {
		if message.Role != "user" {
			continue
		}
		var meta map[string]any
		if err := json.Unmarshal([]byte(message.MetaJSON), &meta); err != nil {
			continue
		}
		if fmt.Sprint(meta["kind"]) != "steer" || fmt.Sprint(meta["steer_id"]) != steerID {
			continue
		}
		if fmt.Sprint(meta["status"]) != status {
			t.Fatalf("expected steer status %q, got meta %#v", status, meta)
		}
		receivedAt, err := time.Parse(time.RFC3339Nano, fmt.Sprint(meta["received_at"]))
		if err != nil {
			t.Fatalf("expected parseable steer received_at, got meta %#v", meta)
		}
		if !message.CreatedAt.Equal(receivedAt) {
			t.Fatalf("steer message created_at must equal received_at, got created_at=%s received_at=%s", message.CreatedAt, receivedAt)
		}
		return
	}
	t.Fatalf("expected stored steer %q in messages: %#v", steerID, messages)
}

func agentEventsContain(events []store.AgentEvent, eventType, steerID string) bool {
	for _, event := range events {
		if event.Type != eventType {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err == nil && fmt.Sprint(payload["steer_id"]) == steerID {
			return true
		}
	}
	return false
}
