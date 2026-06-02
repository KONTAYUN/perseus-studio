package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func SteerMessageMetaJSON(steerID, clientSteerID, runID, status string, receivedSeq int64, receivedAt, appliedAt time.Time) string {
	payload := map[string]any{
		"kind":   "steer",
		"status": strings.TrimSpace(status),
	}
	if payload["status"] == "" {
		payload["status"] = "received"
	}
	if trimmed := strings.TrimSpace(runID); trimmed != "" {
		payload["run_id"] = trimmed
	}
	if trimmed := strings.TrimSpace(steerID); trimmed != "" {
		payload["steer_id"] = trimmed
	}
	if trimmed := strings.TrimSpace(clientSteerID); trimmed != "" {
		payload["client_steer_id"] = trimmed
	}
	if receivedSeq > 0 {
		payload["received_seq"] = receivedSeq
	}
	if !receivedAt.IsZero() {
		payload["received_at"] = receivedAt.UTC().Format(time.RFC3339Nano)
		payload["timeline_at"] = receivedAt.UTC().Format(time.RFC3339Nano)
	}
	if !appliedAt.IsZero() {
		payload["applied_at"] = appliedAt.UTC().Format(time.RFC3339Nano)
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

func (s *ConversationStore) FindSteerMessageID(ctx context.Context, userID, conversationID, steerID, clientSteerID string) (string, error) {
	if s == nil {
		return "", nil
	}
	messages, err := s.ListMessages(ctx, userID, conversationID)
	if err != nil {
		return "", err
	}
	for _, message := range messages {
		if message.Role != "user" || strings.TrimSpace(message.MetaJSON) == "" {
			continue
		}
		var meta map[string]any
		if err := json.Unmarshal([]byte(message.MetaJSON), &meta); err != nil {
			continue
		}
		if fmt.Sprint(meta["kind"]) != "steer" {
			continue
		}
		if strings.TrimSpace(steerID) != "" && fmt.Sprint(meta["steer_id"]) == steerID {
			return message.ID, nil
		}
		if strings.TrimSpace(clientSteerID) != "" && fmt.Sprint(meta["client_steer_id"]) == clientSteerID {
			return message.ID, nil
		}
	}
	return "", nil
}
