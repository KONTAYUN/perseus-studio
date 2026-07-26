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
	ToolTransportJSON         ToolTransport = "json"
	ToolTransportLegacyCompat ToolTransport = "legacy_compat"
)

const ToolCapabilityFileContentEditing = "file_content_editing"

const (
	ToolRiskLow    = "low"
	ToolRiskMedium = "medium"
	ToolRiskHigh   = "high"
)

type ModelActionKind string

const (
	ModelActionJSONTool ModelActionKind = "json_tool"
	ModelActionTerminal ModelActionKind = "terminal_action"
)

type ToolManifestEntry struct {
	Name                string
	Toolset             string
	Group               ToolGroup
	ProviderVisible     bool
	ModelVisible        bool
	Mutation            bool
	ReadOnly            bool
	RiskLevel           string
	RequiresApproval    bool
	ResultKind          string
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
	TerminalActions    []ModelTerminalAction `json:"terminal_actions,omitempty"`
	Capabilities       map[string]any        `json:"capabilities,omitempty"`
	WorkspaceState     *ModelWorkspaceState  `json:"workspace_state,omitempty"`
	NextActionRules    []string              `json:"next_action_rules,omitempty"`
	ToolManifestDigest string                `json:"tool_manifest_digest,omitempty"`
}

type ModelWorkspaceState struct {
	CurrentReadWorkingSet CurrentReadWorkingSet `json:"current_read_working_set,omitempty"`
	ActionState           ActionSelectionState  `json:"action_state,omitempty"`
}

type CurrentReadWorkingSet struct {
	Available bool                  `json:"available"`
	Spans     []CurrentReadSpanView `json:"spans,omitempty"`
}

