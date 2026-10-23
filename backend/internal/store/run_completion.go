package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type CompleteAgentRunInput struct {
	AssistantMessageID string
	TaskID             string
	Outcome            string
}

// Commit the terminal snapshot and its replay event together. Neither should be
// visible on its own, including when the event store rejects the insert.
func (s *AgentRunStore) CompleteRun(ctx context.Context, userID, runID string, input CompleteAgentRunInput) (AgentRun, AgentEvent, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AgentRun{}, AgentEvent{}, err
	}
	defer tx.Rollback()
	run, event, err := completeAgentRun(ctx, tx, userID, runID, input)
	if err != nil {
		return AgentRun{}, AgentEvent{}, err
	}
	if err := tx.Commit(); err != nil {
		return AgentRun{}, AgentEvent{}, err
	}
	return run, event, nil
}

func completeAgentRun(ctx context.Context, tx *sql.Tx, userID, runID string, input CompleteAgentRunInput) (AgentRun, AgentEvent, error) {
	run, err := scanAgentRun(tx.QueryRowContext(ctx, `
SELECT id,user_id,conversation_id,user_message_id,assistant_message_id,model_provider_id,
status,stage,error_message,diagnostic_log_id,created_at,started_at,completed_at,updated_at
FROM agent_runs WHERE id=? AND user_id=?`, runID, userID))
	if err != nil {
		return AgentRun{}, AgentEvent{}, err
	}
	if run.Status == AgentRunSucceeded && run.AssistantMessageID.String == input.AssistantMessageID {
		event, eventErr := scanAgentEvent(tx.QueryRowContext(ctx, `
SELECT id,run_id,user_id,conversation_id,message_id,task_id,type,level,seq,payload_json,created_at
FROM agent_events WHERE run_id=? AND user_id=? AND type='run_completed' AND task_id=? ORDER BY seq DESC LIMIT 1`, runID, userID, input.TaskID))
		return run, event, eventErr
	}
	if run.Status != AgentRunQueued && run.Status != AgentRunRunning {
		return AgentRun{}, AgentEvent{}, fmt.Errorf("cannot complete run in %s state", run.Status)
	}
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE agent_runs SET status='succeeded',stage='completed',
assistant_message_id=?,completed_at=?,updated_at=? WHERE id=? AND user_id=? AND status IN ('queued','running')`, input.AssistantMessageID, now, now, runID, userID)
	if err != nil {
		return AgentRun{}, AgentEvent{}, err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return AgentRun{}, AgentEvent{}, errors.Join(sql.ErrNoRows, err)
	}
	payload := map[string]any{"message": "run completed", "outcome": input.Outcome}
	if input.TaskID != "" {
		payload["completion_kind"] = "background_task_handoff"
		payload["task_id"] = input.TaskID
	}
	event, err := appendAgentEvent(ctx, tx, AgentEventInput{RunID: runID, UserID: userID,
		ConversationID: run.ConversationID, MessageID: input.AssistantMessageID, TaskID: input.TaskID,
		Type: "run_completed", Level: "info", Payload: payload})
	if err != nil {
		return AgentRun{}, AgentEvent{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM agent_run_checkpoints WHERE run_id=? AND user_id=?`, runID, userID); err != nil {
		return AgentRun{}, AgentEvent{}, err
	}
	run.Status, run.Stage = AgentRunSucceeded, sql.NullString{String: "completed", Valid: true}
	run.AssistantMessageID = sql.NullString{String: input.AssistantMessageID, Valid: input.AssistantMessageID != ""}
	run.CompletedAt, run.UpdatedAt = sql.NullTime{Time: now, Valid: true}, now
	return run, event, nil
}
