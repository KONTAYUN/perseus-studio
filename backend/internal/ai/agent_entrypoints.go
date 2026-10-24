package ai

import (
	"context"

	"webide-backend/internal/store"
)

func (s *Service) runOpenAIAgent(ctx context.Context, conversationID, messageID string, provider store.ModelProvider, messages []store.Message, attachments []AttachmentPayload, opts RunOptions) (Reply, error) {
	return s.runAgent(ctx, conversationID, messageID, agentProtocolOpenAI, provider, messages, attachments, opts)
}
func (s *Service) runAnthropicAgent(ctx context.Context, conversationID, messageID string, provider store.ModelProvider, messages []store.Message, attachments []AttachmentPayload, opts RunOptions) (Reply, error) {
	return s.runAgent(ctx, conversationID, messageID, agentProtocolAnthropic, provider, messages, attachments, opts)
}
func (s *Service) runAgent(ctx context.Context, conversationID, messageID string, protocol agentProtocol, provider store.ModelProvider, messages []store.Message, attachments []AttachmentPayload, opts RunOptions) (Reply, error) {
	profile := runtimeProfileForProvider(provider)
	if opts.DocumentAction != nil {
		opts.AllowedWorkspacePaths = []string{opts.DocumentAction.Path}
		attachments = nil
	}
	messages = sanitizeConversationMessages(messages, profile)
	attachments = sanitizeAttachmentPayloads(attachments, profile)
	attachments = s.prefetchWorkspaceFileAttachments(ctx, opts.UserID, attachments)
	runtime := &agentRuntime{
		service:            s,
		protocol:           protocol,
		conversationID:     conversationID,
		messageID:          messageID,
		provider:           provider,
		messages:           messages,
		attachments:        attachments,
		opts:               opts,
		profile:            profile,
		allowTask:          opts.TaskID == "",
		recentReadMemory:   s.recentWorkspaceReadMemory(ctx, opts.UserID, conversationID, attachments),
		loadedToolsets:     initialAgentToolsets(opts, attachments),
		appliedMutations:   map[string]ToolEvent{},
		chunkWrites:        map[string]workspaceChunkWriteState{},
		requiresCompletion: opts.ForceTool != "" || opts.TaskID != "",
		toolLoopActive:     opts.ForceTool != "" || opts.TaskID != "" || attachmentsRequireToolLoop(attachments),
		transcript:         newAgentTranscript(),
		contextManager:     newAgentContextManager(),
	}
	if checkpoint, ok, err := s.loadRunCheckpointStrict(ctx, opts); err != nil {
		return Reply{}, err
	} else if ok {
		runtime.restoreCheckpoint(checkpoint)
	}
	if err := runtime.restoreDurableToolLedger(ctx); err != nil {
		return Reply{}, err
	}
	return runtime.run(ctx)
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

func initialAgentToolsets(opts RunOptions, attachments []AttachmentPayload) map[string]bool {
	if opts.TaskID != "" {
		return allAgentToolsets(false)
	}
	loaded := map[string]bool{"workspace": true}
	if opts.TaskID != "" || opts.ForceTool != "" || attachmentsRequireToolLoop(attachments) {
		loaded["workspace"] = true
	}
	return loaded
}
