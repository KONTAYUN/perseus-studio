package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"webide-backend/internal/idgen"
)

type FollowupAnswerPreparation struct {
	Checkpoint        AgentRuntimeCheckpoint
	TurnID            string
	ToolName          string
	ToolResult        string
	ToolResultPayload string
}

func (s *Service) PrepareFollowupAnswer(ctx context.Context, opts RunOptions, toolCallID, answer, answerMessageID string) (FollowupAnswerPreparation, error) {
	toolCallID = strings.TrimSpace(toolCallID)
	answer = strings.TrimSpace(answer)
	if toolCallID == "" || answer == "" || strings.TrimSpace(answerMessageID) == "" {
		return FollowupAnswerPreparation{}, fmt.Errorf("tool_call_id, answer and answer_message_id are required")
	}
	checkpoint, ok := s.loadRunCheckpoint(ctx, opts)
	if !ok || checkpoint.Transcript == nil {
		return FollowupAnswerPreparation{}, fmt.Errorf("follow-up checkpoint is unavailable")
	}
	toolName, pending := checkpointPendingUserRequest(checkpoint, toolCallID)
	if !pending {
		return FollowupAnswerPreparation{}, fmt.Errorf("user request is not pending")
	}

	result := "<user_message>\n" + answer + "\n</user_message>"
	event := ToolEvent{
		ToolCallID: toolCallID, ToolName: toolName,
		StatusKind: manifestStatusKind(toolName), Label: "已收到用户回答",
		Status: "answered", AnswerMessageID: answerMessageID,
	}
	checkpoint.Transcript.AppendToolResults([]TranscriptToolResult{{
		ToolCallID: toolCallID, ToolName: toolName, Content: result, Event: event,
	}})
	for index := range checkpoint.Events {
		if checkpoint.Events[index].ToolCallID == toolCallID && checkpoint.Events[index].ToolName == toolName {
			checkpoint.Events[index].Status = "answered"
			checkpoint.Events[index].AnswerMessageID = answerMessageID
		}
	}
	checkpoint.CompletedContent = ""
	checkpoint.LastToolFailed = false
	checkpoint.LastToolFailure = runtimeToolFailure{}
	checkpoint.Kernel = normalizeAgentKernelState(checkpoint.Kernel)
	turnID := firstNonEmpty(checkpoint.Kernel.TurnID, opts.RunID)
	checkpoint.Kernel.Items = append(checkpoint.Kernel.Items, AgentTurnItem{
		ID: idgen.New("item"), TurnID: turnID, Kind: "tool_result", Phase: "user_response",
		Status: "answered", Round: checkpoint.CurrentRound, ToolCallID: toolCallID,
		ToolName: toolName, Content: result,
	})
	if len(checkpoint.Kernel.Items) > maxKernelTurnItems {
		checkpoint.Kernel.Items = checkpoint.Kernel.Items[len(checkpoint.Kernel.Items)-maxKernelTurnItems:]
	}
	payload, _ := json.Marshal(map[string]any{
		"tool_call_id": toolCallID, "tool_name": toolName,
		"content": result, "status": "answered", "answer_message_id": answerMessageID,
	})
	return FollowupAnswerPreparation{
		Checkpoint: checkpoint, TurnID: turnID, ToolName: toolName, ToolResult: result, ToolResultPayload: string(payload),
	}, nil
}

func checkpointPendingUserRequest(checkpoint AgentRuntimeCheckpoint, toolCallID string) (string, bool) {
	toolName := ""
	resultFound := false
	for _, entry := range checkpoint.Transcript.Entries {
		for _, call := range entry.ToolCalls {
			if call.ID == toolCallID && isUserRequestTool(call.Name) {
				toolName = call.Name
			}
		}
		for _, result := range entry.ToolResults {
			if result.ToolCallID == toolCallID {
				resultFound = true
			}
		}
	}
	return toolName, toolName != "" && !resultFound
}
