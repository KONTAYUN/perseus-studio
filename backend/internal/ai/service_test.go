package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"webide-backend/internal/store"
	"webide-backend/internal/usersvc"
	"webide-backend/internal/workspace"
)

type testAIEnv struct {
	service       *Service
	factory       *usersvc.Factory
	db            *sql.DB
	root          string
	configs       *store.ConfigStore
	conversations *store.ConversationStore
	diagnostics   *store.DiagnosticStore
	runs          *store.AgentRunStore
	files         *workspace.Service
}

func TestWorkspaceWriteToolsRegistered(t *testing.T) {
	assertToolRegistered(t, openAITools(), "load_perseus_toolset")
	assertToolNotRegistered(t, openAITools(), "create_workspace_file")
	assertToolNotRegistered(t, openAITools(), "perseus_search_web")
	assertToolRegistered(t, anthropicTools(), "load_perseus_toolset")
	assertToolNotRegistered(t, anthropicTools(), "create_workspace_file")
	assertToolNotRegistered(t, anthropicTools(), "perseus_search_web")

	loaded := map[string]bool{
		"workspace": true,
		"command":   true,
		"search":    true,
		"skills":    true,
		"task":      true,
	}
	for _, legacy := range []string{"list_workspace_files", "read_workspace_file", "create_workspace_file", "write_workspace_file", "mkdir_workspace", "rename_workspace_path", "move_workspace_path", "delete_workspace_path"} {
		assertToolNotRegistered(t, openAIToolsForLoadedToolsets(loaded, true), legacy)
		assertToolNotRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), legacy)
	}
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "inspect_workspace")
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "find_workspace_files")
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "search_workspace_text")
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "read_workspace_text")
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "read_workspace_files")
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "edit_workspace_text")
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "save_workspace_file")
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "manage_workspace_path")
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "organize_workspace_matches")
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "run_workspace_command")
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "perseus_search_web")
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "list_skills")
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "read_skill")
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "install_skill_from_attachment")
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "delete_skill")
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "ask_followup_question")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "inspect_workspace")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "find_workspace_files")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "search_workspace_text")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "read_workspace_text")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "read_workspace_files")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "edit_workspace_text")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "save_workspace_file")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "manage_workspace_path")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "organize_workspace_matches")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "run_workspace_command")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "perseus_search_web")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "list_skills")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "read_skill")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "install_skill_from_attachment")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "delete_skill")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "ask_followup_question")
}

func TestOrdinaryUserSkillToolSchemaIsReadOnly(t *testing.T) {
	loaded := map[string]bool{"skills": true}
	names := toolNamesFromOpenAITools(openAIToolsForLoadedToolsets(loaded, true, "user"))
	for _, name := range []string{"list_skills", "read_skill"} {
		if !names[name] {
			t.Fatalf("expected ordinary user skill schema to include %s, got %#v", name, names)
		}
	}
	for _, name := range []string{"install_skill_from_attachment", "delete_skill"} {
		if names[name] {
			t.Fatalf("ordinary user skill schema must not include %s: %#v", name, names)
		}
	}
	result, _, _, _, err := execLoadPerseusToolsetForRole(`{"toolset":"skills"}`, "user")
	if err != nil {
		t.Fatalf("execLoadPerseusToolsetForRole returned error: %v", err)
	}
	if strings.Contains(result, "install_skill_from_attachment") || strings.Contains(result, "delete_skill") {
		t.Fatalf("ordinary user load_perseus_toolset leaked admin skill tools: %s", result)
	}
}

