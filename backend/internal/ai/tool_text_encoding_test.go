package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadObservationsKeepLiteralMarkupThroughModelViews(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	source := "# Guide\n\n-\n\nArchitecture & Configuration\n\n<span>`sample`</span>\nLiteral escape: \\u0026\n"
	if _, err := env.files.CreateTextFile(ctx, "guide.md", source, "test"); err != nil {
		t.Fatal(err)
	}
	profile := providerRuntimeProfile{MaxToolResultRunes: 10000, MaxReadFileContentRunes: 5000}
	for _, tool := range []string{"read_workspace_text", "read_workspace_files"} {
		var result string
		var err error
		if tool == "read_workspace_text" {
			result, _, _, _, err = env.service.execReadWorkspaceFile(ctx, testUserID, `{"path":"guide.md"}`)
		} else {
			result, _, _, _, err = env.service.execReadWorkspaceFiles(ctx, testUserID, `{"paths":["guide.md"]}`)
		}
		if err != nil {
			t.Fatal(err)
		}
		budgeted := budgetToolResultForModel(tool, result, profile)
		observation := providerToolObservationContent(TranscriptToolResult{ToolName: tool, Content: budgeted})
		for _, view := range []string{result, budgeted, observation} {
			if !strings.Contains(view, "Architecture & Configuration") || !strings.Contains(view, "<span>") {
				t.Fatalf("%s read view makes source markup ambiguous: %s", tool, view)
			}
			// The outer provider request must decode back to the same tool text;
			// literal file escape sequences must never be globally unescaped.
			wire, _ := json.Marshal(map[string]string{"content": view})
			var received map[string]string
			if err := json.Unmarshal(wire, &received); err != nil || received["content"] != view {
				t.Fatal("provider envelope changed tool content")
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(view), &payload); err != nil {
				t.Fatal(err)
			}
			if tool == "read_workspace_files" {
				payload = payload["items"].([]any)[0].(map[string]any)
			}
			text := firstNonEmpty(stringFromAny(payload["content"]), stringFromAny(payload["numbered_content"]))
			if !strings.Contains(text, `Literal escape: \u0026`) {
				t.Fatal("literal source escape was decoded")
			}
		}
	}
}

func TestPatchFailureMessageIdentifiesContentMismatch(t *testing.T) {
	r := &agentRuntime{}
	for i := 0; i < 3; i++ {
		r.recordToolFailure("apply_workspace_patch", `{"patch":"rejected"}`, workspacePatchApplyError{Message: "patch apply failed: update hunk 1 did not match"})
	}
	message := r.repeatedToolErrorMessage()
	if !strings.Contains(message, "原文不匹配") || strings.Contains(message, "供应商") {
		t.Fatalf("misleading error: %s", message)
	}
}

func TestLegacyCheckpointReadObservationIsCanonicalized(t *testing.T) {
	for _, tool := range []string{"read_workspace_text", "read_workspace_files"} {
		payload := map[string]any{"numbered_content": "1: Architecture & Configuration <span>\n2: literal \\u0026"}
		if tool == "read_workspace_files" {
			payload = map[string]any{"items": []any{payload}}
		}
		legacy, _ := json.Marshal(payload)
		view := providerToolObservationContent(TranscriptToolResult{ToolName: tool, Content: string(legacy)})
		if !strings.Contains(view, "Architecture & Configuration <span>") {
			t.Fatal("old checkpoint retained ambiguous source encoding")
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(view), &decoded); err != nil {
			t.Fatal(err)
		}
		if tool == "read_workspace_files" {
			decoded = decoded["items"].([]any)[0].(map[string]any)
		}
		if !strings.Contains(stringFromAny(decoded["numbered_content"]), `literal \u0026`) {
			t.Fatal("literal checkpoint escapes changed")
		}
	}
}

