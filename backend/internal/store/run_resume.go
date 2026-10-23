package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Commit the resume marker together with the state change. An event write
// failure must not strand a queued run that the API never scheduled.
func (s *AgentRunStore) ResumeRunWithEvent(ctx context.Context, userID, id string, requestIDs ...string) (AgentRun, AgentEvent, error) {
	requestID := ""
	if len(requestIDs) > 0 {
		requestID = requestIDs[0]
	}
	return s.resumeRun(ctx, userID, id, true, requestID)
}

func (s *AgentRunStore) resumeRun(ctx context.Context, userID, id string, emit bool, requestID string) (AgentRun, AgentEvent, error) {
	if len(requestID) > 128 {
		return AgentRun{}, AgentEvent{}, fmt.Errorf("request id is too long")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AgentRun{}, AgentEvent{}, err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, `
UPDATE agent_runs
SET status = ?, stage = ?, error_message = NULL, diagnostic_log_id = NULL,
    started_at = NULL, completed_at = NULL, updated_at = ?
WHERE id = ? AND user_id = ? AND status IN (?, ?)
  AND EXISTS (SELECT 1 FROM conversations c WHERE c.id = agent_runs.conversation_id AND c.user_id = agent_runs.user_id)
  AND NOT EXISTS (
    SELECT 1 FROM agent_runs AS other
    WHERE other.user_id = agent_runs.user_id AND other.conversation_id = agent_runs.conversation_id
      AND other.id <> agent_runs.id
      AND (other.status IN ('queued', 'running') OR other.created_at > agent_runs.created_at
        OR (other.created_at = agent_runs.created_at AND other.rowid > agent_runs.rowid))
  )`, AgentRunQueued, "queued", now, id, userID, AgentRunFailed, AgentRunCanceled)
	if err != nil {
		return AgentRun{}, AgentEvent{}, err
	}
	if rows, err := result.RowsAffected(); err != nil {
		return AgentRun{}, AgentEvent{}, err
	} else if rows != 1 {
		return AgentRun{}, AgentEvent{}, sql.ErrNoRows
	}
	var event AgentEvent
	if emit {
		payload, _ := json.Marshal(map[string]string{"message": "run resumed", "request_id": requestID})
		event, err = scanAgentEvent(tx.QueryRowContext(ctx, `
INSERT INTO agent_events (id,run_id,user_id,conversation_id,message_id,task_id,type,level,seq,payload_json,created_at)
SELECT ?,id,user_id,conversation_id,COALESCE(user_message_id,''),'','run_resumed','info',
  COALESCE((SELECT MAX(seq)+1 FROM agent_events WHERE run_id = ?),1),?,?
FROM agent_runs WHERE id = ? AND user_id = ?
RETURNING id,run_id,user_id,conversation_id,message_id,task_id,type,level,seq,payload_json,created_at`,
			nextStoreID("event", now), id, string(payload), now, id, userID))
		if err != nil {
			return AgentRun{}, AgentEvent{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return AgentRun{}, AgentEvent{}, err
	}
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	run, err := s.GetRun(readCtx, userID, id)
	return run, event, err
}