func TestOrdinaryUserCanReadPublicSkillOwnedByAdmin(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	adminID := "admin-user-id"
	installPath := filepath.Join(env.root, "users", adminID, "skills", "public-skill")
	if err := os.MkdirAll(installPath, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(installPath, "SKILL.md"), []byte("# Public Skill\n\nShared instruction."), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	adminSkills, err := env.factory.Skills(adminID)
	if err != nil {
		t.Fatalf("factory skills returned error: %v", err)
	}
	enabled := true
	if _, err := adminSkills.Update(ctx, adminID, "public-skill", store.SkillInput{
		Visibility: store.ResourceVisibilityPublic,
		Enabled:    &enabled,
	}); err != nil {
		t.Fatalf("mark public returned error: %v", err)
	}
	if err := adminSkills.Sync(ctx, adminID); err != nil {
		t.Fatalf("resync public skill returned error: %v", err)
	}
	if item, err := adminSkills.Get(ctx, adminID, "public-skill"); err != nil {
		t.Fatalf("get public skill returned error: %v", err)
	} else if item.Visibility != store.ResourceVisibilityPublic || !item.Enabled {
		t.Fatalf("expected public skill to remain public after owner sync, got %#v", item)
	}
	if _, err := env.factory.Skills(testUserID); err != nil {
		t.Fatalf("factory skills for ordinary user returned error: %v", err)
	}
	result, _, _, _, err := env.service.executeTool(ctx, "read_skill", `{"skill_id":"public-skill"}`, RunOptions{
		UserID:   testUserID,
		UserRole: "user",
	})
	if err != nil {
		t.Fatalf("ordinary user read public skill returned error: %v", err)
	}
	if !strings.Contains(result, "Shared instruction.") {
		t.Fatalf("expected public skill content, got %s", result)
	}
}

func TestWorkspacePromptsDoNotExposeLegacyWorkspaceToolNames(t *testing.T) {
	legacyNames := []string{
		"list_workspace_files",
		"read_workspace_file",
		"create_workspace_file",
		"write_workspace_file",
		"mkdir_workspace",
		"rename_workspace_path",
		"move_workspace_path",
		"delete_workspace_path",
	}
	prompts := []string{
		workspaceToolSystemPrompt,
	}

	for _, prompt := range prompts {
		for _, legacyName := range legacyNames {
			pattern := regexp.MustCompile(`(^|[^A-Za-z0-9_])` + regexp.QuoteMeta(legacyName) + `([^A-Za-z0-9_]|$)`)
			if pattern.MatchString(prompt) {
				t.Fatalf("prompt exposed legacy workspace tool %q: %s", legacyName, prompt)
			}
		}
	}
}

func TestToolEventLabelsUseCompletedVoice(t *testing.T) {
	if label := userFriendlyToolLabel("read_workspace_text"); label != "已读取文件" {
		t.Fatalf("expected completed read label, got %q", label)
	}
	if label := userFriendlyToolLabel("read_workspace_files"); label != "已读取文件" {
		t.Fatalf("expected completed batch read label, got %q", label)
	}
	if label := userFriendlyToolLabel("save_workspace_file"); label != "已保存文件" {
		t.Fatalf("expected completed save label, got %q", label)
	}
	if label := userFriendlyToolLabel("ask_followup_question"); label != "已询问用户" {
		t.Fatalf("expected completed question label, got %q", label)
	}

	runtime := &agentRuntime{}
	_, event := runtime.alreadyAppliedToolResult(agentToolCall{ID: "call_1", Name: "save_workspace_file"}, ToolEvent{
		ToolName: "save_workspace_file",
		Status:   "success",
	})
	if event.Label != "已保存文件" {
		t.Fatalf("expected repeated mutation to use completed label, got %q", event.Label)
	}
}

func TestLedgerToolArgumentsJSONKeepsValidJSONForLargeArguments(t *testing.T) {
	raw := mustJSON(map[string]any{
		"path":    "白凌传·造化篇（续）/第三十集.txt",
		"mode":    "overwrite",
		"content": strings.Repeat("很长的正文", 800),
	})
	ledger := ledgerToolArgumentsJSON(raw)
	if len([]rune(ledger)) > 2000 {
		t.Fatalf("expected ledger arguments under 2000 chars, got %d", len([]rune(ledger)))
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(ledger), &decoded); err != nil {
		t.Fatalf("ledger arguments should remain valid JSON: %v\n%s", err, ledger)
	}
	if decoded["path"] != "白凌传·造化篇（续）/第三十集.txt" || decoded["mode"] != "overwrite" {
		t.Fatalf("expected path and mode preserved, got %#v", decoded)
	}
	if decoded["arguments_truncated_for_ledger"] != true {
		t.Fatalf("expected truncation marker, got %#v", decoded)
	}
	content, _ := decoded["content"].(string)
	if !strings.Contains(content, "truncated for ledger") {
		t.Fatalf("expected content to be summarized, got %q", content)
	}
}

func TestHandleFollowupQuestionCompletesRun(t *testing.T) {
	allowFreeform := false
	runtime := &agentRuntime{}
	result, isError, completed := runtime.handleFollowupQuestion("call_question_1", mustJSON(map[string]any{
		"question":       "这篇文章面向谁？",
		"options":        []string{"普通读者", "专业编辑", "普通读者", "  "},
		"allow_freeform": allowFreeform,
	}))
	if isError || !completed {
		t.Fatalf("expected follow-up question to complete successfully, isError=%v completed=%v result=%s", isError, completed, result)
	}
	if runtime.completedContent != "这篇文章面向谁？" {
		t.Fatalf("unexpected completed content: %q", runtime.completedContent)
	}
	if len(runtime.events) != 1 {
		t.Fatalf("expected one event, got %#v", runtime.events)
	}
	event := runtime.events[0]
	if event.ToolName != "ask_followup_question" || event.Status != "success" || event.Question != "这篇文章面向谁？" {
		t.Fatalf("unexpected question event: %#v", event)
	}
	if event.AllowFreeform == nil || *event.AllowFreeform {
		t.Fatalf("expected allow_freeform=false, got %#v", event.AllowFreeform)
	}
	if len(event.Options) != 2 || event.Options[0] != "普通读者" || event.Options[1] != "专业编辑" {
		t.Fatalf("unexpected options: %#v", event.Options)
	}

}

func TestHandleFollowupQuestionRejectsEmptyQuestion(t *testing.T) {
	runtime := &agentRuntime{}
	_, isError, completed := runtime.handleFollowupQuestion("call_question_1", `{"question":"   "}`)
	if !isError || completed {
		t.Fatalf("expected empty question to be rejected, isError=%v completed=%v", isError, completed)
	}
}

func TestHandleCompletionAttemptRequiresUserVisibleResult(t *testing.T) {
	runtime := &agentRuntime{}
	result, isError, completed := runtime.handleCompletionAttempt(`{"summary":"已整理完整步骤。"}`)
	if !isError || completed {
		t.Fatalf("expected summary-only completion to be rejected, isError=%v completed=%v result=%s", isError, completed, result)
	}
	if !strings.Contains(result, "result is required") {
		t.Fatalf("expected result requirement in tool error, got %s", result)
	}

	result, isError, completed = runtime.handleCompletionAttempt(`{"result":"1. 登录服务器\n2. 停止 Hermes 容器","summary":"清理步骤"}`)
	if isError || !completed {
		t.Fatalf("expected result completion to succeed, isError=%v completed=%v result=%s", isError, completed, result)
	}
	if runtime.completedContent != "1. 登录服务器\n2. 停止 Hermes 容器" {
		t.Fatalf("unexpected completed content: %q", runtime.completedContent)
	}
}

func TestHandleCompletionAttemptCanRecoverAfterCompletionArgumentError(t *testing.T) {
	runtime := &agentRuntime{}
	if _, isError, completed := runtime.handleCompletionAttempt(`{"summary":"已整理完整步骤。"}`); !isError || completed {
		t.Fatalf("expected first bad completion attempt to be rejected")
	}
	if _, isError, completed := runtime.handleCompletionAttempt(`{"result":"这是修正后的完整交付。"}`); isError || !completed {
		t.Fatalf("expected corrected completion attempt to succeed, isError=%v completed=%v", isError, completed)
	}
}

func TestHandleCompletionAttemptDiscardsStreamedPreviewWhenRejected(t *testing.T) {
	var discarded int
	runtime := &agentRuntime{
		performedMutation: true,
		opts: RunOptions{
			OnCompletionResultDiscard: func() {
				discarded++
			},
		},
	}
	_, isError, completed := runtime.handleCompletionAttempt(`{"result":"已完成。","changed_paths":[]}`)
	if !isError || completed {
		t.Fatalf("expected completion to be rejected, isError=%v completed=%v", isError, completed)
	}
	if discarded != 1 {
		t.Fatalf("expected streamed completion preview to be discarded once, got %d", discarded)
	}
}

func TestAttemptCompletionSchemaRequiresResult(t *testing.T) {
	params := completionParams()
	required, _ := params["required"].([]string)
	if len(required) != 1 || required[0] != "result" {
		t.Fatalf("attempt_completion must require result, got %#v", params["required"])
	}
	props, _ := params["properties"].(map[string]any)
	if _, ok := props["result"]; !ok {
		t.Fatalf("attempt_completion properties missing result: %#v", props)
	}
}

func TestAgentRecoveryDefaultsFollowMatureRetryPolicy(t *testing.T) {
	t.Setenv("PERSEUS_AGENT_MAX_CHECKPOINT_RESUMES", "")
	if got := agentMaxCheckpointResumes(); got != 2 {
		t.Fatalf("expected default checkpoint resume attempts to be 2, got %d", got)
	}
	t.Setenv("PERSEUS_AGENT_MAX_CHECKPOINT_RESUMES", "6")
	if got := agentMaxCheckpointResumes(); got != 6 {
		t.Fatalf("expected configured checkpoint resume attempts to be 6, got %d", got)
	}
	t.Setenv("PERSEUS_AGENT_MAX_CHECKPOINT_RESUMES", "99")
	if got := agentMaxCheckpointResumes(); got != 20 {
		t.Fatalf("expected checkpoint resume attempts to be capped at 20, got %d", got)
	}
}

func TestAgentRecoveryDelayBackoffCanBeDisabledForTests(t *testing.T) {
	t.Setenv("PERSEUS_AGENT_RECOVERY_BASE_DELAY_MS", "0")
	if delay := agentRecoveryDelay(3); delay != 0 {
		t.Fatalf("expected disabled recovery delay, got %s", delay)
	}
	t.Setenv("PERSEUS_AGENT_RECOVERY_BASE_DELAY_MS", "100")
	if delay := agentRecoveryDelay(3); delay != 400*time.Millisecond {
		t.Fatalf("expected exponential recovery delay, got %s", delay)
	}
}

func TestExecuteToolCreateWorkspaceFile(t *testing.T) {
	service, _, root, files := newTestAIService(t)

	result, _, _, path, err := service.executeTool(context.Background(), "save_workspace_file", `{"path":"notes/new.txt","content":"hello","mode":"create"}`, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("executeTool returned error: %v", err)
	}
	if path != "notes/new.txt" {
		t.Fatalf("unexpected path: %s", path)
	}

	readResult, err := files.ReadFile("notes/new.txt")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if readResult.Content != "hello" {
		t.Fatalf("unexpected file content: %q", readResult.Content)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("invalid result json: %v", err)
	}
	if payload["path"] != "notes/new.txt" {
		t.Fatalf("unexpected result path: %#v", payload["path"])
	}
	if _, err := files.ReadFile(filepath.ToSlash(filepath.Join("notes", "new.txt"))); err != nil {
		t.Fatalf("created file missing under root %s: %v", root, err)
	}
}

func TestExecuteToolWriteWorkspaceFile(t *testing.T) {
	service, db, _, files := newTestAIService(t)

	if _, err := files.CreateFile(context.Background(), "demo.txt", "before"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}

	result, _, _, path, err := service.executeTool(context.Background(), "save_workspace_file", `{"path":"demo.txt","content":"after","mode":"overwrite"}`, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("executeTool returned error: %v", err)
	}
	if path != "demo.txt" {
		t.Fatalf("unexpected path: %s", path)
	}

	readResult, err := files.ReadFile("demo.txt")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if readResult.Content != "after" {
		t.Fatalf("unexpected file content: %q", readResult.Content)
	}

	var versionCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM file_versions WHERE path = ?`, "demo.txt").Scan(&versionCount); err != nil {
		t.Fatalf("query file_versions returned error: %v", err)
	}
	if versionCount != 1 {
		t.Fatalf("expected 1 file version, got %d", versionCount)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("invalid result json: %v", err)
	}
	if payload["version_id"] == "" {
		t.Fatalf("expected version_id in result payload")
	}
}

func TestExecuteToolWriteWorkspaceFileMarkdown(t *testing.T) {
	service, _, _, files := newTestAIService(t)

	if _, err := files.CreateFile(context.Background(), "notes/test.md", "before"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}
	if _, err := files.CreateFile(context.Background(), "notes/readme.markdown", "hello markdown"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}

	if _, _, _, path, err := service.executeTool(context.Background(), "save_workspace_file", `{"path":"notes/test.md","content":"# Updated","mode":"overwrite"}`, RunOptions{UserID: testUserID}); err != nil {
		t.Fatalf("executeTool returned error: %v", err)
	} else if path != "notes/test.md" {
		t.Fatalf("unexpected path: %s", path)
	}

	readResult, err := files.ReadFile("notes/test.md")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if readResult.Content != "# Updated" {
		t.Fatalf("unexpected markdown content: %q", readResult.Content)
	}

	result, _, _, path, err := service.executeTool(context.Background(), "read_workspace_text", `{"path":"notes/readme.markdown"}`, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("read executeTool returned error: %v", err)
	}
	if path != "notes/readme.markdown" {
		t.Fatalf("unexpected read path: %s", path)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("invalid result json: %v", err)
	}
	if payload["content"] != "hello markdown" {
		t.Fatalf("unexpected markdown read payload: %#v", payload["content"])
	}

	batchResult, _, _, _, err := service.executeTool(context.Background(), "read_workspace_files", `{"paths":["notes/readme.markdown","notes/test.md","missing.md"],"max_chars_per_file":5000}`, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("batch read executeTool returned error: %v", err)
	}
	var batchPayload struct {
		Items       []struct{ Path, Content string }
		Failed      []struct{ Path, Error string }
		Count       int `json:"count"`
		FailedCount int `json:"failed_count"`
	}
	if err := json.Unmarshal([]byte(batchResult), &batchPayload); err != nil {
		t.Fatalf("invalid batch result json: %v", err)
	}
	if batchPayload.Count != 2 || batchPayload.FailedCount != 1 || len(batchPayload.Items) != 2 || len(batchPayload.Failed) != 1 {
		t.Fatalf("unexpected batch payload: %#v", batchPayload)
	}
}

func TestExecuteToolMkdirWorkspace(t *testing.T) {
	service, _, _, files := newTestAIService(t)

	result, _, _, path, err := service.executeTool(context.Background(), "manage_workspace_path", `{"action":"mkdir","path":"docs/guides"}`, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("executeTool returned error: %v", err)
	}
	if path != "docs/guides" {
		t.Fatalf("unexpected path: %s", path)
	}

	nodes, _, err := files.List("docs", 3, 50)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(nodes) != 1 || nodes[0].Path != "docs/guides" || nodes[0].Type != "folder" {
		t.Fatalf("unexpected folder list: %#v", nodes)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("invalid result json: %v", err)
	}
	if payload["path"] != "docs/guides" {
		t.Fatalf("unexpected result path: %#v", payload["path"])
	}
	if !containsPathValue(payload["changed_paths"], "docs/guides") {
		t.Fatalf("expected changed_paths to include docs/guides, got %#v", payload["changed_paths"])
	}
}

func TestExecuteToolRenameWorkspacePath(t *testing.T) {
	service, _, _, files := newTestAIService(t)

	if _, err := files.CreateFile(context.Background(), "docs/old.txt", "hello"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}

	result, _, _, path, err := service.executeTool(context.Background(), "manage_workspace_path", `{"action":"rename","path":"docs/old.txt","new_name":"new.txt"}`, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("executeTool returned error: %v", err)
	}
	if path != "docs/new.txt" {
		t.Fatalf("unexpected path: %s", path)
	}

	readResult, err := files.ReadFile("docs/new.txt")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if readResult.Content != "hello" {
		t.Fatalf("unexpected file content: %q", readResult.Content)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("invalid result json: %v", err)
	}
	if payload["old_path"] != "docs/old.txt" || payload["new_path"] != "docs/new.txt" {
		t.Fatalf("unexpected rename payload: %#v", payload)
	}
}

func TestExecuteToolMoveWorkspacePath(t *testing.T) {
	service, _, _, files := newTestAIService(t)

	if _, err := files.CreateFile(context.Background(), "docs/move.txt", "hello"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}
	if _, err := files.MkdirNew("archive"); err != nil {
		t.Fatalf("MkdirNew returned error: %v", err)
	}

	result, _, _, path, err := service.executeTool(context.Background(), "manage_workspace_path", `{"action":"move","path":"docs/move.txt","new_parent_path":"archive"}`, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("executeTool returned error: %v", err)
	}
	if path != "archive/move.txt" {
		t.Fatalf("unexpected path: %s", path)
	}

	readResult, err := files.ReadFile("archive/move.txt")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if readResult.Content != "hello" {
		t.Fatalf("unexpected file content: %q", readResult.Content)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("invalid result json: %v", err)
	}
	if payload["old_path"] != "docs/move.txt" || payload["new_path"] != "archive/move.txt" {
		t.Fatalf("unexpected move payload: %#v", payload)
	}
}

func TestExecuteToolOrganizeWorkspaceMatches(t *testing.T) {
	service, _, _, files := newTestAIService(t)

	if _, err := files.CreateFile(context.Background(), "episodes/show-29-a.txt", "a"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}
	if _, err := files.CreateFile(context.Background(), "episodes/show-29-b.txt", "b"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}
	if _, err := files.CreateFile(context.Background(), "episodes/show-30.txt", "c"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}

	result, summary, _, path, err := service.executeTool(context.Background(), "organize_workspace_matches", `{"source_path":"episodes","match_text":"29","target_folder":"episode-29"}`, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("executeTool returned error: %v", err)
	}
	if path != "episode-29" {
		t.Fatalf("unexpected path: %s", path)
	}
	if !strings.Contains(summary, "organized 2") {
		t.Fatalf("unexpected summary: %s", summary)
	}
	if _, err := files.ReadFile("episode-29/show-29-a.txt"); err != nil {
		t.Fatalf("expected organized file a: %v", err)
	}
	if _, err := files.ReadFile("episode-29/show-29-b.txt"); err != nil {
		t.Fatalf("expected organized file b: %v", err)
	}
	if _, err := files.ReadFile("episodes/show-29-a.txt"); err == nil {
		t.Fatalf("old matched path still exists")
	}
	if _, err := files.ReadFile("episodes/show-30.txt"); err != nil {
		t.Fatalf("non-matching file should remain: %v", err)
	}

	var payload struct {
		MovedCount  int `json:"moved_count"`
		FailedCount int `json:"failed_count"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("invalid result json: %v", err)
	}
	if payload.MovedCount != 2 || payload.FailedCount != 0 {
		t.Fatalf("unexpected organize payload: %#v", payload)
	}
}

