package ai

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"webide-backend/internal/idgen"
)

const (
	agentKernelVersion   = "perseus_agent_kernel_v3"
	agentKernelMode      = "kernel_v3"
	maxKernelProgressLog = 40
	maxKernelTurnItems   = 160
)

type AgentKernelState struct {
	Version        string                     `json:"version"`
	Mode           string                     `json:"mode"`
	TurnID         string                     `json:"turn_id,omitempty"`
	ReadTargets    map[string]AgentReadTarget `json:"read_targets,omitempty"`
	ProgressEvents []AgentProgressEvent       `json:"progress_events,omitempty"`
	Items          []AgentTurnItem            `json:"items,omitempty"`
}

type AgentReadTarget struct {
	ToolName           string            `json:"tool_name"`
	Path               string            `json:"path,omitempty"`
	View               string            `json:"view,omitempty"`
	Mode               string            `json:"mode,omitempty"`
	BlockID            string            `json:"block_id,omitempty"`
	SectionID          string            `json:"section_id,omitempty"`
	BlockIDs           []string          `json:"block_ids,omitempty"`
	SectionIDs         []string          `json:"section_ids,omitempty"`
	CompleteBlockIDs   []string          `json:"complete_block_ids,omitempty"`
	BlockHashes        map[string]string `json:"block_hashes,omitempty"`
	DocHash            string            `json:"doc_hash,omitempty"`
	LedgerID           string            `json:"ledger_id,omitempty"`
	Status             string            `json:"status,omitempty"`
	SourceTruncated    bool              `json:"source_truncated,omitempty"`
	ModelViewTruncated bool              `json:"model_view_truncated,omitempty"`
	Round              int               `json:"round,omitempty"`
}

type AgentProgressEvent struct {
	Round     int    `json:"round"`
	ToolName  string `json:"tool_name"`
	Status    string `json:"status"`
	TargetKey string `json:"target_key"`
	LedgerID  string `json:"ledger_id,omitempty"`
}

type AgentTurnItem struct {
	ID           string   `json:"id"`
	TurnID       string   `json:"turn_id,omitempty"`
	Kind         string   `json:"kind"`
	Phase        string   `json:"phase,omitempty"`
	Status       string   `json:"status"`
	Round        int      `json:"round,omitempty"`
	ToolCallID   string   `json:"tool_call_id,omitempty"`
	ToolName     string   `json:"tool_name,omitempty"`
	Target       string   `json:"target,omitempty"`
	LedgerID     string   `json:"ledger_id,omitempty"`
	Content      string   `json:"content,omitempty"`
	Error        string   `json:"error,omitempty"`
	ChangedPaths []string `json:"changed_paths,omitempty"`
	Path         string   `json:"path,omitempty"`
	Hash         string   `json:"hash,omitempty"`
	View         string   `json:"view,omitempty"`
	Mode         string   `json:"mode,omitempty"`
	BlockID      string   `json:"block_id,omitempty"`
	SectionID    string   `json:"section_id,omitempty"`
	DocHash      string   `json:"doc_hash,omitempty"`
}

func newAgentKernelState() AgentKernelState {
	return AgentKernelState{
		Version:     agentKernelVersion,
		Mode:        agentKernelMode,
		ReadTargets: map[string]AgentReadTarget{},
	}
}

