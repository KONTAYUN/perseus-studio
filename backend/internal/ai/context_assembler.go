package ai

import (
	"encoding/json"
	"sort"
	"strings"

	"webide-backend/internal/store"
)

type ContextAssembler struct{}

type ContextAssemblerInput struct {
	Protocol      agentProtocol
	Items         []store.AgentTurnItem
	Attachments   []AttachmentPayload
	Tools         []map[string]any
	Profile       providerRuntimeProfile
	Round         int
	CurrentTurnID string
	Phase         string
	ForcePrune    bool
}

type ContextAssembly struct {
	Snapshot        ContextSnapshot
	Items           []store.AgentTurnItem
	NeedsCompaction bool
	Tokens          int
	ThresholdTokens int
	PrunedResults   int
	BoundarySeq     int64
	RetainedTurns   int
	EditorPaths     []string
	WorkingSet      []contextWorkingFile
}

func (ContextAssembler) Assemble(input ContextAssemblerInput) ContextAssembly {
	active, boundary := activeAgentContextItems(input.Items)
	active = repairAgentToolPairs(active)
	workingSet := contextWorkingSet(input.Items)
	workingSet = contextWorkingSetWithAvailability(workingSet, active)
	workingSetText := contextWorkingSetText(workingSet)
	snapshot := serializeContextItems(input.Protocol, active, input.Attachments, workingSetText)
	activeAttachments := activeContextImageAttachments(active, input.Attachments)
	textTokens := estimateContextSnapshotTextTokens(snapshot, input.Tools)
	visualTokens, visualSource := estimateVisualInputTokens(input.Profile, activeAttachments)
	tokens := textTokens + visualTokens
	threshold := contextAssemblyBudgetTokens(input.Profile)

	pruned := 0
	explicitProviderOverflow := input.ForcePrune && input.Phase == "overflow_recovery"
	pressure := tokens > threshold || explicitProviderOverflow
	if pressure && input.Profile.PruneToolOutputsEnabled {
		recoverTokens := tokens - threshold
		if explicitProviderOverflow && recoverTokens <= 0 {
			recoverTokens = tokens / 4
			if recoverTokens <= 0 {
				recoverTokens = 1
			}
		}
		active, pruned = pruneOldToolResults(active, input.CurrentTurnID, recoverTokens)
		if pruned > 0 {
			workingSet = contextWorkingSetWithAvailability(workingSet, active)
			workingSetText = contextWorkingSetText(workingSet)
			snapshot = serializeContextItems(input.Protocol, active, input.Attachments, workingSetText)
			activeAttachments = activeContextImageAttachments(active, input.Attachments)
			textTokens = estimateContextSnapshotTextTokens(snapshot, input.Tools)
			visualTokens, visualSource = estimateVisualInputTokens(input.Profile, activeAttachments)
			tokens = textTokens + visualTokens
		}
	}

	transcript := newAgentTranscript()
	snapshot.Transcript = transcript
	snapshot.WorkingSet = append([]contextWorkingFile(nil), workingSet...)
	if input.Protocol == agentProtocolAnthropic {
		snapshot.Summary = summarizeAnthropicContext(snapshot.AnthropicMessages, transcript, nil, input.Round, input.Profile)
	} else {
		snapshot.Summary = summarizeOpenAIContext(snapshot.OpenAIMessages, transcript, nil, input.Round, input.Profile)
	}
	summary := &snapshot.Summary
	summary.ImageCount = len(activeAttachments)
	summary.TextInputTokens = textTokens
	summary.VisualInputTokens = visualTokens
	summary.VisualTokenEstimateSource = visualSource
	summary.ProviderMessagePhase = input.Phase
	summary.ActiveContextTokens = tokens
	summary.EstimatedTokens = tokens
	summary.ContextWatermarkPercent = contextWatermarkPercent(tokens, input.Profile.ContextWindowTokens)
	summary.CompactionThresholdTokens = threshold
	summary.ToolResultsPruned = pruned
	summary.CompactionBoundarySeq = boundary
	summary.RetainedTurns = countContextTurns(active)
	summary.EditorContextPaths = contextEditorPaths(active)
	summary.TaskWorkingSetPaths = contextWorkingSetPaths(workingSet)
	summary.ContextWindowSource = input.Profile.ContextWindowSource
	summary.PayloadBeforeBytes = estimatedContextPayloadBytes(snapshot)
	summary.PayloadAfterBytes = summary.PayloadBeforeBytes
	summary.PayloadCompactionApplied = pruned > 0
	summary.ProviderContextCompacted = boundary > 0
	summary.ContextAssemblyStrategy = "turn_items_token_budget"

	return ContextAssembly{
		Snapshot: snapshot, Items: active, NeedsCompaction: tokens > threshold || (explicitProviderOverflow && pruned == 0),
		Tokens: tokens, ThresholdTokens: threshold, PrunedResults: pruned,
		BoundarySeq: boundary, RetainedTurns: summary.RetainedTurns,
		EditorPaths: append([]string(nil), summary.EditorContextPaths...), WorkingSet: workingSet,
	}
}

