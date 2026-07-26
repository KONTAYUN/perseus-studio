package ai

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"webide-backend/internal/store"
)

func TestProviderImageCapabilityErrorIsStructured(t *testing.T) {
	err := &ProviderResponseError{
		StatusCode: http.StatusBadRequest,
		Message:    "This model does not support image input",
		ErrorCode:  "image_input_not_supported",
	}
	metadata := FailureMetadataFromError(err)
	if metadata.FailureClass != string(ProviderFailureCapabilityUnsupported) {
		t.Fatalf("unexpected failure class: %#v", metadata)
	}
	if metadata.CanResumeNow || metadata.RecommendedAction != "review_model_capabilities" {
		t.Fatalf("image capability failure must recommend a vision model: %#v", metadata)
	}
}

func TestImageBase64IsExcludedFromTextTokenEstimate(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString(make([]byte, 650*1024))
	messages := []map[string]any{{
		"role": "user",
		"content": []map[string]any{
			{"type": "text", "text": "请查看这张截图"},
			{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + encoded}},
		},
	}}
	textTokens, images := estimateRuntimeMessagesTokenParts(messages)
	if images != 1 {
		t.Fatalf("expected one image, got %d", images)
	}
	if textTokens > 500 {
		t.Fatalf("base64 leaked into text token estimate: %d", textTokens)
	}
	if total := estimateRuntimeMessagesTokens(messages); total > 3000 {
		t.Fatalf("ordinary image estimate is unexpectedly huge: %d", total)
	}
}

func TestContextAssemblerSeparatesTextAndVisualTokens(t *testing.T) {
	user := contextTestItem(1, "turn-1", "user_message", "user", map[string]any{"content": "分析截图"})
	user.SourceID = "message-1"
	attachment := AttachmentPayload{
		ID: "image-1", MessageID: "message-1", Kind: "image", MIMEType: "image/png",
		Width: 1280, Height: 720, DataBase64: strings.Repeat("A", 900000),
	}
	profile := contextTestProfile(128000)
	profile.ModelID = "gpt-4o"
	assembly := (ContextAssembler{}).Assemble(ContextAssemblerInput{
		Protocol: agentProtocolOpenAI, Items: []store.AgentTurnItem{user}, Attachments: []AttachmentPayload{attachment},
		Profile: profile, CurrentTurnID: "turn-1",
	})
	withoutImage := (ContextAssembler{}).Assemble(ContextAssemblerInput{
		Protocol: agentProtocolOpenAI, Items: []store.AgentTurnItem{user},
		Profile: profile, CurrentTurnID: "turn-1",
	})
	summary := assembly.Snapshot.Summary
	if summary.ImageCount != 1 || summary.VisualInputTokens <= 0 {
		t.Fatalf("visual input was not estimated separately: %#v", summary)
	}
	if delta := summary.TextInputTokens - withoutImage.Snapshot.Summary.TextInputTokens; delta < 0 || delta > 200 {
		t.Fatalf("image metadata added an implausible text token delta: delta=%d with=%d without=%d", delta, summary.TextInputTokens, withoutImage.Snapshot.Summary.TextInputTokens)
	}
	if summary.ActiveContextTokens != summary.TextInputTokens+summary.VisualInputTokens {
		t.Fatalf("context total does not match text plus visual tokens: %#v", summary)
	}
	if assembly.NeedsCompaction {
		t.Fatalf("ordinary screenshot must not trigger context overflow: %#v", summary)
	}
}

func TestOpenAIAndAnthropicImageBlocksKeepProtocolShape(t *testing.T) {
	attachment := AttachmentPayload{Kind: "image", MIMEType: "image/png", DataBase64: "YWJj"}
	openAI := openAIUserContentWithAttachments("inspect", []AttachmentPayload{attachment})
	if !strings.Contains(mustJSON(openAI), `"type":"image_url"`) {
		t.Fatalf("OpenAI image block is missing: %#v", openAI)
	}
	anthropic := anthropicUserContentWithAttachments("inspect", []AttachmentPayload{attachment})
	if !strings.Contains(mustJSON(anthropic), `"type":"image"`) || !strings.Contains(mustJSON(anthropic), `"type":"base64"`) {
		t.Fatalf("Anthropic image block is missing: %#v", anthropic)
	}
}
