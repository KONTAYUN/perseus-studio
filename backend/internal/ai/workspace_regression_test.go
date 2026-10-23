package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"webide-backend/internal/store"
)

func TestDefaultProvidersExposeAndAcceptPatchTool(t *testing.T) {
	for _, protocol := range []agentProtocol{agentProtocolOpenAI, agentProtocolAnthropic} {
		t.Run(string(protocol), func(t *testing.T) {
			r := &agentRuntime{protocol: protocol, provider: store.ModelProvider{Protocol: string(protocol), BaseURL: "https://compatible.example/v1"}, loadedToolsets: map[string]bool{"workspace": true}}
			if schema := toolInputSchema("apply_workspace_patch", r.tools()); len(schema) == 0 {
				t.Fatal("default provider has no callable file content editing tool")
			}
			call := agentToolCall{Name: "apply_workspace_patch", ArgumentsJSON: `{"patch":"*** Begin Patch\n*** Add File: draft.md\n+hello\n*** End Patch"}`}
			if errText := r.normalizeAndValidateToolCall(&call); errText != "" {
				t.Fatalf("default provider rejects its patch tool: %s", errText)
			}
		})
	}
}

func TestBackgroundTaskCanUseOptionalToolsets(t *testing.T) {
	t.Setenv("PERSEUS_COMMAND_EXECUTION", "trusted_host")
	for _, protocol := range []agentProtocol{agentProtocolOpenAI, agentProtocolAnthropic} {
		r := &agentRuntime{protocol: protocol, opts: RunOptions{TaskID: "task-1", UserRole: "admin"}, loadedToolsets: initialAgentToolsets(RunOptions{TaskID: "task-1"}, nil)}
		for _, name := range []string{"run_workspace_command", "perseus_search_web", "read_skill"} {
			if !r.isKnownTool(name) {
				t.Errorf("%s task cannot use %s and has no loader", protocol, name)
			}
		}
		if r.isKnownTool("start_background_task") {
			t.Fatal("background tasks must not recursively spawn tasks")
		}
	}
}

