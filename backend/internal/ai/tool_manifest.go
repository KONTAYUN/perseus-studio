package ai

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

type ToolTransport string

const (
	ToolTransportJSON               ToolTransport = "json"
	ToolTransportAssistantTextPatch ToolTransport = "assistant_text_patch"
	ToolTransportLegacyCompat       ToolTransport = "legacy_compat"
)

const ToolCapabilityFileContentEditing = "file_content_editing"

type ModelActionKind string

const (
	ModelActionJSONTool ModelActionKind = "json_tool"
	ModelActionText     ModelActionKind = "text_action"
	ModelActionTerminal ModelActionKind = "terminal_action"
)

type ToolManifestEntry struct {
	Name                string
	Toolset             string
	Group               ToolGroup
	ProviderVisible     bool
	ModelVisible        bool
	Mutation            bool
	LegacyCompat        bool
	Transport           ToolTransport
	ActionKind          ModelActionKind
	Terminal            bool
	StatusKind          string
	Label               string
	RecoveryProtocol    string
	PrimaryEditProtocol bool
	Description         string
}

type ModelActionManifest struct {
	JSONTools          []ModelJSONToolAction `json:"json_tools,omitempty"`
	TextActions        []ModelTextAction     `json:"text_actions,omitempty"`
	TerminalActions    []ModelTerminalAction `json:"terminal_actions,omitempty"`
	Capabilities       map[string]any        `json:"capabilities,omitempty"`
	WorkspaceState     *ModelWorkspaceState  `json:"workspace_state,omitempty"`
	NextActionRules    []string              `json:"next_action_rules,omitempty"`
	ToolManifestDigest string                `json:"tool_manifest_digest,omitempty"`
}

type ModelWorkspaceState struct {
	CurrentReadWorkingSet CurrentReadWorkingSet     `json:"current_read_working_set,omitempty"`
	RecentWorkspaceReads  RecentWorkspaceReadMemory `json:"recent_workspace_reads,omitempty"`
	ActionState           ActionSelectionState      `json:"action_state,omitempty"`
}

type CurrentReadWorkingSet struct {
	Available bool                  `json:"available"`
	Spans     []CurrentReadSpanView `json:"spans,omitempty"`
}

type CurrentReadSpanView struct {
	Path                            string `json:"path"`
	StartLine                       int    `json:"start_line,omitempty"`
	EndLine                         int    `json:"end_line,omitempty"`
	TotalLines                      int    `json:"total_lines,omitempty"`
	ReadContentState                string `json:"read_content_state"`
	ContentAvailableInRecentContext bool   `json:"content_available_in_recent_context"`
}

type ModelJSONToolAction struct {
	Name      string        `json:"name"`
	Toolset   string        `json:"toolset,omitempty"`
	Group     ToolGroup     `json:"group,omitempty"`
	Transport ToolTransport `json:"transport"`
}

type ModelTextAction struct {
	Name      string        `json:"name"`
	Toolset   string        `json:"toolset,omitempty"`
	Group     ToolGroup     `json:"group,omitempty"`
	Transport ToolTransport `json:"transport"`
	Syntax    string        `json:"syntax,omitempty"`
	Primary   bool          `json:"primary,omitempty"`
}

type ModelTerminalAction struct {
	Name      string        `json:"name"`
	Group     ToolGroup     `json:"group,omitempty"`
	Transport ToolTransport `json:"transport"`
	Terminal  bool          `json:"terminal"`
}

