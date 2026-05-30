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
			assertOpenAIWriteToolsPresent(t, payload)
			writeOpenAIToolCall(t, w, "call_create_1", "save_workspace_file", `{"path":"ai-test.txt","content":"hello from ai","mode":"create"}`)
		case 2:
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
	if result.Content != "hello from ai" {
		t.Fatalf("unexpected file content: %q", result.Content)
	}
	if !strings.Contains(reply.Content, "ai-test.txt") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	assertToolCallRecord(t, env, "save_workspace_file", "success")
	if requestCount.Load() != 2 {
		t.Fatalf("expected 2 model requests, got %d", requestCount.Load())
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

func TestGenerateReplyOpenAIToolLoopUpgradesFromChatToWorkspaceRoundLimit(t *testing.T) {
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
		if count <= agentChatMaxRounds+1 {
			writeOpenAIToolCall(t, w, fmt.Sprintf("call_inspect_%d", count), "inspect_workspace", `{"path":"","max_depth":1}`)
			return
		}
		writeOpenAIToolCall(t, w, "call_done_1", "attempt_completion", `{"result":"已完成长工具任务。"}`)
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "请连续检查工作区并完成")
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("GenerateReply returned error after workspace tool loop exceeded chat rounds: %v", err)
	}
	if !strings.Contains(reply.Content, "已完成长工具任务") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	if requestCount.Load() <= agentChatMaxRounds {
		t.Fatalf("expected more than chat max rounds, got %d", requestCount.Load())
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
		ID:       "provider_budget_test",
		Name:     "Budget Test",
		Protocol: "openai",
		BaseURL:  server.URL,
		APIKey:   "test-key",
		ModelID:  "gpt-test",
		Enabled:  true,
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
		ID:       "provider_budget_tools_test",
		Name:     "Budget Tool Test",
		Protocol: "openai",
		BaseURL:  server.URL,
		APIKey:   "test-key",
		ModelID:  "gpt-test",
		Enabled:  true,
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

	run, err := env.runs.CreateRun(ctx, store.CreateAgentRunInput{
		ID:              "run_steer_test",
		UserID:          testUserID,
		ConversationID:  "conv_steer_test",
		UserMessageID:   "msg_steer_test",
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
		ConversationID: "conv_steer_test",
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
		ID:       "provider_steer_test",
		Name:     "Steer Test",
		Protocol: "openai",
		BaseURL:  server.URL,
		APIKey:   "test-key",
		ModelID:  "gpt-test",
		Enabled:  true,
	}
	runtime := &agentRuntime{
		service:          env.service,
		protocol:         agentProtocolOpenAI,
		conversationID:   "conv_steer_test",
		messageID:        "msg_steer_test",
		provider:         provider,
		messages:         []store.Message{{ID: "msg_steer_test", Role: "user", Content: "继续处理"}},
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
			assertOpenAIToolChoiceRequired(t, payload)
			writeOpenAIStreamText(t, w, "好，我先看一下 workspace。")
		case 2:
			assertOpenAIToolChoiceRequired(t, payload)
			assertLastUserReminderContains(t, payload, "没有继续调用工具")
			writeOpenAIToolCall(t, w, "call_inspect_1", "inspect_workspace", `{"path":"","max_depth":1}`)
		case 3:
			assertOpenAIToolChoiceAuto(t, payload)
			writeOpenAIStreamText(t, w, "已查看工作区。")
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
			assertOpenAIToolChoiceRequired(t, payload)
			writeOpenAIStreamTextAndToolCall(t, w, "我先查看工作区，再决定下一步。", "call_inspect_1", "inspect_workspace", `{"path":"","max_depth":1}`)
		case 2:
			assertOpenAIToolChoiceAuto(t, payload)
			writeOpenAIStreamText(t, w, "已查看工作区。下面是整理后的结论。")
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
	if !strings.Contains(streamed.String(), "下面是整理后的结论") {
		t.Fatalf("expected final content to stream as assistant text, got %q", streamed.String())
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

		if count := requestCount.Add(1); count != 1 {
			t.Fatalf("unexpected request count: %d", count)
		}
		assertOpenAIToolChoiceAuto(t, payload)
		writeOpenAIStreamText(t, w, "收到，你已经登录成功。下一步执行 pwd 确认当前目录。")
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
		t.Fatalf("expected one ordinary chat request, got %d", requestCount.Load())
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
		if count := requestCount.Add(1); count != 1 {
			t.Fatalf("start_background_task must finish the chat run without a second provider request, got request %d with payload %s", count, string(raw))
		}
		if payload["tool_choice"] != "auto" {
			t.Fatalf("expected ordinary chat request to allow background-task tool selection with auto, got %#v", payload["tool_choice"])
		}
		writeOpenAIStreamTextAndToolCall(t, w,
			"我会创建后台任务来继续写十集。",
			"call_task_1",
			"start_background_task",
			`{"title":"续写向北方10集","goal":"续写《向北方》10集","expected_outputs":[{"type":"file","path":"向北方/向北方 第3-12集.txt","description":"续写10集剧本"}],"suggested_steps":["读取当前第2集","延续人物和悬疑线","分集写作"]}`)
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
	if requestCount.Load() != 1 {
		t.Fatalf("expected exactly one provider request, got %d", requestCount.Load())
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
			assertOpenAIWriteToolsPresent(t, payload)
			writeOpenAIToolCall(t, w, "call_write_1", "save_workspace_file", `{"path":"ai-test.txt","content":"after","mode":"overwrite"}`)
		case 2:
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
	if result.Content != "after" {
		t.Fatalf("unexpected file content: %q", result.Content)
	}
	if !strings.Contains(reply.Content, "ai-test.txt") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	assertToolCallRecord(t, env, "save_workspace_file", "success")
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
		switch requestCount.Add(1) {
		case 1:
			writeOpenAIToolCall(t, w, "call_edit_1", "edit_workspace_text", `{"path":"ai-test.txt","old_text":"before","new_text":"after"}`)
		case 2:
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
			assertOpenAIWriteToolsPresent(t, payload)
			writeOpenAIToolCall(t, w, "call_mkdir_1", "manage_workspace_path", `{"action":"mkdir","path":"ai-folder"}`)
		case 2:
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
			assertOpenAIWriteToolsPresent(t, payload)
			writeOpenAIToolCall(t, w, "call_read_1", "read_workspace_text", `{"path":"delete-me.txt"}`)
		case 2:
			writeOpenAIToolCall(t, w, "call_delete_1", "manage_workspace_path", `{"action":"trash","path":"delete-me.txt"}`)
		case 3:
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
	if requestCount.Load() != 3 {
		t.Fatalf("expected 3 model requests, got %d", requestCount.Load())
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
			assertOpenAIWriteToolsPresent(t, payload)
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
			assertLastUserReminderContains(t, payload, "没有继续调用工具")
			writeOpenAIToolCall(t, w, "call_read_1", "read_workspace_text", `{"path":"draft.txt"}`)
		case 3:
			writeOpenAIToolCall(t, w, "call_done_1", "attempt_completion", `{"result":"已读取 draft.txt 并确认后续步骤。","notes":"尚未写入文件。"} `)
		default:
			t.Fatalf("unexpected request count: %d", count)
		}
	}))
	defer server.Close()

	provider, err := env.configs.CreateModelProvider(ctx, testUserID, store.ModelProviderInput{
		Name:       "Test OpenAI",
		Visibility: store.ResourceVisibilityPublic,
		Protocol:   "openai",
		BaseURL:    server.URL,
		APIKey:     "test-key",
		ModelID:    "gpt-test",
		Enabled:    true,
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

func TestGenerateReplyEmitsMultipleAssistantSegments(t *testing.T) {
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
			writeOpenAIToolCall(t, w, "done", "attempt_completion", `{"result":"已创建 novel-project 并写入 chapter-1.txt。","changed_paths":["novel-project","novel-project/chapter-1.txt"]}`)
		default:
			t.Fatalf("unexpected request count: %d", count)
		}
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "创建一个文件夹并写入第一章")
	var currentSegment strings.Builder
	var assistantBodies []string
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
	if len(assistantBodies) != 3 {
		t.Fatalf("expected 3 assistant segments, got %#v", assistantBodies)
	}
	if !strings.Contains(strings.Join(assistantBodies, "\n"), "我先创建文件夹") || !strings.Contains(strings.Join(assistantBodies, "\n"), "我现在写第一章内容") {
		t.Fatalf("expected assistant content to include both stages, got %#v", assistantBodies)
	}
	if !strings.Contains(assistantBodies[2], "chapter-1.txt") {
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
			writeOpenAIToolCall(t, w, "bad_args", "save_workspace_file", `{"path":"bad.txt","content":"hello","mode":`)
		case 2:
			lastToolContent := lastOpenAIToolContent(t, payload)
			if !strings.Contains(lastToolContent, "tool arguments must be valid JSON") {
				t.Fatalf("expected invalid JSON tool_result feedback, got %s", lastToolContent)
			}
			writeOpenAIToolCall(t, w, "good_args", "save_workspace_file", `{"path":"bad.txt","content":"hello","mode":"create"}`)
		case 3:
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
			writeOpenAIToolCall(t, w, "dup_1", "save_workspace_file", `{"path":"dedupe.txt","content":"hello","mode":"create"}`)
		case 2:
			writeOpenAIToolCall(t, w, "dup_2", "save_workspace_file", `{"path":"dedupe.txt","content":"hello","mode":"create"}`)
		case 3:
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
		count := requestCount.Add(1)
		if count == 1 {
			writeOpenAIToolCall(t, w, "write_1", "save_workspace_file", `{"path":"verified-fallback.txt","content":"done","mode":"create"}`)
			return
		}
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
	if requestCount.Load() != 4 {
		t.Fatalf("expected one tool round plus three no-tool rounds, got %d", requestCount.Load())
	}
}

func TestGenerateReplyOpenAIFallsBackToNonStreamWhenFinalStreamBreaks(t *testing.T) {
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
		t.Fatalf("expected two retries for HTTP 502, got %d requests", requestCount.Load())
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

func TestGenerateReplyRetriesHTTP502OnceThenSucceeds(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		if count == 1 {
			w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("errorcode:502"))
			return
		}
		writeJSON(t, w, map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": "重试后成功。",
					},
				},
			},
		})
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "hello")
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if requestCount.Load() != 2 {
		t.Fatalf("expected one retry for HTTP 502, got %d requests", requestCount.Load())
	}
	if !strings.Contains(reply.Content, "重试后成功") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	logs, err := env.diagnostics.List(ctx, store.DiagnosticLogQuery{UserID: testUserID, Limit: 10})
	if err != nil {
		t.Fatalf("diagnostics.List returned error: %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("expected no diagnostic log after retry success, got %d", len(logs))
	}
}

