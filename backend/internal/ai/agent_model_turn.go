package ai

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
)

func (r *agentRuntime) callModel(ctx context.Context, round int) (agentModelTurn, error) {
	emitToolLifecycleEvent(r.opts, "model_message_started", "info", map[string]any{"round": round + 1})
	logTask(r.opts, "info", fmt.Sprintf("calling model round %d", round+1))
	originalAttemptHook := r.opts.OnProviderAttempt
	r.opts.OnProviderAttempt = func() {
		r.providerAttempts++
		if originalAttemptHook != nil {
			originalAttemptHook()
		}
	}
	defer func() {
		r.opts.OnProviderAttempt = originalAttemptHook
	}()
	if err := r.applyPendingSteers(ctx, round); err != nil {
		return agentModelTurn{}, err
	}
	r.lastContextSnapshot = r.contextManager.Build(r.protocol, r.messages, r.attachments, r.transcript, r.profile, round+1)
	r.annotateRequestSummary()
	emitProviderRequestSummary(r.opts, r.lastContextSnapshot.Summary)
	turn, err := r.service.providerV2(r.protocol).Turn(ctx, providerTurnRequest{
		ConversationID:       r.conversationID,
		MessageID:            r.messageID,
		Provider:             r.provider,
		Protocol:             r.protocol,
		Tools:                r.tools(),
		ToolChoice:           r.toolChoice(),
		Snapshot:             r.lastContextSnapshot,
		PerformedMutation:    r.performedMutation,
		HasToolEvents:        len(r.events) > 0,
		ActionBoundaryActive: r.actionBoundaryActive(),
		Opts:                 r.opts,
	})
	if err == nil || r.contextOverflowRetried || !isContextOverflowError(err) || isContextCanceledError(err) {
		return turn, err
	}
	r.contextOverflowRetried = true
	if retryErr := r.forceContextOverflowCompaction(ctx, round+1); retryErr != nil {
		log.Printf("[ai] context overflow compaction skipped conversation=%s error=%v", r.conversationID, retryErr)
	}
	r.lastContextSnapshot = r.contextManager.Build(r.protocol, r.messages, r.attachments, r.transcript, r.profile, round+1)
	r.annotateRequestSummary()
	emitProviderRequestSummary(r.opts, r.lastContextSnapshot.Summary)
	return r.service.providerV2(r.protocol).Turn(ctx, providerTurnRequest{
		ConversationID:       r.conversationID,
		MessageID:            r.messageID,
		Provider:             r.provider,
		Protocol:             r.protocol,
		Tools:                r.tools(),
		ToolChoice:           r.toolChoice(),
		Snapshot:             r.lastContextSnapshot,
		PerformedMutation:    r.performedMutation,
		HasToolEvents:        len(r.events) > 0,
		ActionBoundaryActive: r.actionBoundaryActive(),
		Opts:                 r.opts,
	})
}

func (r *agentRuntime) annotateRequestSummary() {
	r.lastContextSnapshot.Summary.ProviderAttempts = r.providerAttempts
	r.lastContextSnapshot.Summary.CheckpointResumeAttempt = r.checkpointResumeAttempts
	r.lastContextSnapshot.Summary.LoadedToolsets = loadedToolsetNames(r.loadedToolsets)
	r.lastContextSnapshot.Summary.LoadedCapabilities = loadedToolsetCapabilitiesForDiagnostics(r.loadedToolsets)
	r.lastContextSnapshot.Summary.ForcedTool = strings.TrimSpace(r.opts.ForceTool)
	r.lastContextSnapshot.Summary.ToolLoopActive = r.toolLoopActive
	r.lastContextSnapshot.Summary.ActionBoundaryActive = r.actionBoundaryActive()
	if r.checkpointResumeAttempts > 0 {
		r.lastContextSnapshot.Summary.RecoverableTransientError = true
	}
	if checkpoint, ok := r.service.loadRunCheckpoint(context.Background(), r.opts); ok {
		r.lastContextSnapshot.Summary.CheckpointSeq = checkpoint.Seq
	}
}

func loadedToolsetCapabilitiesForDiagnostics(loaded map[string]bool) map[string]any {
	if !loaded["workspace"] {
		return nil
	}
	return toolsetCapabilities("workspace")
}

func loadedToolsetNames(loaded map[string]bool) []string {
	names := make([]string, 0, len(loaded))
	for name, ok := range loaded {
		if ok && strings.TrimSpace(name) != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func (r *agentRuntime) forceContextOverflowCompaction(ctx context.Context, round int) error {
	r.profile.AggressiveContextPruning = true
	if r.service == nil || r.service.calls == nil || r.opts.UserID == "" || r.conversationID == "" || r.opts.TaskID != "" {
		emitToolLifecycleEvent(r.opts, "context_compaction_started", "info", map[string]any{
			"reason": "overflow_retry",
			"round":  round,
		})
		emitToolLifecycleEvent(r.opts, "context_compaction_completed", "info", map[string]any{
			"reason":  "overflow_retry",
			"round":   round,
			"message": "runtime tool context pruning enabled",
		})
		return nil
	}
	messages, err := r.service.calls.ListMessages(ctx, r.opts.UserID, r.conversationID)
	if err != nil {
		return err
	}
	current, err := r.service.calls.GetConversationSummary(ctx, r.opts.UserID, r.conversationID)
	if err != nil {
		return err
	}
	result := r.service.maybeRefreshConversationSummary(ctx, r.conversationID, r.provider, messages, current, r.opts, r.profile, "overflow_retry", "")
	if result.Updated {
		r.messages = sanitizeConversationMessages(injectConversationSummary(messages, result.Summary, result.SummaryUntilMessageID), r.profile)
		return nil
	}
	r.messages = sanitizeConversationMessages(injectConversationSummary(messages, current.Summary, current.UntilMessageID), r.profile)
	return nil
}

func (r *agentRuntime) tools() []map[string]any {
	includeLoader := r.includeToolsetLoader()
	switch r.protocol {
	case agentProtocolAnthropic:
		return anthropicToolsForLoadedToolsets(r.loadedToolsets, includeLoader, r.opts.UserRole)
	default:
		return openAIToolsForLoadedToolsets(r.loadedToolsets, includeLoader, r.opts.UserRole)
	}
}

func (r *agentRuntime) includeToolsetLoader() bool {
	return r != nil && r.opts.TaskID == ""
}

func (r *agentRuntime) toolChoice() providerToolChoice {
	return providerToolChoice{Mode: providerToolChoiceAuto}
}

func (r *agentRuntime) actionBoundaryActive() bool {
	if r == nil {
		return false
	}
	return r.toolLoopActive ||
		r.requiresCompletion ||
		strings.TrimSpace(r.opts.ForceTool) != "" ||
		strings.TrimSpace(r.opts.TaskID) != "" ||
		attachmentsRequireToolLoop(r.attachments)
}
