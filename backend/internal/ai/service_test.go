package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"webide-backend/internal/failure"
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
	t.Setenv("PERSEUS_COMMAND_EXECUTION", "trusted_host")
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
	assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "apply_workspace_patch")
	assertToolNotRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "edit_workspace_text")
	assertToolNotRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "replace_workspace_lines")
	assertToolNotRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "save_workspace_file")
	assertToolNotRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "write_workspace_chunk")
	assertToolNotRegistered(t, openAIToolsForLoadedToolsets(loaded, true), "finalize_workspace_write")
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
	assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "apply_workspace_patch")
	assertToolNotRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "edit_workspace_text")
	assertToolNotRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "replace_workspace_lines")
	assertToolNotRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "save_workspace_file")
	assertToolNotRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "write_workspace_chunk")
	assertToolNotRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), "finalize_workspace_write")
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

func TestProviderHTTPClientUsesConservativeHTTP1Transport(t *testing.T) {
	client := newProviderHTTPClient()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}
	if transport.ForceAttemptHTTP2 {
		t.Fatal("provider transport must not attempt HTTP/2")
	}
	if transport.TLSNextProto == nil {
		t.Fatal("provider transport must explicitly disable HTTP/2 TLS upgrades")
	}
	if transport.TLSClientConfig == nil {
		t.Fatal("provider transport must set TLSClientConfig for ALPN")
	}
	if got := transport.TLSClientConfig.NextProtos; len(got) != 1 || got[0] != providerHTTPTransportProtocol {
		t.Fatalf("expected provider ALPN %q only, got %#v", providerHTTPTransportProtocol, got)
	}
}

func TestTurnWithExtractedTextPatchCreatesToolCallAndHidesPatchText(t *testing.T) {
	content := "我来编辑文件。\n\n```text\n*** Begin Patch\n*** Add File: draft.md\n+# 标题\n*** End Patch\n```\n\n继续。"
	turn, ok := turnWithExtractedTextPatch(agentModelTurn{Content: content})
	if !ok {
		t.Fatal("expected text patch to be captured")
	}
	if len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Name != "apply_workspace_patch" {
		t.Fatalf("unexpected tool calls: %#v", turn.ToolCalls)
	}
	if !strings.Contains(turn.ToolCalls[0].PatchText, "*** Begin Patch") {
		t.Fatalf("expected raw patch stored on tool call")
	}
	if strings.Contains(turn.Content, "*** Begin Patch") || !strings.Contains(turn.Content, "我来编辑文件") || !strings.Contains(turn.Content, "继续") {
		t.Fatalf("unexpected visible content: %q", turn.Content)
	}
	if strings.Contains(turn.Content, "```") {
		t.Fatalf("patch code fence should not remain visible: %q", turn.Content)
	}
	if strings.Contains(toolCallTranscriptArgumentsJSON(turn.ToolCalls[0]), "# 标题") {
		t.Fatalf("transcript args should not retain full patch content")
	}
}

func TestTurnWithExtractedTextPatchMergesMultiplePatchBlocks(t *testing.T) {
	content := "先写第一份。\n\n```patch\n*** Begin Patch\n*** Add File: a.txt\n+alpha\n*** End Patch\n```\n\n再写第二份。\n\n*** Begin Patch\n*** Add File: b.txt\n+beta\n*** End Patch\n完成。"
	turn, ok := turnWithExtractedTextPatch(agentModelTurn{Content: content})
	if !ok {
		t.Fatal("expected multiple text patches to be captured")
	}
	if len(turn.ToolCalls) != 1 {
		t.Fatalf("expected one internal patch call, got %#v", turn.ToolCalls)
	}
	patch := turn.ToolCalls[0].PatchText
	if strings.Count(patch, "*** Begin Patch") != 1 || strings.Count(patch, "*** End Patch") != 1 {
		t.Fatalf("expected merged single patch envelope, got %q", patch)
	}
	for _, want := range []string{"*** Add File: a.txt", "*** Add File: b.txt", "+alpha", "+beta"} {
		if !strings.Contains(patch, want) {
			t.Fatalf("merged patch missing %q: %q", want, patch)
		}
	}
	if strings.Contains(turn.Content, "*** Begin Patch") || strings.Contains(turn.Content, "```") {
		t.Fatalf("visible content leaked patch syntax: %q", turn.Content)
	}
	if !strings.Contains(turn.Content, "先写第一份") || !strings.Contains(turn.Content, "再写第二份") || !strings.Contains(turn.Content, "完成") {
		t.Fatalf("visible prose should remain, got %q", turn.Content)
	}
}

func TestTextPatchNormalizationSupersedesMixedJSONToolCall(t *testing.T) {
	content := "我来修改。\n\n*** Begin Patch\n*** Update File: draft.md\n@@\n-old\n+new\n*** End Patch"
	result := normalizeTextPatchTurn(agentModelTurn{
		Content: content,
		ToolCalls: []agentToolCall{{
			ID:            "call_search",
			Name:          "search_workspace_text",
			ArgumentsJSON: `{"query":"old"}`,
		}},
		PublicStreamedContent: "我来修改。\n\n",
		PublicTextStreamed:    true,
	})
	if !result.Extracted {
		t.Fatal("expected text patch to be captured even when JSON tool calls are present")
	}
	if result.DiscardedToolCalls != 1 || len(result.DiscardedToolNames) != 1 || result.DiscardedToolNames[0] != "search_workspace_text" {
		t.Fatalf("expected mixed JSON tool call to be recorded as discarded, got %#v", result)
	}
	turn := result.Turn
	if len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Name != "apply_workspace_patch" {
		t.Fatalf("expected internal patch tool call only, got %#v", turn.ToolCalls)
	}
	if strings.Contains(turn.Content, "*** Begin Patch") || strings.Contains(turn.PublicStreamedContent, "*** Begin Patch") {
		t.Fatalf("patch text must not remain visible: content=%q public=%q", turn.Content, turn.PublicStreamedContent)
	}
	if len(turn.OpenAIMessage.ToolCalls) != 1 || turn.OpenAIMessage.ToolCalls[0].Function.Name != "apply_workspace_patch" {
		t.Fatalf("OpenAI transcript should keep internal patch call only, got %#v", turn.OpenAIMessage.ToolCalls)
	}
	if strings.Contains(string(turn.OpenAIMessage.ToolCalls[0].Function.Arguments), "search_workspace_text") {
		t.Fatalf("discarded JSON tool call leaked into patch call arguments: %#v", turn.OpenAIMessage.ToolCalls[0])
	}
}

func TestTextPatchNormalizationKeepsExecutionPatchSeparateFromTranscript(t *testing.T) {
	turn, ok := turnWithExtractedTextPatch(agentModelTurn{Content: "*** Begin Patch\n*** Add File: draft.md\n+hello\n*** End Patch"})
	if !ok {
		t.Fatal("expected text patch to be captured")
	}
	runtime := &agentRuntime{
		protocol:       agentProtocolOpenAI,
		loadedToolsets: allAgentToolsets(true),
	}
	call := turn.ToolCalls[0]
	if errText := runtime.normalizeAndValidateToolCall(&call); errText != "" {
		t.Fatalf("normalizeAndValidateToolCall returned %q", errText)
	}
	if !strings.Contains(executionToolArgumentsJSON(call), "+hello") {
		t.Fatalf("execution arguments lost raw patch: %s", executionToolArgumentsJSON(call))
	}
	if strings.Contains(toolCallTranscriptArgumentsJSON(call), "+hello") {
		t.Fatalf("transcript arguments leaked raw patch: %s", toolCallTranscriptArgumentsJSON(call))
	}
}

func TestApplyWorkspacePatchSupportsJSONAndTextChannels(t *testing.T) {
	loaded := map[string]bool{"workspace": true}
	if !toolNamesFromOpenAITools(openAIToolsForLoadedToolsets(loaded, true))["apply_workspace_patch"] {
		t.Fatal("apply_workspace_patch must be exposed as a JSON function tool")
	}
	turn, ok := turnWithExtractedTextPatch(agentModelTurn{Content: "*** Begin Patch\n*** Add File: draft.md\n+hello\n*** End Patch"})
	if !ok || len(turn.ToolCalls) != 1 {
		t.Fatalf("expected text patch to create internal tool call, got %#v ok=%v", turn.ToolCalls, ok)
	}
	runtime := &agentRuntime{protocol: agentProtocolOpenAI, loadedToolsets: loaded}
	call := turn.ToolCalls[0]
	if errText := runtime.normalizeAndValidateToolCall(&call); errText != "" {
		t.Fatalf("internal text patch call should validate even when hidden from provider tools: %s", errText)
	}
	if errText := runtime.validateToolCall(agentToolCall{
		Name:          "apply_workspace_patch",
		ArgumentsJSON: `{"patch":"*** Begin Patch\n*** Add File: draft.md\n+hello\n*** End Patch"}`,
	}); errText != "" {
		t.Fatalf("JSON patch function calls should validate, got %q", errText)
	}
}

func TestProviderTurnCanStreamAssistantTextOnlyWhenPatchChannelInactive(t *testing.T) {
	if !providerTurnCanStreamAssistantText(providerTurnRequest{}) {
		t.Fatal("plain chat should stream assistant text")
	}
	if providerTurnCanStreamAssistantText(providerTurnRequest{Opts: RunOptions{TaskID: "task_1"}}) {
		t.Fatal("background task text should wait for complete-turn patch capture")
	}
	loaded := map[string]bool{"workspace": true}
	if !providerTurnCanStreamAssistantText(providerTurnRequest{Tools: openAIToolsForLoadedToolsets(loaded, true)}) {
		t.Fatal("workspace tool turns should stream ordinary text; patch blocks are filtered separately")
	}
}