func normalizeAgentKernelState(state AgentKernelState) AgentKernelState {
	if strings.TrimSpace(state.Version) == "" {
		state.Version = agentKernelVersion
	}
	// Historical mode values remain readable, but every new run uses the sole
	// production kernel. Compatibility metadata never selects execution code.
	state.Mode = agentKernelMode
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

func (r *agentRuntime) recordKernelToolProgress(call agentToolCall, isError bool, event ToolEvent, result string) {
	if r == nil {
		return
	}
	r.ensureKernelState()
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

func (r *agentRuntime) emitKernelTurnStarted() {
	if r == nil {
		return
	}
	r.ensureKernelState()
	if strings.TrimSpace(r.messageID) != "" {
		r.kernel.TurnID = r.messageID
	} else if r.kernel.TurnID == "" {
		r.kernel.TurnID = r.opts.RunID
	}
	emitToolLifecycleEvent(r.opts, "turn_started", "info", map[string]any{
		"turn_id":        r.kernel.TurnID,
		"kernel_version": r.kernel.Version,
		"kernel_mode":    r.kernel.Mode,
	})
}

func (r *agentRuntime) appendKernelItem(item AgentTurnItem) {
	if r == nil {
		return
	}
	r.ensureKernelState()
	if strings.TrimSpace(r.messageID) != "" {
		r.kernel.TurnID = r.messageID
	} else if r.kernel.TurnID == "" {
		r.kernel.TurnID = r.opts.RunID
	}
	if item.ID == "" {
		item.ID = idgen.New("item")
	}
	item.TurnID = r.kernel.TurnID
	if item.Round == 0 {
		item.Round = r.currentRound
	}
	if item.Status == "" {
		item.Status = "completed"
	}
	r.kernel.Items = append(r.kernel.Items, item)
	if len(r.kernel.Items) > maxKernelTurnItems {
		r.kernel.Items = r.kernel.Items[len(r.kernel.Items)-maxKernelTurnItems:]
	}
	r.persistKernelTurnItem(item)
	payload := map[string]any{
		"item":           item,
		"turn_id":        item.TurnID,
		"item_id":        item.ID,
		"item_kind":      item.Kind,
		"item_status":    item.Status,
		"kernel_version": r.kernel.Version,
		"kernel_mode":    r.kernel.Mode,
	}
	emitToolLifecycleEvent(r.opts, "item_completed", "info", payload)
	switch item.Kind {
	case "tool_call", "tool_result", "file_change", "context_compaction", "final_message", "task_plan", "external_observation":
		emitToolLifecycleEvent(r.opts, item.Kind, "info", payload)
	}
}

func (r *agentRuntime) appendKernelAssistantItem(phase, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	r.appendKernelItem(AgentTurnItem{
		Kind:    "assistant_message",
		Phase:   phase,
		Status:  "completed",
		Content: content,
	})
}

func (r *agentRuntime) appendKernelToolCallItem(call agentToolCall) {
	r.appendKernelItem(AgentTurnItem{
		Kind:       "tool_call",
		Status:     "started",
		ToolCallID: call.ID,
		ToolName:   call.Name,
		Target:     toolTargetSummary(call.Name, executionToolArgumentsJSON(call)),
		Content:    executionToolArgumentsJSON(call),
	})
}

func (r *agentRuntime) appendKernelToolResultItem(call agentToolCall, event ToolEvent, isError bool, result string) {
	target := kernelReadTargetFromToolResult(call.Name, result)
	status := firstNonEmpty(event.Status, "success")
	if isError {
		status = "failed"
	}
	r.appendKernelItem(AgentTurnItem{
		Kind:         "tool_result",
		Status:       status,
		ToolCallID:   call.ID,
		ToolName:     call.Name,
		Target:       firstNonEmpty(event.TargetSummary, event.Path, target.Path, toolTargetSummary(call.Name, executionToolArgumentsJSON(call))),
		LedgerID:     target.LedgerID,
		Error:        event.Error,
		Content:      result,
		ChangedPaths: append([]string(nil), event.ChangedPaths...),
		Path:         firstNonEmpty(event.Path, target.Path),
		Hash:         event.Hash,
		View:         target.View,
		Mode:         target.Mode,
		BlockID:      target.BlockID,
		SectionID:    target.SectionID,
		DocHash:      target.DocHash,
	})
	if isReadWorkspaceTool(call.Name) && !isError && status == "success" {
		r.appendKernelItem(AgentTurnItem{
			Kind:       "workspace_observation",
			Status:     "completed",
			ToolCallID: call.ID,
			ToolName:   call.Name,
			Target:     firstNonEmpty(target.Path, event.Path),
			LedgerID:   target.LedgerID,
			Path:       firstNonEmpty(target.Path, event.Path),
			Hash:       firstNonEmpty(target.DocHash, event.Hash),
			View:       target.View,
			Mode:       target.Mode,
			BlockID:    target.BlockID,
			SectionID:  target.SectionID,
			DocHash:    target.DocHash,
		})
	}
	if len(event.ChangedPaths) > 0 {
		r.appendKernelItem(AgentTurnItem{
			Kind:         "file_change",
			Status:       status,
			ToolCallID:   call.ID,
			ToolName:     call.Name,
			Target:       firstNonEmpty(event.Path, event.TargetSummary),
			ChangedPaths: append([]string(nil), event.ChangedPaths...),
			Path:         event.Path,
			Hash:         event.Hash,
		})
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
			strings.Join(sortedUniqueStrings(target.BlockIDs), ","),
			strings.Join(sortedUniqueStrings(target.SectionIDs), ","),
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
		ToolName:   toolName,
		Path:       normalizeWorkspaceEventPath(stringFromAny(decoded["path"])),
		View:       strings.TrimSpace(stringFromAny(decoded["view"])),
		Mode:       strings.TrimSpace(stringFromAny(decoded["mode"])),
		BlockID:    strings.TrimSpace(stringFromAny(decoded["block_id"])),
		SectionID:  strings.TrimSpace(stringFromAny(decoded["section_id"])),
		BlockIDs:   sortedUniqueStrings(stringSliceFromAnyLoose(decoded["block_ids"])),
		SectionIDs: sortedUniqueStrings(stringSliceFromAnyLoose(decoded["section_ids"])),
		DocHash:    strings.TrimSpace(firstNonEmpty(stringFromAny(decoded["document_version"]), stringFromAny(decoded["doc_hash"]))),
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
		ToolName:           firstNonEmpty(stringFromAny(payload["tool_name"]), stringFromAny(payload["tool"]), toolName),
		Path:               normalizeWorkspaceEventPath(stringFromAny(payload["path"])),
		View:               strings.TrimSpace(stringFromAny(payload["view"])),
		Mode:               strings.TrimSpace(stringFromAny(payload["mode"])),
		BlockID:            strings.TrimSpace(stringFromAny(payload["block_id"])),
		SectionID:          strings.TrimSpace(stringFromAny(payload["section_id"])),
		BlockIDs:           sortedUniqueStrings(stringSliceFromAnyLoose(payload["covered_block_ids"])),
		SectionIDs:         sortedUniqueStrings(stringSliceFromAnyLoose(payload["section_ids"])),
		CompleteBlockIDs:   sortedUniqueStrings(stringSliceFromAnyLoose(payload["complete_block_ids"])),
		BlockHashes:        stringMapFromAny(payload["block_hashes"]),
		DocHash:            strings.TrimSpace(firstNonEmpty(stringFromAny(payload["document_version"]), stringFromAny(payload["doc_hash"]))),
		LedgerID:           strings.TrimSpace(stringFromAny(payload["ledger_id"])),
		SourceTruncated:    boolFromAny(payload["source_truncated"]) || boolFromAny(payload["truncated"]),
		ModelViewTruncated: boolFromAny(payload["model_view_truncated"]),
	}
	if target.BlockID != "" && len(target.BlockIDs) == 0 {
		target.BlockIDs = []string{target.BlockID}
	}
	if target.SectionID != "" && len(target.SectionIDs) == 0 {
		target.SectionIDs = []string{target.SectionID}
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
	target.BlockIDs = sortedUniqueStrings(append(target.BlockIDs, fallback.BlockIDs...))
	target.SectionIDs = sortedUniqueStrings(append(target.SectionIDs, fallback.SectionIDs...))
	target.CompleteBlockIDs = sortedUniqueStrings(append(target.CompleteBlockIDs, fallback.CompleteBlockIDs...))
	target.SourceTruncated = target.SourceTruncated || fallback.SourceTruncated
	target.ModelViewTruncated = target.ModelViewTruncated || fallback.ModelViewTruncated
	if len(target.BlockHashes) == 0 && len(fallback.BlockHashes) > 0 {
		target.BlockHashes = map[string]string{}
	}
	for id, hash := range fallback.BlockHashes {
		if target.BlockHashes[id] == "" {
			target.BlockHashes[id] = hash
		}
	}
	if target.DocHash == "" {
		target.DocHash = fallback.DocHash
	}
	if target.LedgerID == "" {
		target.LedgerID = fallback.LedgerID
	}
	return target
}

func sortedUniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func stringMapFromAny(value any) map[string]string {
	decoded, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	result := make(map[string]string, len(decoded))
	for key, raw := range decoded {
		if value, ok := raw.(string); ok && strings.TrimSpace(value) != "" {
			result[key] = value
		}
	}
	return result
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