func TestExecuteToolDeleteWorkspacePath(t *testing.T) {
	service, _, _, files := newTestAIService(t)

	if _, err := files.CreateFile(context.Background(), "docs/delete.txt", "trash me"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}

	result, _, _, path, err := service.executeTool(context.Background(), "manage_workspace_path", `{"action":"trash","path":"docs/delete.txt"}`, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("executeTool returned error: %v", err)
	}
	if path == "" || path == "docs/delete.txt" {
		t.Fatalf("unexpected delete path: %s", path)
	}

	items, err := files.TrashItems()
	if err != nil {
		t.Fatalf("TrashItems returned error: %v", err)
	}
	if !hasTrashItem(items, path) {
		t.Fatalf("expected deleted file in trash, got %#v", items)
	}
	if _, err := files.ReadFile("docs/delete.txt"); err == nil {
		t.Fatalf("expected original path to be moved away")
	}
	if _, err := files.RestoreTrashItem(path, false); err != nil {
		t.Fatalf("RestoreTrashItem returned error: %v", err)
	}
	readResult, err := files.ReadFile("docs/delete.txt")
	if err != nil {
		t.Fatalf("ReadFile restored path returned error: %v", err)
	}
	if readResult.Content != "trash me" {
		t.Fatalf("unexpected restored file content: %q", readResult.Content)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("invalid result json: %v", err)
	}
	if payload["old_path"] != "docs/delete.txt" {
		t.Fatalf("unexpected delete payload: %#v", payload)
	}
}

func TestExecuteToolRunWorkspaceCommand(t *testing.T) {
	service, _, _, _ := newTestAIService(t)

	result, _, _, path, err := service.executeTool(context.Background(), "run_workspace_command", `{"command":"go version","timeout_sec":30}`, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("executeTool returned error: %v", err)
	}
	if path != "" {
		t.Fatalf("expected root cwd path to be empty, got %q", path)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("invalid result json: %v", err)
	}
	if payload["command"] != "go version" || payload["timed_out"] != false {
		t.Fatalf("unexpected command payload: %#v", payload)
	}
	if int(payload["exit_code"].(float64)) != 0 {
		t.Fatalf("expected exit code 0, got %#v", payload["exit_code"])
	}
}

func TestExecuteToolRunWorkspaceCommandRejectsDangerousCommand(t *testing.T) {
	service, _, _, _ := newTestAIService(t)

	_, _, _, _, err := service.executeTool(context.Background(), "run_workspace_command", `{"command":"rm -rf /","timeout_sec":30}`, RunOptions{UserID: testUserID})
	if err == nil || !strings.Contains(err.Error(), "safety policy") {
		t.Fatalf("expected safety policy error, got %v", err)
	}
}

func TestExecuteToolRunWorkspaceCommandRejectsPathEscape(t *testing.T) {
	service, _, _, _ := newTestAIService(t)

	_, _, _, _, err := service.executeTool(context.Background(), "run_workspace_command", `{"command":"go version","cwd":"../outside","timeout_sec":30}`, RunOptions{UserID: testUserID})
	if err == nil || err.Error() != "path escapes workspace" {
		t.Fatalf("expected path escape error, got %v", err)
	}
}

func TestExecuteToolRunWorkspaceCommandRejectsAbsoluteCwd(t *testing.T) {
	service, _, _, _ := newTestAIService(t)

	_, _, _, _, err := service.executeTool(context.Background(), "run_workspace_command", `{"command":"go version","cwd":"/tmp","timeout_sec":30}`, RunOptions{UserID: testUserID})
	if err == nil || err.Error() != "invalid path" {
		t.Fatalf("expected invalid path error, got %v", err)
	}
}

