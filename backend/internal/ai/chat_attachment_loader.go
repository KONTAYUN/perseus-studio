package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

func (s *Service) hydrateStoredImageAttachments(ctx context.Context, userID, conversationID string, incoming []AttachmentPayload) ([]AttachmentPayload, error) {
	if s == nil || s.calls == nil || s.factory == nil || userID == "" || conversationID == "" {
		return incoming, nil
	}
	records, err := s.calls.ListConversationMessageChatAttachments(ctx, userID, conversationID)
	if err != nil {
		return nil, err
	}
	byID := map[string]AttachmentPayload{}
	var nonImages []AttachmentPayload
	for _, item := range incoming {
		if item.Kind == "image" && item.ID != "" {
			byID[item.ID] = item
		} else {
			nonImages = append(nonImages, item)
		}
	}
	images := make([]AttachmentPayload, 0, len(records))
	for _, record := range records {
		if existing, ok := byID[record.ID]; ok && existing.DataBase64 != "" {
			existing.MessageID = record.MessageID
			existing.Width = record.Width
			existing.Height = record.Height
			existing.StorageKey = record.StorageKey
			existing.IncludedInContext = record.Included
			images = append(images, existing)
			delete(byID, record.ID)
			continue
		}
		payload, err := s.loadStoredImageAttachment(userID, record.ID, record.MessageID, record.OriginalName,
			record.MIMEType, record.StorageKey, record.SizeBytes, record.Width, record.Height, record.Included)
		if err != nil {
			return nil, fmt.Errorf("image attachment %s could not be loaded: %w", record.ID, err)
		}
		images = append(images, payload)
		delete(byID, record.ID)
	}
	for _, item := range byID {
		images = append(images, item)
	}
	return append(nonImages, images...), nil
}

func (s *Service) loadStoredImageAttachment(userID, id, messageID, name, mimeType, storageKey string, size int64, width, height int, included bool) (AttachmentPayload, error) {
	uploadsDir, err := s.factory.UploadsDir(userID)
	if err != nil {
		return AttachmentPayload{}, err
	}
	root := filepath.Clean(uploadsDir)
	path := filepath.Clean(filepath.Join(root, filepath.FromSlash(storageKey)))
	prefix := root + string(filepath.Separator)
	if path == root || !strings.HasPrefix(strings.ToLower(path), strings.ToLower(prefix)) {
		return AttachmentPayload{}, fmt.Errorf("invalid attachment storage key")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return AttachmentPayload{}, err
	}
	modelMIME := mimeType
	modelRaw := raw
	if mimeType == "image/gif" {
		if decoded, decodeErr := gif.DecodeAll(bytes.NewReader(raw)); decodeErr == nil && len(decoded.Image) > 1 {
			var converted bytes.Buffer
			if encodeErr := png.Encode(&converted, decoded.Image[0]); encodeErr == nil {
				modelRaw = converted.Bytes()
				modelMIME = "image/png"
			}
		}
	}
	return AttachmentPayload{
		ID: id, MessageID: messageID, Name: name, Size: size, MIMEType: modelMIME,
		Kind: "image", Width: width, Height: height, StorageKey: storageKey,
		DataBase64: base64.StdEncoding.EncodeToString(modelRaw), IncludedInContext: included,
	}, nil
}
