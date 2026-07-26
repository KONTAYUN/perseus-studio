package api

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"webide-backend/internal/store"
	"webide-backend/internal/usersvc"
)

func TestChatAttachmentUploadReadAndOwnerIsolation(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	conversations := store.NewConversationStore(db)
	if err := conversations.Migrate(context.Background()); err != nil {
		t.Fatalf("conversations.Migrate returned error: %v", err)
	}
	skills := store.NewSkillStore(db)
	if err := skills.Migrate(context.Background()); err != nil {
		t.Fatalf("skills.Migrate returned error: %v", err)
	}
	server := &Server{
		conversations: conversations,
		factory:       usersvc.NewFactory(db, filepath.Join(t.TempDir(), "users"), skills),
	}
	raw := testPNG(t, 32, 20)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "screen.png")
	if err != nil {
		t.Fatalf("CreateFormFile returned error: %v", err)
	}
	_, _ = part.Write(raw)
	_ = writer.Close()
	upload := httptest.NewRequest(http.MethodPost, "/api/chat-attachments", &body)
	upload.Header.Set("Content-Type", writer.FormDataContentType())
	upload = upload.WithContext(context.WithValue(upload.Context(), ctxSessionKey, &SessionInfo{UserID: "user-a", Role: "user"}))
	uploadResponse := httptest.NewRecorder()
	server.handleChatAttachments(uploadResponse, upload)
	if uploadResponse.Code != http.StatusCreated {
		t.Fatalf("upload returned %d: %s", uploadResponse.Code, uploadResponse.Body.String())
	}
	var item chatAttachmentMetadata
	if err := json.Unmarshal(uploadResponse.Body.Bytes(), &item); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if item.ID == "" || item.MIMEType != "image/png" || item.Width != 32 || item.Height != 20 || !item.Available {
		t.Fatalf("unexpected upload metadata: %#v", item)
	}

	read := httptest.NewRequest(http.MethodGet, "/api/chat-attachments/"+item.ID+"/content", nil)
	read = read.WithContext(context.WithValue(read.Context(), ctxSessionKey, &SessionInfo{UserID: "user-a", Role: "user"}))
	readResponse := httptest.NewRecorder()
	server.handleChatAttachmentRoutes(readResponse, read)
	if readResponse.Code != http.StatusOK || !bytes.Equal(readResponse.Body.Bytes(), raw) {
		t.Fatalf("owner read failed: status=%d bytes=%d", readResponse.Code, readResponse.Body.Len())
	}
	if readResponse.Header().Get("X-Content-Type-Options") != "nosniff" || readResponse.Header().Get("ETag") == "" {
		t.Fatalf("missing secure content headers: %#v", readResponse.Header())
	}

	foreign := httptest.NewRequest(http.MethodGet, "/api/chat-attachments/"+item.ID+"/content", nil)
	foreign = foreign.WithContext(context.WithValue(foreign.Context(), ctxSessionKey, &SessionInfo{UserID: "user-b", Role: "user"}))
	foreignResponse := httptest.NewRecorder()
	server.handleChatAttachmentRoutes(foreignResponse, foreign)
	if foreignResponse.Code != http.StatusNotFound {
		t.Fatalf("non-owner read must return 404, got %d", foreignResponse.Code)
	}

	conversation, err := conversations.CreateConversation(context.Background(), store.CreateConversationInput{
		UserID: "user-a", SelectedModelProviderID: "model-a",
	})
	if err != nil {
		t.Fatalf("CreateConversation returned error: %v", err)
	}
	storedMessage, err := conversations.AppendMessageWithAttachmentIDs(context.Background(), store.MessageInput{
		UserID: "user-a", ConversationID: conversation.ID, Role: "user", Content: "new image",
		AttachmentsJSON: `[{"id":"` + item.ID + `","kind":"image","available":false}]`,
	}, []string{item.ID})
	if err != nil {
		t.Fatalf("AppendMessageWithAttachmentIDs returned error: %v", err)
	}
	legacyMessage, err := conversations.AppendMessage(context.Background(), "user-a", conversation.ID, "user", "old image", `[{"id":"legacy","name":"old.png","kind":"image","type":"image/png"}]`)
	if err != nil {
		t.Fatalf("AppendMessage returned error: %v", err)
	}
	toolEventMessage, err := conversations.AppendMessage(
		context.Background(),
		"user-a",
		conversation.ID,
		"assistant",
		"done",
		`[{"type":"workspace_file_changed","tool_call_id":"call-1","tool_name":"apply_workspace_patch","status_kind":"workspace_edit","label":"已编辑文件","status":"success","path":"note.html"}]`,
	)
	if err != nil {
		t.Fatalf("AppendMessage returned error: %v", err)
	}
	projected := server.projectConversationChatAttachments(context.Background(), "user-a", conversation.ID, []store.Message{storedMessage, legacyMessage, toolEventMessage})
	var storedMetadata, legacyMetadata []chatAttachmentMetadata
	_ = json.Unmarshal(projected[0].Attachments, &storedMetadata)
	_ = json.Unmarshal(projected[1].Attachments, &legacyMetadata)
	if len(storedMetadata) != 1 || !storedMetadata[0].Available || storedMetadata[0].ContentURL == "" {
		t.Fatalf("stored attachment did not project as available: %#v", storedMetadata)
	}
	if len(legacyMetadata) != 1 || legacyMetadata[0].Available || legacyMetadata[0].Status != "legacy_unavailable" {
		t.Fatalf("legacy metadata must remain visible but unavailable: %#v", legacyMetadata)
	}
	var toolEvents []map[string]any
	if err := json.Unmarshal(projected[2].Attachments, &toolEvents); err != nil {
		t.Fatalf("projected tool events are invalid: %v", err)
	}
	if len(toolEvents) != 1 ||
		toolEvents[0]["tool_name"] != "apply_workspace_patch" ||
		toolEvents[0]["label"] != "已编辑文件" ||
		toolEvents[0]["path"] != "note.html" {
		t.Fatalf("non-image presentation event was changed by image projection: %#v", toolEvents)
	}
}

func TestDetectChatImageReadsDimensionsFromContent(t *testing.T) {
	raw := testPNG(t, 2, 2)
	if mimeType, width, height, err := detectChatImage(raw); err != nil || mimeType != "image/png" || width != 2 || height != 2 {
		t.Fatalf("valid PNG was not detected: mime=%s size=%dx%d err=%v", mimeType, width, height, err)
	}
}

func TestChatImageFilenameMustMatchDetectedContent(t *testing.T) {
	raw := testPNG(t, 2, 2)
	mimeType, _, _, err := detectChatImage(raw)
	if err != nil {
		t.Fatalf("detectChatImage returned error: %v", err)
	}
	if err := validateChatImageFilename("screen.jpg", mimeType); err == nil {
		t.Fatal("PNG content with a JPEG filename must be rejected")
	}
	if err := validateChatImageFilename("screen.png", mimeType); err != nil {
		t.Fatalf("matching image filename was rejected: %v", err)
	}
}

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.RGBA{R: 40, G: 120, B: 240, A: 255})
	var raw bytes.Buffer
	if err := png.Encode(&raw, img); err != nil {
		t.Fatalf("png.Encode returned error: %v", err)
	}
	return raw.Bytes()
}