func TestExecuteToolRunWorkspaceCommandTimesOut(t *testing.T) {
	service, _, _, _ := newTestAIService(t)
	command := "sleep 2"
	if runtime.GOOS == "windows" {
		command = "Start-Sleep -Seconds 2"
	}
	args, _ := json.Marshal(map[string]any{
		"command":     command,
		"timeout_sec": 1,
	})

	result, _, _, _, err := service.executeTool(context.Background(), "run_workspace_command", string(args), RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("executeTool returned error: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("invalid result json: %v", err)
	}
	if payload["timed_out"] != true || int(payload["exit_code"].(float64)) != -1 {
		t.Fatalf("expected timeout payload, got %#v", payload)
	}
}

func TestExecuteToolWebSearchWithoutProviderReturnsClearError(t *testing.T) {
	service, _, _, _ := newTestAIService(t)

	_, _, _, _, err := service.executeTool(context.Background(), "perseus_search_web", `{"query":"openai","max_results":3}`, RunOptions{UserID: testUserID})
	if err == nil || err.Error() != "未配置可用搜索工具，请先在设置页配置 Tavily 或 Brave。" {
		t.Fatalf("expected clear missing provider error, got %v", err)
	}
}

func TestToolDiagnosticClassifiesExpectedToolFailures(t *testing.T) {
	missingPathErr := fmt.Errorf("stat /data/users/user-1/workspace/docs/missing.md: no such file or directory")
	diag := buildToolDiagnostic("read_workspace_text", `{"path":"docs/missing.md"}`, "docs/missing.md", 12, missingPathErr)
	if diag.Level != "warn" || !strings.Contains(diag.RequestSummary, `"failure_class":"workspace_path_not_found"`) {
		t.Fatalf("unexpected missing path diagnostic: %#v", diag)
	}
	if strings.Contains(diag.ErrorMessage, "/data/users/") || !strings.Contains(diag.ErrorMessage, "workspace:docs/missing.md") {
		t.Fatalf("expected sanitized workspace path, got %q", diag.ErrorMessage)
	}

	diag = buildToolDiagnostic("perseus_search_web", `{"query":"openai"}`, "“openai”", 3, fmt.Errorf("未配置可用搜索工具，请先在设置页配置 Tavily 或 Brave。"))
	if diag.Level != "warn" || !strings.Contains(diag.RequestSummary, `"failure_class":"search_provider_not_configured"`) {
		t.Fatalf("unexpected search diagnostic: %#v", diag)
	}

	diag = buildToolDiagnostic("save_workspace_file", `{}`, "", 0, fmt.Errorf("invalid tool arguments: mode is required"))
	if diag.Level != "warn" || !strings.Contains(diag.RequestSummary, `"failure_class":"tool_invalid_arguments"`) {
		t.Fatalf("unexpected invalid arguments diagnostic: %#v", diag)
	}
}

func TestValidateToolCallRejectsMissingRequiredSaveArguments(t *testing.T) {
	runtime := &agentRuntime{
		protocol:       agentProtocolOpenAI,
		loadedToolsets: allAgentToolsets(true),
	}

	errText := runtime.validateToolCall(agentToolCall{
		ID:            "call_1",
		Name:          "save_workspace_file",
		ArgumentsJSON: `{}`,
	})
	if !strings.Contains(errText, "path is required") {
		t.Fatalf("expected missing path validation error, got %q", errText)
	}
}

func TestValidateToolCallRejectsEmptyStreamedArguments(t *testing.T) {
	runtime := &agentRuntime{
		protocol:       agentProtocolOpenAI,
		loadedToolsets: allAgentToolsets(true),
	}

	errText := runtime.validateToolCall(agentToolCall{
		ID:            "call_1",
		Name:          "save_workspace_file",
		ArgumentsJSON: "",
	})
	if !strings.Contains(errText, "tool arguments are required") {
		t.Fatalf("expected empty arguments validation error, got %q", errText)
	}
}

func TestNormalizeToolCallAllowsEmptyArgumentsWhenSchemaHasNoRequiredFields(t *testing.T) {
	call := agentToolCall{
		ID:            "call_1",
		Name:          "list_skills",
		ArgumentsJSON: "",
	}
	runtime := &agentRuntime{
		protocol:       agentProtocolOpenAI,
		loadedToolsets: allAgentToolsets(true),
	}

	if errText := runtime.normalizeAndValidateToolCall(&call); errText != "" {
		t.Fatalf("expected empty list_skills arguments to be normalized, got %q", errText)
	}
	if call.ArgumentsJSON != "{}" {
		t.Fatalf("expected empty arguments normalized to {}, got %q", call.ArgumentsJSON)
	}
}

func TestValidateToolCallRejectsInvalidSaveMode(t *testing.T) {
	runtime := &agentRuntime{
		protocol:       agentProtocolOpenAI,
		loadedToolsets: allAgentToolsets(true),
	}

	errText := runtime.validateToolCall(agentToolCall{
		ID:            "call_1",
		Name:          "save_workspace_file",
		ArgumentsJSON: `{"path":"demo.txt","content":"x","mode":"append"}`,
	})
	if !strings.Contains(errText, "mode must be one of") {
		t.Fatalf("expected enum validation error, got %q", errText)
	}
}

func TestValidateToolCallAllowsEmptySaveContent(t *testing.T) {
	runtime := &agentRuntime{
		protocol:       agentProtocolOpenAI,
		loadedToolsets: allAgentToolsets(true),
	}

	errText := runtime.validateToolCall(agentToolCall{
		ID:            "call_1",
		Name:          "save_workspace_file",
		ArgumentsJSON: `{"path":"empty.txt","content":"","mode":"create"}`,
	})
	if errText != "" {
		t.Fatalf("empty file content should be valid save arguments, got %q", errText)
	}
}

func TestBudgetToolResultForModelTruncatesCommandOutput(t *testing.T) {
	raw := mustJSON(map[string]any{
		"command":   "find . -type f",
		"cwd":       "",
		"stdout":    strings.Repeat("a", 100),
		"stderr":    strings.Repeat("b", 80),
		"exit_code": 0,
	})
	budgeted := budgetToolResultForModel("run_workspace_command", raw, providerRuntimeProfile{
		MaxToolResultRunes:    1000,
		MaxCommandOutputRunes: 20,
	})

	var payload map[string]any
	if err := json.Unmarshal([]byte(budgeted), &payload); err != nil {
		t.Fatalf("budgeted result is not json: %v", err)
	}
	if got := len([]rune(payload["stdout"].(string))); got != 20 {
		t.Fatalf("expected stdout truncated to 20 chars, got %d", got)
	}
	if payload["stdout_truncated_for_model"] != true {
		t.Fatalf("expected stdout truncation flag, got %#v", payload)
	}
	if got := len([]rune(payload["stderr"].(string))); got != 10 {
		t.Fatalf("expected stderr truncated to 10 chars, got %d", got)
	}
}

func TestBudgetToolResultForModelTruncatesSearchResults(t *testing.T) {
	results := make([]any, 0, 8)
	for index := 0; index < 8; index++ {
		results = append(results, map[string]any{
			"title":   "result",
			"url":     "https://example.com",
			"snippet": strings.Repeat("s", 50),
			"content": strings.Repeat("c", 80),
		})
	}
	raw := mustJSON(map[string]any{"provider": "tavily", "query": "ai news", "results": results})
	budgeted := budgetToolResultForModel("perseus_search_web", raw, providerRuntimeProfile{
		MaxToolResultRunes:       10000,
		MaxWebSearchSnippetRunes: 12,
	})

	var payload map[string]any
	if err := json.Unmarshal([]byte(budgeted), &payload); err != nil {
		t.Fatalf("budgeted result is not json: %v", err)
	}
	gotResults := payload["results"].([]any)
	if len(gotResults) != 6 {
		t.Fatalf("expected search results limited to 6, got %d", len(gotResults))
	}
	first := gotResults[0].(map[string]any)
	if got := len([]rune(first["snippet"].(string))); got != 12 {
		t.Fatalf("expected snippet truncated to 12 chars, got %d", got)
	}
	if first["content_truncated_for_model"] != true {
		t.Fatalf("expected content truncation flag, got %#v", first)
	}
}

func TestBudgetToolResultForModelPreservesReadWorkspaceTextShape(t *testing.T) {
	raw := mustJSON(map[string]any{
		"tool":    "read_workspace_text",
		"path":    "draft.txt",
		"content": strings.Repeat("x", 5000),
	})
	budgeted := budgetToolResultForModel("read_workspace_text", raw, providerRuntimeProfile{
		MaxToolResultRunes:      900,
		MaxReadFileContentRunes: 5000,
	})

	var payload map[string]any
	if err := json.Unmarshal([]byte(budgeted), &payload); err != nil {
		t.Fatalf("budgeted result is not json: %v", err)
	}
	if payload["path"] != "draft.txt" || payload["content"] == nil || payload["raw_prefix"] != nil {
		t.Fatalf("expected structured read payload, got %#v", payload)
	}
	if payload["content_truncated_for_model"] != true {
		t.Fatalf("expected content truncation flag, got %#v", payload)
	}
	if got := len([]rune(budgeted)); got > 900 {
		t.Fatalf("expected payload within budget, got %d chars", got)
	}
}

func TestBudgetToolResultForModelPreservesBatchReadItemsShape(t *testing.T) {
	raw := mustJSON(map[string]any{
		"tool": "read_workspace_files",
		"items": []any{
			map[string]any{"path": "a.txt", "content": strings.Repeat("a", 2500), "start_line": 1, "end_line": 50},
			map[string]any{"path": "b.txt", "content": strings.Repeat("b", 2500), "start_line": 1, "end_line": 50},
		},
		"failed": []any{map[string]any{"path": "missing.txt", "error": "not found"}},
	})
	budgeted := budgetToolResultForModel("read_workspace_files", raw, providerRuntimeProfile{
		MaxToolResultRunes:      1200,
		MaxReadFileContentRunes: 2500,
	})

	var payload map[string]any
	if err := json.Unmarshal([]byte(budgeted), &payload); err != nil {
		t.Fatalf("budgeted result is not json: %v", err)
	}
	items, ok := payload["items"].([]any)
	if !ok || len(items) == 0 || payload["raw_prefix"] != nil {
		t.Fatalf("expected structured batch read payload, got %#v", payload)
	}
	first := items[0].(map[string]any)
	if first["path"] == "" || first["content"] == nil {
		t.Fatalf("expected item path/content to remain, got %#v", first)
	}
	if got := len([]rune(budgeted)); got > 1200 {
		t.Fatalf("expected payload within budget, got %d chars", got)
	}
}

func TestToolTargetSummaryAvoidsContentAndKeepsSafeTarget(t *testing.T) {
	saveArgs := `{"path":"drafts/chapter-1.md","content":"secret long content","mode":"overwrite"}`
	if got := toolTargetSummary("save_workspace_file", saveArgs); got != "drafts/chapter-1.md" {
		t.Fatalf("unexpected save target summary: %q", got)
	}
	if strings.Contains(toolTargetSummary("save_workspace_file", saveArgs), "secret") {
		t.Fatal("target summary leaked file content")
	}
	if got := toolTargetSummary("read_workspace_files", `{"paths":["a.md","b.md","c.md"]}`); got != "3 个文件" {
		t.Fatalf("unexpected batch read target summary: %q", got)
	}
	if got := toolTargetSummary("search_workspace_text", `{"query":"关键线索"}`); got != "“关键线索”" {
		t.Fatalf("unexpected search target summary: %q", got)
	}
}

func TestSanitizeConversationMessagesTruncatesOlderHistory(t *testing.T) {
	messages := []store.Message{
		{Role: "user", Content: strings.Repeat("old", 100)},
		{Role: "assistant", Content: strings.Repeat("new", 100)},
	}
	sanitized := sanitizeConversationMessages(messages, providerRuntimeProfile{
		MaxHistoryMessages:    10,
		MaxRecentMessageRunes: 20,
		MaxOlderMessageRunes:  10,
	})

	if !strings.Contains(sanitized[0].Content, "truncated") {
		t.Fatalf("expected old message truncation note, got %q", sanitized[0].Content)
	}
	if !strings.Contains(sanitized[1].Content, "truncated") {
		t.Fatalf("expected recent message truncation note, got %q", sanitized[1].Content)
	}
}

func TestInjectConversationSummaryDropsSummarizedPrefix(t *testing.T) {
	messages := []store.Message{
		{ID: "m1", Role: "user", Content: "old"},
		{ID: "m2", Role: "assistant", Content: "summarized"},
		{ID: "m3", Role: "user", Content: "recent"},
	}
	next := injectConversationSummary(messages, "用户正在调试项目。", "m2")

	if len(next) != 2 {
		t.Fatalf("expected summary plus one recent message, got %#v", next)
	}
	if next[0].Role != "system" || !strings.Contains(next[0].Content, "用户正在调试项目") {
		t.Fatalf("expected injected system summary, got %#v", next[0])
	}
	if next[1].ID != "m3" {
		t.Fatalf("expected only messages after summarized prefix, got %#v", next)
	}
}

func TestMaybeRefreshConversationSummaryUsesTokenWatermark(t *testing.T) {
	service, _, _, _ := newTestAIService(t)
	ctx := context.Background()
	provider := store.ModelProvider{
		Name:                        "test",
		Protocol:                    "openai",
		BaseURL:                     "http://127.0.0.1",
		ModelID:                     "gpt-test",
		AutoCompactEnabled:          true,
		AutoCompactThresholdPercent: 85,
		CompactReservedTokens:       100,
		PruneToolOutputsEnabled:     true,
	}
	messages := make([]store.Message, 0, 30)
	for index := 0; index < 30; index++ {
		messages = append(messages, store.Message{
			ID:      fmt.Sprintf("m%d", index),
			Role:    "user",
			Content: strings.Repeat("长上下文", 200),
		})
	}
	var events []string
	result := service.maybeRefreshConversationSummary(ctx, "conv-test", provider, messages, store.ConversationSummary{}, RunOptions{
		UserID: testUserID,
		Event: func(eventType, level string, payload map[string]any) {
			events = append(events, eventType)
		},
	}, providerRuntimeProfile{
		ContextWindowTokens:     1000,
		AutoCompactEnabled:      true,
		CompactThresholdPercent: 80,
		CompactReservedTokens:   100,
	}, "auto_threshold", "")

	if !result.Skipped {
		t.Fatalf("expected summary to skip because test provider is unreachable, got %#v", result)
	}
	if len(events) == 0 || events[0] != "context_compaction_started" {
		t.Fatalf("expected compaction start event, got %#v", events)
	}
}

func TestPressureCompactionCanSummarizeShortButLargeConversation(t *testing.T) {
	service, _, _, _ := newTestAIService(t)
	ctx := context.Background()
	provider := store.ModelProvider{
		Name:                        "test",
		Protocol:                    "openai",
		BaseURL:                     "http://127.0.0.1",
		ModelID:                     "gpt-test",
		AutoCompactEnabled:          true,
		AutoCompactThresholdPercent: 85,
		CompactReservedTokens:       100,
	}
	messages := make([]store.Message, 0, 10)
	for index := 0; index < 10; index++ {
		messages = append(messages, store.Message{
			ID:      fmt.Sprintf("big%d", index),
			Role:    "user",
			Content: strings.Repeat("single huge message ", 600),
		})
	}
	var events []string
	result := service.maybeRefreshConversationSummary(ctx, "conv-short-big", provider, messages, store.ConversationSummary{}, RunOptions{
		UserID: testUserID,
		Event: func(eventType, level string, payload map[string]any) {
			events = append(events, eventType)
		},
	}, providerRuntimeProfile{
		ContextWindowTokens:     1000,
		AutoCompactEnabled:      true,
		CompactThresholdPercent: 80,
		CompactReservedTokens:   100,
	}, "auto_threshold", "")

	if result.CompactedMessageCount == 0 {
		t.Fatalf("expected pressure compaction to select a chunk, got %#v", result)
	}
	if len(events) == 0 || events[0] != "context_compaction_started" {
		t.Fatalf("expected compaction start event, got %#v", events)
	}
}

func TestPickSearchProviderUsesPriority(t *testing.T) {
	service, _, _, _ := newTestAIService(t)
	ctx := context.Background()
	if _, err := service.configs.CreateSearchProvider(ctx, testUserID, store.SearchProviderInput{
		Provider: "brave",
		APIKey:   "brave-key",
		Enabled:  true,
		Priority: 20,
	}); err != nil {
		t.Fatalf("CreateSearchProvider returned error: %v", err)
	}
	time.Sleep(time.Millisecond)
	if _, err := service.configs.CreateSearchProvider(ctx, testUserID, store.SearchProviderInput{
		Provider: "tavily",
		APIKey:   "tavily-key",
		Enabled:  true,
		Priority: 10,
	}); err != nil {
		t.Fatalf("CreateSearchProvider returned error: %v", err)
	}

	provider, err := service.pickSearchProvider(ctx, testUserID)
	if err != nil {
		t.Fatalf("pickSearchProvider returned error: %v", err)
	}
	if provider.Provider != "tavily" {
		t.Fatalf("expected tavily by priority, got %s", provider.Provider)
	}
}

func TestSideEffectToolsRequireCompletionAfterUse(t *testing.T) {
	for _, name := range []string{
		"save_workspace_file",
		"manage_workspace_path",
		"organize_workspace_matches",
		"install_skill_from_attachment",
		"delete_skill",
		"run_workspace_command",
		"start_background_task",
	} {
		if !toolRequiresCompletionAfterUse(name) {
			t.Fatalf("expected %s to require completion after use", name)
		}
	}
	for _, name := range []string{"inspect_workspace", "read_workspace_text", "list_skills", "perseus_search_web"} {
		if toolRequiresCompletionAfterUse(name) {
			t.Fatalf("did not expect %s to require completion after use", name)
		}
	}
}

func TestRuntimeDoesNotPickWorkspaceToolFromUserKeywords(t *testing.T) {
	runtime := &agentRuntime{
		messages: []store.Message{{Role: "user", Content: "请创建 demo.txt"}},
	}
	if runtime.requiresCompletion || runtime.performedMutation {
		t.Fatalf("runtime should not infer completion or mutation from user wording")
	}
}

func TestAttachmentsRequireToolLoop(t *testing.T) {
	if !attachmentsRequireToolLoop([]AttachmentPayload{{Kind: "workspace_file", Path: "draft.txt"}}) {
		t.Fatal("expected workspace file reference to activate tool loop")
	}
	if !attachmentsRequireToolLoop([]AttachmentPayload{{Kind: "workspace_file", Path: "draft.txt", Text: "preview"}}) {
		t.Fatal("expected workspace file reference with preview to activate tool loop")
	}
	if attachmentsRequireToolLoop([]AttachmentPayload{{Kind: "current_file", Path: "draft.txt"}}) {
		t.Fatal("did not expect passive current file context to activate tool loop")
	}
	if attachmentsRequireToolLoop([]AttachmentPayload{{Kind: "editor_panels_context"}}) {
		t.Fatal("did not expect passive editor panels context to activate tool loop")
	}
	if attachmentsRequireToolLoop([]AttachmentPayload{{Kind: "text", Name: "note.txt"}}) {
		t.Fatal("did not expect plain text attachment to activate tool loop")
	}
}

func TestSelectedWorkspaceReferencesConstrainReadAndMutationScope(t *testing.T) {
	runtime := &agentRuntime{
		attachments: []AttachmentPayload{
			{Kind: "workspace_file", Path: "白凌传·造化篇（续）/第三十一集.txt", Name: "第三十一集.txt"},
			{Kind: "workspace_file", Path: "白凌传·造化篇（续）/第三十二集.txt", Name: "第三十二集.txt"},
			{Kind: "workspace_file", Path: "白凌传·造化篇（续）/第三十三集.txt", Name: "第三十三集.txt"},
			{Kind: "workspace_file", Path: "白凌传·造化篇（续）/第三十七集.txt", Name: "第三十七集.txt"},
		},
	}

	if got := runtime.validateSelectedReferenceScope(agentToolCall{
		Name:          "read_workspace_files",
		ArgumentsJSON: `{"paths":["白凌传·造化篇（续）/第三十集.txt","白凌传·造化篇（续）/第三十一集.txt"]}`,
	}); !strings.Contains(got, "outside") {
		t.Fatalf("expected outside-scope read to be rejected, got %q", got)
	}
	if got := runtime.validateSelectedReferenceScope(agentToolCall{
		Name:          "save_workspace_file",
		ArgumentsJSON: `{"path":"白凌传·造化篇（续）/第三十一集.txt","content":"x","mode":"overwrite"}`,
	}); got != "" {
		t.Fatalf("expected selected overwrite to be allowed, got %q", got)
	}
	if got := runtime.validateSelectedReferenceScope(agentToolCall{
		Name:          "manage_workspace_path",
		ArgumentsJSON: `{"action":"move","path":"二十九集.txt","new_parent_path":"白凌传·造化篇（续）"}`,
	}); !strings.Contains(got, "outside") {
		t.Fatalf("expected unreferenced extra episode to be rejected, got %q", got)
	}
	if got := runtime.validateSelectedReferenceScope(agentToolCall{
		Name:          "read_workspace_text",
		ArgumentsJSON: `{"path":"另一个文件夹/第三十一集.txt"}`,
	}); !strings.Contains(got, "outside") {
		t.Fatalf("expected same-basename file in another folder to be rejected, got %q", got)
	}
	if got := runtime.validateSelectedReferenceScope(agentToolCall{
		Name:          "save_workspace_file",
		ArgumentsJSON: `{"path":"白凌传·造化篇（续）/优化说明.txt","content":"x","mode":"create"}`,
	}); got != "" {
		t.Fatalf("expected new output file creation to be allowed, got %q", got)
	}
}

func TestWorkspaceToolNamesAndGroups(t *testing.T) {
	loaded := map[string]bool{"workspace": true}
	for _, name := range []string{"find_workspace_files", "search_workspace_text", "read_workspace_text", "read_workspace_files", "edit_workspace_text"} {
		assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), name)
		assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), name)
	}
	if group := toolGroup("find_workspace_files"); group != ToolGroupSearch {
		t.Fatalf("expected find_workspace_files search group, got %q", group)
	}
	if group := toolGroup("edit_workspace_text"); group != ToolGroupEdit {
		t.Fatalf("expected edit_workspace_text edit group, got %q", group)
	}
	if group := toolGroup("read_workspace_files"); group != ToolGroupRead {
		t.Fatalf("expected read_workspace_files read group, got %q", group)
	}
	if !isWorkspaceMutationTool("edit_workspace_text") {
		t.Fatal("expected edit_workspace_text to be a workspace mutation")
	}
}