type CurrentReadSpanView struct {
	Path                            string `json:"path"`
	ToolName                        string `json:"tool_name,omitempty"`
	Mode                            string `json:"mode,omitempty"`
	View                            string `json:"view,omitempty"`
	BlockID                         string `json:"block_id,omitempty"`
	SectionID                       string `json:"section_id,omitempty"`
	DocHash                         string `json:"doc_hash,omitempty"`
	LedgerID                        string `json:"ledger_id,omitempty"`
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

type ModelTerminalAction struct {
	Name      string        `json:"name"`
	Group     ToolGroup     `json:"group,omitempty"`
	Transport ToolTransport `json:"transport"`
	Terminal  bool          `json:"terminal"`
}

var toolManifestEntries = []ToolManifestEntry{
	{Name: "attempt_completion", Group: ToolGroupCompletion, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionTerminal, Terminal: true, Label: "已完成任务", StatusKind: "completion"},
	{Name: requestUserInputToolName, Group: ToolGroupQuestion, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "等待用户输入", StatusKind: "user_input"},
	{Name: requestUserChoiceToolName, Group: ToolGroupQuestion, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "等待用户选择", StatusKind: "user_choice"},
	{Name: legacyFollowupQuestionToolName, Group: ToolGroupQuestion, LegacyCompat: true, Transport: ToolTransportLegacyCompat, Label: "已询问用户", StatusKind: "question"},
	{Name: "update_task_plan", Group: ToolGroupTask, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已更新任务计划", StatusKind: "task_plan"},
	{Name: "load_perseus_toolset", Group: ToolGroupTask, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已处理", StatusKind: "toolset"},

	{Name: "find_workspace_files", Toolset: "workspace", Group: ToolGroupSearch, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已查看工作区", StatusKind: "workspace_search"},
	{Name: "search_workspace_text", Toolset: "workspace", Group: ToolGroupSearch, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已查看工作区", StatusKind: "workspace_search"},
	{Name: "inspect_workspace", Toolset: "workspace", Group: ToolGroupSearch, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已查看工作区", StatusKind: "workspace_search"},
	{Name: "read_workspace_text", Toolset: "workspace", Group: ToolGroupRead, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已读取文件", StatusKind: "workspace_read"},
	{Name: "read_workspace_note_view", Toolset: "workspace", Group: ToolGroupRead, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已读取笔记", StatusKind: "workspace_read"},
	{Name: "read_workspace_files", Toolset: "workspace", Group: ToolGroupRead, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已读取文件", StatusKind: "workspace_read"},
	{Name: "manage_workspace_path", Toolset: "workspace", Group: ToolGroupEdit, ProviderVisible: true, ModelVisible: true, Mutation: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已管理路径", StatusKind: "workspace_path"},
	{Name: "organize_workspace_matches", Toolset: "workspace", Group: ToolGroupEdit, ProviderVisible: true, ModelVisible: true, Mutation: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已整理文件", StatusKind: "workspace_path"},
	{Name: "apply_workspace_patch", Toolset: "workspace", Group: ToolGroupEdit, ProviderVisible: true, ModelVisible: true, Mutation: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已编辑文件", StatusKind: "workspace_edit", PrimaryEditProtocol: true},
	{Name: "edit_workspace_text", Toolset: "workspace", Group: ToolGroupEdit, Mutation: true, LegacyCompat: true, Transport: ToolTransportLegacyCompat, Label: "已编辑文件", StatusKind: "workspace_edit", RecoveryProtocol: "apply_workspace_patch"},
	{Name: "replace_workspace_lines", Toolset: "workspace", Group: ToolGroupEdit, Mutation: true, LegacyCompat: true, Transport: ToolTransportLegacyCompat, Label: "已编辑文件", StatusKind: "workspace_edit"},
	{Name: "save_workspace_file", Toolset: "workspace", Group: ToolGroupEdit, Mutation: true, LegacyCompat: true, Transport: ToolTransportLegacyCompat, Label: "已编辑文件", StatusKind: "workspace_edit", RecoveryProtocol: "apply_workspace_patch"},
	{Name: "write_workspace_chunk", Toolset: "workspace", Group: ToolGroupEdit, Mutation: true, LegacyCompat: true, Transport: ToolTransportLegacyCompat, Label: "已编辑文件", StatusKind: "workspace_edit", RecoveryProtocol: "apply_workspace_patch"},
	{Name: "finalize_workspace_write", Toolset: "workspace", Group: ToolGroupEdit, Mutation: true, LegacyCompat: true, Transport: ToolTransportLegacyCompat, Label: "已编辑文件", StatusKind: "workspace_edit"},

	{Name: "run_workspace_command", Toolset: "command", Group: ToolGroupCommand, ProviderVisible: true, ModelVisible: true, Mutation: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已运行命令", StatusKind: "command"},
	{Name: "perseus_search_web", Toolset: "search", Group: ToolGroupWeb, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已搜索资料", StatusKind: "web"},
	{Name: "web_search", Toolset: "search", Group: ToolGroupWeb, Transport: ToolTransportLegacyCompat, Label: "已搜索资料", StatusKind: "web"},
	{Name: "web_extract", Toolset: "search", Group: ToolGroupWeb, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已读取网页", StatusKind: "web"},
	{Name: "list_skills", Toolset: "skills", Group: ToolGroupSkill, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已处理技能", StatusKind: "skill"},
	{Name: "read_skill", Toolset: "skills", Group: ToolGroupSkill, ProviderVisible: true, ModelVisible: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已处理技能", StatusKind: "skill"},
	{Name: "install_skill_from_attachment", Toolset: "skills", Group: ToolGroupSkill, ProviderVisible: true, ModelVisible: true, Mutation: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已处理技能", StatusKind: "skill"},
	{Name: "delete_skill", Toolset: "skills", Group: ToolGroupSkill, ProviderVisible: true, ModelVisible: true, Mutation: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已处理技能", StatusKind: "skill"},
	{Name: "start_background_task", Toolset: "task", Group: ToolGroupTask, ProviderVisible: true, ModelVisible: true, Mutation: true, Transport: ToolTransportJSON, ActionKind: ModelActionJSONTool, Label: "已启动后台任务", StatusKind: "task"},
}

var toolManifestByName = buildToolManifestByName()

func buildToolManifestByName() map[string]ToolManifestEntry {
	out := make(map[string]ToolManifestEntry, len(toolManifestEntries))
	for _, entry := range toolManifestEntries {
		entry = normalizeToolManifestEntry(entry)
		out[entry.Name] = entry
	}
	return out
}

func normalizeToolManifestEntry(entry ToolManifestEntry) ToolManifestEntry {
	if entry.ResultKind == "" {
		entry.ResultKind = entry.StatusKind
	}
	if entry.RiskLevel == "" {
		switch {
		case entry.Group == ToolGroupCommand:
			entry.RiskLevel = ToolRiskHigh
		case entry.Mutation:
			entry.RiskLevel = ToolRiskMedium
		default:
			entry.RiskLevel = ToolRiskLow
		}
	}
	if entry.Group == ToolGroupRead || entry.Group == ToolGroupSearch || entry.Group == ToolGroupWeb {
		entry.ReadOnly = true
	}
	return entry
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
		ReadOnly            bool            `json:"read_only,omitempty"`
		RiskLevel           string          `json:"risk_level,omitempty"`
		RequiresApproval    bool            `json:"requires_approval,omitempty"`
		ResultKind          string          `json:"result_kind,omitempty"`
		LegacyCompat        bool            `json:"legacy_compat,omitempty"`
		Transport           ToolTransport   `json:"transport,omitempty"`
		ActionKind          ModelActionKind `json:"action_kind,omitempty"`
		Terminal            bool            `json:"terminal,omitempty"`
		PrimaryEditProtocol bool            `json:"primary_edit_protocol,omitempty"`
	}
	items := make([]digestEntry, 0, len(toolManifestEntries))
	for _, entry := range toolManifestEntries {
		entry = normalizeToolManifestEntry(entry)
		items = append(items, digestEntry{
			Name:                entry.Name,
			Toolset:             entry.Toolset,
			Group:               entry.Group,
			ProviderVisible:     entry.ProviderVisible,
			ModelVisible:        entry.ModelVisible,
			Mutation:            entry.Mutation,
			ReadOnly:            entry.ReadOnly,
			RiskLevel:           entry.RiskLevel,
			RequiresApproval:    entry.RequiresApproval,
			ResultKind:          entry.ResultKind,
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
				"arguments": []string{"patch"},
				"operations": []string{
					"add_file",
					"update_file",
					"delete_file",
					"move_file",
				},
				"execution": "structured_tool_call_verified_by_backend",
			},
		}
	}
	return map[string]any{}
}

func toolsetInstruction(toolset string) string {
	if toolset != "workspace" {
		return "Continue with the newly loaded JSON tools when they are needed for the user's request."
	}
	return "json_tools lists JSON-callable workspace tools for locating, reading, managing paths, and editing file content. Use apply_workspace_patch with a complete patch string for file edits. terminal_actions lists final-only actions; attempt_completion cannot create or modify files."
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

func manifestToolIsReadOnly(name string) bool {
	entry, ok := toolManifestEntry(name)
	return ok && entry.ReadOnly
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

func normalizeModelActionLoadedToolsets(loaded map[string]bool) map[string]bool {
	next := map[string]bool{"workspace": true}
	for key, value := range loaded {
		if value && strings.TrimSpace(key) != "" {
			next[key] = true
		}
	}
	return next
}

func modelActionRules(loaded map[string]bool) []string {
	rules := []string{
		"Use json_tools for locating current state, reading content, managing paths, requesting user input or choice, or loading optional toolsets.",
		"Reuse valid files and blocks from the task working set. Editor paths are metadata without file content: read a referenced or task-relevant file when the task depends on it, and ignore unrelated open files.",
		"Read workspace text only when exact current file content is needed and no usable recent read exists, or when the needed range is missing, truncated, stale after mutation, or outside the preserved recent context.",
		"Use json_tools.apply_workspace_patch for any file content creation, update, delete, or move.",
		"Use terminal_actions.attempt_completion only after required tool actions have succeeded or an unrecoverable failure is honestly reported.",
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
		"read_only":             entry.ReadOnly,
		"risk_level":            entry.RiskLevel,
		"requires_approval":     entry.RequiresApproval,
		"result_kind":           entry.ResultKind,
		"legacy_compat":         entry.LegacyCompat,
		"transport":             string(entry.Transport),
		"status_kind":           entry.StatusKind,
		"recovery_protocol":     entry.RecoveryProtocol,
		"primary_edit_protocol": entry.PrimaryEditProtocol,
	}
}
