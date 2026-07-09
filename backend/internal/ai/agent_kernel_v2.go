package ai

import (
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"
)

const (
	agentKernelV2Version = "kernel_v2"
	agentKernelLegacy    = "legacy"
	maxKernelProgressLog = 40
)

type AgentKernelState struct {
	Version        string                     `json:"version"`
	Mode           string                     `json:"mode"`
	ReadTargets    map[string]AgentReadTarget `json:"read_targets,omitempty"`
	ProgressEvents []AgentProgressEvent       `json:"progress_events,omitempty"`
}

type AgentReadTarget struct {
	ToolName  string `json:"tool_name"`
	Path      string `json:"path,omitempty"`
	View      string `json:"view,omitempty"`
	Mode      string `json:"mode,omitempty"`
	BlockID   string `json:"block_id,omitempty"`
	SectionID string `json:"section_id,omitempty"`
	DocHash   string `json:"doc_hash,omitempty"`
	LedgerID  string `json:"ledger_id,omitempty"`
	Status    string `json:"status,omitempty"`
	Round     int    `json:"round,omitempty"`
}

type AgentProgressEvent struct {
	Round     int    `json:"round"`
	ToolName  string `json:"tool_name"`
	Status    string `json:"status"`
	TargetKey string `json:"target_key"`
	LedgerID  string `json:"ledger_id,omitempty"`
}

func newAgentKernelState() AgentKernelState {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("PERSEUS_AGENT_KERNEL_MODE")))
	if mode == "" {
		mode = agentKernelV2Version
	}
	if mode != agentKernelLegacy && mode != agentKernelV2Version {
		mode = agentKernelV2Version
	}
	return AgentKernelState{
		Version:     agentKernelV2Version,
		Mode:        mode,
		ReadTargets: map[string]AgentReadTarget{},
	}
}

func normalizeAgentKernelState(state AgentKernelState) AgentKernelState {
	if strings.TrimSpace(state.Version) == "" {
		state.Version = agentKernelV2Version
	}
	if strings.TrimSpace(state.Mode) == "" {
		state.Mode = agentKernelV2Version
	}
	if state.ReadTargets == nil {
		state.ReadTargets = map[string]AgentReadTarget{}
	}
	return state
}

func (r *agentRuntime) ensureKernelState() {
	if r == nil {
		return
	}
	r.kernel = normalizeAgentKernelState(r.kernel)
}

func (r *agentRuntime) kernelV2Active() bool {
	if r == nil {
		return true
	}
	r.ensureKernelState()
	return r.kernel.Mode == agentKernelV2Version
}

func (r *agentRuntime) recordKernelToolProgress(call agentToolCall, isError bool, event ToolEvent, result string) {
	if r == nil {
		return
	}
	r.ensureKernelState()
	if r.kernel.Mode != agentKernelV2Version {
		return
	}
	status := firstNonEmpty(event.Status, "success")
	if isError {
		status = "failed"
	}
	argumentTarget := kernelReadTargetFromArguments(call.Name, executionToolArgumentsJSON(call))
	resultTarget := kernelReadTargetFromToolResult(call.Name, result)
	target := mergeKernelReadTarget(resultTarget, argumentTarget)
	target.ToolName = firstNonEmpty(target.ToolName, call.Name, event.ToolName)
	target.Status = status
	target.Round = r.currentRound
	if target.Path == "" {
		target.Path = normalizeWorkspaceEventPath(event.Path)
	}
	key := kernelToolTargetKey(call.Name, executionToolArgumentsJSON(call), event, target)
	if key == "" {
		return
	}
	if isReadWorkspaceTool(call.Name) && status == "success" {
		if target.LedgerID == "" {
			target.LedgerID = key
		}
		r.kernel.ReadTargets[key] = target
	}
	r.kernel.ProgressEvents = append(r.kernel.ProgressEvents, AgentProgressEvent{
		Round:     r.currentRound,
		ToolName:  call.Name,
		Status:    status,
		TargetKey: key,
		LedgerID:  target.LedgerID,
	})
	if len(r.kernel.ProgressEvents) > maxKernelProgressLog {
		r.kernel.ProgressEvents = r.kernel.ProgressEvents[len(r.kernel.ProgressEvents)-maxKernelProgressLog:]
	}
}

func kernelToolTargetKey(toolName, argumentsJSON string, event ToolEvent, target AgentReadTarget) string {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return ""
	}
	if target.ToolName == "" {
		target = kernelReadTargetFromArguments(toolName, argumentsJSON)
	}
	if target.Path == "" {
		target.Path = normalizeWorkspaceEventPath(firstNonEmpty(event.Path, event.NewPath, event.OldPath))
	}
	if toolName == "read_workspace_note_view" {
		return strings.Join([]string{
			toolName,
			normalizeWorkspaceEventPath(target.Path),
			firstNonEmpty(target.Mode, "full"),
			firstNonEmpty(target.View, "structure"),
			target.DocHash,
			target.BlockID,
			target.SectionID,
		}, "\x00")
	}
	if isReadWorkspaceTool(toolName) {
		return strings.Join([]string{
			toolName,
			normalizeWorkspaceEventPath(target.Path),
			target.LedgerID,
		}, "\x00")
	}
	targetSummary := firstNonEmpty(event.TargetSummary, toolTargetSummary(toolName, argumentsJSON))
	if targetSummary == "" {
		targetSummary = normalizeWorkspaceEventPath(target.Path)
	}
	return toolName + "\x00" + targetSummary
}

