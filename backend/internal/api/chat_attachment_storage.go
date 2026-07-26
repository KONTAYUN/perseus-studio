package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"image"
	"image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"webide-backend/internal/ai"
	"webide-backend/internal/idgen"
	"webide-backend/internal/store"
)

const (
	maxChatImageBytes  = 10 << 20
	maxChatImagePixels = 40_000_000
	stagedImageTTL     = 24 * time.Hour
)

var supportedChatImageMIMEs = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

func (s *Server) persistStagedChatImage(ctx context.Context, userID string, header *multipart.FileHeader) (store.ChatAttachment, error) {
	if header == nil {
		return store.ChatAttachment{}, fmt.Errorf("image file is required")
	}
	if header.Size > maxChatImageBytes {
		return store.ChatAttachment{}, fmt.Errorf("image exceeds the 10 MB limit")
	}
	file, err := header.Open()
	if err != nil {
		return store.ChatAttachment{}, fmt.Errorf("open image: %w", err)
	}
	raw, readErr := io.ReadAll(io.LimitReader(file, maxChatImageBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return store.ChatAttachment{}, fmt.Errorf("read image: %w", readErr)
	}
	if closeErr != nil {
		return store.ChatAttachment{}, fmt.Errorf("close image: %w", closeErr)
	}
	if len(raw) > maxChatImageBytes {
		return store.ChatAttachment{}, fmt.Errorf("image exceeds the 10 MB limit")
	}
	return s.persistStagedChatImageBytes(ctx, userID, header.Filename, raw)
}

func (s *Server) persistStagedChatImageBytes(ctx context.Context, userID, originalName string, raw []byte) (store.ChatAttachment, error) {
	if len(raw) > maxChatImageBytes {
		return store.ChatAttachment{}, fmt.Errorf("image exceeds the 10 MB limit")
	}
	mimeType, width, height, err := detectChatImage(raw)
	if err != nil {
		return store.ChatAttachment{}, err
	}
	if err := validateChatImageFilename(originalName, mimeType); err != nil {
		return store.ChatAttachment{}, err
	}
	uploadsDir, err := s.factory.UploadsDir(userID)
	if err != nil {
		return store.ChatAttachment{}, err
	}
	chatDir := filepath.Join(uploadsDir, "chat")
	if err := os.MkdirAll(chatDir, 0o755); err != nil {
		return store.ChatAttachment{}, err
	}
	temp, err := os.CreateTemp(chatDir, ".upload-*")
	if err != nil {
		return store.ChatAttachment{}, err
	}
	tempPath := temp.Name()
	keepTemp := true
	defer func() {
		_ = temp.Close()
		if keepTemp {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := temp.Write(raw); err != nil {
		return store.ChatAttachment{}, err
	}
	if err := temp.Sync(); err != nil {
		return store.ChatAttachment{}, err
	}
	if err := temp.Close(); err != nil {
		return store.ChatAttachment{}, err
	}
	id := idgen.New("chatatt")
	extension := supportedChatImageMIMEs[mimeType]
	storageKey := filepath.ToSlash(filepath.Join("chat", id+extension))
	finalPath := filepath.Join(chatDir, id+extension)
	if err := os.Rename(tempPath, finalPath); err != nil {
		return store.ChatAttachment{}, err
	}
	keepTemp = false
	item, err := s.conversations.CreateChatAttachment(ctx, store.CreateChatAttachmentInput{
		ID: id, UserID: userID, OriginalName: safeChatAttachmentName(originalName),
		MIMEType: mimeType, SizeBytes: int64(len(raw)), Width: width, Height: height,
		SHA256: chatImageSHA256(raw), StorageKey: storageKey,
	})
	if err != nil {
		_ = os.Remove(finalPath)
		return store.ChatAttachment{}, err
	}
	return item, nil
}

func (s *Server) chatAttachmentPayload(item store.ChatAttachment, messageID string) (ai.AttachmentPayload, error) {
	path, err := s.chatAttachmentDiskPath(item)
	if err != nil {
		return ai.AttachmentPayload{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return ai.AttachmentPayload{}, err
	}
	mimeType := item.MIMEType
	modelRaw := raw
	if mimeType == "image/gif" {
		if decoded, decodeErr := gif.DecodeAll(bytes.NewReader(raw)); decodeErr == nil && len(decoded.Image) > 1 {
			var converted bytes.Buffer
			if encodeErr := png.Encode(&converted, decoded.Image[0]); encodeErr == nil {
				modelRaw = converted.Bytes()
				mimeType = "image/png"
			}
		}
	}
	return ai.AttachmentPayload{
		ID: item.ID, MessageID: messageID, Name: item.OriginalName, Size: item.SizeBytes,
		MIMEType: mimeType, Kind: "image", Width: item.Width, Height: item.Height,
		StorageKey: item.StorageKey, DataBase64: base64.StdEncoding.EncodeToString(modelRaw),
		IncludedInContext: item.Included || item.Status == store.ChatAttachmentStatusStaged,
	}, nil
}

func (s *Server) chatAttachmentDiskPath(item store.ChatAttachment) (string, error) {
	uploadsDir, err := s.factory.UploadsDir(item.UserID)
	if err != nil {
		return "", err
	}
	root := filepath.Clean(uploadsDir)
	path := filepath.Clean(filepath.Join(root, filepath.FromSlash(item.StorageKey)))
	prefix := root + string(filepath.Separator)
	if path == root || !strings.HasPrefix(strings.ToLower(path), strings.ToLower(prefix)) {
		return "", fmt.Errorf("invalid attachment storage key")
	}
	return path, nil
}

func (s *Server) removeChatAttachmentFiles(items []store.ChatAttachment) {
	for _, item := range items {
		if path, err := s.chatAttachmentDiskPath(item); err == nil {
			_ = os.Remove(path)
		}
	}
}

func (s *Server) deleteConversationMessage(ctx context.Context, userID, conversationID, messageID string) error {
	items, err := s.conversations.DeleteMessageWithAttachments(ctx, userID, conversationID, messageID)
	if err != nil {
		return err
	}
	s.removeChatAttachmentFiles(items)
	return nil
}

func (s *Server) deleteConversationMessagesAfter(ctx context.Context, userID, conversationID, messageID string) error {
	items, err := s.conversations.DeleteMessagesAfterWithAttachments(ctx, userID, conversationID, messageID)
	if err != nil {
		return err
	}
	s.removeChatAttachmentFiles(items)
	return nil
}

func chatAttachmentMetadataFromRecord(item store.ChatAttachment) chatAttachmentMetadata {
	return chatAttachmentMetadata{
		ID: item.ID, Name: item.OriginalName, Size: item.SizeBytes,
		Type: item.MIMEType, Kind: "image", MIMEType: item.MIMEType,
		Width: item.Width, Height: item.Height, Status: item.Status, Available: true,
		ContentURL:        "/api/chat-attachments/" + item.ID + "/content",
		DownloadURL:       "/api/chat-attachments/" + item.ID + "/download",
		IncludedInContext: item.Included || item.Status == store.ChatAttachmentStatusStaged,
	}
}

func (s *Server) StartChatAttachmentCleanup(ctx context.Context) {
	cleanup := func() {
		if err := s.cleanupExpiredStagedChatAttachments(context.Background()); err != nil {
			log.Printf("cleanup staged chat attachments: %v", err)
		}
	}
	cleanup()
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cleanup()
			}
		}
	}()
}

func (s *Server) cleanupExpiredStagedChatAttachments(ctx context.Context) error {
	before := time.Now().UTC().Add(-stagedImageTTL)
	items, err := s.conversations.ListExpiredStagedChatAttachments(ctx, before)
	if err != nil {
		return err
	}
	for _, item := range items {
		if err := s.conversations.DeleteExpiredStagedChatAttachment(ctx, item.ID, before); err != nil {
			return err
		}
		if path, pathErr := s.chatAttachmentDiskPath(item); pathErr == nil {
			_ = os.Remove(path)
		}
	}
	return nil
}

func detectChatImage(raw []byte) (string, int, int, error) {
	if len(raw) == 0 {
		return "", 0, 0, fmt.Errorf("image is empty")
	}
	mimeType := http.DetectContentType(raw)
	if isWebP(raw) {
		mimeType = "image/webp"
	}
	if _, ok := supportedChatImageMIMEs[mimeType]; !ok {
		return "", 0, 0, fmt.Errorf("only PNG, JPEG, WebP, and GIF images are supported")
	}
	width, height, err := decodeChatImageDimensions(mimeType, raw)
	if err != nil || width <= 0 || height <= 0 {
		return "", 0, 0, fmt.Errorf("invalid image content")
	}
	if int64(width)*int64(height) > maxChatImagePixels {
		return "", 0, 0, fmt.Errorf("image exceeds the 40 megapixel limit")
	}
	return mimeType, width, height, nil
}

func decodeChatImageDimensions(mimeType string, raw []byte) (int, int, error) {
	if mimeType == "image/webp" {
		return decodeWebPDimensions(raw)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(raw))
	return config.Width, config.Height, err
}

func isWebP(raw []byte) bool {
	return len(raw) >= 12 && string(raw[:4]) == "RIFF" && string(raw[8:12]) == "WEBP"
}

func decodeWebPDimensions(raw []byte) (int, int, error) {
	if !isWebP(raw) {
		return 0, 0, fmt.Errorf("invalid webp")
	}
	for offset := 12; offset+8 <= len(raw); {
		kind := string(raw[offset : offset+4])
		size := int(binary.LittleEndian.Uint32(raw[offset+4 : offset+8]))
		start := offset + 8
		end := start + size
		if size < 0 || end > len(raw) {
			return 0, 0, fmt.Errorf("invalid webp chunk")
		}
		data := raw[start:end]
		switch kind {
		case "VP8X":
			if len(data) >= 10 {
				return 1 + int(data[4]) + int(data[5])<<8 + int(data[6])<<16,
					1 + int(data[7]) + int(data[8])<<8 + int(data[9])<<16, nil
			}
		case "VP8 ":
			if len(data) >= 10 && bytes.Equal(data[3:6], []byte{0x9d, 0x01, 0x2a}) {
				return int(binary.LittleEndian.Uint16(data[6:8]) & 0x3fff),
					int(binary.LittleEndian.Uint16(data[8:10]) & 0x3fff), nil
			}
		case "VP8L":
			if len(data) >= 5 && data[0] == 0x2f {
				bits := binary.LittleEndian.Uint32(data[1:5])
				return int(bits&0x3fff) + 1, int((bits>>14)&0x3fff) + 1, nil
			}
		}
		offset = end + size%2
	}
	return 0, 0, fmt.Errorf("webp dimensions not found")
}

func chatImageSHA256(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func safeChatAttachmentName(name string) string {
	name = strings.TrimSpace(filepath.Base(name))
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." {
		return "image"
	}
	return name
}

func validateChatImageFilename(name, mimeType string) error {
	extension := strings.ToLower(filepath.Ext(strings.TrimSpace(name)))
	if extension == "" {
		return nil
	}
	valid := map[string]map[string]bool{
		"image/png":  {".png": true},
		"image/jpeg": {".jpg": true, ".jpeg": true, ".jfif": true},
		"image/webp": {".webp": true},
		"image/gif":  {".gif": true},
	}
	if !valid[mimeType][extension] {
		return fmt.Errorf("image filename extension does not match its content")
	}
	return nil
}
