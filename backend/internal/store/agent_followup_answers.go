package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type AgentFollowupAnswerInput struct {
	UserID, RunID, ToolCallID, AnswerID, Answer, AnswerMessageID string
	AssistantMessageID, AssistantAttachmentsJSON                 string
	TurnID, ToolResultPayloadJSON                                string
	CheckpointJSON, MessageMetaJSON                              string
}

type AgentFollowupAnswerResult struct {
	Run            AgentRun `json:"run"`
	UserMessage    Message  `json:"user_message"`
	AlreadyApplied bool     `json:"already_applied"`
}

type AgentFollowupAnswerConflictError struct{ Message string }

func (e *AgentFollowupAnswerConflictError) Error() string { return e.Message }

func (s *AgentRunStore) ExistingFollowupAnswer(ctx context.Context, userID, runID, answerID string) (AgentFollowupAnswerResult, string, string, bool, error) {
	var toolCallID, answer, messageID string
	err := s.db.QueryRowContext(ctx, `SELECT tool_call_id, answer, answer_message_id FROM agent_followup_answers WHERE user_id = ? AND run_id = ? AND answer_id = ?`, userID, runID, answerID).
		Scan(&toolCallID, &answer, &messageID)
	if err == sql.ErrNoRows {
		return AgentFollowupAnswerResult{}, "", "", false, nil
	}
	if err != nil {
		return AgentFollowupAnswerResult{}, "", "", false, err
	}
	result, err := s.followupAnswerResult(ctx, userID, runID, messageID, true)
	return result, toolCallID, answer, true, err
}

