package ai

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

const currentFilePrefetchMaxRunes = 12000

type CurrentFilePrefetchView struct {
	Path            string `json:"path"`
	NumberedContent string `json:"numbered_content"`
	StartLine       int    `json:"start_line"`
	EndLine         int    `json:"end_line"`
	TotalLines      int    `json:"total_lines"`
}

func (s *Service) prefetchWorkspaceFileAttachments(ctx context.Context, userID string, attachments []AttachmentPayload) []AttachmentPayload {
	if s == nil || s.factory == nil || strings.TrimSpace(userID) == "" || len(attachments) == 0 {
		return attachments
	}
	files, err := s.factory.Workspace(userID)
	if err != nil {
		return attachments
	}
	next := append([]AttachmentPayload(nil), attachments...)
	for index := range next {
		attachment := &next[index]
		if attachment.Kind != "workspace_file" || strings.TrimSpace(attachment.Path) == "" || !workspaceAttachmentLooksTextual(*attachment) {
			continue
		}
		result, readErr := files.ReadTextFileRange(attachment.Path, 0, 0, currentFilePrefetchMaxRunes)
		if readErr != nil || result.Truncated || strings.TrimSpace(result.NumberedContent) == "" {
			continue
		}
		view := CurrentFilePrefetchView{
			Path:            result.Path,
			NumberedContent: result.NumberedContent,
			StartLine:       result.StartLine,
			EndLine:         result.EndLine,
			TotalLines:      result.TotalLines,
		}
		attachment.Text = fmt.Sprintf("path=%s\nstart_line=%d\nend_line=%d\ntotal_lines=%d\nnumbered_content:\n%s", view.Path, view.StartLine, view.EndLine, view.TotalLines, view.NumberedContent)
		attachment.IncludedInContext = true
		attachment.Truncated = false
	}
	return next
}

func workspaceAttachmentLooksTextual(attachment AttachmentPayload) bool {
	fileType := strings.ToLower(strings.TrimSpace(attachment.FileType))
	if fileType == "" || fileType == "text" || fileType == "markdown" || fileType == "code" {
		return true
	}
	mimeType := strings.ToLower(strings.TrimSpace(attachment.MIMEType))
	if strings.HasPrefix(mimeType, "text/") || strings.Contains(mimeType, "json") || strings.Contains(mimeType, "xml") {
		return true
	}
	switch strings.ToLower(filepath.Ext(attachment.Path)) {
	case ".txt", ".md", ".markdown", ".json", ".jsonl", ".yaml", ".yml", ".toml", ".csv", ".tsv", ".js", ".ts", ".tsx", ".jsx", ".go", ".py", ".rs", ".java", ".c", ".cpp", ".h", ".hpp", ".css", ".scss", ".html", ".xml":
		return true
	default:
		return false
	}
}
