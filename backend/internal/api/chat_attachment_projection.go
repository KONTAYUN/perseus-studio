package api

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"webide-backend/internal/store"
)

func (s *Server) projectConversationChatAttachments(ctx context.Context, userID, conversationID string, messages []store.Message) []store.Message {
	records, err := s.conversations.ListConversationMessageChatAttachments(ctx, userID, conversationID)
	if err != nil {
		return messages
	}
	byMessage := map[string][]store.ChatAttachment{}
	for _, item := range records {
		byMessage[item.MessageID] = append(byMessage[item.MessageID], item)
	}
	projected := append([]store.Message(nil), messages...)
	for index := range projected {
		var metadata []json.RawMessage
		if err := json.Unmarshal([]byte(projected[index].AttachmentsJSON), &metadata); err != nil {
			continue
		}
		currentRecords := byMessage[projected[index].ID]
		linked := map[string]bool{}
		for _, item := range currentRecords {
			linked[item.ID] = true
		}
		clean := make([]json.RawMessage, 0, len(metadata)+len(currentRecords))
		for _, rawItem := range metadata {
			var envelope struct {
				ID       string `json:"id"`
				Kind     string `json:"kind"`
				Type     string `json:"type"`
				MIMEType string `json:"mime_type"`
			}
			if err := json.Unmarshal(rawItem, &envelope); err != nil {
				clean = append(clean, rawItem)
				continue
			}
			itemMIME := envelope.MIMEType
			if itemMIME == "" {
				itemMIME = envelope.Type
			}
			isImage := envelope.Kind == "image" || strings.HasPrefix(strings.ToLower(itemMIME), "image/")
			if !isImage {
				clean = append(clean, rawItem)
				continue
			}
			if linked[envelope.ID] {
				continue
			}
			var item chatAttachmentMetadata
			if err := json.Unmarshal(rawItem, &item); err != nil {
				clean = append(clean, rawItem)
				continue
			}
			item.Available = false
			item.Status = "legacy_unavailable"
			item.Note = "旧图片未保存，无法预览"
			item.ContentURL = ""
			item.DownloadURL = ""
			if encoded, err := json.Marshal(item); err == nil {
				clean = append(clean, encoded)
			} else {
				clean = append(clean, rawItem)
			}
		}
		for _, record := range currentRecords {
			item := chatAttachmentMetadataFromRecord(record)
			if path, pathErr := s.chatAttachmentDiskPath(record); pathErr != nil {
				item.Available = false
				item.Status = "missing"
			} else if _, statErr := os.Stat(path); statErr != nil {
				item.Available = false
				item.Status = "missing"
			}
			if encoded, err := json.Marshal(item); err == nil {
				clean = append(clean, encoded)
			}
		}
		raw, marshalErr := json.Marshal(clean)
		if marshalErr != nil {
			continue
		}
		projected[index].AttachmentsJSON = string(raw)
		projected[index].Attachments = raw
	}
	return projected
}
