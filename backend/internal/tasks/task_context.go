package tasks

import (
	"context"
	"fmt"
	"webide-backend/internal/store"
)

// Freeze the conversation at the source user message: later foreground turns
// must not change a queued task's input. The execution prompt follows history.
func (m *Manager) taskMessages(ctx context.Context, userID string, payload AIAgentPayload, prompt string) ([]store.Message, error) {
	if m.conversations == nil || payload.ConversationID == "" || payload.UserMessageID == "" {
		return []store.Message{{Role: "user", Content: prompt}}, nil
	}
	messages, err := m.conversations.ListMessages(ctx, userID, payload.ConversationID)
	if err != nil {
		return nil, err
	}
	for i, message := range messages {
		if message.ID == payload.UserMessageID && message.Role == "user" {
			messages = messages[:i+1]
			messages = append(messages, store.Message{Role: "user", Content: prompt})
			return messages, nil
		}
	}
	return nil, fmt.Errorf("background task source message unavailable")
}
