package ai

import (
	"context"
	"time"

	"webide-backend/internal/store"
)

type providerTurnRequest struct {
	ConversationID                string
	MessageID                     string
	Provider                      store.ModelProvider
	Protocol                      agentProtocol
	Tools                         []map[string]any
	Snapshot                      ContextSnapshot
	PerformedMutation             bool
	HasToolEvents                 bool
	HasVerifiedCompletionEvidence bool
	Opts                          RunOptions
}

type providerAdapterV2 interface {
	Turn(ctx context.Context, req providerTurnRequest) (agentModelTurn, error)
}

func (s *Service) providerV2(protocol agentProtocol) providerAdapterV2 {
	switch protocol {
	case agentProtocolAnthropic:
		return anthropicProviderV2{service: s}
	default:
		return openAIProviderV2{service: s}
	}
}

func emitProviderRequestSummary(opts RunOptions, summary ProviderRequestSummary) {
	if opts.Event == nil {
		return
	}
	opts.Event("model_request_summary", "info", map[string]any{
		"round":                       summary.Round,
		"protocol":                    summary.Protocol,
		"message_count":               summary.MessageCount,
		"transcript_entries":          summary.TranscriptEntries,
		"tool_result_count":           summary.ToolResultCount,
		"tool_loop_active":            summary.ToolLoopActive,
		"estimated_chars":             summary.EstimatedChars,
		"estimated_tool_result_chars": summary.EstimatedToolResultChars,
		"max_segment_chars":           summary.MaxSegmentChars,
		"compaction_actions":          summary.CompactionActions,
	})
}

func (s *Service) recordProviderDiagnosticWithSummary(ctx context.Context, opts RunOptions, provider store.ModelProvider, conversationID, messageID, stage, level string, durationMs int64, err error, summary ProviderRequestSummary) string {
	if s.diag == nil || opts.UserID == "" || err == nil {
		return ""
	}
	return s.recordProviderDiagnosticSummary(ctx, opts, provider, conversationID, messageID, stage, level, durationFromMilliseconds(durationMs), err, summary.JSON())
}

func durationFromMilliseconds(ms int64) time.Duration { return time.Duration(ms) * time.Millisecond }