func TestHandleNoToolTurnRejectsPlainTextBeforeToolSucceeds(t *testing.T) {
	runtime := &agentRuntime{
		requiresCompletion: true,
		toolLoopActive:     true,
		transcript:         newAgentTranscript(),
	}
	reply, ok, err := runtime.handleNoToolTurn(context.Background(), agentModelTurn{
		Content:      "我先建文件夹，再写内容。",
		FinishReason: "stop",
	})
	if err != nil {
		t.Fatalf("expected no hard error, got %v", err)
	}
	if ok {
		t.Fatalf("expected corrective loop, got reply %#v", reply)
	}
	if len(runtime.transcript.Entries) == 0 {
		t.Fatal("expected corrective message appended to transcript")
	}
	last := runtime.transcript.Entries[len(runtime.transcript.Entries)-1]
	if last.Kind != transcriptEntryUser || !strings.Contains(last.Text, "没有继续调用工具") {
		t.Fatalf("unexpected corrective transcript entry: %#v", last)
	}
}

func TestHandleNoToolTurnAllowsTextCompletionAfterVerifiedTool(t *testing.T) {
	runtime := &agentRuntime{
		requiresCompletion: true,
		toolLoopActive:     true,
		transcript:         newAgentTranscript(),
		events: []ToolEvent{{
			ToolName: "read_workspace_text",
			Status:   "success",
			Path:     "demo.txt",
		}},
	}
	reply, ok, err := runtime.handleNoToolTurn(context.Background(), agentModelTurn{
		Content:      "已读取 demo.txt，下面是整理后的结论。",
		FinishReason: "stop",
	})
	if err != nil || !ok {
		t.Fatalf("expected text completion after verified tool, ok=%v err=%v", ok, err)
	}
	if !strings.Contains(reply.Content, "整理后的结论") {
		t.Fatalf("unexpected reply: %#v", reply)
	}
	if runtime.completedContent != reply.Content {
		t.Fatalf("expected completed content to be recorded, got %q", runtime.completedContent)
	}
}

