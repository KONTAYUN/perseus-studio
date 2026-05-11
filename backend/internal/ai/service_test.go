package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"path/filepath"
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
	db            *sql.DB
	root          string
	configs       *store.ConfigStore
	conversations *store.ConversationStore
	diagnostics   *store.DiagnosticStore
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
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "read_workspace_text")
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "save_workspace_file")
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "manage_workspace_path")
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "organize_workspace_matches")
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "run_workspace_command")
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "perseus_search_web")
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "list_skills")
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "read_skill")
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "install_skill_from_attachment")
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "delete_skill")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "inspect_workspace")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "read_workspace_text")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "save_workspace_file")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "manage_workspace_path")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "organize_workspace_matches")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "run_workspace_command")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "perseus_search_web")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "list_skills")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "read_skill")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "install_skill_from_attachment")
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "delete_skill")
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
			if strings.Contains(prompt, legacyName) {
				t.Fatalf("prompt exposed legacy workspace tool %q: %s", legacyName, prompt)
			}
		}
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
	if !attachmentsRequireToolLoop([]AttachmentPayload{{Kind: "current_file", Path: "draft.txt"}}) {
		t.Fatal("expected current file reference to activate tool loop")
	}
	if !attachmentsRequireToolLoop([]AttachmentPayload{{Kind: "editor_panels_context"}}) {
		t.Fatal("expected editor panels context to activate tool loop")
	}
	if attachmentsRequireToolLoop([]AttachmentPayload{{Kind: "text", Name: "note.txt"}}) {
		t.Fatal("did not expect plain text attachment to activate tool loop")
	}
}

func TestHandleNoToolTurnRejectsPlainTextWhenToolLoopActive(t *testing.T) {
	runtime := &agentRuntime{
		toolLoopActive: true,
		transcript:     newAgentTranscript(),
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

func TestRecoverableFinalResponseErrorFinishesFromVerifiedMutation(t *testing.T) {
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
	if err != nil || !ok {
		t.Fatalf("expected verified fallback reply, ok=%v err=%v", ok, err)
	}
	if !strings.Contains(reply.Content, "demo.txt") {
		t.Fatalf("expected changed path in reply, got %q", reply.Content)
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
		db:            db,
		root:          root,
		configs:       configs,
		conversations: conversations,
		diagnostics:   diagnostics,
		files:         files,
	}
}
