package ai

import (
	"strings"
	"testing"
)

func TestAgentActionPolicyPrioritizesAvailableEvidenceBeforeQuestions(t *testing.T) {
	for _, expected := range []string{
		"Your goal is to complete the user's task, not to make the user investigate facts that you can obtain yourself.",
		"use available read, list, search, or inspection tools for facts they can obtain",
		"call request_user_input only when progress requires open-ended information",
		"call request_user_choice only when the decision belongs to the user",
		"do not turn open input into artificial choices",
		"Simple conversation may complete immediately without irrelevant tool calls.",
		"文件仅仅处于打开状态且与任务无关时，不读取",
		"用户明确指向当前文档，或完成任务所需事实可能位于该文档时，主动使用相应读取工具",
		"切换标签只更新当前编辑器环境，不改变任务目标",
	} {
		if !strings.Contains(workspaceToolSystemPrompt, expected) {
			t.Fatalf("workspace prompt missing action policy %q", expected)
		}
	}
}

func TestUserRequestToolsUseSameBoundaryAcrossProtocols(t *testing.T) {
	assertDescription := func(t *testing.T, openAI, anthropic map[string]any, expected string) {
		t.Helper()
		function, _ := openAI["function"].(map[string]any)
		openAIDescription, _ := function["description"].(string)
		anthropicDescription, _ := anthropic["description"].(string)
		if openAIDescription != expected || anthropicDescription != expected {
			t.Fatalf("tool descriptions diverged: openai=%q anthropic=%q", openAIDescription, anthropicDescription)
		}
	}
	assertDescription(t, openAIUserInputTool(), anthropicUserInputTool(), userInputToolDescription)
	assertDescription(t, openAIUserChoiceTool(), anthropicUserChoiceTool(), userChoiceToolDescription)
	if !strings.Contains(userInputToolDescription, "only the user can provide") || !strings.Contains(userInputToolDescription, "do not manufacture suggested answers") {
		t.Fatalf("input tool description has the wrong boundary: %q", userInputToolDescription)
	}
	if !strings.Contains(userChoiceToolDescription, "bounded decision owned by the user") || !strings.Contains(userChoiceToolDescription, "two or three") {
		t.Fatalf("choice tool description has the wrong boundary: %q", userChoiceToolDescription)
	}
}

func TestEditorContextRemainsPathOnlyWhileExplorationStaysAvailable(t *testing.T) {
	text := buildUserTextWithAttachments("Use the current document as evidence.", []AttachmentPayload{{
		Kind: "current_file", Path: "notes/reference.html", Name: "reference.html", FileType: "html", Text: "private body",
	}})
	for _, expected := range []string{
		"current workspace file reference: path=notes/reference.html",
		"不包含文件内容",
		"任务所需事实可能位于其中",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("editor context missing %q: %s", expected, text)
		}
	}
	if strings.Contains(text, "private body") {
		t.Fatalf("editor context leaked file content: %s", text)
	}
}