func TestRuntimeToolChoiceAllowsTextAfterVerifiedTool(t *testing.T) {
	runtime := &agentRuntime{
		requiresCompletion: true,
		toolLoopActive:     true,
		events: []ToolEvent{{
			ToolName: "read_workspace_text",
			Status:   "success",
			Path:     "demo.txt",
		}},
	}
	if choice := runtime.toolChoice(); choice.Mode != providerToolChoiceAuto {
		t.Fatalf("expected auto tool choice after verified tool so final text can stream, got %#v", choice)
	}
	if got := openAIProviderToolChoice(runtime.toolChoice()); got != "auto" {
		t.Fatalf("expected OpenAI auto tool choice after verified tool, got %#v", got)
	}
}

func TestRuntimeToolChoiceRequiresToolsBeforeVerifiedTool(t *testing.T) {
	runtime := &agentRuntime{requiresCompletion: true, toolLoopActive: true}
	if choice := runtime.toolChoice(); choice.Mode != providerToolChoiceRequired {
		t.Fatalf("expected required tool choice, got %#v", choice)
	}
	if got := openAIProviderToolChoice(runtime.toolChoice()); got != "required" {
		t.Fatalf("expected OpenAI required tool choice, got %#v", got)
	}
	anthropic := anthropicProviderToolChoice(runtime.toolChoice())
	if anthropic["type"] != "any" || anthropic["disable_parallel_tool_use"] != true {
		t.Fatalf("expected Anthropic any tool choice with serial tools, got %#v", anthropic)
	}
}

func TestNativeReasoningPolicyMapsThinkingModes(t *testing.T) {
	openAIProvider := store.ModelProvider{Protocol: "openai", ModelID: "gpt-5.4", ReasoningCapability: store.ReasoningCapabilityOpenAIReasoning}
	cases := map[string]string{
		ThinkingModeLight:    "low",
		ThinkingModeStandard: "medium",
		ThinkingModeDeep:     "high",
		ThinkingModeExpert:   "xhigh",
	}
	for mode, expected := range cases {
		payload := map[string]any{}
		applyOpenAIReasoningPayload(payload, openAIProvider, mode)
		if payload["reasoning_effort"] != expected {
			t.Fatalf("expected OpenAI effort %s for mode %s, got %#v", expected, mode, payload)
		}
	}

	anthropicProvider := store.ModelProvider{Protocol: "anthropic", ModelID: "claude-opus-4-6", ReasoningCapability: store.ReasoningCapabilityAnthropicAdaptive}
	anthropicCases := map[string]string{
		ThinkingModeStandard: "medium",
		ThinkingModeDeep:     "high",
		ThinkingModeExpert:   "max",
	}
	for mode, expected := range anthropicCases {
		payload := map[string]any{}
		applyAnthropicReasoningPayload(payload, anthropicProvider, mode)
		outputConfig, _ := payload["output_config"].(map[string]any)
		if outputConfig["effort"] != expected {
			t.Fatalf("expected Anthropic effort %s for mode %s, got %#v", expected, mode, payload)
		}
	}
}

