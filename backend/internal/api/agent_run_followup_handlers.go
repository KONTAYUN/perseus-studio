package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"webide-backend/internal/ai"
	"webide-backend/internal/idgen"
	"webide-backend/internal/store"
	"webide-backend/internal/tasks"
)

type answerAgentFollowupRequest struct {
	ToolCallID string `json:"tool_call_id"`
	Answer     string `json:"answer"`
	AnswerID   string `json:"answer_id"`
}

func (s *Server) answerAgentFollowupQuestion(ctx context.Context, userID, runID string, body io.Reader) (map[string]any, error) {
	var req answerAgentFollowupRequest
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		return nil, fmt.Errorf("invalid json body")
	}
	req.ToolCallID = strings.TrimSpace(req.ToolCallID)
	req.Answer = strings.TrimSpace(req.Answer)
	req.AnswerID = strings.TrimSpace(req.AnswerID)
	if req.ToolCallID == "" || req.Answer == "" || req.AnswerID == "" {
		return nil, fmt.Errorf("tool_call_id, answer and answer_id are required")
	}
	run, err := s.runs.GetRun(ctx, userID, runID)
	if err != nil {
		return nil, err
	}
	originalMessage, err := s.conversations.GetMessage(ctx, userID, nullableStringValue(run.UserMessageID))
	if err != nil {
		return nil, err
	}
	if existing, existingToolCallID, existingAnswer, ok, lookupErr := s.runs.ExistingFollowupAnswer(ctx, userID, runID, req.AnswerID); lookupErr != nil {
		return nil, lookupErr
	} else if ok {
		if existingToolCallID != req.ToolCallID || existingAnswer != req.Answer {
			return nil, &store.AgentFollowupAnswerConflictError{Message: "answer_id was already used for a different follow-up answer"}
		}
		return map[string]any{"run": existing.Run, "user_message": existing.UserMessage, "already_applied": true}, nil
	}
	answerMessageID := idgen.New("msg")
	preparation, err := s.ai.PrepareFollowupAnswer(ctx, ai.RunOptions{UserID: userID, RunID: runID}, req.ToolCallID, req.Answer, answerMessageID)
	if err != nil {
		return nil, err
	}
	checkpointJSON, err := json.Marshal(preparation.Checkpoint)
	if err != nil {
		return nil, err
	}

	assistantMessageID := nullableStringValue(run.AssistantMessageID)
	assistantAttachmentsJSON := ""
	if assistantMessageID != "" {
		message, getErr := s.conversations.GetMessage(ctx, userID, assistantMessageID)
		if getErr != nil {
			return nil, getErr
		}
		assistantAttachmentsJSON, err = answeredUserRequestAttachments(message.AttachmentsJSON, req.ToolCallID, preparation.ToolName, answerMessageID)
		if err != nil {
			return nil, err
		}
	}

	stored, err := s.runs.AnswerFollowupQuestion(ctx, store.AgentFollowupAnswerInput{
		UserID: userID, RunID: runID, ToolCallID: req.ToolCallID,
		AnswerID: req.AnswerID, Answer: req.Answer, AnswerMessageID: answerMessageID,
		AssistantMessageID: assistantMessageID, AssistantAttachmentsJSON: assistantAttachmentsJSON,
		TurnID: preparation.TurnID, ToolResultPayloadJSON: preparation.ToolResultPayload,
		CheckpointJSON:  string(checkpointJSON),
		MessageMetaJSON: messageMetaJSON("user_response", 0, runID),
	})
	if err != nil {
		return nil, err
	}
	if stored.AlreadyApplied {
		return map[string]any{"run": stored.Run, "user_message": stored.UserMessage, "already_applied": true}, nil
	}

	event := s.appendRunEvent(ctx, store.AgentEventInput{
		RunID: runID, UserID: userID, ConversationID: run.ConversationID,
		MessageID: stored.UserMessage.ID, Type: "user_response_answered", Level: "info",
		Payload: map[string]any{
			"tool_call_id": req.ToolCallID, "answer_id": req.AnswerID,
			"tool_name": preparation.ToolName, "request_kind": userRequestKind(preparation.ToolName),
			"answer_message_id": stored.UserMessage.ID, "user_message": stored.UserMessage,
			"message": "user response received",
		},
	})
	s.runBroker.Publish(runID, tasks.Event{Type: "run", Task: stored.Run})

	user := MustUserFromContext(ctx)
	attachmentPayloads := s.payloadsFromStoredMessageAttachments(ctx, user.UserID, originalMessage)
	go s.executeConversationRun(userID, user.Role, run.ConversationID, originalMessage.ID, run.ModelProviderID, originalMessage.Content, originalMessage.AttachmentsJSON, attachmentPayloads, "", runID)

	var assistantMessage any
	if assistantMessageID != "" {
		assistantMessage, _ = s.conversations.GetMessage(context.Background(), userID, assistantMessageID)
	}
	return map[string]any{
		"run": stored.Run, "user_message": stored.UserMessage,
		"assistant_message": assistantMessage, "resume_event": event,
		"answered_at": time.Now().UTC(),
	}, nil
}

func answeredUserRequestAttachments(raw, toolCallID, toolName, answerMessageID string) (string, error) {
	var attachments []map[string]any
	if err := json.Unmarshal([]byte(followupFirstNonEmpty(raw, "[]")), &attachments); err != nil {
		return "", fmt.Errorf("invalid follow-up message attachments")
	}
	found := false
	for _, attachment := range attachments {
		if attachment["tool_name"] == toolName && attachment["tool_call_id"] == toolCallID {
			attachment["status"] = "answered"
			attachment["answer_message_id"] = answerMessageID
			delete(attachment, "allow_freeform")
			found = true
		}
	}
	if !found {
		return "", fmt.Errorf("user request attachment was not found")
	}
	encoded, err := json.Marshal(attachments)
	return string(encoded), err
}

func userRequestKind(toolName string) string {
	switch strings.TrimSpace(toolName) {
	case "request_user_input":
		return "input"
	case "request_user_choice":
		return "choice"
	default:
		return "legacy_followup"
	}
}

func followupFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