func TestActionBoundaryActiveBuffersTextUntilActionAppears(t *testing.T) {
	var streamed strings.Builder
	stream := "我会先查看。\n"
	turn, err := parseOpenAIStreamWithOptions(strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"我会先查看。\n"}}]}`,
		`data: {"choices":[{"finish_reason":"stop","delta":{}}]}`,
		`data: [DONE]`,
		``,
	}, "\n\n")), providerStreamParseOptions{
		EmitText:            func(text string) { streamed.WriteString(text) },
		RevealTextAfterTool: true,
	})
	if err != nil {
		t.Fatalf("parseOpenAIStreamWithOptions returned error: %v", err)
	}
	if turn.Content != stream {
		t.Fatalf("expected turn content to keep candidate text for runtime correction, got %q", turn.Content)
	}
	if streamed.String() != "" || turn.PublicTextStreamed {
		t.Fatalf("action-boundary text without an action must not stream, streamed=%q public=%q flag=%v", streamed.String(), turn.PublicStreamedContent, turn.PublicTextStreamed)
	}
}

func TestProviderVisibleTextKeepsPatchBlocksForRuntimeCapture(t *testing.T) {
	text := providerVisibleText("前文\n*** Begin Patch\n*** Add File: a.md\n+x\n*** End Patch\n后文")
	if !strings.Contains(text, "*** Begin Patch") || !strings.Contains(text, "+x") {
		t.Fatalf("unexpected visible text: %q", text)
	}
}

func TestProviderPayloadDiagnosticsRecordHTTPTransportPolicy(t *testing.T) {
	var summary ProviderRequestSummary
	annotatePayloadDiagnostics(&summary, map[string]any{"model": "test", "messages": []map[string]any{}}, nil, store.ModelProvider{
		BaseURL:  "https://api.jellix.top/v1",
		Protocol: "openai",
		ModelID:  "gpt-5.5",
	}, providerReasoningPolicy{})
	if summary.ProviderHTTPProtocol != providerHTTPTransportProtocol {
		t.Fatalf("expected provider http protocol %q, got %q", providerHTTPTransportProtocol, summary.ProviderHTTPProtocol)
	}
	if !summary.ProviderHTTP2Disabled {
		t.Fatal("expected provider HTTP/2 disabled diagnostic flag")
	}
}

func TestDecodeOpenAIConversationTitleResponseAcceptsSSEDespiteNonStreamRequest(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(strings.NewReader(
			"data: {\"choices\":[{\"delta\":{\"content\":\"向北方\"}}]}\n\n" +
				"data: {\"choices\":[{\"delta\":{\"content\":\"改写\"}}]}\n\n" +
				"data: [DONE]\n\n",
		)),
	}
	resp.Header.Set("Content-Type", "text/event-stream")

	data, stats, mismatch, err := decodeOpenAIConversationTitleResponse(resp, "https://example.test/v1/chat/completions")
	if err != nil {
		t.Fatalf("decodeOpenAIConversationTitleResponse returned error: %v", err)
	}
	if !mismatch {
		t.Fatalf("expected protocol mismatch flag for SSE title response")
	}
	if stats.Chunks != 2 || stats.ChoicesSeen != 2 {
		t.Fatalf("unexpected SSE stats: %#v", stats)
	}
	if got := data.titleText(); got != "向北方改写" {
		t.Fatalf("unexpected title text: %q", got)
	}
}

func TestDecodeOpenAIConversationTitleResponseIgnoresUsageOnlySSEChunks(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":1}}\n\ndata: [DONE]\n\n")),
	}
	resp.Header.Set("Content-Type", "text/event-stream")

	data, stats, mismatch, err := decodeOpenAIConversationTitleResponse(resp, "https://example.test/v1/chat/completions")
	if err != nil {
		t.Fatalf("decodeOpenAIConversationTitleResponse returned error: %v", err)
	}
	if !mismatch || stats.Chunks != 1 || stats.ChoicesSeen != 0 {
		t.Fatalf("unexpected SSE mismatch/stats: mismatch=%v stats=%#v", mismatch, stats)
	}
	if got := data.titleText(); got != "" {
		t.Fatalf("expected no title text from usage-only SSE, got %q", got)
	}
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
	if label := userFriendlyToolLabel("save_workspace_file"); label != "已编辑文件" {
		t.Fatalf("expected completed compatibility write label, got %q", label)
	}
	if label := userFriendlyToolLabel("ask_followup_question"); label != "已询问用户" {
		t.Fatalf("expected completed question label, got %q", label)
	}

	runtime := &agentRuntime{}
	_, event := runtime.alreadyAppliedToolResult(agentToolCall{ID: "call_1", Name: "save_workspace_file"}, ToolEvent{
		ToolName: "save_workspace_file",
		Status:   "success",
	})
	if event.Label != "已编辑文件" {
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

func TestHandleCompletionAttemptAllowsExplanationsOfInternalEditProtocolLeak(t *testing.T) {
	r := &agentRuntime{}
	result, isError, completed := r.handleCompletionAttempt(`{"result":"apply_workspace_patch 用结构化参数提交 *** Begin Patch 示例；尚未运行测试。"}`)
	if isError || !completed || !strings.Contains(result, "apply_workspace_patch") {
		t.Fatalf("display prose rejected: %s", result)
	}
}

func TestHandleCompletionAttemptAllowsExplanationsOfChineseInternalEditProtocolLeak(t *testing.T) {
	r := &agentRuntime{}
	result, isError, completed := r.handleCompletionAttempt(`{"result":"apply_workspace_patch 用结构化参数提交 *** Begin Patch 示例；尚未运行测试。"}`)
	if isError || !completed || !strings.Contains(result, "apply_workspace_patch") {
		t.Fatalf("display prose rejected: %s", result)
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

func TestRuntimeCheckpointPreservesLastToolFailureDetails(t *testing.T) {
	runtime := &agentRuntime{
		transcript:       newAgentTranscript(),
		appliedMutations: map[string]ToolEvent{},
		chunkWrites:      map[string]workspaceChunkWriteState{},
		loadedToolsets:   map[string]bool{"workspace": true},
	}
	runtime.recordToolFailure("manage_workspace_path", `{"action":"mkdir","path":"docs"}`, workspace.ErrPathExists)
	checkpoint := runtime.checkpoint()
	restored := &agentRuntime{}
	restored.restoreCheckpoint(checkpoint)
	if !restored.lastToolFailed || !restored.lastToolFailure.Recoverable || restored.lastToolFailure.FailureClass != "workspace_path_already_exists" {
		t.Fatalf("expected restored checkpoint to preserve recoverable failure, got failed=%v detail=%#v", restored.lastToolFailed, restored.lastToolFailure)
	}
}

func TestHandleCompletionAttemptAllowsPartialAfterRecoverablePatchFailure(t *testing.T) {
	r := &agentRuntime{toolLoopActive: true}
	r.recordToolFailure("apply_workspace_patch", "{}", failure.New("workspace_patch_apply_failed", "mismatch", nil))
	result, isError, completed := r.handleCompletionAttempt(`{"outcome":"partial","result":"已检查，文件尚未修改。","failed_items":["draft.txt"]}`)
	if isError || !completed || r.completedOutcome != "partial" {
		t.Fatalf("honest partial rejected: %s", result)
	}
	if r.hasSuccessfulWorkspaceMutation() {
		t.Fatal("fabricated a mutation")
	}
}

func TestHandleCompletionAttemptAllowsPartialAfterExistingFolderMkdirFailure(t *testing.T) {
	r := &agentRuntime{toolLoopActive: true}
	r.recordToolFailure("apply_workspace_patch", "{}", failure.New("workspace_patch_apply_failed", "mismatch", nil))
	result, isError, completed := r.handleCompletionAttempt(`{"outcome":"partial","result":"已检查，文件尚未修改。","failed_items":["draft.txt"]}`)
	if isError || !completed || r.completedOutcome != "partial" {
		t.Fatalf("honest partial rejected: %s", result)
	}
	if r.hasSuccessfulWorkspaceMutation() {
		t.Fatal("fabricated a mutation")
	}
}

func TestHandleCompletionAttemptAllowsPartialFailedItemsWithoutFailedObservation(t *testing.T) {
	r := &agentRuntime{toolLoopActive: true}
	r.recordToolFailure("apply_workspace_patch", "{}", failure.New("workspace_patch_apply_failed", "mismatch", nil))
	result, isError, completed := r.handleCompletionAttempt(`{"outcome":"partial","result":"已检查，文件尚未修改。","failed_items":["draft.txt"]}`)
	if isError || !completed || r.completedOutcome != "partial" {
		t.Fatalf("honest partial rejected: %s", result)
	}
	if r.hasSuccessfulWorkspaceMutation() {
		t.Fatal("fabricated a mutation")
	}
}

func TestHandleCompletionAttemptAllowsPartialSelfReportedUnfinishedWriteWithoutFailedObservation(t *testing.T) {
	r := &agentRuntime{toolLoopActive: true}
	r.recordToolFailure("apply_workspace_patch", "{}", failure.New("workspace_patch_apply_failed", "mismatch", nil))
	result, isError, completed := r.handleCompletionAttempt(`{"outcome":"partial","result":"已检查，文件尚未修改。","failed_items":["draft.txt"]}`)
	if isError || !completed || r.completedOutcome != "partial" {
		t.Fatalf("honest partial rejected: %s", result)
	}
	if r.hasSuccessfulWorkspaceMutation() {
		t.Fatal("fabricated a mutation")
	}
}

func TestHandleCompletionAttemptRejectsUnverifiedChangedPaths(t *testing.T) {
	runtime := &agentRuntime{
		toolLoopActive: true,
		toolCalls:      1,
		events: []ToolEvent{{
			ToolName: "read_workspace_text",
			Status:   "success",
			Path:     "draft.txt",
		}},
	}
	result, isError, completed := runtime.handleCompletionAttempt(`{"result":"已修改 draft.txt。","changed_paths":["draft.txt"]}`)
	if !isError || completed {
		t.Fatalf("expected unverified changed_paths to be rejected, isError=%v completed=%v result=%s", isError, completed, result)
	}
	if !strings.Contains(result, "unverified workspace changes") {
		t.Fatalf("expected unverified changed_paths rejection, got %s", result)
	}
}

func TestHandleCompletionAttemptAllowsHonestUnsupportedFileFailure(t *testing.T) {
	runtime := &agentRuntime{}
	runtime.recordToolError("read_workspace_text", `{"path":"demo.docx"}`, "unsupported file type")
	_, isError, completed := runtime.handleCompletionAttempt(`{"result":"当前 workspace 文本工具不支持读取 demo.docx。","failed_items":["demo.docx"]}`)
	if isError || !completed {
		t.Fatalf("expected non-recoverable unsupported-file failure to allow honest completion, isError=%v completed=%v", isError, completed)
	}
}

func TestHandleCompletionAttemptRejectsToolLoopCompletionBeforeAnyEvidence(t *testing.T) {
	runtime := &agentRuntime{toolLoopActive: true}
	result, isError, completed := runtime.handleCompletionAttempt(`{"result":"完成"}`)
	if !isError || completed {
		t.Fatalf("expected completion before tool evidence to be rejected, isError=%v completed=%v result=%s", isError, completed, result)
	}
	if !strings.Contains(result, "还没有执行任何工具") {
		t.Fatalf("expected missing tool rejection, got %s", result)
	}
}

func TestHandleCompletionAttemptAllowsPrefetchedWorkspaceFileContent(t *testing.T) {
	runtime := &agentRuntime{
		toolLoopActive: true,
		attachments: []AttachmentPayload{{
			Kind:              "workspace_file",
			Path:              "draft.txt",
			Text:              "path=draft.txt\nnumbered_content:\n1: hello",
			IncludedInContext: true,
		}},
	}
	result, isError, completed := runtime.handleCompletionAttempt(`{"result":"draft.txt 的内容是 hello。"}`)
	if isError || !completed {
		t.Fatalf("expected prefetched read-only completion, isError=%v completed=%v result=%s", isError, completed, result)
	}
	if !strings.Contains(runtime.completedContent, "hello") {
		t.Fatalf("expected completion content to be preserved, got %q", runtime.completedContent)
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
	if _, ok := props["changed_paths"]; !ok {
		t.Fatalf("attempt_completion schema must not expose changed_paths; backend derives verified paths")
	}
}

func TestModelActionManifestExposesPatchAsTextAction(t *testing.T) {
	manifest := modelActionManifest(map[string]bool{"workspace": true}, true, "admin")
	if !containsModelJSONTool(manifest, "read_workspace_text") || !containsModelJSONTool(manifest, "manage_workspace_path") {
		t.Fatalf("workspace JSON tools missing from action manifest: %#v", manifest.JSONTools)
	}
	if containsModelTextAction(manifest, "apply_workspace_patch") {
		t.Fatalf("apply_workspace_patch must be a model-visible text action: %#v", manifest.TextActions)
	}
	if !containsModelTerminalAction(manifest, "attempt_completion") {
		t.Fatalf("attempt_completion must be terminal action: %#v", manifest.TerminalActions)
	}
	if !containsModelJSONTool(manifest, "apply_workspace_patch") {
		t.Fatalf("apply_workspace_patch must also be a JSON tool action: %#v", manifest.JSONTools)
	}
}

func TestModelActionManifestCanExposeNativePatchTool(t *testing.T) {
	manifest := modelActionManifestForProvider(map[string]bool{"workspace": true}, true, "admin", true)
	if !containsModelJSONTool(manifest, "apply_workspace_patch") {
		t.Fatalf("native patch provider should expose apply_workspace_patch JSON tool: %#v", manifest.JSONTools)
	}
	if containsModelTextAction(manifest, "apply_workspace_patch") {
		t.Fatalf("fallback text action should remain documented: %#v", manifest.TextActions)
	}
}

func containsModelJSONTool(manifest ModelActionManifest, name string) bool {
	for _, item := range manifest.JSONTools {
		if item.Name == name {
			return true
		}
	}
	return false
}

func containsModelTextAction(manifest ModelActionManifest, name string) bool {
	for _, item := range manifest.TextActions {
		if item.Name == name {
			return true
		}
	}
	return false
}

func containsModelTerminalAction(manifest ModelActionManifest, name string) bool {
	for _, item := range manifest.TerminalActions {
		if item.Name == name {
			return true
		}
	}
	return false
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

func TestProviderRetryPolicyDefaultsAndEnv(t *testing.T) {
	t.Setenv("PERSEUS_PROVIDER_REQUEST_MAX_ATTEMPTS", "")
	t.Setenv("PERSEUS_PROVIDER_RETRY_BASE_DELAY_MS", "")
	t.Setenv("PERSEUS_PROVIDER_RETRY_MAX_DELAY_MS", "")
	policy := currentProviderRetryPolicy()
	if policy.MaxAttempts != 3 {
		t.Fatalf("expected default provider max attempts to be 3, got %d", policy.MaxAttempts)
	}
	if policy.BaseDelay != 750*time.Millisecond {
		t.Fatalf("expected default provider base delay 750ms, got %s", policy.BaseDelay)
	}
	if policy.MaxDelay != 5*time.Second {
		t.Fatalf("expected default provider max delay 5s, got %s", policy.MaxDelay)
	}

	t.Setenv("PERSEUS_PROVIDER_REQUEST_MAX_ATTEMPTS", "0")
	t.Setenv("PERSEUS_PROVIDER_RETRY_BASE_DELAY_MS", "9000")
	t.Setenv("PERSEUS_PROVIDER_RETRY_MAX_DELAY_MS", "1000")
	policy = currentProviderRetryPolicy()
	if policy.MaxAttempts != 1 {
		t.Fatalf("expected provider max attempts floor to 1, got %d", policy.MaxAttempts)
	}
	if policy.BaseDelay != time.Second || policy.MaxDelay != time.Second {
		t.Fatalf("expected base delay to be clamped to max delay, got base=%s max=%s", policy.BaseDelay, policy.MaxDelay)
	}
}

func TestProviderRetryAfterParsing(t *testing.T) {
	if got := parseRetryAfter("1.5", time.Now()); got != 1500*time.Millisecond {
		t.Fatalf("expected 1.5s retry-after, got %s", got)
	}
	now := time.Date(2026, 6, 2, 1, 2, 3, 0, time.UTC)
	if got := parseRetryAfter(now.Add(2*time.Second).Format(http.TimeFormat), now); got != 2*time.Second {
		t.Fatalf("expected http-date retry-after, got %s", got)
	}
}

func TestProviderStreamFallbackEnabledDefaultAndEnv(t *testing.T) {
	t.Setenv("PERSEUS_PROVIDER_STREAM_FALLBACK_ENABLED", "")
	if !providerStreamFallbackEnabled() {
		t.Fatal("expected stream fallback to be enabled by default")
	}
	t.Setenv("PERSEUS_PROVIDER_STREAM_FALLBACK_ENABLED", "false")
	if providerStreamFallbackEnabled() {
		t.Fatal("expected stream fallback to be disabled by env")
	}
	t.Setenv("PERSEUS_PROVIDER_STREAM_FALLBACK_ENABLED", "0")
	if !providerStreamFallbackEnabled() {
		t.Fatal("only explicit false should disable stream fallback")
	}
}

func TestProviderPlainEOFIsRecoverableTransportError(t *testing.T) {
	if !isTransientTransportError(io.EOF) {
		t.Fatal("plain io.EOF should be treated as a transient provider transport error")
	}
	if !isTransientTransportError(fmt.Errorf("provider stream read: %w", io.EOF)) {
		t.Fatal("wrapped io.EOF should be treated as a transient provider transport error")
	}
	err := fmt.Errorf(`Post "https://api.jellix.top/v1/chat/completions": EOF`)
	if !isCheckpointRecoverableProviderError(err) {
		t.Fatal("wrapped provider EOF should be checkpoint recoverable")
	}
	if got := providerFailureClass(err, ProviderRequestSummary{}); got != "provider_transport_eof" {
		t.Fatalf("expected provider_transport_eof, got %q", got)
	}
	if got := sanitizeUserFacingAIError(err.Error()); got != "模型供应商连接中断，本轮请求没有拿到有效响应" {
		t.Fatalf("unexpected sanitized EOF message: %q", got)
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

func TestExecuteToolApplyWorkspacePatchAddUpdateDelete(t *testing.T) {
	service, _, _, files := newTestAIService(t)
	ctx := context.Background()

	if _, err := files.CreateFile(ctx, "draft.txt", "one\ntwo\nthree\n"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}
	if _, err := files.CreateFile(ctx, "old.txt", "remove me\n"); err != nil {
		t.Fatalf("CreateFile old returned error: %v", err)
	}
	patch := "*** Begin Patch\n" +
		"*** Add File: notes/new.md\n" +
		"+# Title\n" +
		"+body\n" +
		"*** Update File: draft.txt\n" +
		"@@\n" +
		" one\n" +
		"-two\n" +
		"+TWO\n" +
		" three\n" +
		"*** Delete File: old.txt\n" +
		"*** End Patch"
	args := mustJSON(map[string]any{"patch": patch})
	result, _, _, path, err := service.executeTool(ctx, "apply_workspace_patch", args, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("executeTool apply_workspace_patch returned error: %v result=%s", err, result)
	}
	if path == "" {
		t.Fatalf("expected primary path")
	}
	created, err := files.ReadFile("notes/new.md")
	if err != nil || created.Content != "# Title\nbody\n" {
		t.Fatalf("unexpected created file: content=%q err=%v", created.Content, err)
	}
	updated, err := files.ReadFile("draft.txt")
	if err != nil || updated.Content != "one\nTWO\nthree\n" {
		t.Fatalf("unexpected updated file: content=%q err=%v", updated.Content, err)
	}
	if _, err := files.ReadFile("old.txt"); err == nil {
		t.Fatal("expected deleted file to be moved out of normal workspace reads")
	}
	var payload struct {
		ToolName     string   `json:"tool_name"`
		ChangedPaths []string `json:"changed_paths"`
		DiffSummary  string   `json:"diff_summary"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("invalid result JSON: %v", err)
	}
	if payload.ToolName != "apply_workspace_patch" || len(payload.ChangedPaths) == 0 || !strings.Contains(payload.DiffSummary, "added") {
		t.Fatalf("unexpected patch payload: %#v", payload)
	}
}

