package api

import (
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
