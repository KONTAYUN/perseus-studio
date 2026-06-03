package ai

import (
	"fmt"
	"sort"
	"strings"

	"webide-backend/internal/store"
)

const (
	providerMessageBuilderTriggerBytes   = 64 * 1024
	providerMessageBuilderTargetBytes    = 48 * 1024
	providerMessageBuilderTriggerEntries = 16
	providerMessageBuilderRecentEntries  = 8
	providerMessageBuilderSummaryRunes   = 3600
)

type ProviderMessageBuilder struct{}

type ProviderMessageBuildInput struct {
	Protocol      agentProtocol
	Messages      []store.Message
	Attachments   []AttachmentPayload
	Transcript    *AgentTranscript
	Actions       []string
	Profile       providerRuntimeProfile
	Round         int
	Options       ProviderMessageBuildOptions
	BeforePayload int
}

type ProviderMessageBuildResult struct {
	Applied                  bool
	Strategy                 string
	Phase                    string
	Messages                 []store.Message
	Transcript               *AgentTranscript
	Actions                  []string
	PayloadBeforeBytes       int
	CompactedEntries         int
	PreservedRecentEntries   int
	ContinuationSummaryChars int
	PreservedChangedPaths    []string
	PreservedReadRanges      []string
}

type ProviderContinuationSummary struct {
	SuccessfulTools  []string
	FailedTools      []string
	ReadPaths        []string
	ChangedPaths     []string
	Failures         []string
}

func (b ProviderMessageBuilder) Build(input ProviderMessageBuildInput) ProviderMessageBuildResult {
	transcript := input.Transcript.Clone()
	if transcript == nil {
		transcript = newAgentTranscript()
	}
	options := normalizeProviderMessageBuildOptions(input.Options)
	beforePayload := input.BeforePayload
	if beforePayload <= 0 {
		beforePayload = providerMessageBuilderPayloadEstimate(input.Protocol, input.Messages, input.Attachments, transcript, input.Actions, input.Profile, input.Round)
	}
	shouldCompact := options.ForceCompact || beforePayload > options.TriggerBytes || len(transcript.Entries) > options.TriggerEntries
	if !shouldCompact {
		return ProviderMessageBuildResult{Transcript: transcript, Messages: input.Messages, Actions: input.Actions, PayloadBeforeBytes: beforePayload, Phase: options.Phase}
	}
	preserveRecent := options.PreserveRecent
	if preserveRecent <= 0 {
		preserveRecent = providerMessageBuilderRecentEntries
	}
	if preserveRecent >= len(transcript.Entries) {
		preserveRecent = len(transcript.Entries)
	}
	compactedCount := len(transcript.Entries) - preserveRecent
	if compactedCount > 0 && compactedCount < len(transcript.Entries) && transcript.Entries[compactedCount].Kind == transcriptEntryTool {
		compactedCount--
		preserveRecent++
	}
	if compactedCount <= 0 {
		return ProviderMessageBuildResult{Transcript: transcript, Messages: input.Messages, Actions: input.Actions, PayloadBeforeBytes: beforePayload, Phase: options.Phase}
	}
	older := transcript.Entries[:compactedCount]
	recent := cloneTranscriptEntries(transcript.Entries[compactedCount:])
	summary := buildProviderContinuationSummary(older, input.Actions)
	preservedChangedPaths := changedPathsFromTranscript(transcript.Entries)
	preservedReadRanges := readRangeLabelsFromTranscript(recent)
	summaryText := formatProviderContinuationSummary(summary)
	summaryText, _ = truncatePlainForModel(summaryText, providerMessageBuilderSummaryRunes)
	nextTranscript := newAgentTranscript()
	nextTranscript.AppendCompactionSummary(summaryText, map[string]any{
		"reason":            "provider_message_builder_compaction",
		"strategy":          strategyName(options),
		"compacted_entries": compactedCount,
		"source":            "environment",
	})
	nextTranscript.Entries = append(nextTranscript.Entries, cloneTranscriptEntries(recent)...)
	result := ProviderMessageBuildResult{
		Applied:                  true,
		Strategy:                 strategyName(options),
		Phase:                    options.Phase,
		Messages:                 compactConversationMessagesForProvider(input.Messages),
		Transcript:               nextTranscript,
		Actions:                  appendProviderMessageBuilderAction(input.Actions, compactedCount, len(recent), options),
		PayloadBeforeBytes:       beforePayload,
		CompactedEntries:         compactedCount,
		PreservedRecentEntries:   len(recent),
		ContinuationSummaryChars: len([]rune(summaryText)),
		PreservedChangedPaths:    preservedChangedPaths,
		PreservedReadRanges:      preservedReadRanges,
	}
	return result.fitTarget(input, options)
}