func TestExecuteToolApplyWorkspacePatchMergesMultiplePatchBlocksAtomically(t *testing.T) {
	service, _, _, files := newTestAIService(t)
	ctx := context.Background()

	patch := "准备写入。\n" +
		"*** Begin Patch\n" +
		"*** Add File: first.txt\n" +
		"+first\n" +
		"*** End Patch\n" +
		"继续第二份。\n" +
		"*** Begin Patch\n" +
		"*** Add File: second.txt\n" +
		"+second\n" +
		"*** End Patch\n"
	args := mustJSON(map[string]any{"patch": patch})
	if _, _, _, _, err := service.executeTool(ctx, "apply_workspace_patch", args, RunOptions{UserID: testUserID}); err != nil {
		t.Fatalf("executeTool apply_workspace_patch returned error: %v", err)
	}
	for path, want := range map[string]string{"first.txt": "first\n", "second.txt": "second\n"} {
		read, err := files.ReadFile(path)
		if err != nil || read.Content != want {
			t.Fatalf("expected %s=%q, got %q err=%v", path, want, read.Content, err)
		}
	}
}

func TestExecuteToolApplyWorkspacePatchMultipleBlocksRollbackOnFailure(t *testing.T) {
	service, _, _, files := newTestAIService(t)
	ctx := context.Background()

	patch := "*** Begin Patch\n" +
		"*** Add File: created.txt\n" +
		"+created\n" +
		"*** End Patch\n" +
		"*** Begin Patch\n" +
		"*** Update File: missing.txt\n" +
		"@@\n" +
		"-old\n" +
		"+new\n" +
		"*** End Patch\n"
	args := mustJSON(map[string]any{"patch": patch})
	if _, _, _, _, err := service.executeTool(ctx, "apply_workspace_patch", args, RunOptions{UserID: testUserID}); err == nil {
		t.Fatal("expected multi-block patch failure")
	}
	if _, err := files.ReadFile("created.txt"); err == nil {
		t.Fatal("created file should have been rolled back after later block failed")
	}
}

func TestExecuteToolApplyWorkspacePatchRejectsAmbiguousHunkWithoutPartialWrite(t *testing.T) {
	service, _, _, files := newTestAIService(t)
	ctx := context.Background()

	if _, err := files.CreateFile(ctx, "draft.txt", "same\nsame\n"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}
	patch := "*** Begin Patch\n" +
		"*** Update File: draft.txt\n" +
		"@@\n" +
		"-same\n" +
		"+changed\n" +
		"*** End Patch"
	args := mustJSON(map[string]any{"patch": patch})
	_, _, _, _, err := service.executeTool(ctx, "apply_workspace_patch", args, RunOptions{UserID: testUserID})
	if err == nil || !strings.Contains(err.Error(), "matched 2 times") {
		t.Fatalf("expected ambiguous hunk error, got %v", err)
	}
	var patchErr workspacePatchApplyError
	if !errorAsWorkspacePatchApplyError(err, &patchErr) || patchErr.Path != "draft.txt" || patchErr.OperationIndex != 1 || patchErr.HunkIndex != 1 || patchErr.Reason == "" {
		t.Fatalf("expected structured patch error, got %#v", err)
	}
	read, err := files.ReadFile("draft.txt")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if read.Content != "same\nsame\n" {
		t.Fatalf("patch should not partially modify file, got %q", read.Content)
	}
}

func TestExecuteAndRecordToolApplyWorkspacePatchFailureKeepsStructuredPayload(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	if _, err := env.files.CreateFile(ctx, "draft.txt", "alpha\n"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}
	patch := "*** Begin Patch\n" +
		"*** Update File: draft.txt\n" +
		"@@\n" +
		"-missing\n" +
		"+beta\n" +
		"*** End Patch"
	result, event, err := env.service.executeAndRecordTool(ctx, "conv-1", "msg-1", "apply_workspace_patch", mustJSON(map[string]any{
		"patch":     patch,
		"transport": "assistant_text_patch",
	}), "", RunOptions{UserID: testUserID}, runtimeProfileForProvider(store.ModelProvider{}))
	if err != nil {
		t.Fatalf("executeAndRecordTool returned execution error: %v", err)
	}
	if event.Status != "failed" {
		t.Fatalf("expected failed event, got %#v", event)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("invalid tool result JSON: %v\n%s", err, result)
	}
	for _, key := range []string{"applied", "files_changed", "failure_stage", "path", "operation_index", "hunk_index", "reason", "patch_paths"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("expected structured patch failure field %q in %#v", key, payload)
		}
	}
	if payload["failure_stage"] != "patch_apply_verification" || payload["applied"] != false || payload["files_changed"] != false {
		t.Fatalf("unexpected structured patch failure payload: %#v", payload)
	}
	hint, _ := payload["recovery_hint"].(string)
	if !strings.Contains(hint, "patch was captured") || !strings.Contains(hint, "*** Begin Patch") {
		t.Fatalf("unexpected recovery hint: %q", hint)
	}
	var resultJSON string
	if err := env.db.QueryRowContext(ctx, `SELECT result_json FROM tool_calls WHERE tool_name = 'apply_workspace_patch' AND status = 'failed'`).Scan(&resultJSON); err != nil {
		t.Fatalf("QueryRowContext returned error: %v", err)
	}
	if !strings.Contains(resultJSON, "patch_apply_verification") || !strings.Contains(resultJSON, "hunk_index") {
		t.Fatalf("ledger lost structured failure payload: %s", resultJSON)
	}
	read, err := env.files.ReadFile("draft.txt")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if read.Content != "alpha\n" {
		t.Fatalf("patch failure should not modify file, got %q", read.Content)
	}
}

