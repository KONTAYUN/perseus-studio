package ai

import (
	"context"
	"fmt"
	"strings"
)

func (r *agentRuntime) callModelForTurnEngine(ctx context.Context, round int) (agentModelTurn, error) {
	emitToolLifecycleEvent(r.opts, "model_message_started", "info", map[string]any{"round": round + 1})
	logTask(r.opts, "info", fmt.Sprintf("calling model round %d", round+1))
	originalAttemptHook := r.opts.OnProviderAttempt
	r.opts.OnProviderAttempt = func() {
		r.providerAttempts++
		if originalAttemptHook != nil {
			originalAttemptHook()
		}
	}
	defer func() { r.opts.OnProviderAttempt = originalAttemptHook }()
	if err := r.applyPendingSteers(ctx, round); err != nil {
		return agentModelTurn{}, err
	}

	phase := "turn_item_history"
	forceCompact := false
	for {
		request, err := r.buildTurnEngineProviderRequest(ctx, round, phase, forceCompact)
		if err != nil {
			return agentModelTurn{}, err
		}
		turn, err := r.service.providerV2(r.protocol).Turn(ctx, request)
		if err == nil {
			r.recordProviderTurnDiagnostics(turn)
			return turn, nil
		}
		if !r.contextOverflowRetried && isContextOverflowError(err) && !isContextCanceledError(err) {
			r.contextOverflowRetried = true
			phase = "overflow_recovery"
			forceCompact = true
			emitToolLifecycleEvent(r.opts, "context_overflow_recovered", "info", map[string]any{"round": round + 1})
			continue
		}
		if r.shouldCheckpointResume(ctx, err) && r.restoreLatestCheckpointForRecovery(ctx, err) {
			phase = "checkpoint_recovery"
			forceCompact = false
			continue
		}
		if r.checkpointResumeAttempts >= agentMaxCheckpointResumes() && isCheckpointRecoverableProviderError(err) {
			err = r.checkpointRecoveryFailedError(ctx, err)
		}
		return turn, err
	}
}

