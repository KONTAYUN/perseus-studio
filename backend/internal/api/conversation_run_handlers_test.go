package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSplitAssistantDeltaKeepsShortTextWhole(t *testing.T) {
	chunks := splitAssistantDelta("hello", 10)
	if len(chunks) != 1 || chunks[0] != "hello" {
		t.Fatalf("unexpected chunks: %#v", chunks)
	}
}

func TestSplitAssistantDeltaSplitsLargeTextByRune(t *testing.T) {
	text := strings.Repeat("你", 5)
	chunks := splitAssistantDelta(text, 2)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %#v", chunks)
	}
	if strings.Join(chunks, "") != text {
		t.Fatalf("chunks did not preserve text: %#v", chunks)
	}
	if chunks[0] != "你你" || chunks[2] != "你" {
		t.Fatalf("unexpected chunk boundaries: %#v", chunks)
	}
}

func TestMissingAssistantFinalDeltaReturnsUnstreamedTail(t *testing.T) {
	got := missingAssistantFinalDelta("第一段", "第一段第二段")
	if got != "第二段" {
		t.Fatalf("expected missing tail, got %q", got)
	}
}

func TestMissingAssistantFinalDeltaAvoidsDuplicateCompletedContent(t *testing.T) {
	got := missingAssistantFinalDelta("第一段第二段", " 第一段第二段 ")
	if got != "" {
		t.Fatalf("expected no missing delta, got %q", got)
	}
}

func TestMissingAssistantFinalDeltaSkipsDivergedContent(t *testing.T) {
	got := missingAssistantFinalDelta("旧内容", "新内容")
	if got != "" {
		t.Fatalf("expected diverged content to be skipped, got %q", got)
	}
}

func TestAssistantSegmentBufferSuppressesPlaceholderEllipsis(t *testing.T) {
	buffer := &assistantSegmentBuffer{}
	buffer.AppendDelta("...")
	buffer.Flush("")
	buffer.Flush("……")
	buffer.Flush("我先读取文件。")

	if len(buffer.Items) != 1 || buffer.Items[0] != "我先读取文件。" {
		t.Fatalf("unexpected buffered segments: %#v", buffer.Items)
	}
}

func TestShouldFinalizeModelMessageCompletedSkipsAlreadyFinalizedContent(t *testing.T) {
	if !shouldFinalizeModelMessageCompleted("已完成。", "") {
		t.Fatal("expected first final content to be finalized")
	}
	if shouldFinalizeModelMessageCompleted(" 已完成。 ", "已完成。") {
		t.Fatal("same completed content should not be finalized into a duplicate assistant segment")
	}
	if shouldFinalizeModelMessageCompleted("", "已完成。") {
		t.Fatal("empty final content should not be finalized")
	}
}

func TestShouldAppendModelDeltaSuppressesTextWhileCompletionPreviewIsActive(t *testing.T) {
	if !shouldAppendModelDelta(assistantStreamState{}) {
		t.Fatal("ordinary assistant text should append without a completion preview")
	}
	if !shouldAppendModelDelta(assistantStreamState{ActiveMessageID: "msg_preamble", CompletionPreviewID: "msg_completion"}) {
		t.Fatal("assistant text for a different active message should append")
	}
	if shouldAppendModelDelta(assistantStreamState{ActiveMessageID: "msg_completion", CompletionPreviewID: "msg_completion"}) {
		t.Fatal("ordinary model_delta must not be appended into the completion preview message")
	}
}

func TestMessageMetaJSONIncludesRunOwnership(t *testing.T) {
	raw := messageMetaJSON("assistant_segment", 3, " run-1 ")
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("invalid meta json: %v", err)
	}
	if payload["kind"] != "assistant_segment" {
		t.Fatalf("unexpected kind: %#v", payload["kind"])
	}
	if int(payload["segment_index"].(float64)) != 3 {
		t.Fatalf("unexpected segment index: %#v", payload["segment_index"])
	}
	if payload["run_id"] != "run-1" {
		t.Fatalf("expected run ownership in meta, got %#v", payload["run_id"])
	}
}

func TestMessageMetaJSONOmitsBlankRunOwnership(t *testing.T) {
	raw := messageMetaJSON("assistant_segment", 1, " ")
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("invalid meta json: %v", err)
	}
	if _, ok := payload["run_id"]; ok {
		t.Fatalf("blank run id should be omitted: %#v", payload)
	}
}
