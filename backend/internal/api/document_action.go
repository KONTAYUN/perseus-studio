package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"webide-backend/internal/ai"
	"webide-backend/internal/document"
	"webide-backend/internal/store"
)

type documentActionKey struct{}

func setRequestDocumentAction(r *http.Request, a *document.Action) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if a != nil {
		*r = *r.WithContext(context.WithValue(r.Context(), documentActionKey{}, a))
	}
	return nil
}
func parseRequestDocumentAction(r *http.Request, raw string) error {
	if raw == "" {
		return nil
	}
	if len(raw) > 32000 {
		return fmt.Errorf("document action is too large")
	}
	var action document.Action
	if err := json.Unmarshal([]byte(raw), &action); err != nil {
		return err
	}
	return setRequestDocumentAction(r, &action)
}
func (s *Server) checkDocumentAction(ctx context.Context, userID string, a *document.Action) error {
	if a == nil {
		return nil
	}
	if err := a.Validate(); err != nil {
		return err
	}
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return err
	}
	file, err := files.ReadFile(a.Path)
	if err != nil {
		return err
	}
	if file.Path != a.Path || file.Hash != a.BaseHash {
		return fmt.Errorf("文稿版本已变化，请重新检查后再修复")
	}
	return ctx.Err()
}
func (s *Server) documentActionForRun(ctx context.Context, userID, messageID string) (*document.Action, error) {
	input, err := s.conversations.GetMessage(ctx, userID, messageID)
	if err != nil {
		return nil, err
	}
	action, err := document.ActionFromMeta(input.MetaJSON)
	if err != nil {
		return nil, err
	}
	return action, s.checkDocumentAction(ctx, userID, action)
}
func documentActionPaths(action *document.Action) []string {
	if action == nil {
		return nil
	}
	return []string{action.Path}
}
func documentRunContext(action *document.Action, items []store.Message, attachments []ai.AttachmentPayload, id string) ([]store.Message, []ai.AttachmentPayload) {
	if action == nil {
		return items, attachments
	}
	for _, item := range items {
		if item.ID == id {
			return []store.Message{item}, nil
		}
	}
	return nil, nil
}
