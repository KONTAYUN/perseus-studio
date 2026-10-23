package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Failure and its terminal event have one owner and one transaction. An explicit
// resume moves the run back to queued, allowing one terminal for that attempt.
func (s *AgentRunStore) FailRun(ctx context.Context, userID, runID string, status AgentRunStatus, stage string, input UpdateAgentRunInput, payload any) (AgentRun, AgentEvent, error) {
	if status != AgentRunFailed && status != AgentRunCanceled {
		return AgentRun{}, AgentEvent{}, fmt.Errorf("invalid failure status")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AgentRun{}, AgentEvent{}, err
	}
	defer tx.Rollback()
	run, err := scanAgentRun(tx.QueryRowContext(ctx, `SELECT id,user_id,conversation_id,user_message_id,assistant_message_id,model_provider_id,status,stage,error_message,diagnostic_log_id,created_at,started_at,completed_at,updated_at FROM agent_runs WHERE id=? AND user_id=?`, runID, userID))
	if err != nil {
		return run, AgentEvent{}, err
	}
	if run.Status != AgentRunRunning && run.Status != AgentRunQueued {
		return run, AgentEvent{}, nil
	}
	now := time.Now().UTC()
	message := ""
	if input.ErrorMessage != nil {
		message = *input.ErrorMessage
	}
	_, err = tx.ExecContext(ctx, `UPDATE agent_runs SET status=?,stage=?,error_message=NULLIF(?,''),diagnostic_log_id=NULLIF(?,''),assistant_message_id=CASE WHEN ?='' THEN assistant_message_id ELSE ? END,completed_at=?,updated_at=? WHERE id=? AND user_id=?`, status, stage, message, input.DiagnosticLogID, input.AssistantMessageID, input.AssistantMessageID, now, now, runID, userID)
	if err != nil {
		return run, AgentEvent{}, err
	}
	kind := "run_failed"
	level := "error"
	if status == AgentRunCanceled {
		kind = "run_canceled"
		level = "warn"
	}
	event, err := appendAgentEvent(ctx, tx, AgentEventInput{RunID: runID, UserID: userID, ConversationID: run.ConversationID, MessageID: run.UserMessageID.String, Type: kind, Level: level, Payload: payload})
	if err != nil {
		return run, AgentEvent{}, err
	}
	if err = tx.Commit(); err != nil {
		return run, AgentEvent{}, err
	}
	run.Status = status
	run.Stage = sql.NullString{String: stage, Valid: true}
	run.ErrorMessage = sql.NullString{String: message, Valid: message != ""}
	run.DiagnosticLogID = sql.NullString{String: input.DiagnosticLogID, Valid: input.DiagnosticLogID != ""}
	run.CompletedAt = sql.NullTime{Time: now, Valid: true}
	run.UpdatedAt = now
	if input.AssistantMessageID != "" {
		run.AssistantMessageID = sql.NullString{String: input.AssistantMessageID, Valid: true}
	}
	return run, event, nil
}