func (r ProviderMessageBuildResult) fitTarget(input ProviderMessageBuildInput, options ProviderMessageBuildOptions) ProviderMessageBuildResult {
	target := options.TargetBytes
	if target <= 0 {
		target = providerMessageBuilderTargetBytes
	}
	if target <= 0 || r.Transcript == nil {
		return r
	}
	payload := providerMessageBuilderPayloadEstimate(input.Protocol, r.Messages, input.Attachments, r.Transcript, r.Actions, input.Profile, input.Round)
	if payload <= target {
		return r
	}
	for _, limit := range []int{2400, 1600, 1000} {
		if len(r.Transcript.Entries) == 0 || r.Transcript.Entries[0].Kind != transcriptEntryCompactionSummary {
			break
		}
		text, truncated := truncatePlainForModel(r.Transcript.Entries[0].Text, limit)
		if !truncated {
			continue
		}
		r.Transcript.Entries[0].Text = text
		r.ContinuationSummaryChars = len([]rune(r.Transcript.Entries[0].Text))
		payload = providerMessageBuilderPayloadEstimate(input.Protocol, r.Messages, input.Attachments, r.Transcript, r.Actions, input.Profile, input.Round)
		if payload <= target {
			break
		}
	}
	allowCurrentReadCompaction := options.ForceCompact && options.Phase != "tool_loop"
	if payload > target && compactLargeReadEntriesForProvider(r.Transcript.Entries, allowCurrentReadCompaction) {
		r.PreservedReadRanges = readRangeLabelsFromTranscript(r.Transcript.Entries)
		payload = providerMessageBuilderPayloadEstimate(input.Protocol, r.Messages, input.Attachments, r.Transcript, r.Actions, input.Profile, input.Round)
	}
	return r
}

func normalizeProviderMessageBuildOptions(options ProviderMessageBuildOptions) ProviderMessageBuildOptions {
	if options.TriggerBytes <= 0 {
		options.TriggerBytes = providerMessageBuilderTriggerBytes
	}
	if options.TargetBytes <= 0 {
		options.TargetBytes = providerMessageBuilderTargetBytes
	}
	if options.TriggerEntries <= 0 {
		options.TriggerEntries = providerMessageBuilderTriggerEntries
	}
	if options.PreserveRecent <= 0 {
		options.PreserveRecent = providerMessageBuilderRecentEntries
	}
	if strings.TrimSpace(options.Reason) == "" {
		options.Reason = "payload_or_transcript_pressure"
	}
	if strings.TrimSpace(options.Phase) == "" {
		options.Phase = options.Reason
	}
	return options
}

func strategyName(options ProviderMessageBuildOptions) string {
	reason := strings.TrimSpace(options.Reason)
	if reason == "" {
		reason = "payload_or_transcript_pressure"
	}
	return "deterministic_continuation_summary:" + reason
}

func appendProviderMessageBuilderAction(actions []string, compacted, preserved int, options ProviderMessageBuildOptions) []string {
	next := append([]string(nil), actions...)
	next = append(next, fmt.Sprintf("provider_message_builder_compacted:%s:entries=%d:preserved=%d", strings.TrimPrefix(strategyName(options), "deterministic_continuation_summary:"), compacted, preserved))
	return dedupeStrings(next)
}

