package store

import (
	"encoding/json"
	"strings"
	"time"
)

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func scanConversation(s scanner) (Conversation, error) {
	var item Conversation
	err := s.Scan(&item.ID, &item.UserID, &item.Title, &item.TitleSource, &item.SelectedModelProviderID, &item.CreatedAt, &item.UpdatedAt)
	if item.TitleSource == "" {
		item.TitleSource = "default"
	}
	return item, err
}

func nullTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func scanMessage(s scanner) (Message, error) {
	var item Message
	err := s.Scan(&item.ID, &item.UserID, &item.ConversationID, &item.Role, &item.Content, &item.AttachmentsJSON, &item.Status, &item.SegmentIndex, &item.MetaJSON, &item.CreatedAt, &item.UpdatedAt, &item.AttachmentPayloadsJSON)
	item.Attachments = json.RawMessage(item.AttachmentsJSON)
	if len(item.Attachments) == 0 {
		item.Attachments = json.RawMessage("[]")
	}
	item.Meta = json.RawMessage(item.MetaJSON)
	if len(item.Meta) == 0 {
		item.Meta = json.RawMessage("{}")
	}
	if item.Status == "" {
		item.Status = "final"
	}
	return item, err
}