func TestGenerateReplyRetriesHTTP502TwiceThenSucceeds(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		if count <= 2 {
			w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("errorcode:502"))
			return
		}
		writeJSON(t, w, map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": "第二次重试后成功。",
					},
				},
			},
		})
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "hello")
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if requestCount.Load() != 3 {
		t.Fatalf("expected two retries for HTTP 502, got %d requests", requestCount.Load())
	}
	if !strings.Contains(reply.Content, "第二次重试后成功") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	logs, err := env.diagnostics.List(ctx, store.DiagnosticLogQuery{UserID: testUserID, Limit: 10})
	if err != nil {
		t.Fatalf("diagnostics.List returned error: %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("expected no diagnostic log after retry success, got %d", len(logs))
	}
}

func TestGenerateReplyRetriesHTTP429ThenSucceeds(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		if count == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
			return
		}
		writeJSON(t, w, map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": "限流重试后成功。",
					},
				},
			},
		})
	}))
	defer server.Close()

	provider, conversation, userMessage, messages := prepareOpenAIConversation(t, ctx, env, server.URL, "hello")
	reply, err := env.service.GenerateReply(ctx, conversation.ID, userMessage.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if requestCount.Load() != 2 {
		t.Fatalf("expected one retry for HTTP 429, got %d requests", requestCount.Load())
	}
	if !strings.Contains(reply.Content, "限流重试后成功") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
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
		body := string(raw)
		count := requestCount.Add(1)
		if strings.Contains(body, "上一轮模型供应商连接中断") {
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
			writeOpenAIToolCall(t, w, "read_before_failure", "read_workspace_text", `{"path":"draft.txt"}`)
		case 2:
			if payload["stream"] != true {
				t.Fatalf("expected second request to stream before fallback, got %#v", payload["stream"])
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
		case 3:
			if payload["stream"] != false {
				t.Fatalf("expected non-stream fallback before checkpoint recovery, got %#v", payload["stream"])
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(529)
			_, _ = w.Write([]byte(`{"error":{"message":"fallback overloaded"}}`))
		case 4, 5:
			if payload["stream"] != false {
				t.Fatalf("expected SDK retry to stay on non-stream fallback, got %#v", payload["stream"])
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(529)
			_, _ = w.Write([]byte(`{"error":{"message":"fallback overloaded"}}`))
		case 6:
			if payload["stream"] != true {
				t.Fatalf("expected checkpoint resume to rebuild a streaming provider turn, got %#v", payload["stream"])
			}
			if !strings.Contains(string(raw), "上一轮模型供应商连接中断") {
				t.Fatalf("expected checkpoint recovery reminder in request %d: %s", requestCount.Load(), string(raw))
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
		t.Fatalf("expected non-stream fallback plus SDK retries before checkpoint resume, got %d fallback requests", fallbackRequests.Load())
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
			writeOpenAIToolCall(t, w, "call_read_1", "read_workspace_text", `{"path":"draft.txt"}`)
		case 2:
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
							"role":    "assistant",
							"content": "非流式恢复成功。",
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
		t.Fatalf("expected stream request plus non-stream fallback, got %d", requestCount.Load())
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
					{"type": "text", "text": "非流式恢复成功。"},
				},
				"stop_reason": "end_turn",
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

func TestGenerateReplyAnthropicFallsBackToNonStreamWhenFinalStreamBreaks(t *testing.T) {
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
		fmt.Fprint(w, "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"max_tokens\"},\"usage\":{\"output_tokens\":8192}}\n\n")
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
	if summary.StreamUsage.OutputTokens != 8192 {
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
	if call.ArgumentsJSONValid || call.ArgumentsJSONError == "" || !call.LooksLikeLongWrite || call.RecommendedTool != "write_workspace_chunk" {
		t.Fatalf("expected invalid long-write details, got %#v", call)
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
		if payload["tool_choice"] != "required" {
			t.Fatalf("expected required tool_choice for explicit workspace-file tool loop, got %#v", payload["tool_choice"])
		}
		names := openAIToolNamesFromPayload(t, payload)
		if !names["attempt_completion"] || !names["ask_followup_question"] {
			t.Fatalf("expected workflow tools in payload, got %#v", names)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"tool_choice required is not supported by this provider"}}`))
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
		"request_kind":  "chat_completions",
		"stream":        true,
		"tool_choice":   "required",
		"requires_tool": true,
		"failure_class": "provider_rejected_required_tool_choice",
	}
	for key, value := range expected {
		if summary[key] != value {
			t.Fatalf("expected summary[%s]=%#v, got %#v in %#v", key, value, summary[key], summary)
		}
	}
	if got, _ := summary["tools_count"].(float64); got < 2 {
		t.Fatalf("expected tools_count >= 2, got %#v in %#v", summary["tools_count"], summary)
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
		if payload["tool_choice"] != "required" {
			t.Fatalf("expected required tool_choice, got %#v", payload["tool_choice"])
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
	if !summaryContainsUnsupportedPath(summary, "openai_schema_default:perseus_search_web.parameters.max_results.default") {
		t.Fatalf("expected concrete OpenAI schema warning for perseus_search_web default, got %#v", summary)
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
		Name:       "Test OpenAI",
		Visibility: store.ResourceVisibilityPublic,
		Protocol:   "openai",
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

func assertOpenAIWriteToolsPresent(t *testing.T, payload map[string]any) {
	t.Helper()

	if payload["parallel_tool_calls"] != false {
		t.Fatalf("expected parallel_tool_calls=false for stable serial tool execution, got %#v", payload["parallel_tool_calls"])
	}

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

	mutationTools := []string{
		"save_workspace_file",
		"write_workspace_chunk",
		"manage_workspace_path",
		"organize_workspace_matches",
	}
	hasMutationTool := false
	for _, name := range mutationTools {
		if names[name] {
			hasMutationTool = true
			break
		}
	}
	if !hasMutationTool {
		t.Fatalf("expected at least one mutation tool in payload: %#v", names)
	}

	if len(names) == 1 {
		return
	}

	for _, expected := range []string{"inspect_workspace", "read_workspace_text"} {
		if !names[expected] {
			t.Fatalf("tool %q missing from payload: %#v", expected, names)
		}
	}
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
		"工具工作完成后，可以用普通 assistant 正文交付最终结果",
		"find_workspace_files 按名称/路径/glob 找文件",
		"save_workspace_file 创建或整文件覆盖短文本文件",
		"write_workspace_chunk 分块创建/覆盖/追加长文本文件",
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
	for _, expected := range []string{"attempt_completion", "save_workspace_file", "manage_workspace_path", "run_workspace_command"} {
		if !names[expected] {
			t.Fatalf("expected %q in agent tool payload, got %#v", expected, names)
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