func buildProviderContinuationSummary(entries []TranscriptEntry, actions []string) ProviderContinuationSummary {
	summary := ProviderContinuationSummary{}
	successfulTools := map[string]bool{}
	failedTools := map[string]bool{}
	readPaths := map[string]bool{}
	changedPaths := map[string]bool{}
	failures := map[string]bool{}
	_ = actions
	for _, entry := range entries {
		switch entry.Kind {
		case transcriptEntryAssistant:
			for _, call := range entry.ToolCalls {
				if strings.TrimSpace(call.Name) != "" {
					// Tool calls are summarized by tool name below.
				}
			}
		case transcriptEntryUser:
			continue
		case transcriptEntryCompactionSummary:
			continue
		case transcriptEntryTool:
			for _, result := range entry.ToolResults {
				name := strings.TrimSpace(result.ToolName)
				if name == "" {
					name = strings.TrimSpace(result.Event.ToolName)
				}
				if result.IsError || strings.EqualFold(result.Event.Status, "failed") {
					failedTools[name] = true
					if result.Event.Error != "" {
						failures[name+": "+compactTextPreview(result.Event.Error, 180)] = true
					} else if text := compactTextPreview(result.Content, 180); text != "" {
						failures[name+": "+text] = true
					}
				} else {
					successfulTools[name] = true
				}
				for _, path := range workspaceEventPaths(result.Event) {
					if path != "" && isReadWorkspaceTool(name) {
						readPaths[path] = true
					}
					if path != "" && isWorkspaceMutationTool(name) {
						changedPaths[path] = true
					}
				}
			}
		}
	}
	summary.SuccessfulTools = sortedBoolMapKeys(successfulTools)
	summary.FailedTools = sortedBoolMapKeys(failedTools)
	summary.ReadPaths = sortedBoolMapKeys(readPaths)
	summary.ChangedPaths = sortedBoolMapKeys(changedPaths)
	summary.Failures = sortedBoolMapKeys(failures)
	return summary
}

func changedPathsFromTranscript(entries []TranscriptEntry) []string {
	paths := map[string]bool{}
	for _, entry := range entries {
		if entry.Kind != transcriptEntryTool {
			continue
		}
		for _, result := range entry.ToolResults {
			name := strings.TrimSpace(result.ToolName)
			if name == "" {
				name = strings.TrimSpace(result.Event.ToolName)
			}
			if result.IsError || strings.EqualFold(result.Event.Status, "failed") || !isWorkspaceMutationTool(name) {
				continue
			}
			for _, path := range workspaceEventPaths(result.Event) {
				if path != "" {
					paths[path] = true
				}
			}
		}
	}
	return sortedBoolMapKeys(paths)
}

func readRangeLabelsFromTranscript(entries []TranscriptEntry) []string {
	seen := map[string]bool{}
	var labels []string
	for _, entry := range entries {
		if entry.Kind != transcriptEntryTool {
			continue
		}
		for _, result := range entry.ToolResults {
			if result.IsError || !isReadWorkspaceTool(result.ToolName) {
				continue
			}
			for _, label := range readSpanLabels(readResultSpans(&result)) {
				if label != "" && !seen[label] {
					seen[label] = true
					labels = append(labels, label)
				}
			}
		}
	}
	sort.Strings(labels)
	return labels
}

func compactLargeReadEntriesForProvider(entries []TranscriptEntry, allowCurrentReadCompaction bool) bool {
	currentReadCalls := currentReadWorkingSetToolCalls(&AgentTranscript{Entries: entries}, 1)
	changed := false
	for entryIndex := range entries {
		if entries[entryIndex].Kind != transcriptEntryTool {
			continue
		}
		for resultIndex := range entries[entryIndex].ToolResults {
			result := &entries[entryIndex].ToolResults[resultIndex]
			if result.IsError || !isReadWorkspaceTool(result.ToolName) {
				continue
			}
			if currentReadCalls[result.ToolCallID] && !allowCurrentReadCompaction {
				continue
			}
			if len([]byte(result.Content)) <= 1600 || isCompactedToolResultContent(result.Content) {
				continue
			}
			result.Content = compactedToolResultNotice(result.ToolName, result.Event, "Use the recorded path and line range as historical context. Read this path/range again if exact text is needed.")
			changed = true
		}
	}
	return changed
}