func TestExecuteAndRecordToolWorkspaceMutationMarksTypedChangeEvent(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	patch := "*** Begin Patch\n" +
		"*** Add File: typed-change.md\n" +
		"+hello\n" +
		"*** End Patch"
	var changedPayload map[string]any
	_, event, err := env.service.executeAndRecordTool(ctx, "conv-typed", "msg-typed", "apply_workspace_patch", mustJSON(map[string]any{
		"patch":     patch,
		"transport": "assistant_text_patch",
	}), "", RunOptions{
		UserID: testUserID,
		Event: func(eventType, level string, payload map[string]any) {
			if eventType == "workspace_file_changed" {
				changedPayload = payload
			}
		},
	}, runtimeProfileForProvider(store.ModelProvider{}))
	if err != nil {
		t.Fatalf("executeAndRecordTool returned execution error: %v", err)
	}
	if event.Type != "workspace_file_changed" || event.Status != "success" {
		t.Fatalf("expected typed successful ToolEvent, got %#v", event)
	}
	if event.Operation != "patch" || !stringSliceContains(event.ChangedPaths, "typed-change.md") {
		t.Fatalf("expected patch change event, got %#v", event)
	}
	if changedPayload == nil {
		t.Fatal("expected workspace_file_changed lifecycle event")
	}
	if changedPayload["type"] != "workspace_file_changed" || changedPayload["operation"] != "patch" || !containsPathValueAny(changedPayload["changed_paths"], "typed-change.md") {
		t.Fatalf("unexpected workspace_file_changed payload: %#v", changedPayload)
	}
}

func TestExecuteAndRecordToolApplyWorkspacePatchParseFailureKeepsLocationPayload(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()

	patch := "*** Begin Patch\n" +
		"*** Add File: draft.txt\n" +
		"+hello\n" +
		"this line is missing a + prefix\n" +
		"*** End Patch\n"
	result, event, err := env.service.executeAndRecordTool(ctx, "conv-parse", "msg-parse", "apply_workspace_patch", mustJSON(map[string]any{
		"patch":     patch,
		"transport": "assistant_text_patch",
	}), "", RunOptions{UserID: testUserID}, runtimeProfileForProvider(store.ModelProvider{}))
	if err != nil {
		t.Fatalf("executeAndRecordTool returned execution error: %v", err)
	}
	if event.Status != "failed" {
		t.Fatalf("expected failed event, got %#v", event)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("invalid tool result JSON: %v\n%s", err, result)
	}
	for _, key := range []string{"reason", "failure_line", "failure_context", "candidate_paths", "patch_head", "patch_tail", "patch_chars", "patch_sha", "patch_block_count"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("expected parse failure payload field %q in %#v", key, payload)
		}
	}
	if payload["reason"] != "add file lines must start with +" {
		t.Fatalf("unexpected parse reason: %#v", payload)
	}
}

func TestExecuteToolApplyWorkspacePatchRollsBackEarlierWritesOnLaterFailure(t *testing.T) {
	service, _, _, files := newTestAIService(t)
	ctx := context.Background()

	if _, err := files.CreateFile(ctx, "existing.txt", "alpha\n"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}
	patch := "*** Begin Patch\n" +
		"*** Add File: created.txt\n" +
		"+created\n" +
		"*** Update File: existing.txt\n" +
		"@@\n" +
		"-missing\n" +
		"+beta\n" +
		"*** End Patch"
	args := mustJSON(map[string]any{"patch": patch})
	_, _, _, _, err := service.executeTool(ctx, "apply_workspace_patch", args, RunOptions{UserID: testUserID})
	if err == nil || !strings.Contains(err.Error(), "did not match") {
		t.Fatalf("expected patch failure, got %v", err)
	}
	if _, err := files.ReadFile("created.txt"); err == nil {
		t.Fatal("created file should have been rolled back")
	}
	read, err := files.ReadFile("existing.txt")
	if err != nil {
		t.Fatalf("ReadFile existing returned error: %v", err)
	}
	if read.Content != "alpha\n" {
		t.Fatalf("existing file should not change after rollback, got %q", read.Content)
	}
}

func TestExecuteToolApplyWorkspacePatchUsesContextAnchors(t *testing.T) {
	service, _, _, files := newTestAIService(t)
	ctx := context.Background()

	if _, err := files.CreateFile(ctx, "draft.txt", "## A\nsame\n\n## B\nsame\n"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}
	patch := "*** Begin Patch\n" +
		"*** Update File: draft.txt\n" +
		"@@ ## B\n" +
		"-same\n" +
		"+changed\n" +
		"*** End Patch"
	args := mustJSON(map[string]any{"patch": patch})
	if _, _, _, _, err := service.executeTool(ctx, "apply_workspace_patch", args, RunOptions{UserID: testUserID}); err != nil {
		t.Fatalf("executeTool apply_workspace_patch returned error: %v", err)
	}
	read, err := files.ReadFile("draft.txt")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if read.Content != "## A\nsame\n\n## B\nchanged\n" {
		t.Fatalf("unexpected anchored patch result: %q", read.Content)
	}
}

func TestExecuteToolApplyWorkspacePatchMoveAndEndOfFile(t *testing.T) {
	service, _, _, files := newTestAIService(t)
	ctx := context.Background()

	if _, err := files.CreateFile(ctx, "draft.txt", "one\ntwo\n"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}
	patch := "*** Begin Patch\n" +
		"*** Update File: draft.txt\n" +
		"*** Move to: final.txt\n" +
		"@@\n" +
		"-two\n" +
		"+TWO\n" +
		"*** End of File\n" +
		"*** End Patch"
	args := mustJSON(map[string]any{"patch": patch})
	if _, _, _, _, err := service.executeTool(ctx, "apply_workspace_patch", args, RunOptions{UserID: testUserID}); err != nil {
		t.Fatalf("executeTool apply_workspace_patch returned error: %v", err)
	}
	if _, err := files.ReadFile("draft.txt"); err == nil {
		t.Fatal("expected old path to be moved")
	}
	read, err := files.ReadFile("final.txt")
	if err != nil {
		t.Fatalf("ReadFile final returned error: %v", err)
	}
	if read.Content != "one\nTWO\n" {
		t.Fatalf("unexpected moved content: %q", read.Content)
	}
}

func TestExecuteToolApplyWorkspacePatchRejectsMoveTargetConflict(t *testing.T) {
	service, _, _, files := newTestAIService(t)
	ctx := context.Background()

	if _, err := files.CreateFile(ctx, "a.txt", "a\n"); err != nil {
		t.Fatalf("CreateFile a returned error: %v", err)
	}
	if _, err := files.CreateFile(ctx, "b.txt", "b\n"); err != nil {
		t.Fatalf("CreateFile b returned error: %v", err)
	}
	patch := "*** Begin Patch\n" +
		"*** Update File: a.txt\n" +
		"*** Move to: b.txt\n" +
		"@@\n" +
		"-a\n" +
		"+A\n" +
		"*** End Patch"
	args := mustJSON(map[string]any{"patch": patch})
	if _, _, _, _, err := service.executeTool(ctx, "apply_workspace_patch", args, RunOptions{UserID: testUserID}); err == nil || !strings.Contains(err.Error(), "move target already exists") {
		t.Fatalf("expected move target conflict, got %v", err)
	}
}

func TestApplyWorkspacePatchSelectedScopeRejectsOutsidePatchPath(t *testing.T) {
	runtime := &agentRuntime{opts: RunOptions{AllowedWorkspacePaths: []string{"selected.txt"}},
		attachments: []AttachmentPayload{{Kind: "workspace_file", Path: "selected.txt", Name: "selected.txt"}},
	}
	patch := "*** Begin Patch\n" +
		"*** Update File: other.txt\n" +
		"@@\n" +
		"-old\n" +
		"+new\n" +
		"*** End Patch"
	got := runtime.validateWorkspacePathScope(agentToolCall{
		Name:          "apply_workspace_patch",
		ArgumentsJSON: mustJSON(map[string]any{"patch": patch}),
	})
	if !strings.Contains(got, "outside") {
		t.Fatalf("expected outside-scope patch to be rejected, got %q", got)
	}
}

func TestApplyWorkspacePatchSelectedScopeAllowsNewMoveTarget(t *testing.T) {
	runtime := &agentRuntime{
		attachments: []AttachmentPayload{{Kind: "workspace_file", Path: "selected.txt", Name: "selected.txt"}},
	}
	patch := "*** Begin Patch\n" +
		"*** Update File: selected.txt\n" +
		"*** Move to: other.txt\n" +
		"@@\n" +
		"-old\n" +
		"+new\n" +
		"*** End Patch"
	got := runtime.validateWorkspacePathScope(agentToolCall{
		Name:          "apply_workspace_patch",
		ArgumentsJSON: mustJSON(map[string]any{"patch": patch}),
	})
	if got != "" {
		t.Fatalf("selected file should be movable to a new output path, got %q", got)
	}
}

func TestExecuteToolApplyWorkspacePatchRejectsAmbiguousContextAnchor(t *testing.T) {
	service, _, _, files := newTestAIService(t)
	ctx := context.Background()

	if _, err := files.CreateFile(ctx, "draft.txt", "## Scene\nsame\n\n## Scene\nsame\n"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}
	patch := "*** Begin Patch\n" +
		"*** Update File: draft.txt\n" +
		"@@ ## Scene\n" +
		"-same\n" +
		"+changed\n" +
		"*** End Patch"
	args := mustJSON(map[string]any{"patch": patch})
	if _, _, _, _, err := service.executeTool(ctx, "apply_workspace_patch", args, RunOptions{UserID: testUserID}); err == nil || !strings.Contains(err.Error(), "context") || !strings.Contains(err.Error(), "matched 2 times") {
		t.Fatalf("expected ambiguous context error, got %v", err)
	}
	read, err := files.ReadFile("draft.txt")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if read.Content != "## Scene\nsame\n\n## Scene\nsame\n" {
		t.Fatalf("patch should not partially modify file, got %q", read.Content)
	}
}