func contextAssemblyBudgetTokens(profile providerRuntimeProfile) int {
	if profile.AutoCompactEnabled {
		return contextBudgetTokens(profile)
	}
	window := profile.ContextWindowTokens
	if window <= 0 {
		window = defaultContextWindowTokens
	}
	reserved := profile.CompactReservedTokens
	if reserved <= 0 {
		reserved = defaultCompactReservedTokens(window)
	}
	budget := window - reserved
	if budget <= 0 {
		return window * 3 / 4
	}
	return budget
}

func estimateContextSnapshotTokens(snapshot ContextSnapshot, tools []map[string]any) int {
	textTokens := estimateContextSnapshotTextTokens(snapshot, tools)
	imageCount := 0
	if len(snapshot.OpenAIMessages) > 0 {
		_, imageCount = estimateRuntimeMessagesTokenParts(snapshot.OpenAIMessages)
	} else {
		_, imageCount = estimateRuntimeMessagesTokenParts(snapshot.AnthropicMessages)
	}
	return textTokens + imageCount*2048
}

func estimateContextSnapshotTextTokens(snapshot ContextSnapshot, tools []map[string]any) int {
	total := 0
	if len(snapshot.OpenAIMessages) > 0 {
		textTokens, _ := estimateRuntimeMessagesTokenParts(snapshot.OpenAIMessages)
		total += textTokens
	} else {
		total += estimateTextTokens(snapshot.AnthropicSystem)
		textTokens, _ := estimateRuntimeMessagesTokenParts(snapshot.AnthropicMessages)
		total += textTokens
	}
	for _, tool := range tools {
		sanitized, _ := sanitizeTokenEstimateValue(tool)
		raw, _ := json.Marshal(sanitized)
		total += estimateTextTokens(string(raw))
	}
	return total
}

func activeContextImageAttachments(items []store.AgentTurnItem, attachments []AttachmentPayload) []AttachmentPayload {
	activeMessages := map[string]bool{}
	latestUser := latestUserSourceID(items)
	for _, item := range items {
		if item.Kind == "user_message" && item.SourceID != "" {
			activeMessages[item.SourceID] = true
		}
	}
	result := make([]AttachmentPayload, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.Kind != "image" {
			continue
		}
		if activeMessages[attachment.MessageID] || (attachment.MessageID == "" && latestUser != "") {
			result = append(result, attachment)
		}
	}
	return result
}

func pruneOldToolResults(items []store.AgentTurnItem, currentTurnID string, tokensToRecover int) ([]store.AgentTurnItem, int) {
	if tokensToRecover <= 0 {
		return append([]store.AgentTurnItem(nil), items...), 0
	}
	type candidate struct {
		index  int
		tokens int
		seq    int64
	}
	var candidates []candidate
	protectedCalls := protectedContextToolCallIDs(items)
	for index := len(items) - 1; index >= 0; index-- {
		item := items[index]
		if item.Kind != "tool_result" || item.TurnID == currentTurnID || !successfulToolResult(item) {
			continue
		}
		payload := decodeContextItemPayload(item)
		if protectedCalls[contextItemToolCallID(item)] || len(contextPayloadStrings(payload, "changed_paths")) > 0 || manifestToolIsMutation(contextItemToolName(item)) {
			continue
		}
		content := contextItemContent(item)
		if strings.TrimSpace(content) == "" {
			continue
		}
		contentTokens := estimateTextTokens(content)
		candidates = append(candidates, candidate{index: index, tokens: contentTokens, seq: item.Seq})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].seq < candidates[j].seq })
	result := append([]store.AgentTurnItem(nil), items...)
	recovered := 0
	pruned := 0
	for _, item := range candidates {
		if recovered >= tokensToRecover {
			break
		}
		payload := decodeContextItemPayload(result[item.index])
		original := contextItemContent(result[item.index])
		payload["content"] = compactedToolResultReference(result[item.index], payload)
		payload["model_content_pruned"] = true
		raw, _ := json.Marshal(payload)
		result[item.index].PayloadJSON = string(raw)
		result[item.index].Payload = raw
		recovered += estimateTextTokens(original) - estimateTextTokens(contextPayloadString(payload, "content"))
		pruned++
	}
	return result, pruned
}

