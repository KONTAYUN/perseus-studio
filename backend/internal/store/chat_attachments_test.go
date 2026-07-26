package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestChatAttachmentLifecycleAndConversationBranching(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	conversations := NewConversationStore(db)
	if err := conversations.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	conversation, err := conversations.CreateConversation(ctx, CreateConversationInput{
		UserID: "user-a", SelectedModelProviderID: "model-a",
	})
	if err != nil {
		t.Fatalf("CreateConversation returned error: %v", err)
	}
	attachment, err := conversations.CreateChatAttachment(ctx, CreateChatAttachmentInput{
		ID: "attachment-a", UserID: "user-a", OriginalName: "screen.png", MIMEType: "image/png",
		SizeBytes: 1024, Width: 640, Height: 480, SHA256: "hash-a", StorageKey: "chat/attachment-a.png",
	})
	if err != nil {
		t.Fatalf("CreateChatAttachment returned error: %v", err)
	}
	message, err := conversations.AppendMessageWithAttachmentIDs(ctx, MessageInput{
		UserID: "user-a", ConversationID: conversation.ID, Role: "user", Content: "inspect",
		AttachmentsJSON: `[{"id":"attachment-a","kind":"image"}]`,
	}, []string{attachment.ID})
	if err != nil {
		t.Fatalf("AppendMessageWithAttachmentIDs returned error: %v", err)
	}
	linked, err := conversations.ListMessageChatAttachments(ctx, "user-a", message.ID)
	if err != nil || len(linked) != 1 || linked[0].Status != ChatAttachmentStatusAttached || !linked[0].Included {
		t.Fatalf("unexpected message attachment relation: %#v err=%v", linked, err)
	}
	if _, err := conversations.ListMessageChatAttachments(ctx, "user-b", message.ID); err != nil {
		t.Fatalf("cross-user lookup should return an empty list without leaking details: %v", err)
	}

	branched, _, err := conversations.BranchConversation(ctx, "user-a", conversation.ID, message.ID)
	if err != nil {
		t.Fatalf("BranchConversation returned error: %v", err)
	}
	if removed, err := conversations.DeleteConversationWithAttachments(ctx, "user-a", conversation.ID); err != nil || len(removed) != 0 {
		t.Fatalf("shared branch attachment must remain: removed=%#v err=%v", removed, err)
	}
	removed, err := conversations.DeleteConversationWithAttachments(ctx, "user-a", branched.ID)
	if err != nil || len(removed) != 1 || removed[0].ID != attachment.ID {
		t.Fatalf("last conversation link must release attachment: removed=%#v err=%v", removed, err)
	}
}

func TestAppendMessageRejectsAnotherUsersAttachment(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	conversations := NewConversationStore(db)
	if err := conversations.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	conversation, _ := conversations.CreateConversation(ctx, CreateConversationInput{UserID: "user-b", SelectedModelProviderID: "model"})
	attachment, _ := conversations.CreateChatAttachment(ctx, CreateChatAttachmentInput{
		ID: "private-image", UserID: "user-a", OriginalName: "private.png", MIMEType: "image/png",
		SizeBytes: 12, Width: 1, Height: 1, SHA256: "hash", StorageKey: "chat/private-image.png",
	})
	if _, err := conversations.AppendMessageWithAttachmentIDs(ctx, MessageInput{
		UserID: "user-b", ConversationID: conversation.ID, Role: "user", Content: "steal",
	}, []string{attachment.ID}); err == nil {
		t.Fatal("another user's staged attachment must not be attachable")
	}
}

func TestDeleteMessageReturnsNewlyOrphanedAttachment(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()
	conversations := NewConversationStore(db)
	if err := conversations.Migrate(ctx); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	conversation, err := conversations.CreateConversation(ctx, CreateConversationInput{UserID: "user-a", SelectedModelProviderID: "model-a"})
	if err != nil {
		t.Fatalf("CreateConversation returned error: %v", err)
	}
	attachment, err := conversations.CreateChatAttachment(ctx, CreateChatAttachmentInput{
		ID: "attachment-delete", UserID: "user-a", OriginalName: "screen.png", MIMEType: "image/png",
		SizeBytes: 10, Width: 2, Height: 2, SHA256: "hash", StorageKey: "chat/attachment-delete.png",
	})
	if err != nil {
		t.Fatalf("CreateChatAttachment returned error: %v", err)
	}
	message, err := conversations.AppendMessageWithAttachmentIDs(ctx, MessageInput{
		UserID: "user-a", ConversationID: conversation.ID, Role: "user", Content: "image",
	}, []string{attachment.ID})
	if err != nil {
		t.Fatalf("AppendMessageWithAttachmentIDs returned error: %v", err)
	}
	removed, err := conversations.DeleteMessageWithAttachments(ctx, "user-a", conversation.ID, message.ID)
	if err != nil {
		t.Fatalf("DeleteMessageWithAttachments returned error: %v", err)
	}
	if len(removed) != 1 || removed[0].ID != attachment.ID {
		t.Fatalf("expected orphaned attachment to be returned, got %#v", removed)
	}
	if _, err := conversations.GetChatAttachment(ctx, attachment.ID); err == nil {
		t.Fatal("orphaned attachment record was not deleted")
	}
}