func TestExecuteToolWriteWorkspaceChunkAppendsLongFile(t *testing.T) {
	service, db, _, files := newTestAIService(t)

	if _, _, _, path, err := service.executeTool(context.Background(), "write_workspace_chunk", `{"path":"drafts/long.txt","content":"第一段\n","mode":"create","chunk_index":1}`, RunOptions{UserID: testUserID}); err != nil {
		t.Fatalf("create chunk returned error: %v", err)
	} else if path != "drafts/long.txt" {
		t.Fatalf("unexpected create path: %s", path)
	}
	if _, _, _, path, err := service.executeTool(context.Background(), "write_workspace_chunk", `{"path":"drafts/long.txt","content":"第二段","mode":"append","chunk_index":2}`, RunOptions{UserID: testUserID}); err != nil {
		t.Fatalf("append chunk returned error: %v", err)
	} else if path != "drafts/long.txt" {
		t.Fatalf("unexpected append path: %s", path)
	}

	readResult, err := files.ReadFile("drafts/long.txt")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if readResult.Content != "第一段\n第二段" {
		t.Fatalf("unexpected file content: %q", readResult.Content)
	}
	var versionCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM file_versions WHERE path = ?`, "drafts/long.txt").Scan(&versionCount); err != nil {
		t.Fatalf("query file_versions returned error: %v", err)
	}
	if versionCount != 1 {
		t.Fatalf("expected append to create one prior version, got %d", versionCount)
	}
}

func TestAgentRuntimeChunkWriteFinalizeSuccess(t *testing.T) {
	service, _, _, files := newTestAIService(t)
	runtime := &agentRuntime{
		service:          service,
		protocol:         agentProtocolOpenAI,
		conversationID:   "conv_chunk_success",
		messageID:        "msg_chunk_success",
		opts:             RunOptions{UserID: testUserID},
		loadedToolsets:   allAgentToolsets(true),
		appliedMutations: map[string]ToolEvent{},
		chunkWrites:      map[string]workspaceChunkWriteState{},
	}

	calls := []agentToolCall{
		{ID: "chunk_1", Name: "write_workspace_chunk", ArgumentsJSON: `{"path":"drafts/long.txt","content":"一\n","mode":"create","chunk_index":1}`},
		{ID: "chunk_2", Name: "write_workspace_chunk", ArgumentsJSON: `{"path":"drafts/long.txt","content":"二\n","mode":"append","chunk_index":2}`},
		{ID: "chunk_3", Name: "write_workspace_chunk", ArgumentsJSON: `{"path":"drafts/long.txt","content":"三","mode":"append","chunk_index":3}`},
		{ID: "finalize_1", Name: "finalize_workspace_write", ArgumentsJSON: `{"path":"drafts/long.txt","expected_chunks":3,"expected_lines":3,"expected_chars":5}`},
	}
	for _, call := range calls {
		content, isError, request, completed, err := runtime.processOneToolCall(context.Background(), call)
		if err != nil || isError || request != nil || completed {
			t.Fatalf("call %s failed: content=%s isError=%v request=%v completed=%v err=%v", call.Name, content, isError, request, completed, err)
		}
		if call.Name == "write_workspace_chunk" && !strings.Contains(content, `"needs_finalize":true`) {
			t.Fatalf("chunk result should tell model to finalize, got %s", content)
		}
	}
	if _, ok := runtime.chunkWrites["drafts/long.txt"]; ok {
		t.Fatalf("expected finalize to clear chunk write state")
	}
	readResult, err := files.ReadFile("drafts/long.txt")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if readResult.Content != "一\n二\n三" {
		t.Fatalf("unexpected file content: %q", readResult.Content)
	}
	if last := runtime.events[len(runtime.events)-1]; last.ToolName != "finalize_workspace_write" || last.Status != "success" || last.Path != "drafts/long.txt" {
		t.Fatalf("unexpected final event: %#v", last)
	}
}

func TestAgentRuntimeChunkWriteRejectsOutOfOrderAppend(t *testing.T) {
	service, _, _, _ := newTestAIService(t)
	runtime := &agentRuntime{
		service:          service,
		protocol:         agentProtocolOpenAI,
		conversationID:   "conv_chunk_order",
		messageID:        "msg_chunk_order",
		opts:             RunOptions{UserID: testUserID},
		loadedToolsets:   allAgentToolsets(true),
		appliedMutations: map[string]ToolEvent{},
		chunkWrites: map[string]workspaceChunkWriteState{
			"drafts/long.txt": {Path: "drafts/long.txt", ChunksWritten: 1, TotalChars: 2},
		},
	}

	content, isError, _, _, err := runtime.processOneToolCall(context.Background(), agentToolCall{
		ID:            "chunk_3",
		Name:          "write_workspace_chunk",
		ArgumentsJSON: `{"path":"drafts/long.txt","content":"三","mode":"append","chunk_index":3}`,
	})
	if err != nil || !isError || !strings.Contains(content, "chunk_index must be 2") {
		t.Fatalf("expected out-of-order chunk error, content=%s isError=%v err=%v", content, isError, err)
	}
}

func TestAgentRuntimeFinalizeRejectsMissingChunkState(t *testing.T) {
	service, _, _, files := newTestAIService(t)
	if _, err := files.CreateFile(context.Background(), "drafts/long.txt", "一\n二"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}
	runtime := &agentRuntime{
		service:          service,
		protocol:         agentProtocolOpenAI,
		conversationID:   "conv_finalize_missing",
		messageID:        "msg_finalize_missing",
		opts:             RunOptions{UserID: testUserID},
		loadedToolsets:   allAgentToolsets(true),
		appliedMutations: map[string]ToolEvent{},
		chunkWrites:      map[string]workspaceChunkWriteState{},
	}

	content, isError, _, _, err := runtime.processOneToolCall(context.Background(), agentToolCall{
		ID:            "finalize_1",
		Name:          "finalize_workspace_write",
		ArgumentsJSON: `{"path":"drafts/long.txt","expected_chunks":1,"expected_lines":2,"expected_chars":3}`,
	})
	if err != nil || !isError || !strings.Contains(content, "no chunked write state") {
		t.Fatalf("expected missing chunk state error, content=%s isError=%v err=%v", content, isError, err)
	}
}

func TestExecuteFinalizeWorkspaceWriteRejectsMismatchedCounts(t *testing.T) {
	service, _, _, files := newTestAIService(t)
	if _, err := files.CreateFile(context.Background(), "drafts/long.txt", "一\n二"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}

	result, _, _, _, err := service.executeTool(context.Background(), "finalize_workspace_write", `{"path":"drafts/long.txt","expected_chunks":1,"expected_lines":3,"expected_chars":3}`, RunOptions{UserID: testUserID})
	if err == nil || !strings.Contains(err.Error(), "expected_lines mismatch") {
		t.Fatalf("expected line mismatch error, got result=%s err=%v", result, err)
	}
	if !strings.Contains(result, `"actual_lines":2`) || !strings.Contains(result, `"expected_lines":3`) {
		t.Fatalf("expected structured mismatch payload, got %s", result)
	}

	result, _, _, _, err = service.executeTool(context.Background(), "finalize_workspace_write", `{"path":"drafts/long.txt","expected_chunks":1,"expected_lines":2,"expected_chars":4}`, RunOptions{UserID: testUserID})
	if err == nil || !strings.Contains(err.Error(), "expected_chars mismatch") {
		t.Fatalf("expected char mismatch error, got result=%s err=%v", result, err)
	}

	result, _, _, _, err = service.executeTool(context.Background(), "finalize_workspace_write", `{"path":"drafts/long.txt","expected_chunks":1,"expected_lines":2,"expected_chars":3,"final_hash":"bad"}`, RunOptions{UserID: testUserID})
	if err == nil || !strings.Contains(err.Error(), "final_hash mismatch") {
		t.Fatalf("expected hash mismatch error, got result=%s err=%v", result, err)
	}
}

func TestExecuteToolReplaceWorkspaceLinesUsesLineRange(t *testing.T) {
	service, db, _, files := newTestAIService(t)
	if _, err := files.CreateFile(context.Background(), "script.txt", "第19集\n旧A\n旧B\n第19集\n保留\n"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}

	result, _, _, path, err := service.executeTool(context.Background(), "replace_workspace_lines", `{"path":"script.txt","start_line":2,"end_line":3,"new_text":"新A\n新B\n"}`, RunOptions{UserID: testUserID})
	if err != nil {
		t.Fatalf("executeTool returned error: %v", err)
	}
	if path != "script.txt" {
		t.Fatalf("unexpected path: %s", path)
	}
	readResult, err := files.ReadFile("script.txt")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if readResult.Content != "第19集\n新A\n新B\n第19集\n保留\n" {
		t.Fatalf("unexpected file content: %q", readResult.Content)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("invalid result json: %v", err)
	}
	if payload["operation"] != "replace_lines" || int(payload["start_line"].(float64)) != 2 || int(payload["end_line"].(float64)) != 3 {
		t.Fatalf("unexpected result payload: %#v", payload)
	}
	var versionCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM file_versions WHERE path = ?`, "script.txt").Scan(&versionCount); err != nil {
		t.Fatalf("query file_versions returned error: %v", err)
	}
	if versionCount != 1 {
		t.Fatalf("expected line replace to create one prior version, got %d", versionCount)
	}
}

func TestExecuteToolReplaceWorkspaceLinesRejectsInvalidRange(t *testing.T) {
	service, _, _, files := newTestAIService(t)
	if _, err := files.CreateFile(context.Background(), "script.txt", "一\n二\n"); err != nil {
		t.Fatalf("CreateFile returned error: %v", err)
	}

	_, _, _, _, err := service.executeTool(context.Background(), "replace_workspace_lines", `{"path":"script.txt","start_line":3,"end_line":4,"new_text":"三\n"}`, RunOptions{UserID: testUserID})
	if err == nil || !strings.Contains(err.Error(), "line range") {
		t.Fatalf("expected invalid line range error, got %v", err)
	}
}