func protectedContextToolCallIDs(items []store.AgentTurnItem) map[string]bool {
	protected := map[string]bool{}
	resultIDs := map[string]bool{}
	for _, item := range items {
		if item.Kind == "tool_result" || item.Kind == "aborted" {
			resultIDs[contextItemToolCallID(item)] = true
		}
	}
	latestWorkspaceRead := map[string]store.AgentTurnItem{}
	for _, item := range items {
		callID := contextItemToolCallID(item)
		switch item.Kind {
		case "tool_call":
			if callID != "" && !resultIDs[callID] {
				protected[callID] = true
			}
		case "tool_result", "aborted":
			if !successfulToolResult(item) || manifestToolIsMutation(contextItemToolName(item)) {
				protected[callID] = true
			}
		case "workspace_observation":
			payload := decodeContextItemPayload(item)
			path := normalizeWorkspaceEventPath(contextPayloadString(payload, "path", "target"))
			if path == "" {
				continue
			}
			key := path + "\x00" + contextPayloadString(payload, "view") + "\x00" + contextPayloadString(payload, "mode")
			latestWorkspaceRead[key] = item
		case "file_change":
			protected[callID] = true
		}
	}
	for _, item := range latestWorkspaceRead {
		if callID := contextItemToolCallID(item); callID != "" {
			protected[callID] = true
		}
	}
	return protected
}

func latestContextTurnIDs(items []store.AgentTurnItem, limit int) map[string]bool {
	result := map[string]bool{}
	for index := len(items) - 1; index >= 0 && len(result) < limit; index-- {
		if turnID := strings.TrimSpace(items[index].TurnID); turnID != "" {
			result[turnID] = true
		}
	}
	return result
}

func successfulToolResult(item store.AgentTurnItem) bool {
	status := strings.ToLower(strings.TrimSpace(item.Status))
	return status == "" || status == "success" || status == "succeeded" || status == "completed"
}

func compactedToolResultReference(item store.AgentTurnItem, payload contextItemPayload) string {
	summary := contextPayloadString(payload, "summary")
	if summary == "" {
		summary = toolResultContextPreview(contextItemContent(item), 1200)
	}
	metadata := map[string]any{
		"status": item.Status, "tool_name": contextItemToolName(item),
		"tool_call_id":         contextItemToolCallID(item),
		"path":                 contextPayloadString(payload, "path", "target"),
		"hash":                 contextPayloadString(payload, "hash", "content_hash", "doc_hash"),
		"ledger_id":            contextPayloadString(payload, "ledger_id"),
		"summary":              summary,
		"model_content_pruned": true,
	}
	raw, _ := json.Marshal(metadata)
	return string(raw)
}

func toolResultContextPreview(content string, maxRunes int) string {
	content = strings.TrimSpace(content)
	if content == "" || maxRunes <= 0 {
		return ""
	}
	var payload map[string]any
	if json.Unmarshal([]byte(content), &payload) == nil {
		for _, key := range []string{"summary", "message", "content", "text"} {
			if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
				content = strings.TrimSpace(value)
				break
			}
		}
	}
	return truncatePlainRunes(content, maxRunes)
}

func countContextTurns(items []store.AgentTurnItem) int {
	seen := map[string]bool{}
	for _, item := range items {
		if item.TurnID != "" {
			seen[item.TurnID] = true
		}
	}
	return len(seen)
}

func contextWorkingSetPaths(items []contextWorkingFile) []string {
	paths := make([]string, 0, len(items))
	for _, item := range items {
		paths = append(paths, item.Path)
	}
	return paths
}