func (s *AgentRunStore) AnswerFollowupQuestion(ctx context.Context, input AgentFollowupAnswerInput) (AgentFollowupAnswerResult, error) {
	input.UserID = strings.TrimSpace(input.UserID)
	input.RunID = strings.TrimSpace(input.RunID)
	input.ToolCallID = strings.TrimSpace(input.ToolCallID)
	input.AnswerID = strings.TrimSpace(input.AnswerID)
	input.Answer = strings.TrimSpace(input.Answer)
	if input.UserID == "" || input.RunID == "" || input.ToolCallID == "" || input.AnswerID == "" || input.Answer == "" || input.AnswerMessageID == "" {
		return AgentFollowupAnswerResult{}, fmt.Errorf("follow-up answer fields are required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AgentFollowupAnswerResult{}, err
	}
	defer tx.Rollback()

	var existingToolCallID, existingAnswer, existingMessageID string
	err = tx.QueryRowContext(ctx, `SELECT tool_call_id, answer, answer_message_id FROM agent_followup_answers WHERE user_id = ? AND run_id = ? AND answer_id = ?`, input.UserID, input.RunID, input.AnswerID).
		Scan(&existingToolCallID, &existingAnswer, &existingMessageID)
	if err == nil {
		if existingToolCallID != input.ToolCallID || existingAnswer != input.Answer {
			return AgentFollowupAnswerResult{}, &AgentFollowupAnswerConflictError{Message: "answer_id was already used for a different follow-up answer"}
		}
		if err := tx.Commit(); err != nil {
			return AgentFollowupAnswerResult{}, err
		}
		return s.followupAnswerResult(ctx, input.UserID, input.RunID, existingMessageID, true)
	}
	if err != sql.ErrNoRows {
		return AgentFollowupAnswerResult{}, err
	}
	var previousAnswerID string
	if err := tx.QueryRowContext(ctx, `SELECT answer_id FROM agent_followup_answers WHERE user_id = ? AND run_id = ? AND tool_call_id = ?`, input.UserID, input.RunID, input.ToolCallID).Scan(&previousAnswerID); err == nil {
		return AgentFollowupAnswerResult{}, &AgentFollowupAnswerConflictError{Message: "follow-up question was already answered"}
	} else if err != sql.ErrNoRows {
		return AgentFollowupAnswerResult{}, err
	}

	var runStatus AgentRunStatus
	var conversationID string
	if err := tx.QueryRowContext(ctx, `SELECT status, conversation_id FROM agent_runs WHERE id = ? AND user_id = ?`, input.RunID, input.UserID).Scan(&runStatus, &conversationID); err != nil {
		return AgentFollowupAnswerResult{}, err
	}
	if runStatus != AgentRunWaiting {
		return AgentFollowupAnswerResult{}, fmt.Errorf("run is not waiting for a follow-up answer")
	}
	var toolName, toolStatus string
	if err := tx.QueryRowContext(ctx, `SELECT tool_name, status FROM tool_calls WHERE id = ? AND user_id = ? AND run_id = ?`, input.ToolCallID, input.UserID, input.RunID).Scan(&toolName, &toolStatus); err != nil {
		return AgentFollowupAnswerResult{}, err
	}
	if !isUserRequestToolName(toolName) || toolStatus != "running" {
		return AgentFollowupAnswerResult{}, fmt.Errorf("user request tool call is not pending")
	}

	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `INSERT INTO messages (id, user_id, conversation_id, role, content, attachments_json, status, segment_index, meta_json, created_at, updated_at) VALUES (?, ?, ?, 'user', ?, '[]', 'final', 0, ?, ?, ?)`, input.AnswerMessageID, input.UserID, conversationID, input.Answer, input.MessageMetaJSON, now, now); err != nil {
		return AgentFollowupAnswerResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE conversations SET updated_at = ? WHERE id = ? AND user_id = ?`, now, conversationID, input.UserID); err != nil {
		return AgentFollowupAnswerResult{}, err
	}
	resultJSON, _ := json.Marshal(map[string]any{"status": "answered", "answer_id": input.AnswerID, "answer_message_id": input.AnswerMessageID})
	toolUpdate, err := tx.ExecContext(ctx, `UPDATE tool_calls SET result_summary = 'user answered the pending request', status = 'success', error = '', completed_at = ?, result_json = ? WHERE id = ? AND user_id = ? AND run_id = ? AND status = 'running'`, now, string(resultJSON), input.ToolCallID, input.UserID, input.RunID)
	if err != nil {
		return AgentFollowupAnswerResult{}, err
	}
	if affected, err := toolUpdate.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return AgentFollowupAnswerResult{}, err
		}
		return AgentFollowupAnswerResult{}, fmt.Errorf("follow-up tool call changed before the answer was stored")
	}
	if _, err := appendAgentTurnItemTx(ctx, tx, AgentTurnItemInput{
		UserID: input.UserID, ConversationID: conversationID, RunID: input.RunID, TurnID: input.TurnID,
		Kind: "user_message", Role: "user", Status: "completed", Visibility: AgentItemVisibilityUI,
		SourceID: input.AnswerMessageID, PayloadJSON: mustStoreJSON(map[string]any{"content": input.Answer}), CreatedAt: now,
	}); err != nil {
		return AgentFollowupAnswerResult{}, err
	}
	if _, err := appendAgentTurnItemTx(ctx, tx, AgentTurnItemInput{
		UserID: input.UserID, ConversationID: conversationID, RunID: input.RunID, TurnID: input.TurnID,
		Kind: "tool_result", Role: "tool", Status: "answered", Visibility: AgentItemVisibilityModel,
		SourceID: input.RunID + ":tool_result:" + input.ToolCallID, PayloadJSON: input.ToolResultPayloadJSON, CreatedAt: now,
	}); err != nil {
		return AgentFollowupAnswerResult{}, err
	}
	if input.AssistantMessageID != "" && input.AssistantAttachmentsJSON != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE messages SET attachments_json = ?, updated_at = ? WHERE id = ? AND user_id = ?`, input.AssistantAttachmentsJSON, now, input.AssistantMessageID, input.UserID); err != nil {
			return AgentFollowupAnswerResult{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO agent_followup_answers (id, answer_id, user_id, run_id, tool_call_id, answer, answer_message_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, nextStoreID("fa", now), input.AnswerID, input.UserID, input.RunID, input.ToolCallID, input.Answer, input.AnswerMessageID, now); err != nil {
		return AgentFollowupAnswerResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO agent_run_checkpoints (id, run_id, user_id, seq, state_json, created_at) VALUES (?, ?, ?, COALESCE((SELECT MAX(seq) + 1 FROM agent_run_checkpoints WHERE run_id = ?), 1), ?, ?)`, nextStoreID("cp", now), input.RunID, input.UserID, input.RunID, input.CheckpointJSON, now); err != nil {
		return AgentFollowupAnswerResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_runs SET status = ?, stage = 'queued', error_message = NULL, diagnostic_log_id = NULL, failure_json = NULL, started_at = NULL, completed_at = NULL, updated_at = ? WHERE id = ? AND user_id = ?`, AgentRunQueued, now, input.RunID, input.UserID); err != nil {
		return AgentFollowupAnswerResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return AgentFollowupAnswerResult{}, err
	}
	return s.followupAnswerResult(ctx, input.UserID, input.RunID, input.AnswerMessageID, false)
}

func (s *AgentRunStore) followupAnswerResult(ctx context.Context, userID, runID, messageID string, already bool) (AgentFollowupAnswerResult, error) {
	run, err := s.GetRun(ctx, userID, runID)
	if err != nil {
		return AgentFollowupAnswerResult{}, err
	}
	row := s.db.QueryRowContext(ctx, `SELECT id, user_id, conversation_id, role, content, attachments_json, status, segment_index, meta_json, created_at, updated_at FROM messages WHERE id = ? AND user_id = ?`, messageID, userID)
	message, err := scanMessage(row)
	return AgentFollowupAnswerResult{Run: run, UserMessage: message, AlreadyApplied: already}, err
}

func isUserRequestToolName(name string) bool {
	switch strings.TrimSpace(name) {
	case "request_user_input", "request_user_choice", "ask_followup_question":
		return true
	default:
		return false
	}
}