func TestExecuteToolWriteWorkspaceChunkRequiresFirstChunkIndexForCreate(t *testing.T) {
	service, _, _, _ := newTestAIService(t)

	_, _, _, _, err := service.executeTool(context.Background(), "write_workspace_chunk", `{"path":"drafts/long.txt","content":"第一段","mode":"create","chunk_index":2}`, RunOptions{UserID: testUserID})
	if err == nil || err.Error() != "invalid tool arguments: create chunk_index must be 1" {
		t.Fatalf("expected first chunk validation error, got %v", err)
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

func TestExecuteAndRecordToolMkdirExistingFolderReturnsStructuredRecoverableObservation(t *testing.T) {
	service, _, _, files := newTestAIService(t)
	if _, err := files.MkdirNew("docs/guides"); err != nil {
		t.Fatalf("MkdirNew returned error: %v", err)
	}

	result, event, err := service.executeAndRecordTool(context.Background(), "conv", "msg", "manage_workspace_path", `{"action":"mkdir","path":"docs/guides"}`, "", RunOptions{UserID: testUserID}, providerRuntimeProfile{MaxToolResultRunes: 4000})
	if err != nil {
		t.Fatalf("executeAndRecordTool returned wrapper error: %v", err)
	}
	if event.Status != "failed" || event.Error != "path already exists" {
		t.Fatalf("expected failed already_exists event, got %#v", event)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("invalid result json: %v\n%s", err, result)
	}
	if payload["reason"] != "already_exists" || payload["path_type"] != "folder" || payload["path"] != "docs/guides" {
		t.Fatalf("expected structured already_exists observation, got %#v", payload)
	}
	if payload["ok"] != false || payload["status"] != "failed" {
		t.Fatalf("failed already_exists observation should not claim success, got %#v", payload)
	}
	if _, ok := payload["changed_paths"]; ok {
		t.Fatalf("failed mkdir should not report changed_paths: %#v", payload)
	}
	if !strings.Contains(fmt.Sprint(payload["recovery_hint"]), "Continue using this directory") {
		t.Fatalf("expected recovery hint to continue using existing folder, got %#v", payload["recovery_hint"])
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
	t.Setenv("PERSEUS_COMMAND_EXECUTION", "trusted_host")
	service, _, _, _ := newTestAIService(t)

	// Exercise the shell without Go's detached telemetry process writing into
	// the isolated command home while t.TempDir removes the test directory.
	const command = "echo perseus-command-ok"
	args, _ := json.Marshal(map[string]any{"command": command, "timeout_sec": 30})
	result, _, _, path, err := service.executeTool(context.Background(), "run_workspace_command", string(args), RunOptions{UserID: testUserID, UserRole: "admin"})
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
	if payload["command"] != command || payload["timed_out"] != false {
		t.Fatalf("unexpected command payload: %#v", payload)
	}
	if int(payload["exit_code"].(float64)) != 0 {
		t.Fatalf("expected exit code 0, got %#v", payload["exit_code"])
	}
	if output, ok := payload["stdout"].(string); !ok || strings.TrimSpace(output) != "perseus-command-ok" {
		t.Fatalf("unexpected command stdout: %#v", payload["stdout"])
	}
	if _, ok := payload["changed_paths"]; ok {
		t.Fatalf("read-only command should not report changed paths: %#v", payload)
	}
}

func TestExecuteAndRecordRunWorkspaceCommandEmitsWorkspaceChangeWhenFilesChange(t *testing.T) {
	t.Setenv("PERSEUS_COMMAND_EXECUTION", "trusted_host")
	service, _, _, files := newTestAIService(t)
	ctx := context.Background()
	command := "printf hello > command-output.txt"
	if runtime.GOOS == "windows" {
		command = "Set-Content -NoNewline -Path command-output.txt -Value hello"
	}
	args, _ := json.Marshal(map[string]any{"command": command, "timeout_sec": 30})
	var events []string
	var changed map[string]any
	result, event, err := service.executeAndRecordTool(ctx, "conv-test", "msg-test", "run_workspace_command", string(args), "", RunOptions{
		UserID: testUserID, UserRole: "admin",
		Event: func(eventType, _ string, payload map[string]any) {
			events = append(events, eventType)
			if eventType == "workspace_file_changed" {
				changed = payload
			}
		},
	}, providerRuntimeProfile{})
	if err != nil {
		t.Fatalf("executeAndRecordTool returned error: %v result=%s", err, result)
	}
	if event.Status != "success" || len(event.ChangedPaths) != 1 || event.ChangedPaths[0] != "command-output.txt" {
		t.Fatalf("expected command mutation event, got %#v", event)
	}
	if changed == nil || !containsAnyPath(changed["changed_paths"], "command-output.txt") || changed["tool_name"] != "run_workspace_command" || changed["operation"] != "command_unknown" {
		t.Fatalf("expected workspace_file_changed event for command mutation, events=%#v changed=%#v", events, changed)
	}
	read, err := files.ReadFile("command-output.txt")
	if err != nil || read.Content != "hello" {
		t.Fatalf("expected command-created file, content=%q err=%v", read.Content, err)
	}
}

func containsAnyPath(value any, expected string) bool {
	switch items := value.(type) {
	case []string:
		for _, item := range items {
			if item == expected {
				return true
			}
		}
	case []any:
		for _, item := range items {
			if item == expected {
				return true
			}
		}
	}
	return containsPathValue(value, expected)
}

func TestCommandWorkspaceMutationRequiresCompletionChangedPath(t *testing.T) {
	runtime := &agentRuntime{
		performedMutation: true,
		events: []ToolEvent{{
			ToolName:     "run_workspace_command",
			Status:       "success",
			ChangedPaths: []string{"command-output.txt"},
		}},
	}
	_, isError, completed := runtime.handleCompletionAttempt(`{"result":"完成","changed_paths":["other.txt"]}`)
	if !isError || completed {
		t.Fatal("expected completion to be rejected when command-changed path is missing")
	}
	_, isError, completed = runtime.handleCompletionAttempt(`{"result":"完成","changed_paths":["command-output.txt"]}`)
	if isError || !completed {
		t.Fatal("expected completion to accept declared command-changed path")
	}
}

func TestExecuteToolRunWorkspaceCommandRejectsDangerousCommand(t *testing.T) {
	t.Setenv("PERSEUS_COMMAND_EXECUTION", "")
	service, _, _, _ := newTestAIService(t)

	_, _, _, _, err := service.executeTool(context.Background(), "run_workspace_command", `{"command":"echo blocked","timeout_sec":30}`, RunOptions{UserID: testUserID, UserRole: "admin"})
	if err == nil || failure.Code(err) != "command_disabled" {
		t.Fatalf("expected safety policy error, got %v", err)
	}
}

func TestExecuteToolRunWorkspaceCommandRejectsPathEscape(t *testing.T) {
	t.Setenv("PERSEUS_COMMAND_EXECUTION", "trusted_host")
	service, _, _, _ := newTestAIService(t)

	_, _, _, _, err := service.executeTool(context.Background(), "run_workspace_command", `{"command":"go version","cwd":"../outside","timeout_sec":30}`, RunOptions{UserID: testUserID, UserRole: "admin"})
	if err == nil || err.Error() != "path escapes workspace" {
		t.Fatalf("expected path escape error, got %v", err)
	}
}

func TestExecuteToolRunWorkspaceCommandRejectsAbsoluteCwd(t *testing.T) {
	t.Setenv("PERSEUS_COMMAND_EXECUTION", "trusted_host")
	service, _, _, _ := newTestAIService(t)

	_, _, _, _, err := service.executeTool(context.Background(), "run_workspace_command", `{"command":"go version","cwd":"/tmp","timeout_sec":30}`, RunOptions{UserID: testUserID, UserRole: "admin"})
	if err == nil || err.Error() != "invalid path" {
		t.Fatalf("expected invalid path error, got %v", err)
	}
}

func TestExecuteToolRunWorkspaceCommandTimesOut(t *testing.T) {
	t.Setenv("PERSEUS_COMMAND_EXECUTION", "trusted_host")
	service, _, _, _ := newTestAIService(t)
	command := "sleep 2"
	if runtime.GOOS == "windows" {
		command = "Start-Sleep -Seconds 2"
	}
	args, _ := json.Marshal(map[string]any{
		"command":     command,
		"timeout_sec": 1,
	})

	result, _, _, _, err := service.executeTool(context.Background(), "run_workspace_command", string(args), RunOptions{UserID: testUserID, UserRole: "admin"})
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
	for _, tc := range []struct {
		name, code string
		err        error
	}{
		{"read_workspace_text", "workspace_path_not_found", fmt.Errorf("read wrapped: %w", os.ErrNotExist)},
		{"manage_workspace_path", "workspace_path_policy", workspace.ErrTrashManagedOnly},
		{"edit_workspace_text", "workspace_exact_edit_unstable", failure.New("workspace_exact_edit_unstable", "arbitrary translated text", nil)},
		{"apply_workspace_patch", "workspace_patch_apply_failed", workspacePatchApplyError{Message: "arbitrary translated text"}},
	} {
		diag := buildToolDiagnostic(tc.name, "{}", "", 0, tc.err)
		if !strings.Contains(diag.RequestSummary, tc.code) {
			t.Fatalf("lost identity: %s", diag.RequestSummary)
		}
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
	if !strings.Contains(errText, "requires path") {
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

func TestExecuteToolSaveWorkspaceFileRejectsOversizedContent(t *testing.T) {
	service, _, _, _ := newTestAIService(t)
	raw := mustJSON(map[string]any{
		"path":    "long.txt",
		"content": strings.Repeat("长", recommendedWorkspaceChunkChars+1),
		"mode":    "create",
	})

	_, _, _, _, err := service.executeTool(context.Background(), "save_workspace_file", raw, RunOptions{UserID: testUserID})
	if err == nil || !strings.Contains(err.Error(), "*** Begin Patch") {
		t.Fatalf("expected oversized content execution guard, got %v", err)
	}
}

func TestValidateToolCallRejectsMissingChunkIndex(t *testing.T) {
	runtime := &agentRuntime{
		protocol:       agentProtocolOpenAI,
		loadedToolsets: allAgentToolsets(true),
	}

	errText := runtime.validateToolCall(agentToolCall{
		ID:            "call_1",
		Name:          "write_workspace_chunk",
		ArgumentsJSON: `{"path":"long.txt","content":"x","mode":"append"}`,
	})
	if !strings.Contains(errText, "requires chunk_index") {
		t.Fatalf("expected chunk_index validation error, got %q", errText)
	}
}

func TestSelectedReferenceScopeAllowsAppendingEarlierGeneratedChunkFile(t *testing.T) {
	runtime := &agentRuntime{
		attachments: []AttachmentPayload{{Kind: "workspace_file", Path: "source.txt"}},
		events: []ToolEvent{{
			ToolName:     "write_workspace_chunk",
			Status:       "success",
			Path:         "drafts/output.txt",
			ChangedPaths: []string{"drafts/output.txt"},
		}},
	}

	errText := runtime.validateWorkspacePathScope(agentToolCall{
		Name:          "write_workspace_chunk",
		ArgumentsJSON: `{"path":"drafts/output.txt","content":"next","mode":"append","chunk_index":2}`,
	})
	if errText != "" {
		t.Fatalf("expected append to generated output to stay in scope, got %q", errText)
	}
}

func TestExplicitScopeRejectsAppendingUnrelatedChunkFile(t *testing.T) {
	r := &agentRuntime{opts: RunOptions{AllowedWorkspacePaths: []string{"selected.md"}}}
	if r.validateWorkspacePathScope(agentToolCall{Name: "write_workspace_chunk", ArgumentsJSON: `{"path":"other.md","mode":"append","content":"x","chunk_index":1}`}) == "" {
		t.Fatal("explicit scope bypass")
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

func TestBudgetToolResultForModelUsesSingleNumberedReadView(t *testing.T) {
	raw := mustJSON(map[string]any{
		"tool":             "read_workspace_text",
		"path":             "draft.txt",
		"content":          strings.Repeat("plain", 1200),
		"numbered_content": strings.Repeat("1: numbered\n", 1200),
		"start_line":       1,
		"end_line":         1200,
		"total_lines":      1200,
	})
	budgeted := budgetToolResultForModel("read_workspace_text", raw, providerRuntimeProfile{
		MaxToolResultRunes:      1200,
		MaxReadFileContentRunes: 10000,
	})

	var payload map[string]any
	if err := json.Unmarshal([]byte(budgeted), &payload); err != nil {
		t.Fatalf("budgeted result is not json: %v", err)
	}
	if _, ok := payload["content"]; ok {
		t.Fatalf("expected duplicate plain content to be omitted when numbered view exists: %#v", payload)
	}
	if payload["content_omitted_for_model"] != true {
		t.Fatalf("expected content omission flag, got %#v", payload)
	}
	if payload["numbered_content"] == nil {
		t.Fatalf("expected numbered content to remain as model view, got %#v", payload)
	}
	if payload["numbered_content_truncated_for_model"] != true {
		t.Fatalf("expected numbered content to be budgeted, got %#v", payload)
	}
	if got := len([]rune(budgeted)); got > 1200 {
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

func TestBudgetToolResultForModelBudgetsBatchNumberedContent(t *testing.T) {
	raw := mustJSON(map[string]any{
		"tool": "read_workspace_files",
		"items": []any{
			map[string]any{
				"path":             "a.txt",
				"content":          strings.Repeat("a", 2400),
				"numbered_content": strings.Repeat("1: a\n", 2400),
				"start_line":       1,
				"end_line":         2400,
			},
			map[string]any{
				"path":             "b.txt",
				"content":          strings.Repeat("b", 2400),
				"numbered_content": strings.Repeat("1: b\n", 2400),
				"start_line":       1,
				"end_line":         2400,
			},
		},
	})
	budgeted := budgetToolResultForModel("read_workspace_files", raw, providerRuntimeProfile{
		MaxToolResultRunes:      1800,
		MaxReadFileContentRunes: 10000,
	})

	var payload map[string]any
	if err := json.Unmarshal([]byte(budgeted), &payload); err != nil {
		t.Fatalf("budgeted result is not json: %v", err)
	}
	items := payload["items"].([]any)
	for _, rawItem := range items {
		item := rawItem.(map[string]any)
		if _, ok := item["content"]; ok {
			t.Fatalf("expected duplicate plain content omitted for batch item, got %#v", item)
		}
		if item["numbered_content"] == nil {
			t.Fatalf("expected numbered content model view for batch item, got %#v", item)
		}
	}
	if got := len([]rune(budgeted)); got > 1800 {
		t.Fatalf("expected payload within budget, got %d chars", got)
	}
}

func TestLedgerToolResultJSONKeepsLargeReadResultValid(t *testing.T) {
	raw := mustJSON(map[string]any{
		"tool_name": "read_workspace_files",
		"status":    "success",
		"items": []any{
			map[string]any{
				"path":             "a.txt",
				"numbered_content": strings.Repeat("1: a long line\n", 3000),
				"start_line":       1,
				"end_line":         3000,
				"total_lines":      3000,
				"truncated":        false,
			},
			map[string]any{
				"path":             "b.txt",
				"numbered_content": strings.Repeat("1: b long line\n", 3000),
				"start_line":       1,
				"end_line":         3000,
				"total_lines":      3000,
				"truncated":        true,
			},
		},
		"paths":     []string{"a.txt", "b.txt"},
		"truncated": true,
	})
	ledger := ledgerToolResultJSON("read_workspace_files", raw)
	if len([]rune(ledger)) > ledgerToolResultMaxRunes {
		t.Fatalf("expected ledger result under max size, got %d", len([]rune(ledger)))
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(ledger), &payload); err != nil {
		t.Fatalf("ledger result should be valid JSON: %v\n%s", err, ledger)
	}
	if payload["ledger_result_truncated"] != true {
		t.Fatalf("expected ledger truncation flag, got %#v", payload)
	}
	if payload["source_read_truncated"] != true {
		t.Fatalf("expected source read truncation flag, got %#v", payload)
	}
	items, ok := payload["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("expected item snapshots, got %#v", payload["items"])
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

func TestSanitizeConversationMessagesDropsAssistantEllipsisPlaceholders(t *testing.T) {
	messages := []store.Message{
		{Role: "user", Content: "..."},
		{Role: "assistant", Content: "..."},
		{Role: "assistant", Content: "我先读取文件。"},
		{Role: "assistant", Content: "……"},
	}
	sanitized := sanitizeConversationMessages(messages, providerRuntimeProfile{
		MaxHistoryMessages:    10,
		MaxRecentMessageRunes: 100,
		MaxOlderMessageRunes:  100,
	})

	if len(sanitized) != 2 {
		t.Fatalf("expected two non-placeholder messages, got %#v", sanitized)
	}
	if sanitized[0].Role != "user" || sanitized[0].Content != "..." {
		t.Fatalf("user ellipsis must be preserved, got %#v", sanitized[0])
	}
	if sanitized[1].Content != "我先读取文件。" {
		t.Fatalf("assistant useful text must be preserved, got %#v", sanitized[1])
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
		"write_workspace_chunk",
		"finalize_workspace_write",
		"replace_workspace_lines",
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

func TestReferencesAreContextAndExplicitScopeRestrictsTools(t *testing.T) {
	r := &agentRuntime{attachments: []AttachmentPayload{{Kind: "workspace_file", Path: "a.md"}}}
	call := agentToolCall{Name: "read_workspace_text", ArgumentsJSON: `{"path":"b.md"}`}
	if got := r.validateWorkspacePathScope(call); got != "" {
		t.Fatal("reference became permission: " + got)
	}
	r.opts.AllowedWorkspacePaths = []string{"a.md"}
	if got := r.validateWorkspacePathScope(call); got == "" {
		t.Fatal("explicit scope ignored")
	}
	for _, call := range []agentToolCall{{Name: "run_workspace_command", ArgumentsJSON: `{"command":"echo test"}`}, {Name: "save_workspace_file", ArgumentsJSON: `{"path":"b.md","mode":"create","content":"x"}`}} {
		if r.validateWorkspacePathScope(call) == "" {
			t.Fatal("scope bypass")
		}
	}
}

func TestWorkspaceToolNamesAndGroups(t *testing.T) {
	loaded := map[string]bool{"workspace": true}
	for _, name := range []string{"find_workspace_files", "search_workspace_text", "read_workspace_text", "read_workspace_files", "manage_workspace_path", "organize_workspace_matches", "apply_workspace_patch"} {
		assertToolRegistered(t, openAIToolsForLoadedToolsets(loaded, true), name)
		assertToolRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), name)
	}
	for _, name := range []string{"edit_workspace_text", "replace_workspace_lines", "save_workspace_file", "write_workspace_chunk", "finalize_workspace_write"} {
		assertToolNotRegistered(t, openAIToolsForLoadedToolsets(loaded, true), name)
		assertToolNotRegistered(t, anthropicToolsForLoadedToolsets(loaded, true), name)
	}
	if group := toolGroup("find_workspace_files"); group != ToolGroupSearch {
		t.Fatalf("expected find_workspace_files search group, got %q", group)
	}
	if group := toolGroup("apply_workspace_patch"); group != ToolGroupEdit {
		t.Fatalf("expected apply_workspace_patch edit group, got %q", group)
	}
	if group := toolGroup("manage_workspace_path"); group != ToolGroupEdit {
		t.Fatalf("expected manage_workspace_path edit group, got %q", group)
	}
	if group := toolGroup("read_workspace_files"); group != ToolGroupRead {
		t.Fatalf("expected read_workspace_files read group, got %q", group)
	}
	// Compatibility mutation tools remain executable internally for old transcripts/tasks.
	if !isWorkspaceMutationTool("edit_workspace_text") {
		t.Fatal("expected edit_workspace_text to be a workspace mutation")
	}
	if !isWorkspaceMutationTool("replace_workspace_lines") {
		t.Fatal("expected replace_workspace_lines to be a workspace mutation")
	}
	if !isWorkspaceMutationTool("finalize_workspace_write") {
		t.Fatal("expected finalize_workspace_write to be a workspace mutation")
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
	if last.Kind != transcriptEntryRuntimeObservation || !strings.Contains(last.Text, `"status":"action_rejected"`) || strings.Contains(last.Text, `"required_next_action":"apply_workspace_patch"`) {
		t.Fatalf("unexpected corrective transcript entry: %#v", last)
	}
	if !strings.Contains(last.Text, `"allowed_next_actions"`) {
		t.Fatalf("expected allowed next actions in corrective observation: %#v", last)
	}
}

func TestHandleNoToolTurnUsesWorkspaceReferenceActionBoundary(t *testing.T) {
	runtime := &agentRuntime{
		attachments: []AttachmentPayload{{Kind: "workspace_file", Path: "draft.txt"}},
		transcript:  newAgentTranscript(),
	}
	reply, ok, err := runtime.handleNoToolTurn(context.Background(), agentModelTurn{
		Content:      "我会先读取这个文件。",
		FinishReason: "stop",
	})
	if err != nil {
		t.Fatalf("expected no hard error, got %v", err)
	}
	if ok {
		t.Fatalf("workspace reference action boundary must reject plain text completion, got reply %#v", reply)
	}
	if len(runtime.transcript.Entries) == 0 {
		t.Fatal("expected corrective message appended to transcript")
	}
	last := runtime.transcript.Entries[len(runtime.transcript.Entries)-1]
	if last.Kind != transcriptEntryRuntimeObservation || !strings.Contains(last.Text, `"status":"action_rejected"`) || !strings.Contains(last.Text, `"model_action_manifest"`) {
		t.Fatalf("unexpected corrective transcript entry: %#v", last)
	}
	if strings.Contains(last.Text, `"required_next_action":"apply_workspace_patch"`) {
		t.Fatalf("read-only workspace reference should not force patch action: %#v", last)
	}
}

func TestHandleNoToolTurnRejectsPlainTextAfterReadOnlyTool(t *testing.T) {
	var discarded []string
	runtime := &agentRuntime{
		requiresCompletion:       true,
		toolLoopActive:           true,
		transcript:               newAgentTranscript(),
		activeAssistantMessageID: "active_1",
		activeAssistantSegment:   1,
		opts: RunOptions{
			OnAssistantMessageDiscarded: func(messageID string, reason string, segmentIndex int) {
				discarded = append(discarded, messageID+"|"+reason)
			},
		},
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
	if err != nil {
		t.Fatalf("expected no hard error after read-only tool text, ok=%v err=%v", ok, err)
	}
	if ok {
		t.Fatalf("expected corrective loop after read-only tool text, got reply %#v", reply)
	}
	if strings.TrimSpace(reply.Content) != "" {
		t.Fatalf("expected no completion after read-only tool, got reply %#v", reply)
	}
	if len(discarded) != 1 || !strings.Contains(discarded[0], "action_required_plain_text_rejected") {
		t.Fatalf("expected streamed planning text to be discarded, got %#v", discarded)
	}
}

func TestHandleNoToolTurnAcceptsReadOnlyContentWithoutSelectedWorkspaceReference(t *testing.T) {
	runtime := &agentRuntime{
		toolLoopActive: true,
		transcript:     newAgentTranscript(),
		events: []ToolEvent{{
			ToolName: "read_workspace_text",
			Status:   "success",
			Path:     "demo.txt",
		}},
	}
	reply, ok, err := runtime.handleNoToolTurn(context.Background(), agentModelTurn{
		Content:      "demo.txt 的核心问题是节奏需要更紧。",
		FinishReason: "stop",
	})
	if err != nil {
		t.Fatalf("expected no hard error after read-only text, got %v", err)
	}
	if !ok || !strings.Contains(reply.Content, "节奏") {
		t.Fatalf("expected read-only content completion, ok=%v reply=%#v", ok, reply)
	}
	if len(runtime.transcript.Entries) != 0 {
		t.Fatalf("did not expect corrective observation, got %#v", runtime.transcript.Entries)
	}
}

func TestHandleNoToolTurnRejectsContentAfterMutation(t *testing.T) {
	runtime := &agentRuntime{
		toolLoopActive:    true,
		performedMutation: true,
		transcript:        newAgentTranscript(),
		events: []ToolEvent{{
			ToolName: "apply_workspace_patch",
			Status:   "success",
			Path:     "demo.txt",
		}},
	}
	reply, ok, err := runtime.handleNoToolTurn(context.Background(), agentModelTurn{
		Content:      "demo.txt 已经改好了。",
		FinishReason: "stop",
	})
	if err != nil {
		t.Fatalf("expected corrective loop, got %v", err)
	}
	if ok || strings.TrimSpace(reply.Content) != "" {
		t.Fatalf("expected mutation content to be rejected, ok=%v reply=%#v", ok, reply)
	}
	if len(runtime.transcript.Entries) == 0 || runtime.transcript.Entries[len(runtime.transcript.Entries)-1].Kind != transcriptEntryRuntimeObservation {
		t.Fatalf("expected corrective observation, got %#v", runtime.transcript.Entries)
	}
}

func TestRecentWorkspaceReadMemoryUsesLedgerAndMutationInvalidation(t *testing.T) {
	env := newTestAIEnv(t)
	ctx := context.Background()
	conversation, err := env.conversations.CreateConversation(ctx, store.CreateConversationInput{
		UserID: testUserID,
		Title:  "recent reads",
	})
	if err != nil {
		t.Fatalf("CreateConversation returned error: %v", err)
	}
	readResult := mustJSON(map[string]any{
		"status":           "success",
		"tool_name":        "read_workspace_text",
		"path":             "draft.txt",
		"numbered_content": "1: 第一行\n2: 第二行",
		"start_line":       1,
		"end_line":         2,
		"total_lines":      2,
	})
	if _, err := env.conversations.RecordToolCall(ctx, store.ToolCallInput{
		UserID:         testUserID,
		ConversationID: conversation.ID,
		ToolName:       "read_workspace_text",
		Status:         "success",
		ResultJSON:     readResult,
	}); err != nil {
		t.Fatalf("RecordToolCall read returned error: %v", err)
	}
	memory := env.service.recentWorkspaceReadMemory(ctx, testUserID, conversation.ID, []AttachmentPayload{{Kind: "current_file", Path: "draft.txt"}})
	if !memory.Available || len(memory.Reads) != 1 || !strings.Contains(memory.Reads[0].NumberedContent, "第一行") {
		t.Fatalf("expected recent read memory with content, got %#v", memory)
	}
	changedPaths, _ := json.Marshal([]string{"draft.txt"})
	if _, err := env.conversations.RecordToolCall(ctx, store.ToolCallInput{
		UserID:           testUserID,
		ConversationID:   conversation.ID,
		ToolName:         "apply_workspace_patch",
		Status:           "success",
		ResultJSON:       mustJSON(map[string]any{"path": "draft.txt", "changed_paths": []string{"draft.txt"}}),
		ChangedPathsJSON: string(changedPaths),
	}); err != nil {
		t.Fatalf("RecordToolCall mutation returned error: %v", err)
	}
	memory = env.service.recentWorkspaceReadMemory(ctx, testUserID, conversation.ID, []AttachmentPayload{{Kind: "current_file", Path: "draft.txt"}})
	if memory.Available {
		t.Fatalf("expected mutation to invalidate older read memory, got %#v", memory)
	}
}

func TestAnthropicSystemBlockBuilderKeepsActionManifestDynamic(t *testing.T) {
	system := "stable prompt\n\n" + actionManifestAnchorPrefix + `{"json_tools":[]}`
	payload := AnthropicSystemBlockBuilder{}.Build(system, true)
	blocks, ok := payload.([]map[string]any)
	if !ok || len(blocks) != 2 {
		t.Fatalf("expected two Anthropic system blocks, got %#v", payload)
	}
	if _, ok := blocks[0]["cache_control"]; !ok || strings.Contains(stringFromAny(blocks[0]["text"]), "current action manifest") {
		t.Fatalf("expected only stable prompt to be cached, got %#v", blocks)
	}
	if !strings.Contains(stringFromAny(blocks[1]["text"]), "current action manifest") {
		t.Fatalf("expected dynamic action manifest block, got %#v", blocks)
	}
}

func TestNormalizeTextPatchTurnStripsRepeatedEndPatchResidue(t *testing.T) {
	turn, ok := turnWithExtractedTextPatch(agentModelTurn{
		Content: "*** Begin Patch\n*** Add File: draft.md\n+hello\n*** End Patch\n*** End Patch\n我会核对文件。",
	})
	if !ok {
		t.Fatal("expected patch to be captured")
	}
	if strings.Contains(turn.Content, "*** End Patch") || strings.Contains(turn.PublicStreamedContent, "*** End Patch") {
		t.Fatalf("patch end marker leaked into visible content: content=%q public=%q", turn.Content, turn.PublicStreamedContent)
	}
	if strings.TrimSpace(turn.Content) != "我会核对文件。" {
		t.Fatalf("unexpected visible content: %q", turn.Content)
	}
}

func TestRuntimeToolChoiceRequiresCompletionActionAfterVerifiedTool(t *testing.T) {
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
		t.Fatalf("expected provider-auto while local action boundary enforces completion/action, got %#v", choice)
	}
	if got := openAIProviderToolChoice(runtime.toolChoice()); got != "auto" {
		t.Fatalf("expected OpenAI auto tool choice after verified tool, got %#v", got)
	}
	if !runtime.actionBoundaryActive() {
		t.Fatal("expected local action boundary to stay active after verified tool")
	}
}

func TestRuntimeToolChoiceRequiresToolsBeforeVerifiedTool(t *testing.T) {
	runtime := &agentRuntime{requiresCompletion: true, toolLoopActive: true}
	if choice := runtime.toolChoice(); choice.Mode != providerToolChoiceAuto {
		t.Fatalf("expected provider-auto before verified tool, got %#v", choice)
	}
	if got := openAIProviderToolChoice(runtime.toolChoice()); got != "auto" {
		t.Fatalf("expected OpenAI auto tool choice, got %#v", got)
	}
	anthropic := anthropicProviderToolChoice(runtime.toolChoice())
	if anthropic["type"] != "auto" || anthropic["disable_parallel_tool_use"] != true {
		t.Fatalf("expected Anthropic auto tool choice with serial tools, got %#v", anthropic)
	}
	if !runtime.actionBoundaryActive() {
		t.Fatal("expected local action boundary before verified tool")
	}
}

func TestRuntimeTaskToolChoiceAllowsTextPatchChannel(t *testing.T) {
	runtime := &agentRuntime{opts: RunOptions{TaskID: "task_1"}, requiresCompletion: true, toolLoopActive: true}
	if choice := runtime.toolChoice(); choice.Mode != providerToolChoiceAuto {
		t.Fatalf("expected background task to allow auto tool choice for text patch capture, got %#v", choice)
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
		BaseURL:             "https://api.openai.com/v1",
		ModelID:             "gpt-5.4",
		ReasoningCapability: store.ReasoningCapabilityAuto,
	}
	payload := map[string]any{}
	policy := applyOpenAIReasoningPayload(payload, provider, ThinkingModeExpert)
	if policy.Support != store.ReasoningSupportOpenAI || payload["reasoning_effort"] != "xhigh" {
		t.Fatalf("expected source-backed OpenAI model reasoning params, policy=%#v payload=%#v", policy, payload)
	}
}

func TestNativeReasoningPolicyDoesNotApplyOfficialOpenAITableToCompatibleProxy(t *testing.T) {
	provider := store.ModelProvider{
		Protocol:            "openai",
		BaseURL:             "https://api.jellix.top/v1",
		ModelID:             "gpt-5.5",
		ReasoningCapability: store.ReasoningCapabilityAuto,
	}
	payload := map[string]any{}
	policy := applyOpenAIReasoningPayload(payload, provider, ThinkingModeExpert)
	if policy.Support != store.ReasoningSupportNone || len(payload) != 0 {
		t.Fatalf("expected OpenAI-compatible proxy to omit official reasoning params unless discovered/explicit, policy=%#v payload=%#v", policy, payload)
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
	if choice := runtime.toolChoice(); choice.Mode != providerToolChoiceAuto {
		t.Fatalf("expected provider-auto even when local ForceTool is set, got %#v", choice)
	}
	if got := openAIProviderToolChoice(runtime.toolChoice()); got != "auto" {
		t.Fatalf("expected OpenAI auto tool choice, got %#v", got)
	}
	runtime.toolCalls = 1
	runtime.requiresCompletion = true
	if choice := runtime.toolChoice(); choice.Mode != providerToolChoiceAuto {
		t.Fatalf("expected provider-auto after first forced attempt before verified result, got %#v", choice)
	}
	runtime.events = []ToolEvent{{ToolName: "save_workspace_file", Status: "success", ChangedPaths: []string{"demo.txt"}}}
	runtime.performedMutation = true
	if choice := runtime.toolChoice(); choice.Mode != providerToolChoiceAuto {
		t.Fatalf("expected provider-auto after verified forced tool result, got %#v", choice)
	}
	if !runtime.actionBoundaryActive() {
		t.Fatal("expected local action boundary after forced tool attempt")
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

func TestProviderFailureClassKeepsTransportSubclasses(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "connection reset",
			err:  fmt.Errorf(`Post "https://api.jellix.top/v1/chat/completions": read tcp 127.0.0.1:1->127.0.0.1:2: connection reset by peer`),
			want: "provider_connection_interrupted",
		},
		{
			name: "timeout",
			err:  fmt.Errorf(`Post "https://api.jellix.top/v1/chat/completions": context deadline exceeded`),
			want: "provider_transport_timeout",
		},
		{
			name: "tls",
			err:  fmt.Errorf(`Post "https://api.jellix.top/v1/chat/completions": local error: tls: bad record mac`),
			want: "provider_tls_interrupted",
		},
		{
			name: "malformed response",
			err:  fmt.Errorf(`Post "https://api.jellix.top/v1/chat/completions": net/http: HTTP/1.x transport connection broken: malformed HTTP response`),
			want: "provider_transport_protocol_error",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !isCheckpointRecoverableProviderError(tc.err) {
				t.Fatalf("expected %v to be checkpoint recoverable", tc.err)
			}
			if got := providerFailureClass(tc.err, ProviderRequestSummary{}); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestProviderFailureClassDoesNotGuessRequiredToolChoice(t *testing.T) {
	err := &ProviderResponseError{
		StatusCode:      http.StatusBadRequest,
		Message:         "模型供应商请求失败（HTTP 400）：openai_error",
		ResponseSnippet: `{"message":"openai_error","type":"bad_response_status_code","code":"bad_response_status_code"}`,
	}
	summary := ProviderRequestSummary{
		ToolChoice:      "required",
		RequiresTool:    true,
		ReasoningEffort: "medium",
		ToolsCount:      20,
	}
	if got := providerFailureClass(err, summary); got != "provider_bad_request_with_reasoning" {
		t.Fatalf("expected generic bad request with reasoning, got %q", got)
	}
}

func TestProviderFailureClassUsesRequiredOnlyWhenProviderSaysSo(t *testing.T) {
	err := &ProviderResponseError{
		StatusCode:      http.StatusBadRequest,
		Message:         "tool_choice required is not supported by this provider",
		ResponseSnippet: `{"error":{"message":"tool_choice required is not supported by this provider"}}`,
	}
	summary := ProviderRequestSummary{
		ToolChoice:   "required",
		RequiresTool: true,
		ToolsCount:   20,
	}
	if got := providerFailureClass(err, summary); got != "provider_rejected_required_tool_choice" {
		t.Fatalf("expected explicit required rejection, got %q", got)
	}
}

func TestExecuteToolWriteWorkspaceFileRejectsMissingFile(t *testing.T) {
	service, _, _, _ := newTestAIService(t)

	_, _, _, _, err := service.executeTool(context.Background(), "save_workspace_file", `{"path":"missing.txt","content":"x","mode":"overwrite"}`, RunOptions{UserID: testUserID})
	if err == nil || !errors.Is(err, os.ErrNotExist) {
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

func containsPathValueAny(value any, expected string) bool {
	if containsPathValue(value, expected) {
		return true
	}
	if items, ok := value.([]string); ok {
		return stringSliceContains(items, expected)
	}
	return false
}

func stringSliceContains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
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