func TestDefaultProviderEditsExistingFileThroughJSONPatch(t *testing.T) {
	for _, protocol := range []agentProtocol{agentProtocolOpenAI, agentProtocolAnthropic} {
		t.Run(string(protocol), func(t *testing.T) {
			env := newTestAIEnv(t)
			ctx := context.Background()
			if _, err := env.files.CreateTextFile(ctx, "draft.md", "before\n", "test"); err != nil {
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
				args := map[string]any{"path": "draft.md"}
				switch requests {
				case 2:
					name = "apply_workspace_patch"
					args = map[string]any{"patch": "*** Begin Patch\n*** Update File: draft.md\n@@\n-before\n+after\n*** End Patch"}
				case 3:
					name = "attempt_completion"
					args = map[string]any{"result": "已修改 draft.md。", "changed_paths": []string{"draft.md"}}
				default:
					if requests > 3 {
						t.Errorf("edit failed to finish after read, patch and completion: request %d", requests)
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
			provider, conv, message, messages := prepare(t, ctx, env, server.URL, "把 draft.md 的 before 改为 after")
			reply, err := env.service.GenerateReply(ctx, conv.ID, message.ID, provider.ID, messages, nil, RunOptions{UserID: testUserID})
			if err != nil {
				t.Fatal(err)
			}
			file, err := env.files.ReadFile("draft.md")
			if err != nil || file.Content != "after\n" || !strings.Contains(reply.Content, "draft.md") {
				t.Fatalf("edit did not reach disk: file=%#v reply=%#v err=%v", file, reply, err)
			}
		})
	}
}

func TestPatchCanFillAnEmptyExistingFile(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	if _, err := env.files.CreateTextFile(ctx, "empty.md", "", "test"); err != nil {
		t.Fatal(err)
	}
	_, err := applyWorkspacePatch(ctx, env.files, "*** Begin Patch\n*** Update File: empty.md\n@@\n+hello\n*** End Patch")
	if err != nil {
		t.Fatalf("cannot edit an empty existing file: %v", err)
	}
	file, err := env.files.ReadFile("empty.md")
	if err != nil || file.Content != "hello\n" {
		t.Fatalf("unexpected content %q: %v", file.Content, err)
	}
}

func TestPatchAcceptsBlankSeparatorsWithoutChangingFileWhitespace(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	if _, err := env.files.CreateFile(ctx, "draft.md", "# Test\nstatus: pending\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := env.files.CreateFile(ctx, "empty.md", ""); err != nil {
		t.Fatal(err)
	}
	patch := "*** Begin Patch\n*** Update File: draft.md\n@@\n # Test\n-status: pending\n+status: done\n\n*** Update File: empty.md\n@@\n+EMPTY_FILE_FIXED\n\n*** Add File: result.md\n+LIVE_EDIT_OK\n+\n+last line\n\n*** End Patch"
	if _, err := applyWorkspacePatch(ctx, env.files, patch); err != nil {
		t.Fatal(err)
	}
	for path, expected := range map[string]string{"draft.md": "# Test\nstatus: done\n", "empty.md": "EMPTY_FILE_FIXED\n", "result.md": "LIVE_EDIT_OK\n\nlast line\n"} {
		file, err := env.files.ReadFile(path)
		if err != nil || file.Content != expected {
			t.Fatalf("%s: content=%q expected=%q err=%v", path, file.Content, expected, err)
		}
	}
	if _, err := parseWorkspacePatch("*** Begin Patch\n*** Update File: draft.md\n@@\n # Test\n\n-status: pending\n+status: done\n*** End Patch"); err == nil {
		t.Fatal("accepted ambiguous unmarked blank inside a hunk")
	}
}

func TestPatchRejectsDuplicatePathAliasesBeforeWriting(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	if _, err := env.files.CreateFile(ctx, "draft.md", "before\n"); err != nil {
		t.Fatal(err)
	}
	patch := "*** Begin Patch\n*** Update File: draft.md\n@@\n-before\n+first\n*** Update File: ./draft.md\n@@\n-before\n+second\n*** End Patch"
	if _, err := applyWorkspacePatch(ctx, env.files, patch); err == nil {
		t.Fatal("accepted duplicate path aliases")
	}
	file, err := env.files.ReadFile("draft.md")
	if err != nil {
		t.Fatal(err)
	}
	if file.Content != "before\n" {
		t.Fatalf("partially applied duplicate patch: %q", file.Content)
	}
}

func TestSelectedFilePatchScopeAllowsOutputsAndRename(t *testing.T) {
	r := &agentRuntime{attachments: []AttachmentPayload{{Kind: "workspace_file", Path: "selected.md"}}}
	for _, patch := range []string{
		"*** Begin Patch\n*** Add File: summary.md\n+summary\n*** End Patch",
		"*** Begin Patch\n*** Update File: selected.md\n*** Move to: renamed.md\n@@\n-old\n+new\n*** End Patch",
	} {
		call := textPatchToolCall(patch)
		if errText := r.validateWorkspacePathScope(call); errText != "" {
			t.Errorf("valid output/rename blocked by selected-file scope: %s", errText)
		}
	}
	r.opts.AllowedWorkspacePaths = []string{"selected.md", "summary.md", "renamed.md"}
	if errText := r.validateWorkspacePathScope(agentToolCall{Name: "read_workspace_text", ArgumentsJSON: `{"path":"renamed.md"}`}); errText != "" {
		t.Fatalf("cannot verify renamed output: %s", errText)
	}
	if errText := r.validateWorkspacePathScope(textPatchToolCall("*** Begin Patch\n*** Update File: unselected.md\n@@\n-old\n+new\n*** End Patch")); errText == "" {
		t.Fatal("unselected existing files must remain protected")
	}
}

func TestPatchPublishesRenameAndTrashDestinations(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	for _, path := range []string{"move.md", "delete.md"} {
		if _, err := env.files.CreateTextFile(ctx, path, "old\n", "test"); err != nil {
			t.Fatal(err)
		}
	}
	result, err := applyWorkspacePatch(ctx, env.files, "*** Begin Patch\n*** Update File: move.md\n*** Move to: renamed.md\n@@\n-old\n+new\n*** Delete File: delete.md\n*** End Patch")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Moved) != 2 || result.Moved[0].NewPath != "renamed.md" || !strings.HasPrefix(result.Moved[1].NewPath, "回收站/") {
		t.Fatalf("patch lost rename/delete metadata: %#v", result.Moved)
	}
}