func (r *agentRuntime) buildTurnEngineProviderRequest(ctx context.Context, round int, phase string, forceCompact bool) (providerTurnRequest, error) {
	items, err := r.loadPersistentContextItems(ctx)
	if err != nil {
		return providerTurnRequest{}, err
	}
	tools := r.turnEngineTools()
	toolChoice := r.turnEngineToolChoice()
	assemble := func(forcePrune bool) ContextAssembly {
		return (ContextAssembler{}).Assemble(ContextAssemblerInput{
			Protocol: r.protocol, Items: items, Attachments: r.attachments, Tools: tools,
			Profile: r.profile, Round: round + 1, CurrentTurnID: firstNonEmpty(r.kernel.TurnID, r.messageID),
			Phase: phase, ForcePrune: forcePrune,
		})
	}
	assembly := assemble(forceCompact)
	if assembly.PrunedResults > 0 {
		emitToolLifecycleEvent(r.opts, "tool_results_pruned", "info", map[string]any{
			"count": assembly.PrunedResults, "active_context_tokens": assembly.Tokens,
		})
	}
	if assembly.NeedsCompaction && (r.profile.AutoCompactEnabled || forceCompact) {
		changed, compactErr := r.compactAgentItemContext(ctx, firstNonEmpty(phase, "auto_threshold"))
		if compactErr != nil {
			return providerTurnRequest{}, compactErr
		}
		if changed {
			items, err = r.loadPersistentContextItems(ctx)
			if err != nil {
				return providerTurnRequest{}, err
			}
			assembly = assemble(false)
		}
	}
	if assembly.NeedsCompaction {
		return providerTurnRequest{}, &ContextWindowError{EstimatedTokens: assembly.Tokens, BudgetTokens: assembly.ThresholdTokens, Stage: "context_assembly"}
	}
	manifest := r.turnEngineActionManifest()
	finalizeSnapshot := func(current ContextAssembly) (ContextSnapshot, int) {
		snapshot := injectTurnEngineProtocolInstructions(r.protocol, current.Snapshot)
		snapshot = injectModelActionManifest(r.protocol, snapshot, manifest)
		return snapshot, estimateContextSnapshotTokens(snapshot, tools)
	}
	var visibleTokens int
	r.lastContextSnapshot, visibleTokens = finalizeSnapshot(assembly)
	if visibleTokens > assembly.ThresholdTokens {
		changed, compactErr := r.compactAgentItemContext(ctx, "protocol_overhead")
		if compactErr != nil {
			return providerTurnRequest{}, compactErr
		}
		if changed {
			items, err = r.loadPersistentContextItems(ctx)
			if err != nil {
				return providerTurnRequest{}, err
			}
			assembly = assemble(false)
			if assembly.NeedsCompaction {
				return providerTurnRequest{}, &ContextWindowError{EstimatedTokens: assembly.Tokens, BudgetTokens: assembly.ThresholdTokens, Stage: "post_compaction"}
			}
			r.lastContextSnapshot, visibleTokens = finalizeSnapshot(assembly)
		}
	}
	r.lastContextSnapshot.Summary.ActiveContextTokens = visibleTokens
	r.lastContextSnapshot.Summary.EstimatedTokens = visibleTokens
	r.lastContextSnapshot.Summary.ContextWatermarkPercent = contextWatermarkPercent(visibleTokens, r.profile.ContextWindowTokens)
	if visibleTokens > assembly.ThresholdTokens {
		return providerTurnRequest{}, &ContextWindowError{EstimatedTokens: visibleTokens, BudgetTokens: assembly.ThresholdTokens, Stage: "protocol_serialization"}
	}
	r.annotateTurnEngineRequestSummary(manifest, tools, toolChoice)
	emitProviderRequestSummary(r.opts, r.lastContextSnapshot.Summary)
	return providerTurnRequest{
		ConversationID:    r.conversationID,
		MessageID:         r.messageID,
		Provider:          r.provider,
		Protocol:          r.protocol,
		Tools:             tools,
		ToolChoice:        toolChoice,
		Snapshot:          r.lastContextSnapshot,
		PerformedMutation: r.performedMutation,
		HasToolEvents:     len(r.events) > 0,
		// Tool-capable responses buffer assistant text until the provider has
		// classified the completed item. A response with a tool call becomes
		// progress; a response without one becomes the final assistant message.
		ActionBoundaryActive: true,
		Opts:                 r.opts,
	}, nil
}

func injectTurnEngineProtocolInstructions(protocol agentProtocol, snapshot ContextSnapshot) ContextSnapshot {
	instructions := strings.TrimSpace(turnEngineProtocolInstructions)
	if instructions == "" {
		return snapshot
	}
	if protocol == agentProtocolAnthropic {
		snapshot.AnthropicSystem = strings.TrimSpace(snapshot.AnthropicSystem + "\n\n" + instructions)
		return snapshot
	}
	message := map[string]any{"role": "system", "content": instructions}
	insertAt := 0
	for insertAt < len(snapshot.OpenAIMessages) {
		role, _ := snapshot.OpenAIMessages[insertAt]["role"].(string)
		if role != "system" {
			break
		}
		insertAt++
	}
	next := make([]map[string]any, 0, len(snapshot.OpenAIMessages)+1)
	next = append(next, snapshot.OpenAIMessages[:insertAt]...)
	next = append(next, message)
	next = append(next, snapshot.OpenAIMessages[insertAt:]...)
	snapshot.OpenAIMessages = next
	return snapshot
}