func TestNativeReasoningPolicyDoesNotSendUnknownModelParams(t *testing.T) {
	provider := store.ModelProvider{Protocol: "openai", ModelID: "qwen-plus", ReasoningCapability: store.ReasoningCapabilityAuto}
	payload := map[string]any{}
	policy := applyOpenAIReasoningPayload(payload, provider, ThinkingModeExpert)
	if policy.Support != store.ReasoningSupportNone || len(payload) != 0 {
		t.Fatalf("expected unknown model to omit reasoning params, policy=%#v payload=%#v", policy, payload)
	}
}

func TestNativeReasoningPolicyUsesSourceBackedOpenAIModelTable(t *testing.T) {
	provider := store.ModelProvider{
		Protocol:            "openai",
		BaseURL:             "https://example.com/v1",
		ModelID:             "gpt-5.4",
		ReasoningCapability: store.ReasoningCapabilityAuto,
	}
	payload := map[string]any{}
	policy := applyOpenAIReasoningPayload(payload, provider, ThinkingModeExpert)
	if policy.Support != store.ReasoningSupportOpenAI || payload["reasoning_effort"] != "xhigh" {
		t.Fatalf("expected source-backed OpenAI model reasoning params, policy=%#v payload=%#v", policy, payload)
	}
}

func TestNativeReasoningPolicyDoesNotUseThinkingNameHeuristics(t *testing.T) {
	provider := store.ModelProvider{
		Protocol:            "openai",
		BaseURL:             "https://example.com/v1",
		ModelID:             "kimi-k2-thinking",
		ReasoningCapability: store.ReasoningCapabilityAuto,
	}
	payload := map[string]any{}
	policy := applyOpenAIReasoningPayload(payload, provider, ThinkingModeExpert)
	if policy.Support != store.ReasoningSupportNone || len(payload) != 0 {
		t.Fatalf("expected thinking-like unknown model to omit reasoning params, policy=%#v payload=%#v", policy, payload)
	}
}

func TestNativeReasoningPolicyPreservesVariableDiscoveryLevels(t *testing.T) {
	provider := store.ModelProvider{
		Protocol:                  "openai",
		BaseURL:                   "https://proxy.example/v1",
		ModelID:                   "provider/gpt",
		ReasoningCapability:       store.ReasoningCapabilityOpenAIReasoning,
		ReasoningCapabilitySource: store.ReasoningCapabilitySourceProviderDiscovery,
		ReasoningProfileLevels:    "light,deep",
	}
	payload := map[string]any{}
	policy := applyOpenAIReasoningPayload(payload, provider, ThinkingModeExpert)
	if payload["reasoning_effort"] != "low" || policy.ThinkingMode != ThinkingModeLight {
		t.Fatalf("expected unsupported mode to clamp to discovery default light, policy=%#v payload=%#v", policy, payload)
	}
	payload = map[string]any{}
	policy = applyOpenAIReasoningPayload(payload, provider, ThinkingModeDeep)
	if payload["reasoning_effort"] != "high" || policy.ThinkingMode != ThinkingModeDeep {
		t.Fatalf("expected supported discovery level to be sent, policy=%#v payload=%#v", policy, payload)
	}
}

func TestNativeReasoningPolicyIgnoresMismatchedCapability(t *testing.T) {
	provider := store.ModelProvider{
		Protocol:            "anthropic",
		BaseURL:             "https://api.anthropic.com/v1",
		ModelID:             "claude-opus-4-6",
		ReasoningCapability: store.ReasoningCapabilityOpenAIReasoning,
	}
	payload := map[string]any{}
	policy := applyAnthropicReasoningPayload(payload, provider, ThinkingModeExpert)
	if policy.Support != store.ReasoningSupportNone || len(payload) != 0 {
		t.Fatalf("expected mismatched protocol/capability to omit reasoning params, policy=%#v payload=%#v", policy, payload)
	}
}

func TestAnthropicReasoningUsesAutoToolChoice(t *testing.T) {
	required := providerToolChoice{Mode: providerToolChoiceRequired}
	choice := anthropicProviderToolChoiceForReasoning(required, true)
	if choice["type"] != "auto" || choice["disable_parallel_tool_use"] != true {
		t.Fatalf("expected adaptive thinking to use auto tool choice, got %#v", choice)
	}
}

func TestRuntimeForceToolOnlyBeforeFirstToolAttempt(t *testing.T) {
	runtime := &agentRuntime{opts: RunOptions{ForceTool: "save_workspace_file"}}
	if choice := runtime.toolChoice(); choice.Mode != providerToolChoiceForced || choice.ToolName != "save_workspace_file" {
		t.Fatalf("expected forced save tool choice before tool attempt, got %#v", choice)
	}
	if got := openAIProviderToolChoice(runtime.toolChoice()); fmt.Sprint(got) == "required" {
		t.Fatalf("expected specific OpenAI tool choice before first attempt, got %#v", got)
	}
	runtime.toolCalls = 1
	runtime.requiresCompletion = true
	if choice := runtime.toolChoice(); choice.Mode != providerToolChoiceRequired {
		t.Fatalf("expected required-any after first forced attempt before verified result, got %#v", choice)
	}
	runtime.events = []ToolEvent{{ToolName: "save_workspace_file", Status: "success", ChangedPaths: []string{"demo.txt"}}}
	runtime.performedMutation = true
	if choice := runtime.toolChoice(); choice.Mode != providerToolChoiceAuto {
		t.Fatalf("expected auto after verified forced tool result so final text can stream, got %#v", choice)
	}
}

func TestRecoverableFinalResponseErrorFailsInsteadOfFakeSuccess(t *testing.T) {
	runtime := &agentRuntime{
		performedMutation: true,
		events: []ToolEvent{{
			ToolName:     "save_workspace_file",
			Status:       "success",
			Path:         "demo.txt",
			ChangedPaths: []string{"demo.txt"},
		}},
	}
	reply, ok, err := runtime.handleNoToolTurn(context.Background(), agentModelTurn{
		FinishReason: "recoverable_final_response_error",
	})
	if err == nil || !ok {
		t.Fatalf("expected recoverable final response error, ok=%v err=%v reply=%#v", ok, err, reply)
	}
	if !strings.Contains(err.Error(), "可继续本轮") {
		t.Fatalf("expected continue hint in error, got %v", err)
	}
}

func TestCompletionPathsMustCoverEachWorkspaceMutation(t *testing.T) {
	runtime := &agentRuntime{
		performedMutation: true,
		events: []ToolEvent{
			{
				ToolName:     "save_workspace_file",
				Status:       "success",
				Path:         "episodes/31.txt",
				ChangedPaths: []string{"episodes/31.txt"},
			},
			{
				ToolName:     "save_workspace_file",
				Status:       "success",
				Path:         "episodes/32.txt",
				ChangedPaths: []string{"episodes/32.txt"},
			},
		},
	}

	if missing := runtime.completionPathsMissingMutations([]string{"episodes/31.txt"}); len(missing) != 1 || missing[0] != "episodes/32.txt" {
		t.Fatalf("expected only the second mutation to be missing, got %#v", missing)
	}
	if missing := runtime.completionPathsMissingMutations([]string{"episodes/31.txt", "episodes/32.txt"}); len(missing) != 0 {
		t.Fatalf("expected all mutations to be covered, got %#v", missing)
	}
}

func TestCompletionPathCoverageAllowsRenameNewPath(t *testing.T) {
	runtime := &agentRuntime{
		performedMutation: true,
		events: []ToolEvent{{
			ToolName:     "manage_workspace_path",
			Status:       "success",
			Path:         "draft-old.txt",
			OldPath:      "draft-old.txt",
			NewPath:      "draft-new.txt",
			ChangedPaths: []string{"draft-old.txt", "draft-new.txt"},
		}},
	}

	if missing := runtime.completionPathsMissingMutations([]string{"draft-new.txt"}); len(missing) != 0 {
		t.Fatalf("expected rename to be covered by new path, got %#v", missing)
	}
}

func TestApplyAnthropicHeadersUsesClaudeCodeProxyBearerAuth(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://example.com/messages", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}

	applyAnthropicHeaders(req, store.ModelProvider{
		BaseURL: "https://wanqing.streamlakeapi.com/api/gateway/coding/kat-coder-pro-v2/claude-code-proxy",
		APIKey:  "test-token",
	})

	if req.Header.Get("Authorization") != "Bearer test-token" {
		t.Fatalf("expected bearer auth, got %q", req.Header.Get("Authorization"))
	}
	if req.Header.Get("x-api-key") != "" {
		t.Fatalf("did not expect x-api-key for claude-code-proxy, got %q", req.Header.Get("x-api-key"))
	}
	if req.Header.Get("anthropic-version") == "" {
		t.Fatalf("expected anthropic-version header")
	}
}

