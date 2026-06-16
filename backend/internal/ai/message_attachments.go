package ai

import (
	"fmt"
	"strings"
)

func buildUserTextWithAttachments(content string, attachments []AttachmentPayload) string {
	var builder strings.Builder
	builder.WriteString(strings.TrimSpace(content))

	var textParts []string
	var notes []string
	for _, attachment := range attachments {
		switch attachment.Kind {
		case "text":
			if attachment.Text == "" {
				continue
			}
			text := fmt.Sprintf("文件名：%s\n内容：\n%s", attachment.Name, attachment.Text)
			if attachment.Truncated {
				text += "\n[该文件内容已截断]"
			}
			textParts = append(textParts, text)
		case "workspace_file":
			note := fmt.Sprintf("workspace file reference: path=%s, name=%s, file_type=%s。这里只提供用户明确引用的工作区路径 metadata，不包含文件内容，也不代表必须读取；需要文件真实当前内容且没有可复用 recent read observation/current_read_working_set 时，调用 read_workspace_text 或 read_workspace_files。", attachment.Path, attachment.Name, attachment.FileType)
			notes = append(notes, note)
		case "current_file":
			note := fmt.Sprintf("current workspace file reference: path=%s, name=%s, file_type=%s。用户说“当前文件/这个文件/这里”时优先理解为此路径；这里只提供 metadata，不包含文件内容，也不代表本轮必须读取；需要文件真实当前内容且没有可复用 recent read observation/current_read_working_set 时，调用 read_workspace_text 或 read_workspace_files。", attachment.Path, attachment.Name, attachment.FileType)
			notes = append(notes, note)
		case "editor_panels_context":
			var lines []string
			for _, panel := range attachment.EditorPanels {
				tabs := strings.Join(panel.OpenTabs, ", ")
				if tabs == "" {
					tabs = "无"
				}
				lines = append(lines, fmt.Sprintf("%s: panel_id=%s, active_file=%s, open_tabs=[%s]", panel.PositionLabel, panel.PanelID, panel.ActiveFilePath, tabs))
			}
			if len(lines) > 0 {
				notes = append(notes, "当前打开的编辑器面板如下。用户提到左边/右边/中间编辑栏时按 position_label 定位；这里只提供路径和标签页，不包含文件全文，也不代表必须读取；需要文件真实当前内容且没有可复用 recent read observation/current_read_working_set 时，请调用 read_workspace_text 或 read_workspace_files。\n  "+strings.Join(lines, "\n  "))
			}
		case "selected_skill":
			if attachment.Text != "" {
				text := fmt.Sprintf("用户为本轮对话选择了 Skill：%s (%s)\nSKILL.md：\n%s", attachment.Name, attachment.SkillID, attachment.Text)
				if attachment.Truncated {
					text += "\n[该 Skill 内容已截断]"
				}
				textParts = append(textParts, text)
			} else if attachment.Note != "" {
				notes = append(notes, attachment.Note)
			} else {
				notes = append(notes, fmt.Sprintf("用户为本轮对话选择了 Skill：%s (%s)。如需具体步骤，请先加载 skills 工具组并调用 read_skill；不要读取未启用 Skill。", attachment.Name, attachment.SkillID))
			}
		case "task":
			notes = append(notes, "此附件是后台任务状态卡片，不是用户上传文件；不要把它当作 workspace 文件读取。")
		case "tool_event":
			notes = append(notes, "此附件是历史工具调用记录，不是用户上传文件；只能作为操作历史参考。")
		case "image":
			if attachment.DataBase64 != "" {
				notes = append(notes, fmt.Sprintf("图片附件 %q 已作为视觉输入提供。", attachment.Name))
			} else if attachment.Note != "" {
				notes = append(notes, attachment.Note)
			}
		default:
			if attachment.Note != "" {
				notes = append(notes, attachment.Note)
			}
		}
	}

	if len(textParts) > 0 {
		builder.WriteString("\n\n以下是用户上传文本附件内容：\n\n")
		builder.WriteString(strings.Join(textParts, "\n\n---\n\n"))
	}
	if len(notes) > 0 {
		builder.WriteString("\n\n附件说明：\n")
		for _, note := range notes {
			builder.WriteString("- ")
			builder.WriteString(note)
			builder.WriteString("\n")
		}
	}
	return builder.String()
}