func TestMarkdownPatchFailureRemainsAtomicAndCorrectedPatchSucceeds(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	source := "# Guide\n\n-\n\n文档类型：**Architecture & Configuration\n"
	if _, err := env.files.CreateTextFile(ctx, "guide.md", source, "test"); err != nil {
		t.Fatal(err)
	}
	bad := "*** Begin Patch\n*** Update File: guide.md\n@@\n # Guide\n \n-\n-文档类型：**Architecture \\u0026 Configuration\n+文档类型：**Architecture & Configuration**\n*** End Patch"
	result, _, _, _, err := env.service.execApplyWorkspacePatch(ctx, testUserID, mustJSON(map[string]any{"patch": bad}))
	if err == nil {
		t.Fatal("invalid patch changed file")
	}
	file, _ := env.files.ReadFile("guide.md")
	if file.Content != source {
		t.Fatal("failed patch changed original")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatal(err)
	}
	hint := stringFromAny(payload["recovery_hint"])
	if !strings.Contains(hint, "--") || !strings.Contains(hint, "JSON") {
		t.Fatalf("missing actionable recovery: %s", hint)
	}
	good := "*** Begin Patch\n*** Update File: guide.md\n@@\n # Guide\n \n--\n \n-文档类型：**Architecture & Configuration\n+文档类型：**Architecture & Configuration**\n*** End Patch"
	if _, err := applyWorkspacePatch(ctx, env.files, good); err != nil {
		t.Fatal(err)
	}
	file, _ = env.files.ReadFile("guide.md")
	if file.Content != "# Guide\n\n\n文档类型：**Architecture & Configuration**\n" {
		t.Fatalf("unexpected corrected document: %q", file.Content)
	}
}

func TestProviderRecoversMarkdownPatchAfterFreshRead(t *testing.T) {
	for _, protocol := range []agentProtocol{agentProtocolOpenAI, agentProtocolAnthropic} {
		t.Run(string(protocol), func(t *testing.T) {
			env := newTestAIEnv(t)
			ctx := context.Background()
			source := "# Guide\n\n-\n\nArchitecture & Configuration\n"
			if _, err := env.files.CreateTextFile(ctx, "guide.md", source, "test"); err != nil {
				t.Fatal(err)
			}
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				defer req.Body.Close()
				var payload map[string]any
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				requests++
				name := "read_workspace_text"
				args := map[string]any{"path": "guide.md"}
				// Examine the actual adapter message after its outer JSON envelope
				// has been decoded, rather than only testing a serialization helper.
				if requests == 2 || requests == 4 {
					found := false
					var inspect func(any)
					inspect = func(value any) {
						switch v := value.(type) {
						case string:
							if strings.Contains(v, `"numbered_content"`) && strings.Contains(v, "Architecture & Configuration") {
								found = true
							}
						case map[string]any:
							for _, child := range v {
								inspect(child)
							}
						case []any:
							for _, child := range v {
								inspect(child)
							}
						}
					}
					inspect(payload["messages"])
					if !found {
						t.Error("adapter escaped source ampersands in the actual model message")
					}
				}
				switch requests {
				case 2:
					name = "apply_workspace_patch"
					args = map[string]any{"patch": "*** Begin Patch\n*** Update File: guide.md\n@@\n # Guide\n \n-\n-Architecture \\u0026 Configuration\n+Architecture & Configuration fixed\n*** End Patch"}
				case 3:
					file, _ := env.files.ReadFile("guide.md")
					if file.Content != source {
						t.Error("failed patch changed original")
					}
					args = map[string]any{"path": "guide.md", "start_line": 1, "end_line": 5}
				case 4:
					name = "apply_workspace_patch"
					args = map[string]any{"patch": "*** Begin Patch\n*** Update File: guide.md\n@@\n # Guide\n \n--\n \n-Architecture & Configuration\n+Architecture & Configuration fixed\n*** End Patch"}
				case 5:
					name = "attempt_completion"
					args = map[string]any{"result": "已修复 guide.md", "changed_paths": []string{"guide.md"}}
				default:
					if requests > 5 {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
				}
				if protocol == agentProtocolOpenAI {
					writeOpenAIToolCall(t, w, "call_"+name, name, mustJSON(args))
				} else {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{"id": "msg_test", "type": "message", "role": "assistant", "stop_reason": "tool_use", "content": []map[string]any{{"type": "tool_use", "id": "call_" + name, "name": name, "input": args}}})
				}
			}))
			defer server.Close()
			prepare := prepareOpenAIConversation
			if protocol == agentProtocolAnthropic {
				prepare = prepareAnthropicConversation
			}
			provider, conv, message, messages := prepare(t, ctx, env, server.URL, "修复 guide.md 格式")
			_, err := env.service.GenerateReply(ctx, conv.ID, message.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
			if err != nil {
				t.Fatal(err)
			}
			file, _ := env.files.ReadFile("guide.md")
			if file.Content != "# Guide\n\n\nArchitecture & Configuration fixed\n" || requests != 5 {
				t.Fatalf("recovery did not finish: %d requests, %q", requests, file.Content)
			}
		})
	}
}
