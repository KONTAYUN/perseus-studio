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
			writeOpenAIToolCall(t, w, "call_done_1", "attempt_completion", `{"summary":"已创建 ai-test.txt。","changed_paths":["ai-test.txt"]}`)
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
			writeOpenAIToolCall(t, w, "call_done_1", "attempt_completion", `{"summary":"已修改 ai-test.txt。","changed_paths":["ai-test.txt"]}`)
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

func TestGenerateReplyOpenAIWriteSucceedsWhenFinalReplyTimesOut(t *testing.T) {
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
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if !strings.Contains(reply.Content, "notes/test.md") || !strings.Contains(reply.Content, "工具验证结果收尾") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	if len(reply.ToolEvents) != 1 || reply.ToolEvents[0].ToolName != "save_workspace_file" || reply.ToolEvents[0].Status != "success" {
		t.Fatalf("unexpected tool events: %#v", reply.ToolEvents)
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

func TestGenerateReplyOpenAIReadThenTimeoutFallsBackToExplicitVersionReplacement(t *testing.T) {
	env := newTestAIEnv(t)
	env.service.client.Timeout = 20 * time.Millisecond
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
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if !strings.Contains(reply.Content, "已根据你的明确替换指令") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
	}
	readResult, err := env.files.ReadFile("docs/book.md")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if readResult.Content != "白皮书版本：14.0.7\n" {
		t.Fatalf("unexpected file content: %q", readResult.Content)
	}
	assertToolCallRecord(t, env, "read_workspace_text", "success")
	assertToolCallRecord(t, env, "save_workspace_file", "success")
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
			writeOpenAIToolCall(t, w, "call_done_1", "attempt_completion", `{"summary":"已创建 ai-folder。","changed_paths":["ai-folder"]}`)
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
			writeOpenAIToolCall(t, w, "call_done_1", "attempt_completion", `{"summary":"已将 delete-me.txt 移入回收站。","changed_paths":["回收站/delete-me.txt"]}`)
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
			writeOpenAIToolCall(t, w, "call_done_1", "attempt_completion", `{"summary":"已读取 draft.txt 并确认后续步骤。","notes":"尚未写入文件。"} `)
		default:
			t.Fatalf("unexpected request count: %d", count)
		}
	}))
	defer server.Close()

	provider, err := env.configs.CreateModelProvider(ctx, testUserID, store.ModelProviderInput{
		Name:     "Test OpenAI",
		Protocol: "openai",
		BaseURL:  server.URL,
		APIKey:   "test-key",
		ModelID:  "gpt-test",
		Enabled:  true,
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
			writeOpenAIToolCall(t, w, "done", "attempt_completion", `{"summary":"已创建 novel-project 并写入 chapter-1.txt。","changed_paths":["novel-project","novel-project/chapter-1.txt"]}`)
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
			writeOpenAIToolCall(t, w, "done", "attempt_completion", `{"summary":"已创建 bad.txt。","changed_paths":["bad.txt"]}`)
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
			writeOpenAIToolCall(t, w, "done", "attempt_completion", `{"summary":"已创建 dedupe.txt。","changed_paths":["dedupe.txt"]}`)
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

func TestGenerateReplyOpenAIFinishesFromVerifiedMutationWhenModelNeverCompletes(t *testing.T) {
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
	if err != nil {
		t.Fatalf("GenerateReply returned error: %v", err)
	}
	if !strings.Contains(reply.Content, "verified-fallback.txt") || !strings.Contains(reply.Content, "已完成并验证") {
		t.Fatalf("unexpected reply content: %q", reply.Content)
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

func prepareOpenAIConversation(t *testing.T, ctx context.Context, env *testAIEnv, baseURL, prompt string) (store.ModelProvider, store.Conversation, store.Message, []store.Message) {
	t.Helper()

	provider, err := env.configs.CreateModelProvider(ctx, testUserID, store.ModelProviderInput{
		Name:      "Test OpenAI",
		Protocol:  "openai",
		BaseURL:   baseURL,
		APIKey:    "test-key",
		ModelID:   "test-model",
		Enabled:   true,
		IsDefault: true,
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
		"最后调用 attempt_completion",
		"save_workspace_file 创建或覆盖文本文件",
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
