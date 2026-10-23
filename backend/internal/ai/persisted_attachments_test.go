package ai

import (
	"encoding/json"
	"strings"
	"testing"
	"webide-backend/internal/store"
)

func TestHistoricalUploadsReachBothProvidersAndBudgets(t *testing.T) {
	payload := PersistedAttachmentPayloads([]AttachmentPayload{{Kind: "text", Text: strings.Repeat("附件事实", 100), Name: "source.txt"}, {Kind: "image", MIMEType: "image/png", DataBase64: "IMAGE_DATA"}, {Kind: "current_file", Text: "TRANSIENT_EDITOR"}})
	messages := []store.Message{{Role: "user", Content: "第一轮", AttachmentPayloadsJSON: payload}, {Role: "assistant", Content: "收到"}, {Role: "user", Content: "第二轮"}}
	openAI := buildOpenAIMessages(messages, nil)
	_, anthropic := buildAnthropicMessages(messages, nil)
	for _, items := range [][]map[string]any{openAI, anthropic} {
		raw, _ := json.Marshal(items)
		if !strings.Contains(string(raw), "附件事实") || !strings.Contains(string(raw), "IMAGE_DATA") || strings.Contains(string(raw), "TRANSIENT_EDITOR") {
			t.Fatalf("historical input lost: %s", raw)
		}
		last, _ := json.Marshal(items[len(items)-1])
		if strings.Contains(string(last), "附件事实") {
			t.Fatal("attachment moved to new turn")
		}
	}
	without := append([]store.Message(nil), messages...)
	without[0].AttachmentPayloadsJSON = ""
	if estimateMessagesTokens(messages)-estimateMessagesTokens(without) < 2000 {
		t.Fatal("attachment budget undercounted")
	}
	restored := attachmentsForMessage(messages[0], nil, true)
	if len(restored) != 2 {
		t.Fatal("resume missing attachments")
	}
}

func TestTaskDoesNotInheritFutureConversationSummary(t *testing.T) {
	env := newTestAIEnv(t)
	conv, err := env.conversations.CreateConversation(t.Context(), store.CreateConversationInput{UserID: testUserID})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.conversations.UpdateConversationSummary(t.Context(), testUserID, conv.ID, "FUTURE_FACT", "future"); err != nil {
		t.Fatal(err)
	}
	input := []store.Message{{ID: "source", Role: "user", Content: "source", AttachmentPayloadsJSON: PersistedAttachmentPayloads([]AttachmentPayload{{Kind: "text", Text: "ATTACHED_FACT"}})}}
	result := env.service.prepareMessagesWithConversationSummary(t.Context(), conv.ID, store.ModelProvider{}, input, RunOptions{UserID: testUserID, TaskID: "task"})
	if len(result) != 1 || result[0].ID != "source" {
		t.Fatal("future summary leaked")
	}
	if !strings.Contains(buildConversationSummaryPrompt("", input, ""), "ATTACHED_FACT") {
		t.Fatal("attachment excluded from summary input")
	}
}
