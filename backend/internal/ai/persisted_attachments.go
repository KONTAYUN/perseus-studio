package ai

import (
	"encoding/json"
	"webide-backend/internal/store"
)

// Editor/workspace references describe a moment in the UI. Only uploaded
// content is durable input; references are restored from existing metadata.
func PersistedAttachmentPayloads(items []AttachmentPayload) string {
	var uploads []AttachmentPayload
	for _, item := range items {
		switch item.Kind {
		case "text", "image", "binary", "zip":
			uploads = append(uploads, item)
		}
	}
	if len(uploads) == 0 {
		return ""
	}
	raw, _ := json.Marshal(uploads)
	return string(raw)
}

func StoredMessageAttachments(message store.Message) []AttachmentPayload {
	var items []AttachmentPayload
	_ = json.Unmarshal([]byte(message.AttachmentPayloadsJSON), &items)
	return items
}

func attachmentsForMessage(message store.Message, current []AttachmentPayload, isLastUser bool) []AttachmentPayload {
	if message.Role != "user" {
		return nil
	}
	if isLastUser && len(current) > 0 {
		return current
	}
	return StoredMessageAttachments(message)
}
