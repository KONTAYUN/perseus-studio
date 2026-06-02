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

type ToolManifestEntry struct {
	Name                string
	Toolset             string
	Group               ToolGroup
	ProviderVisible     bool
	Mutation            bool
	LegacyCompat        bool
	Transport           ToolTransport
	StatusKind          string
	Label               string
	RecoveryProtocol    string
	PrimaryEditProtocol bool
	Description         string
}

var toolManifestEntries = []ToolManifestEntry{
	{Name: "attempt_completion", Group: ToolGroupCompletion, ProviderVisible: true, Transport: ToolTransportJSON, Label: "已完成任务", StatusKind: "completion"},
	{Name: "ask_followup_question", Group: ToolGroupQuestion, ProviderVisible: true, Transport: ToolTransportJSON, Label: "已询问用户", StatusKind: "question"},
	{Name: "load_perseus_toolset", Group: ToolGroupTask, ProviderVisible: true, Transport: ToolTransportJSON, Label: "已处理", StatusKind: "toolset"},

	{Name: "find_workspace_files", Toolset: "workspace", Group: ToolGroupSearch, ProviderVisible: true, Transport: ToolTransportJSON, Label: "已查看工作区", StatusKind: "workspace_search"},
	{Name: "search_workspace_text", Toolset: "workspace", Group: ToolGroupSearch, ProviderVisible: true, Transport: ToolTransportJSON, Label: "已查看工作区", StatusKind: "workspace_search"},
	{Name: "inspect_workspace", Toolset: "workspace", Group: ToolGroupSearch, ProviderVisible: true, Transport: ToolTransportJSON, Label: "已查看工作区", StatusKind: "workspace_search"},
	{Name: "read_workspace_text", Toolset: "workspace", Group: ToolGroupRead, ProviderVisible: true, Transport: ToolTransportJSON, Label: "已读取文件", StatusKind: "workspace_read"},
	{Name: "read_workspace_files", Toolset: "workspace", Group: ToolGroupRead, ProviderVisible: true, Transport: ToolTransportJSON, Label: "已读取文件", StatusKind: "workspace_read"},
	{Name: "manage_workspace_path", Toolset: "workspace", Group: ToolGroupEdit, ProviderVisible: true, Mutation: true, Transport: ToolTransportJSON, Label: "已管理路径", StatusKind: "workspace_path"},
	{Name: "organize_workspace_matches", Toolset: "workspace", Group: ToolGroupEdit, ProviderVisible: true, Mutation: true, Transport: ToolTransportJSON, Label: "已整理文件", StatusKind: "workspace_path"},
	{Name: "apply_workspace_patch", Toolset: "workspace", Group: ToolGroupEdit, Mutation: true, Transport: ToolTransportAssistantTextPatch, Label: "已编辑文件", StatusKind: "workspace_edit", RecoveryProtocol: "assistant_text_patch", PrimaryEditProtocol: true},
	{Name: "edit_workspace_text", Toolset: "workspace", Group: ToolGroupEdit, Mutation: true, LegacyCompat: true, Transport: ToolTransportLegacyCompat, Label: "已编辑文件", StatusKind: "workspace_edit", RecoveryProtocol: "assistant_text_patch"},
	{Name: "replace_workspace_lines", Toolset: "workspace", Group: ToolGroupEdit, Mutation: true, LegacyCompat: true, Transport: ToolTransportLegacyCompat, Label: "已编辑文件", StatusKind: "workspace_edit", RecoveryProtocol: "assistant_text_patch"},
	{Name: "save_workspace_file", Toolset: "workspace", Group: ToolGroupEdit, Mutation: true, LegacyCompat: true, Transport: ToolTransportLegacyCompat, Label: "已编辑文件", StatusKind: "workspace_edit", RecoveryProtocol: "assistant_text_patch"},
	{Name: "write_workspace_chunk", Toolset: "workspace", Group: ToolGroupEdit, Mutation: true, LegacyCompat: true, Transport: ToolTransportLegacyCompat, Label: "已编辑文件", StatusKind: "workspace_edit", RecoveryProtocol: "assistant_text_patch"},
	{Name: "finalize_workspace_write", Toolset: "workspace", Group: ToolGroupEdit, Mutation: true, LegacyCompat: true, Transport: ToolTransportLegacyCompat, Label: "已编辑文件", StatusKind: "workspace_edit", RecoveryProtocol: "assistant_text_patch"},

	{Name: "run_workspace_command", Toolset: "command", Group: ToolGroupCommand, ProviderVisible: true, Transport: ToolTransportJSON, Label: "已运行命令", StatusKind: "command"},
	{Name: "perseus_search_web", Toolset: "search", Group: ToolGroupWeb, ProviderVisible: true, Transport: ToolTransportJSON, Label: "已搜索资料", StatusKind: "web"},
	{Name: "web_search", Toolset: "search", Group: ToolGroupWeb, Transport: ToolTransportLegacyCompat, Label: "已搜索资料", StatusKind: "web"},
	{Name: "web_extract", Toolset: "search", Group: ToolGroupWeb, ProviderVisible: true, Transport: ToolTransportJSON, Label: "已读取网页", StatusKind: "web"},
	{Name: "list_skills", Toolset: "skills", Group: ToolGroupSkill, ProviderVisible: true, Transport: ToolTransportJSON, Label: "已处理技能", StatusKind: "skill"},
	{Name: "read_skill", Toolset: "skills", Group: ToolGroupSkill, ProviderVisible: true, Transport: ToolTransportJSON, Label: "已处理技能", StatusKind: "skill"},
	{Name: "install_skill_from_attachment", Toolset: "skills", Group: ToolGroupSkill, ProviderVisible: true, Transport: ToolTransportJSON, Label: "已处理技能", StatusKind: "skill"},
	{Name: "delete_skill", Toolset: "skills", Group: ToolGroupSkill, ProviderVisible: true, Transport: ToolTransportJSON, Label: "已处理技能", StatusKind: "skill"},
	{Name: "start_background_task", Toolset: "task", Group: ToolGroupTask, ProviderVisible: true, Transport: ToolTransportJSON, Label: "已启动后台任务", StatusKind: "task"},
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
		Name                string        `json:"name"`
		Toolset             string        `json:"toolset,omitempty"`
		Group               ToolGroup     `json:"group,omitempty"`
		ProviderVisible     bool          `json:"provider_visible,omitempty"`
		Mutation            bool          `json:"mutation,omitempty"`
		LegacyCompat        bool          `json:"legacy_compat,omitempty"`
		Transport           ToolTransport `json:"transport,omitempty"`
		PrimaryEditProtocol bool          `json:"primary_edit_protocol,omitempty"`
	}
	items := make([]digestEntry, 0, len(toolManifestEntries))
	for _, entry := range toolManifestEntries {
		items = append(items, digestEntry{
			Name:                entry.Name,
			Toolset:             entry.Toolset,
			Group:               entry.Group,
			ProviderVisible:     entry.ProviderVisible,
			Mutation:            entry.Mutation,
			LegacyCompat:        entry.LegacyCompat,
			Transport:           entry.Transport,
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
	return "json_tools lists JSON-callable workspace tools for locating, reading, and managing paths. File content editing is provided by capabilities.file_content_editing: output a complete *** Begin Patch / *** End Patch patch block directly in assistant text, and the backend captures, verifies, and applies it as the workspace edit."
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
