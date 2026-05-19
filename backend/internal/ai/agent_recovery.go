package ai

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func (r *agentRuntime) callModelWithRecovery(ctx context.Context, round int) (agentModelTurn, error) {
	for {
		turn, err := r.callModel(ctx, round)
		if err == nil || !r.shouldCheckpointResume(ctx, err) {
			if err != nil && r.checkpointResumeAttempts >= agentMaxCheckpointResumes() && isCheckpointRecoverableProviderError(err) {
				err = r.checkpointRecoveryFailedError(ctx, err)
			}
			return turn, err
		}
		if !r.restoreLatestCheckpointForRecovery(ctx, err) {
			return turn, err
		}
	}
}

func (r *agentRuntime) shouldCheckpointResume(ctx context.Context, err error) bool {
	if err == nil || isContextCanceledError(err) || !isCheckpointRecoverableProviderError(err) {
		return false
	}
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	return r.checkpointResumeAttempts < agentMaxCheckpointResumes()
}

func (r *agentRuntime) restoreLatestCheckpointForRecovery(ctx context.Context, err error) bool {
	checkpoint, ok := r.service.loadRunCheckpoint(ctx, r.opts)
	if !ok || checkpoint.Transcript == nil || len(checkpoint.Transcript.Entries) == 0 {
		return false
	}
	r.checkpointResumeAttempts++
	maxAttempts := agentMaxCheckpointResumes()
	delay := agentRecoveryDelay(r.checkpointResumeAttempts)
	diagID := DiagnosticIDFromError(err)
	if diagID != "" {
		r.checkpointResumeDiagIDs = append(r.checkpointResumeDiagIDs, diagID)
	}
	if r.service != nil {
		r.service.closeIdleProviderConnections()
	}
	payload := map[string]any{
		"reason":         sanitizeUserFacingAIError(err.Error()),
		"attempt":        r.checkpointResumeAttempts,
		"max_attempts":   maxAttempts,
		"checkpoint_seq": checkpoint.Seq,
		"round":          checkpoint.CurrentRound,
		"diagnostic_id":  diagID,
		"delay_ms":       delay.Milliseconds(),
	}
	if r.checkpointResumeAttempts == 1 {
		emitToolLifecycleEvent(r.opts, "agent_recovery_started", "warn", payload)
	}
	emitToolLifecycleEvent(r.opts, "agent_recovery_retrying", "warn", payload)
	if !sleepForAgentRecovery(ctx, delay) {
		return false
	}
	r.restoreCheckpoint(checkpoint)
	r.transcript.AppendUserText("上一轮模型供应商连接中断。请从已有工具结果和当前断点继续，不要重复已经成功执行的 workspace 变更；需要更多信息时继续调用工具，完成后调用 attempt_completion。", map[string]any{
		"reason":         "checkpoint_resume_after_provider_error",
		"attempt":        r.checkpointResumeAttempts,
		"checkpoint_seq": checkpoint.Seq,
		"diagnostic_id":  diagID,
	})
	return true
}

func (r *agentRuntime) emitCheckpointRecoveryCompleted() {
	if r.checkpointResumeAttempts == 0 {
		return
	}
	emitToolLifecycleEvent(r.opts, "agent_recovery_completed", "info", map[string]any{
		"attempt":        r.checkpointResumeAttempts,
		"max_attempts":   agentMaxCheckpointResumes(),
		"diagnostic_ids": r.checkpointResumeDiagIDs,
	})
}

func (r *agentRuntime) checkpointRecoveryFailedError(ctx context.Context, err error) error {
	if r.checkpointResumeAttempts == 0 {
		return err
	}
	emitToolLifecycleEvent(r.opts, "agent_recovery_failed", "error", map[string]any{
		"reason":         sanitizeUserFacingAIError(err.Error()),
		"attempt":        r.checkpointResumeAttempts,
		"max_attempts":   agentMaxCheckpointResumes(),
		"diagnostic_id":  DiagnosticIDFromError(err),
		"diagnostic_ids": r.checkpointResumeDiagIDs,
	})
	diagCtx := ctx
	if diagCtx == nil || diagCtx.Err() != nil {
		diagCtx = context.Background()
	}
	summary := r.lastContextSnapshot.Summary
	summary.ProviderAttempts = r.providerAttempts
	summary.CheckpointResumeAttempt = r.checkpointResumeAttempts
	summary.RecoverableTransientError = true
	diagID := r.service.recordProviderDiagnosticWithSummary(diagCtx, r.opts, r.provider, r.conversationID, r.messageID, "checkpoint_resume_failed", "error", 0, err, summary)
	if diagID == "" {
		return err
	}
	message := fmt.Errorf("%s，已自动从断点继续 %d/%d 次但仍失败。可以稍后点击“继续本轮”，或切换到更稳定的模型/中转后再试", sanitizeUserFacingAIError(err.Error()), r.checkpointResumeAttempts, agentMaxCheckpointResumes())
	return &DiagnosticError{ID: diagID, Err: message}
}

func agentMaxCheckpointResumes() int {
	value := strings.TrimSpace(os.Getenv("PERSEUS_AGENT_MAX_CHECKPOINT_RESUMES"))
	if value == "" {
		return defaultAgentMaxCheckpointResumes
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultAgentMaxCheckpointResumes
	}
	if parsed < 0 {
		return 0
	}
	if parsed > 20 {
		return 20
	}
	return parsed
}

func agentRecoveryDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	base := 1 * time.Second
	if value := strings.TrimSpace(os.Getenv("PERSEUS_AGENT_RECOVERY_BASE_DELAY_MS")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			if parsed <= 0 {
				return 0
			}
			base = time.Duration(parsed) * time.Millisecond
		}
	}
	delay := base
	for i := 1; i < attempt; i++ {
		delay *= 2
	}
	if delay > 30*time.Second {
		return 30 * time.Second
	}
	return delay
}

func sleepForAgentRecovery(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return true
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
