package ai

import (
	"strings"
	"testing"
)

func TestBuildUserTextWithEditorPanelsContextDoesNotIncludeFileContent(t *testing.T) {
	text := buildUserTextWithAttachments("看看左边编辑栏", []AttachmentPayload{
		{
			Kind: "editor_panels_context",
			EditorPanels: []EditorPanelContext{
				{
					PanelID:        "editor_1",
					Index:          1,
					PositionLabel:  "left",
					ActiveFilePath: "notes/plan.md",
					ActiveFileName: "plan.md",
					OpenTabs:       []string{"notes/plan.md", "draft.txt"},
				},
				{
					PanelID:        "editor_2",
					Index:          2,
					PositionLabel:  "right",
					ActiveFilePath: "src/app.js",
					ActiveFileName: "app.js",
					OpenTabs:       []string{"src/app.js"},
				},
			},
		},
		{
			Kind: "current_file",
			Path: "legacy/current.md",
			Name: "current.md",
			Text: "legacy current body",
		},
	})

	for _, expected := range []string{
		"看看左边编辑栏",
		"当前打开的编辑器面板如下",
		"left: panel_id=editor_1, active_file=notes/plan.md, open_tabs=[notes/plan.md, draft.txt]",
		"right: panel_id=editor_2, active_file=src/app.js, open_tabs=[src/app.js]",
		"打开状态本身不是读取指令",
		"任务明确引用某个文档或所需事实可能位于其中",
		"切换标签只更新当前环境，不改变任务目标和文件工作集",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected context to contain %q, got:\n%s", expected, text)
		}
	}

	for _, forbidden := range []string{
		"secret file body",
		"完整文件内容",
		"内容：",
		"legacy current body",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("editor panels context leaked file content marker %q:\n%s", forbidden, text)
		}
	}
}

func TestBuildUserTextWithTextAttachmentIncludesOnlyExplicitTextFiles(t *testing.T) {
	text := buildUserTextWithAttachments("总结这个附件", []AttachmentPayload{
		{
			Kind:      "text",
			Name:      "uploaded.txt",
			Text:      "explicit uploaded text",
			Truncated: true,
		},
		{
			Kind:     "workspace_file",
			Path:     "workspace/private.md",
			Name:     "private.md",
			FileType: "text",
			Text:     "private workspace file body",
		},
	})

	for _, expected := range []string{
		"总结这个附件",
		"以下是用户上传文本附件内容",
		"文件名：uploaded.txt",
		"explicit uploaded text",
		"[该文件内容已截断]",
		"workspace file reference: path=workspace/private.md",
		"不包含文件内容",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected attachment text to contain %q, got:\n%s", expected, text)
		}
	}

	if strings.Contains(text, "private workspace file body") {
		t.Fatalf("workspace_file metadata unexpectedly included file body:\n%s", text)
	}
}

func TestBuildUserTextWithWorkspaceAndCurrentFileReferencesNeverIncludeText(t *testing.T) {
	text := buildUserTextWithAttachments("重写这两份", []AttachmentPayload{
		{
			Kind:      "workspace_file",
			Name:      "chapter.txt",
			Path:      "draft/chapter.txt",
			FileType:  "text",
			Text:      "chapter body",
			Truncated: true,
		},
		{
			Kind:     "current_file",
			Name:     "current.txt",
			Path:     "draft/current.txt",
			FileType: "text",
			Text:     "current body",
		},
	})

	for _, expected := range []string{
		"workspace file reference: path=draft/chapter.txt",
		"current workspace file reference: path=draft/current.txt",
		"不包含文件内容",
		"打开状态本身不是读取指令",
		"任务所需事实可能位于其中",
		"若与任务无关则不要读取",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected attachment text to contain %q, got:\n%s", expected, text)
		}
	}

	for _, forbidden := range []string{
		"chapter body",
		"current body",
		"optional preview",
		"文件预览",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("workspace/current file reference leaked %q:\n%s", forbidden, text)
		}
	}
}