func (r *agentRuntime) annotateTurnEngineRequestSummary(manifest ModelActionManifest, tools []map[string]any, toolChoice providerToolChoice) {
	r.ensureKernelState()
	summary := &r.lastContextSnapshot.Summary
	summary.KernelVersion = r.kernel.Version
	summary.KernelMode = r.kernel.Mode
	summary.PromptPolicyVersion = agentActionPolicyVersion
	summary.KernelProgressEvents = len(r.kernel.ProgressEvents)
	summary.KernelReadTargets = len(r.kernel.ReadTargets)
	summary.ProviderAttempts = r.providerAttempts
	summary.CheckpointResumeAttempt = r.checkpointResumeAttempts
	summary.LoadedToolsets = loadedToolsetNames(r.loadedToolsets)
	summary.LoadedCapabilities = loadedToolsetCapabilitiesForDiagnostics(r.loadedToolsets)
	if r.protocol == agentProtocolAnthropic {
		summary.ToolNames = anthropicToolNames(tools)
	} else {
		summary.ToolNames = openAIToolNames(tools)
	}
	summary.ToolsCount = len(summary.ToolNames)
	summary.HasTools = summary.ToolsCount > 0
	summary.ProviderVisibleTools = append([]string(nil), summary.ToolNames...)
	summary.ToolChoice = providerToolChoiceSummary(toolChoice)
	summary.RequiresTool = toolChoice.requiresTool()
	summary.HasRequiredToolChoice = toolChoice.requiresTool()
	if summary.HasTools {
		summary.ParallelToolCalls = boolPtr(false)
	}
	summary.ModelActionManifestDigest = modelActionManifestDigest(manifest)
	summary.ModelTerminalActions = modelTerminalActionNames(manifest)
	summary.ProviderRoundCount = r.currentRound
	summary.ForcedTool = strings.TrimSpace(r.opts.ForceTool)
	summary.ToolLoopActive = false
	// Keep action-boundary buffering on for the whole turn-engine request.
	// This prevents a tool preamble from being published as a chat message
	// before the streamed response has been classified.
	summary.ActionBoundaryActive = true
	summary.AgentExecutionMode = "structured_tools"
	summary.ActionSelectionState = ActionSelectionState{}
	summary.LoopGuardState = AgentLoopGuard{}
}

func (r *agentRuntime) turnEngineTools() []map[string]any {
	includeLoader := r.includeToolsetLoader()
	if r.protocol == agentProtocolAnthropic {
		return anthropicToolsForLoadedToolsets(r.loadedToolsets, includeLoader, r.opts.UserRole)
	}
	return openAIToolsForLoadedToolsets(r.loadedToolsets, includeLoader, r.opts.UserRole)
}

func turnEngineToolPresent(tools []map[string]any, name string) bool {
	for _, tool := range tools {
		if turnEngineToolName(tool) == name {
			return true
		}
	}
	return false
}

func turnEngineToolName(tool map[string]any) string {
	if name, _ := tool["name"].(string); strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	function, _ := tool["function"].(map[string]any)
	name, _ := function["name"].(string)
	return strings.TrimSpace(name)
}

func (r *agentRuntime) turnEngineActionManifest() ModelActionManifest {
	manifest := modelActionManifest(r.loadedToolsets, r.includeToolsetLoader(), r.opts.UserRole)
	manifest.NextActionRules = []string{
		"Every response must select a structured tool action. Tool results are returned before the next model step.",
		"Plain assistant text cannot end an agent turn. Use attempt_completion when done, request_user_input for user-only open input, request_user_choice for a bounded user decision, or another tool to continue.",
		"Use apply_workspace_patch for file-content changes. Assistant text is presentation only and never executes edits.",
		"Terminal actions must be the only tool call in their response.",
	}
	return manifest
}

func (r *agentRuntime) turnEngineToolChoice() providerToolChoice {
	forced := strings.TrimSpace(r.opts.ForceTool)
	if forced != "" && r.toolCalls == 0 && turnEngineToolPresent(r.turnEngineTools(), forced) {
		return providerToolChoice{Mode: providerToolChoiceForced, ToolName: forced}
	}
	return providerToolChoice{Mode: providerToolChoiceAuto}
}