func kernelReadTargetFromArguments(toolName, argumentsJSON string) AgentReadTarget {
	var decoded map[string]any
	if err := json.Unmarshal([]byte(argumentsJSON), &decoded); err != nil {
		return AgentReadTarget{ToolName: toolName}
	}
	target := AgentReadTarget{
		ToolName:  toolName,
		Path:      normalizeWorkspaceEventPath(stringFromAny(decoded["path"])),
		View:      strings.TrimSpace(stringFromAny(decoded["view"])),
		Mode:      strings.TrimSpace(stringFromAny(decoded["mode"])),
		BlockID:   strings.TrimSpace(stringFromAny(decoded["block_id"])),
		SectionID: strings.TrimSpace(stringFromAny(decoded["section_id"])),
		DocHash:   strings.TrimSpace(stringFromAny(decoded["doc_hash"])),
	}
	if target.Mode == "" && toolName == "read_workspace_note_view" {
		target.Mode = "full"
	}
	if target.View == "" && toolName == "read_workspace_note_view" {
		target.View = "structure"
	}
	if toolName == "read_workspace_text" {
		start := intFromAny(decoded["start_line"])
		end := intFromAny(decoded["end_line"])
		if start > 0 || end > 0 {
			target.LedgerID = normalizeWorkspaceEventPath(target.Path) + ":" + intString(start) + "-" + intString(end)
		}
	}
	if toolName == "read_workspace_files" {
		paths := normalizedProviderPaths(stringSliceFromAnyLoose(decoded["paths"]))
		if len(paths) > 0 {
			target.Path = strings.Join(paths, ",")
			target.LedgerID = target.Path + ":" + intString(intFromAny(decoded["start_line"])) + "-" + intString(intFromAny(decoded["end_line"]))
		}
	}
	return target
}

func kernelReadTargetFromToolResult(toolName, result string) AgentReadTarget {
	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		return AgentReadTarget{ToolName: toolName}
	}
	target := AgentReadTarget{
		ToolName:  firstNonEmpty(stringFromAny(payload["tool_name"]), stringFromAny(payload["tool"]), toolName),
		Path:      normalizeWorkspaceEventPath(stringFromAny(payload["path"])),
		View:      strings.TrimSpace(stringFromAny(payload["view"])),
		Mode:      strings.TrimSpace(stringFromAny(payload["mode"])),
		BlockID:   strings.TrimSpace(stringFromAny(payload["block_id"])),
		SectionID: strings.TrimSpace(stringFromAny(payload["section_id"])),
		DocHash:   strings.TrimSpace(stringFromAny(payload["doc_hash"])),
		LedgerID:  strings.TrimSpace(stringFromAny(payload["ledger_id"])),
	}
	if toolName == "read_workspace_text" && target.LedgerID == "" {
		start := intFromAny(payload["start_line"])
		end := intFromAny(payload["end_line"])
		if start > 0 || end > 0 {
			target.LedgerID = normalizeWorkspaceEventPath(target.Path) + ":" + intString(start) + "-" + intString(end)
		}
	}
	if toolName == "read_workspace_files" && target.LedgerID == "" {
		paths := normalizedProviderPaths(stringSliceFromAnyLoose(payload["paths"]))
		if len(paths) > 0 {
			target.Path = strings.Join(paths, ",")
			target.LedgerID = target.Path + ":" + intString(intFromAny(payload["start_line"])) + "-" + intString(intFromAny(payload["end_line"]))
		}
	}
	return target
}

func mergeKernelReadTarget(primary, fallback AgentReadTarget) AgentReadTarget {
	target := primary
	if target.ToolName == "" {
		target.ToolName = fallback.ToolName
	}
	if target.Path == "" {
		target.Path = fallback.Path
	}
	if target.View == "" {
		target.View = fallback.View
	}
	if target.Mode == "" {
		target.Mode = fallback.Mode
	}
	if target.BlockID == "" {
		target.BlockID = fallback.BlockID
	}
	if target.SectionID == "" {
		target.SectionID = fallback.SectionID
	}
	if target.DocHash == "" {
		target.DocHash = fallback.DocHash
	}
	if target.LedgerID == "" {
		target.LedgerID = fallback.LedgerID
	}
	return target
}

func (r *agentRuntime) kernelProgressFingerprint() string {
	if r == nil || len(r.kernel.ReadTargets) == 0 {
		return ""
	}
	keys := make([]string, 0, len(r.kernel.ReadTargets))
	for key := range r.kernel.ReadTargets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return strings.Join(keys, "|")
}

func agentKernelLedgerID(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			clean = append(clean, part)
		}
	}
	return strings.Join(clean, ":")
}

func intString(value int) string {
	if value <= 0 {
		return ""
	}
	return strconv.Itoa(value)
}