var toolManifestEntries = []ToolManifestEntry{
	{Name: "attempt_completion", Group: ToolGroupCompletion, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionTerminal, Terminal: true, Label: "已完成任务", StatusKind: "completion"},
	{Name: "ask_followup_question", Group: ToolGroupQuestion, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已询问用户", StatusKind: "question"},
	{Name: "load_perseus_toolset", Group: ToolGroupTask, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已处理", StatusKind: "toolset"},

	{Name: "find_workspace_files", Toolset: "workspace", Group: ToolGroupSearch, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已查看工作区", StatusKind: "workspace_search"},
	{Name: "search_workspace_text", Toolset: "workspace", Group: ToolGroupSearch, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已查看工作区", StatusKind: "workspace_search"},
	{Name: "inspect_workspace", Toolset: "workspace", Group: ToolGroupSearch, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已查看工作区", StatusKind: "workspace_search"},
	{Name: "read_workspace_text", Toolset: "workspace", Group: ToolGroupRead, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已读取文件", StatusKind: "workspace_read"},
	{Name: "read_workspace_files", Toolset: "workspace", Group: ToolGroupRead, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已读取文件", StatusKind: "workspace_read"},
	{Name: "manage_workspace_path", Toolset: "workspace", Group: ToolGroupEdit, ProviderVisible: true, ModelVisible: true, Mutation: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已管理路径", StatusKind: "workspace_path"},
	{Name: "organize_workspace_matches", Toolset: "workspace", Group: ToolGroupEdit, ProviderVisible: true, ModelVisible: true, Mutation: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已整理文件", StatusKind: "workspace_path"},
	{Name: "apply_workspace_patch", Toolset: "workspace", Group: ToolGroupEdit, ModelVisible: true, Mutation: true, Transport: ToolTransportAssistantTextPatch, ActionKind: ModelActionText, Label: "已编辑文件", StatusKind: "workspace_edit", RecoveryProtocol: "assistant_text_patch", PrimaryEditProtocol: true},
	{Name: "edit_workspace_text", Toolset: "workspace", Group: ToolGroupEdit, Mutation: true, LegacyCompat: true, Transport: ToolTransportLegacyCompat, Label: "已编辑文件", StatusKind: "workspace_edit", RecoveryProtocol: "assistant_text_patch"},
	{Name: "replace_workspace_lines", Toolset: "workspace", Group: ToolGroupEdit, Mutation: true, LegacyCompat: true, Transport: ToolTransportLegacyCompat, Label: "已编辑文件", StatusKind: "workspace_edit", RecoveryProtocol: "assistant_text_patch"},
	{Name: "save_workspace_file", Toolset: "workspace", Group: ToolGroupEdit, Mutation: true, LegacyCompat: true, Transport: ToolTransportLegacyCompat, Label: "已编辑文件", StatusKind: "workspace_edit", RecoveryProtocol: "assistant_text_patch"},
	{Name: "write_workspace_chunk", Toolset: "workspace", Group: ToolGroupEdit, Mutation: true, LegacyCompat: true, Transport: ToolTransportLegacyCompat, Label: "已编辑文件", StatusKind: "workspace_edit", RecoveryProtocol: "assistant_text_patch"},
	{Name: "finalize_workspace_write", Toolset: "workspace", Group: ToolGroupEdit, Mutation: true, LegacyCompat: true, Transport: ToolTransportLegacyCompat, Label: "已编辑文件", StatusKind: "workspace_edit", RecoveryProtocol: "assistant_text_patch"},

	{Name: "run_workspace_command", Toolset: "command", Group: ToolGroupCommand, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已运行命令", StatusKind: "command"},
	{Name: "perseus_search_web", Toolset: "search", Group: ToolGroupWeb, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已搜索资料", StatusKind: "web"},
	{Name: "web_search", Toolset: "search", Group: ToolGroupWeb, Transport: ToolTransportLegacyCompat, Label: "已搜索资料", StatusKind: "web"},
	{Name: "web_extract", Toolset: "search", Group: ToolGroupWeb, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已读取网页", StatusKind: "web"},
	{Name: "list_skills", Toolset: "skills", Group: ToolGroupSkill, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已处理技能", StatusKind: "skill"},
	{Name: "read_skill", Toolset: "skills", Group: ToolGroupSkill, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已处理技能", StatusKind: "skill"},
	{Name: "install_skill_from_attachment", Toolset: "skills", Group: ToolGroupSkill, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已处理技能", StatusKind: "skill"},
	{Name: "delete_skill", Toolset: "skills", Group: ToolGroupSkill, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已处理技能", StatusKind: "skill"},
	{Name: "start_background_task", Toolset: "task", Group: ToolGroupTask, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已启动后台任务", StatusKind: "task"},
}

var toolManifestByName = buildToolManifestByName()

func buildToolManifestByName() map[string]ToolManifestEntry {
	out := make(map[string]ToolManifestEntry, len(toolManifestEntries))
	for _, entry := range toolManifestEntries {
		out[entry.Name] = entry
	}
	return out
}

func toolManifestEntry(name string) (ToolManifestEntry, bool) {
	entry, ok := toolManifestByName[name]
	return entry, ok
}

func toolManifestDigest() string {
	type digestEntry struct {
		Name                string          `json:"name"`
		Toolset             string          `json:"toolset,omitempty"`
		Group               ToolGroup       `json:"group,omitempty"`
		ProviderVisible     bool            `json:"provider_visible,omitempty"`
		ModelVisible        bool            `json:"model_visible,omitempty"`
		Mutation            bool            `json:"mutation,omitempty"`
		LegacyCompat        bool            `json:"legacy_compat,omitempty"`
		Transport           ToolTransport   `json:"transport,omitempty"`
		ActionKind          ModelActionKind `json:"action_kind,omitempty"`
		Terminal            bool            `json:"terminal,omitempty"`
		PrimaryEditProtocol bool            `json:"primary_edit_protocol,omitempty"`
	}
	items := make([]digestEntry, 0, len(toolManifestEntries))
	for _, entry := range toolManifestEntries {
		items = append(items, digestEntry{
			Name:                entry.Name,
			Toolset:             entry.Toolset,
			Group:               entry.Group,
			ProviderVisible:     entry.ProviderVisible,
			ModelVisible:        entry.ModelVisible,
			Mutation:            entry.Mutation,
			LegacyCompat:        entry.LegacyCompat,
			Transport:           entry.Transport,
			ActionKind:          entry.ActionKind,
			Terminal:            entry.Terminal,
			PrimaryEditProtocol: entry.PrimaryEditProtocol,
		})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	raw, _ := json.Marshal(items)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}

func providerVisibleToolNamesForToolset(toolset, role string) []string {
	var names []string
	for _, entry := range toolManifestEntries {
		if entry.Toolset != toolset || !entry.ProviderVisible {
			continue
		}
		if !toolVisibleForRole(entry.Name, role) {
			continue
		}
		names = append(names, entry.Name)
	}
	return names
}

func toolVisibleForRole(name, role string) bool {
	if normalizeRunUserRole(role) == "admin" {
		return true
	}
	switch name {
	case "install_skill_from_attachment", "delete_skill":
		return false
	default:
		return true
	}
}

func toolsetCapabilities(toolset string) map[string]any {
	if toolset != "workspace" {
		return map[string]any{}
	}
	for _, entry := range toolManifestEntries {
		if entry.Name != "apply_workspace_patch" || !entry.PrimaryEditProtocol {
			continue
		}
		return map[string]any{
			ToolCapabilityFileContentEditing: map[string]any{
				"available": true,
				"transport": string(entry.Transport),
				"syntax":    "*** Begin Patch ... *** End Patch",
				"operations": []string{
					"add_file",
					"update_file",
					"delete_file",
					"move_file",
				},
				"execution": "captured_and_verified_by_backend",
			},
		}
	}
	return map[string]any{}
}

func toolsetInstruction(toolset string) string {
	if toolset != "workspace" {
		return "Continue with the newly loaded JSON tools when they are needed for the user's request."
	}
	return "json_tools lists JSON-callable workspace tools for locating, reading, and managing paths. text_actions lists first-class non-JSON actions. File content editing uses the apply_workspace_patch text action: output one complete *** Begin Patch / *** End Patch block directly in assistant text, and the backend captures, verifies, and applies it as the workspace edit. terminal_actions lists final-only actions; attempt_completion cannot create or modify files."
}

func providerVisibleToolNames(loaded map[string]bool, includeLoader bool, role string) []string {
	var names []string
	for _, entry := range toolManifestEntries {
		if !entry.ProviderVisible {
			continue
		}
		if entry.Toolset == "" {
			if entry.Name == "load_perseus_toolset" && !includeLoader {
				continue
			}
			if !toolVisibleForRole(entry.Name, role) {
				continue
			}
			names = append(names, entry.Name)
			continue
		}
		if loaded[entry.Toolset] && toolVisibleForRole(entry.Name, role) {
			names = append(names, entry.Name)
		}
	}
	return names
}

func manifestRecommendedProtocol(name string) string {
	entry, ok := toolManifestEntry(name)
	if !ok {
		return ""
	}
	if entry.RecoveryProtocol != "" {
		return entry.RecoveryProtocol
	}
	if entry.PrimaryEditProtocol {
		return string(entry.Transport)
	}
	return ""
}

func manifestStatusKind(name string) string {
	if entry, ok := toolManifestEntry(name); ok {
		return strings.TrimSpace(entry.StatusKind)
	}
	return ""
}

func manifestPrimaryEditProtocol() string {
	for _, entry := range toolManifestEntries {
		if entry.PrimaryEditProtocol {
			return string(entry.Transport)
		}
	}
	return ""
}

func manifestToolIsMutation(name string) bool {
	entry, ok := toolManifestEntry(name)
	return ok && entry.Mutation
}

func manifestToolIsLegacyCompat(name string) bool {
	entry, ok := toolManifestEntry(name)
	return ok && entry.LegacyCompat
}

func modelActionManifest(loaded map[string]bool, includeLoader bool, role string) ModelActionManifest {
	role = normalizeRunUserRole(role)
	loaded = normalizeModelActionLoadedToolsets(loaded)
	manifest := ModelActionManifest{
		Capabilities:       map[string]any{},
		ToolManifestDigest: toolManifestDigest(),
	}
	for _, entry := range toolManifestEntries {
		if !entry.ModelVisible || !toolVisibleForRole(entry.Name, role) {
			continue
		}
		if entry.Toolset != "" && !loaded[entry.Toolset] {
			continue
		}
		if entry.Toolset == "" && entry.Name == "load_perseus_toolset" && !includeLoader {
			continue
		}
		switch entry.ActionKind {
		case ModelActionTerminal:
			manifest.TerminalActions = append(manifest.TerminalActions, ModelTerminalAction{
				Name:      entry.Name,
				Group:     entry.Group,
				Transport: entry.Transport,
				Terminal:  true,
			})
		case ModelActionText:
			manifest.TextActions = append(manifest.TextActions, ModelTextAction{
				Name:      entry.Name,
				Toolset:   entry.Toolset,
				Group:     entry.Group,
				Transport: entry.Transport,
				Syntax:    textActionSyntax(entry),
				Primary:   entry.PrimaryEditProtocol,
			})
		default:
			if !entry.ProviderVisible {
				continue
			}
			manifest.JSONTools = append(manifest.JSONTools, ModelJSONToolAction{
				Name:      entry.Name,
				Toolset:   entry.Toolset,
				Group:     entry.Group,
				Transport: entry.Transport,
			})
		}
	}
	if loaded["workspace"] {
		manifest.Capabilities = toolsetCapabilities("workspace")
	}
	if len(manifest.Capabilities) == 0 {
		manifest.Capabilities = nil
	}
	manifest.NextActionRules = modelActionRules(loaded)
	return manifest
}

func modelActionManifestForProvider(loaded map[string]bool, includeLoader bool, role string, nativePatch bool) ModelActionManifest {
	manifest := modelActionManifest(loaded, includeLoader, role)
	if nativePatch && normalizeModelActionLoadedToolsets(loaded)["workspace"] && !containsModelJSONToolName(manifest.JSONTools, "apply_workspace_patch") {
		manifest.JSONTools = append(manifest.JSONTools, ModelJSONToolAction{
			Name:      "apply_workspace_patch",
			Toolset:   "workspace",
			Group:     ToolGroupEdit,
			Transport: ToolTransportJSON,
		})
	}
	return manifest
}

func containsModelJSONToolName(tools []ModelJSONToolAction, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func normalizeModelActionLoadedToolsets(loaded map[string]bool) map[string]bool {
	next := map[string]bool{"workspace": true}
	for key, value := range loaded {
		if value && strings.TrimSpace(key) != "" {
			next[key] = true
		}
	}
	return next
}

func textActionSyntax(entry ToolManifestEntry) string {
	if entry.PrimaryEditProtocol && entry.Transport == ToolTransportAssistantTextPatch {
		return "*** Begin Patch ... *** End Patch"
	}
	return ""
}

func modelActionRules(loaded map[string]bool) []string {
	rules := []string{
		"Use json_tools for locating current state, reading file ranges, managing paths, asking follow-up questions, or loading optional toolsets.",
		"Reuse current_read_working_set and recent read observations when read_content_state is available; do not reread the same current file just because current_file_context metadata is present.",
		"Read workspace text only when exact current file content is needed and no usable recent read exists, or when the needed range is missing, truncated, stale after mutation, or outside the preserved recent context.",
		"Use text_actions.apply_workspace_patch for any file content creation, update, delete, or move; output the complete patch block as assistant text, not JSON.",
		"Use terminal_actions.attempt_completion only after required tool/text actions have succeeded or an unrecoverable failure is honestly reported.",
	}
	if loaded["workspace"] {
		rules = append(rules, "Workspace file content changes are not performed by attempt_completion; they require a verified workspace edit action first.")
	}
	return rules
}

func manifestToolTransport(name string) ToolTransport {
	entry, ok := toolManifestEntry(name)
	if !ok {
		return ""
	}
	return entry.Transport
}

func manifestProviderVisibleToolNames(loaded map[string]bool, includeLoader bool, role string) []string {
	return providerVisibleToolNames(loaded, includeLoader, role)
}

func manifestToolDiagnosticMetadata(name string) map[string]any {
	entry, ok := toolManifestEntry(name)
	if !ok {
		return nil
	}
	return map[string]any{
		"name":                  entry.Name,
		"toolset":               entry.Toolset,
		"group":                 string(entry.Group),
		"provider_visible":      entry.ProviderVisible,
		"mutation":              entry.Mutation,
		"legacy_compat":         entry.LegacyCompat,
		"transport":             string(entry.Transport),
		"status_kind":           entry.StatusKind,
		"recovery_protocol":     entry.RecoveryProtocol,
		"primary_edit_protocol": entry.PrimaryEditProtocol,
	}
}