func TestApplyAnthropicHeadersKeepsStandardAnthropicAPIKey(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://example.com/messages", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}

	applyAnthropicHeaders(req, store.ModelProvider{
		BaseURL: "https://api.anthropic.com/v1",
		APIKey:  "test-key",
	})

	if req.Header.Get("x-api-key") != "test-key" {
		t.Fatalf("expected x-api-key, got %q", req.Header.Get("x-api-key"))
	}
	if req.Header.Get("Authorization") != "" {
		t.Fatalf("did not expect authorization for standard anthropic, got %q", req.Header.Get("Authorization"))
	}
}

func TestApplyOpenAIHeadersUsesBearerAuth(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://example.com/chat/completions", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}

	applyOpenAIHeaders(req, store.ModelProvider{APIKey: "test-key"})

	if req.Header.Get("Authorization") != "Bearer test-key" {
		t.Fatalf("expected bearer auth, got %q", req.Header.Get("Authorization"))
	}
	if req.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("expected json content type, got %q", req.Header.Get("Content-Type"))
	}
}

func TestOpenAIChatEndpointTrimsTrailingSlash(t *testing.T) {
	expected := "https://example.com/v1/chat/completions"
	if actual := openAIChatEndpoint("https://example.com/v1/"); actual != expected {
		t.Fatalf("expected %q, got %q", expected, actual)
	}
}

func TestAnthropicMessagesEndpointUsesClaudeCodeProxyV1Path(t *testing.T) {
	baseURL := "https://wanqing.streamlakeapi.com/api/gateway/coding/kat-coder-pro-v2/claude-code-proxy"
	expected := baseURL + "/v1/messages"
	if actual := anthropicMessagesEndpoint(baseURL); actual != expected {
		t.Fatalf("expected %q, got %q", expected, actual)
	}
}

func TestAnthropicMessagesEndpointKeepsStandardPath(t *testing.T) {
	baseURL := "https://api.anthropic.com/v1"
	expected := baseURL + "/messages"
	if actual := anthropicMessagesEndpoint(baseURL); actual != expected {
		t.Fatalf("expected %q, got %q", expected, actual)
	}
}

func TestProviderFailureClassSeparatesConfigFromTransientHTTP(t *testing.T) {
	err := &ProviderResponseError{
		StatusCode:      http.StatusServiceUnavailable,
		Message:         "模型供应商请求失败（HTTP 503）：No available channel for model claude-opus-4-7-thinking under group default",
		ResponseSnippet: `{"error":{"code":"model_not_found","message":"No available channel for model claude-opus-4-7-thinking"}}`,
	}
	if got := providerFailureClass(err, ProviderRequestSummary{}); got != "provider_model_unavailable" {
		t.Fatalf("expected provider_model_unavailable, got %q", got)
	}
	if got := providerFailureClass(fmt.Errorf(`Post "https://pomo.zanbi.fun/v1/chat/completions": dial tcp: lookup pomo.zanbi.fun on 127.0.0.11:53: no such host`), ProviderRequestSummary{}); got != "provider_dns_lookup_failed" {
		t.Fatalf("expected provider_dns_lookup_failed, got %q", got)
	}
}

func TestExecuteToolWriteWorkspaceFileRejectsMissingFile(t *testing.T) {
	service, _, _, _ := newTestAIService(t)

	_, _, _, _, err := service.executeTool(context.Background(), "save_workspace_file", `{"path":"missing.txt","content":"x","mode":"overwrite"}`, RunOptions{UserID: testUserID})
	if err == nil || err.Error() != "file does not exist" {
		t.Fatalf("expected missing file error, got %v", err)
	}
}

func TestExecuteToolWriteWorkspaceFileRejectsPathEscape(t *testing.T) {
	service, _, _, _ := newTestAIService(t)

	_, _, _, _, err := service.executeTool(context.Background(), "save_workspace_file", `{"path":"../bad.txt","content":"x","mode":"overwrite"}`, RunOptions{UserID: testUserID})
	if err == nil || err.Error() != "path escapes workspace" {
		t.Fatalf("expected path escape error, got %v", err)
	}
}

func TestExecuteToolMoveWorkspacePathRejectsPathEscape(t *testing.T) {
	service, _, _, _ := newTestAIService(t)

	_, _, _, _, err := service.executeTool(context.Background(), "manage_workspace_path", `{"action":"move","path":"demo.txt","new_parent_path":"../bad"}`, RunOptions{UserID: testUserID})
	if err == nil || err.Error() != "path escapes workspace" {
		t.Fatalf("expected path escape error, got %v", err)
	}
}

func TestExecuteToolDeleteWorkspacePathRejectsTrashSelf(t *testing.T) {
	service, _, _, _ := newTestAIService(t)

	args := `{"action":"trash","path":"` + workspace.TrashDirName + `"}`
	_, _, _, _, err := service.executeTool(context.Background(), "manage_workspace_path", args, RunOptions{UserID: testUserID})
	expected := "cannot delete " + workspace.TrashDirName + " itself"
	if err == nil || err.Error() != expected {
		t.Fatalf("expected trash guard error %q, got %v", expected, err)
	}
}

func TestExecuteToolRejectsLegacyWorkspaceTool(t *testing.T) {
	service, _, _, _ := newTestAIService(t)

	_, _, _, _, err := service.executeTool(context.Background(), "write_workspace_file", `{"path":"demo.txt","content":"x"}`, RunOptions{UserID: testUserID})
	if err == nil || !strings.Contains(err.Error(), "legacy workspace tool") {
		t.Fatalf("expected legacy tool rejection, got %v", err)
	}
}

func assertToolRegistered(t *testing.T, tools []map[string]any, name string) {
	t.Helper()
	for _, tool := range tools {
		if fn, ok := tool["function"].(map[string]any); ok && fn["name"] == name {
			return
		}
		if tool["name"] == name {
			return
		}
	}
	t.Fatalf("tool %q not registered", name)
}

func assertToolNotRegistered(t *testing.T, tools []map[string]any, name string) {
	t.Helper()
	for _, tool := range tools {
		if fn, ok := tool["function"].(map[string]any); ok && fn["name"] == name {
			t.Fatalf("tool %q should not be registered", name)
		}
		if tool["name"] == name {
			t.Fatalf("tool %q should not be registered", name)
		}
	}
}

func containsPathValue(value any, expected string) bool {
	items, ok := value.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		if text, ok := item.(string); ok && text == expected {
			return true
		}
	}
	return false
}

func toolNamesFromOpenAITools(tools []map[string]any) map[string]bool {
	names := map[string]bool{}
	for _, tool := range tools {
		function, _ := tool["function"].(map[string]any)
		name, _ := function["name"].(string)
		if name != "" {
			names[name] = true
		}
	}
	return names
}

func newTestAIService(t *testing.T) (*Service, *sql.DB, string, *workspace.Service) {
	t.Helper()
	env := newTestAIEnv(t)
	return env.service, env.db, env.root, env.files
}

const testUserID = "test-user-id"

func newTestAIEnv(t *testing.T) *testAIEnv {
	t.Helper()

	root := t.TempDir()
	dbPath := filepath.Join(root, "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	skillStore := store.NewSkillStore(db)
	if err := skillStore.Migrate(context.Background()); err != nil {
		t.Fatalf("skillStore.Migrate returned error: %v", err)
	}

	factory := usersvc.NewFactory(db, filepath.Join(root, "users"), skillStore)
	if err := factory.EnsureRoot(); err != nil {
		t.Fatalf("factory.EnsureRoot returned error: %v", err)
	}

	files, err := factory.Workspace(testUserID)
	if err != nil {
		t.Fatalf("factory.Workspace returned error: %v", err)
	}

	configs := store.NewConfigStore(db)
	if err := configs.Migrate(context.Background()); err != nil {
		t.Fatalf("configs.Migrate returned error: %v", err)
	}

	conversations := store.NewConversationStore(db)
	if err := conversations.Migrate(context.Background()); err != nil {
		t.Fatalf("conversations.Migrate returned error: %v", err)
	}
	diagnostics := store.NewDiagnosticStore(db)
	if err := diagnostics.Migrate(context.Background()); err != nil {
		t.Fatalf("diagnostics.Migrate returned error: %v", err)
	}
	runs := store.NewAgentRunStore(db)
	if err := runs.Migrate(context.Background()); err != nil {
		t.Fatalf("runs.Migrate returned error: %v", err)
	}
	service := NewService(configs, factory, conversations, diagnostics)
	service.AttachRunStore(runs)

	return &testAIEnv{
		service:       service,
		factory:       factory,
		db:            db,
		root:          root,
		configs:       configs,
		conversations: conversations,
		diagnostics:   diagnostics,
		runs:          runs,
		files:         files,
	}
}