func formatProviderContinuationSummary(summary ProviderContinuationSummary) string {
	var b strings.Builder
	b.WriteString("Earlier task context summary. This is not user input and is not current file contents.\n")
	writeSummaryList(&b, "Successful tools", summary.SuccessfulTools)
	writeSummaryList(&b, "Failed tools", summary.FailedTools)
	writeSummaryList(&b, "Workspace files read earlier", summary.ReadPaths)
	writeSummaryList(&b, "Verified changed paths", summary.ChangedPaths)
	writeSummaryList(&b, "Earlier failures to respect", summary.Failures)
	b.WriteString("\nContinue from these facts plus the preserved recent transcript below. Do not repeat verified workspace mutations; use changed paths above as already applied facts. If exact file text is needed, call the workspace read tool for the specific path and line range.")
	return b.String()
}

func writeSummaryList(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		return
	}
	b.WriteString("\n")
	b.WriteString(title)
	b.WriteString(":\n")
	for _, item := range items {
		b.WriteString("- ")
		b.WriteString(item)
		b.WriteString("\n")
	}
}

func compactConversationMessagesForProvider(messages []store.Message) []store.Message {
	messages = dropAssistantPlaceholderMessages(messages)
	if len(messages) <= 6 {
		return append([]store.Message(nil), messages...)
	}
	firstUser := -1
	lastUser := -1
	for index, message := range messages {
		if message.Role == "user" {
			if firstUser < 0 {
				firstUser = index
			}
			lastUser = index
		}
	}
	keep := map[int]bool{}
	for index, message := range messages {
		if message.Role == "system" {
			keep[index] = true
		}
	}
	if firstUser >= 0 {
		keep[firstUser] = true
	}
	if lastUser >= 0 {
		keep[lastUser] = true
	}
	for index := len(messages) - 4; index < len(messages); index++ {
		if index >= 0 {
			keep[index] = true
		}
	}
	next := make([]store.Message, 0, len(keep)+1)
	omitted := 0
	for index, message := range messages {
		if keep[index] {
			next = append(next, message)
		} else {
			omitted++
		}
	}
	if omitted > 0 {
		insertAt := 0
		for insertAt < len(next) && next[insertAt].Role == "system" {
			insertAt++
		}
		summary := store.Message{
			Role:    "system",
			Content: fmt.Sprintf("Conversation history note: %d older chat message(s) were folded out of this provider request. The active task state is preserved in the provider continuation summary and recent messages.", omitted),
		}
		next = append(next, store.Message{})
		copy(next[insertAt+1:], next[insertAt:])
		next[insertAt] = summary
	}
	return next
}

func providerMessageBuilderPayloadEstimate(protocol agentProtocol, messages []store.Message, attachments []AttachmentPayload, transcript *AgentTranscript, actions []string, profile providerRuntimeProfile, round int) int {
	snapshot := newAgentContextManager().buildFromCompacted(protocol, messages, attachments, transcript, actions, profile, round)
	return estimatedContextPayloadBytes(snapshot)
}

func cloneTranscriptEntries(entries []TranscriptEntry) []TranscriptEntry {
	if len(entries) == 0 {
		return nil
	}
	tmp := &AgentTranscript{Entries: entries}
	return tmp.Clone().Entries
}

func compactTextPreview(text string, limit int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = strings.Join(strings.Fields(text), " ")
	text, _ = truncatePlainForModel(text, limit)
	return text
}

func appendLimitedUnique(items []string, item string, limit int) []string {
	item = strings.TrimSpace(item)
	if item == "" {
		return items
	}
	for _, existing := range items {
		if existing == item {
			return items
		}
	}
	if limit > 0 && len(items) >= limit {
		return items
	}
	return append(items, item)
}

func sortedBoolMapKeys(values map[string]bool) []string {
	if len(values) == 0 {
		return nil
	}
	items := make([]string, 0, len(values))
	for value := range values {
		if strings.TrimSpace(value) != "" {
			items = append(items, value)
		}
	}
	sort.Strings(items)
	return items
}
