package ai

import (
	"context"

	"webide-backend/internal/store"
)

func (s *Service) runOpenAIAgent(ctx context.Context, conversationID, messageID string, provider store.ModelProvider, _ []store.Message, attachments []AttachmentPayload, opts RunOptions) (Reply, error) {
	if err := validateAgentProviderCapabilities(provider, attachments); err != nil {
		return Reply{}, err
	}
	profile := runtimeProfileForProvider(provider)
	contextItems, err := s.prepareAgentTurnHistory(ctx, conversationID, messageID, attachments, opts)
	if err != nil {
		return Reply{}, err
	}
	kernel := newAgentKernelState()
	runtime := &agentRuntime{
		service:          s,
		protocol:         agentProtocolOpenAI,
		conversationID:   conversationID,
		messageID:        messageID,
		provider:         provider,
		attachments:      attachments,
		opts:             opts,
		profile:          profile,
		allowTask:        opts.TaskID == "",
		loadedToolsets:   initialAgentToolsets(opts, attachments),
		appliedMutations: map[string]ToolEvent{},
		chunkWrites:      map[string]workspaceChunkWriteState{},
		transcript:       newAgentTranscript(),
		kernel:           kernel,
		contextItems:     contextItems,
	}
	if checkpoint, ok := s.loadRunCheckpoint(ctx, opts); ok {
		runtime.restoreCheckpoint(checkpoint)
	}
	runtime.beginPresentationSegment()
	return runtime.runTurnEngine(ctx)
}

func (s *Service) runAnthropicAgent(ctx context.Context, conversationID, messageID string, provider store.ModelProvider, _ []store.Message, attachments []AttachmentPayload, opts RunOptions) (Reply, error) {
	if err := validateAgentProviderCapabilities(provider, attachments); err != nil {
		return Reply{}, err
	}
	profile := runtimeProfileForProvider(provider)
	contextItems, err := s.prepareAgentTurnHistory(ctx, conversationID, messageID, attachments, opts)
	if err != nil {
		return Reply{}, err
	}
	kernel := newAgentKernelState()
	runtime := &agentRuntime{
		service:          s,
		protocol:         agentProtocolAnthropic,
		conversationID:   conversationID,
		messageID:        messageID,
		provider:         provider,
		attachments:      attachments,
		opts:             opts,
		profile:          profile,
		allowTask:        opts.TaskID == "",
		loadedToolsets:   initialAgentToolsets(opts, attachments),
		appliedMutations: map[string]ToolEvent{},
		chunkWrites:      map[string]workspaceChunkWriteState{},
		transcript:       newAgentTranscript(),
		kernel:           kernel,
		contextItems:     contextItems,
	}
	if checkpoint, ok := s.loadRunCheckpoint(ctx, opts); ok {
		runtime.restoreCheckpoint(checkpoint)
	}
	runtime.beginPresentationSegment()
	return runtime.runTurnEngine(ctx)
}

func validateAgentProviderCapabilities(provider store.ModelProvider, attachments []AttachmentPayload) error {
	if provider.SupportsTools == store.ModelCapabilityUnsupported {
		return &ProviderCapabilityError{Capability: "tools", ModelID: provider.ModelID}
	}
	if provider.SupportsVision != store.ModelCapabilityUnsupported {
		return nil
	}
	for _, attachment := range attachments {
		if attachment.Kind == "image" && attachment.DataBase64 != "" {
			return &ProviderCapabilityError{Capability: "vision", ModelID: provider.ModelID}
		}
	}
	return nil
}

func allAgentToolsets(allowTask bool) map[string]bool {
	loaded := map[string]bool{
		"workspace": true,
		"command":   true,
		"search":    true,
		"skills":    true,
	}
	if allowTask {
		loaded["task"] = true
	}
	return loaded
}

func initialAgentToolsets(_ RunOptions, _ []AttachmentPayload) map[string]bool {
	return map[string]bool{"workspace": true}
}
