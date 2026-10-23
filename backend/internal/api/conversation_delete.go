package api

import (
	"context"
	"database/sql"
	"errors"

	"webide-backend/internal/store"
)

func (s *Server) deleteConversation(ctx context.Context, userID, conversationID string) error {
	// A canceled durable status can precede worker cleanup. Keep its history
	// until the registered execution owner has released it as well.
	s.runCancelMu.Lock()
	defer s.runCancelMu.Unlock()
	for runID := range s.runCancels {
		run, err := s.runs.GetRun(ctx, userID, runID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if run.ConversationID == conversationID {
			return store.ErrConversationBusy
		}
	}
	return s.conversations.DeleteConversation(ctx, userID, conversationID)
}
