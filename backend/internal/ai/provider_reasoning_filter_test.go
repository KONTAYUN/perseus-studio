package ai

import (
	"strings"
	"testing"
)

func TestProviderVisibleTextRemovesThinkingAndThinkBlocks(t *testing.T) {
	input := "before\n<thinking>private</thinking>\nvisible\n<THINK>also private</THINK>\nafter"
	got := providerVisibleText(input)
	if got != "before\n\nvisible\n\nafter" {
		t.Fatalf("unexpected visible text: %q", got)
	}
	if strings.Contains(got, "private") {
		t.Fatalf("reasoning leaked: %q", got)
	}
}

func TestProviderVisibleTextFilterHandlesSplitTags(t *testing.T) {
	filter := newProviderVisibleTextFilter()
	var got strings.Builder
	got.WriteString(filter.Feed("ok <thin"))
	got.WriteString(filter.Feed("king>hidden</think"))
	got.WriteString(filter.Feed("ing> visible"))
	got.WriteString(filter.Close())
	if strings.TrimSpace(got.String()) != "ok  visible" {
		t.Fatalf("unexpected visible text: %q", got.String())
	}
}

func TestProviderVisibleTextDropsUnclosedReasoningAtEnd(t *testing.T) {
	got := providerVisibleText("visible\n<thinking>private")
	if got != "visible" {
		t.Fatalf("unexpected visible text: %q", got)
	}
}

func TestProviderVisibleTextKeepsTextBeforeStrayClosingTag(t *testing.T) {
	got := providerVisibleText("visible</thinking> tail")
	if got != "visible tail" {
		t.Fatalf("unexpected visible text: %q", got)
	}
}

func TestProviderVisibleTextRemovesToolCallProtocolBlocks(t *testing.T) {
	got := providerVisibleText("先看文件\n<tool_call>{\"name\":\"read_workspace_text\"}</tool_call>\n继续")
	if strings.Contains(got, "tool_call") || strings.Contains(got, "read_workspace_text") {
		t.Fatalf("tool protocol leaked: %q", got)
	}
	if strings.TrimSpace(got) != "先看文件\n\n继续" {
		t.Fatalf("unexpected visible text: %q", got)
	}
}
