package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"webide-backend/internal/ai"
	"webide-backend/internal/idgen"
)

type workspaceFileRef struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	FileType string `json:"file_type"`
}

type editorPanelsContextRef struct {
	ID     string                  `json:"id"`
	Type   string                  `json:"type"`
	Kind   string                  `json:"kind"`
	Panels []ai.EditorPanelContext `json:"panels"`
}

type selectedSkillRef struct {
	ID          string `json:"id"`
	SkillID     string `json:"skill_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func parseWorkspaceFileRefs(raw string) []workspaceFileRef {
	var refs []workspaceFileRef
	_ = json.Unmarshal([]byte(raw), &refs)
	return refs
}

func parseCurrentFileContext(raw string) workspaceFileRef {
	var ref workspaceFileRef
	_ = json.Unmarshal([]byte(raw), &ref)
	return ref
}

func parseEditorPanelsContext(raw string) editorPanelsContextRef {
	var ref editorPanelsContextRef
	_ = json.Unmarshal([]byte(raw), &ref)
	return ref
}

func parseSelectedSkillRefs(raw string) []selectedSkillRef {
	var refs []selectedSkillRef
	_ = json.Unmarshal([]byte(raw), &refs)
	return refs
}

func cleanWorkspaceReferencePath(path string) (string, error) {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	if clean == "" || clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, "../") || clean == ".." || strings.Contains(clean, "/../") {
		return "", fmt.Errorf("invalid workspace file reference path")
	}
	return clean, nil
}

func buildChatAttachment(id, name string, size int64, raw []byte, totalTextChars *int) (ai.AttachmentPayload, chatAttachmentMetadata) {
	mimeType := detectAttachmentMIME(name, raw)
	id = strings.TrimSpace(id)
	if id == "" {
		id = idgen.New("att")
	}
	metadata := chatAttachmentMetadata{
		ID:   id,
		Name: name,
		Size: size,
		Type: mimeType,
	}
	payload := ai.AttachmentPayload{
		ID:       id,
		Name:     name,
		Size:     size,
		MIMEType: mimeType,
	}

	if isSupportedImageMIME(mimeType) {
		payload.Kind = "image"
		payload.DataBase64 = base64.StdEncoding.EncodeToString(raw)
		payload.IncludedInContext = true
		metadata.Kind = "image"
		metadata.IncludedInContext = true
		return payload, metadata
	}

	if strings.EqualFold(mimeType, "image/gif") {
		note := fmt.Sprintf("图片附件 %q 是 GIF，第一版暂未传入模型，仅保存 metadata。", name)
		payload.Kind = "image"
		payload.Note = note
		metadata.Kind = "image"
		metadata.Note = note
		return payload, metadata
	}

	if isTextAttachment(name, mimeType, raw) {
		text := string(raw)
		payload.Kind = "text"
		metadata.Kind = "text"
		remaining := 50000 - *totalTextChars
		if remaining <= 0 {
			note := fmt.Sprintf("文本附件 %q 超过本轮附件总字符限制，未参与上下文。", name)
			payload.Note = note
			metadata.Note = note
			return payload, metadata
		}
		limit := minInt(20000, remaining)
		runes := []rune(text)
		if len(runes) > limit {
			text = string(runes[:limit])
			payload.Truncated = true
			metadata.Truncated = true
		}
		*totalTextChars += len([]rune(text))
		payload.Text = text
		payload.IncludedInContext = true
		metadata.IncludedInContext = true
		return payload, metadata
	}

	if strings.EqualFold(filepath.Ext(name), ".zip") {
		payload.Kind = "zip"
		payload.RawBase64 = base64.StdEncoding.EncodeToString(raw)
		payload.Note = fmt.Sprintf("zip 附件 %q 可通过 install_skill_from_attachment 安装；attachment_id=%s。", name, id)
		metadata.Kind = "zip"
		metadata.Note = payload.Note
		return payload, metadata
	}

	note := fmt.Sprintf("附件 %q 暂不支持预览或传入模型，仅保存 metadata。", name)
	payload.Kind = "binary"
	payload.Note = note
	metadata.Kind = "binary"
	metadata.Note = note
	return payload, metadata
}

func detectAttachmentMIME(name string, raw []byte) string {
	if fromExt := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); fromExt != "" {
		return strings.Split(fromExt, ";")[0]
	}
	if len(raw) > 0 {
		return http.DetectContentType(raw)
	}
	return "application/octet-stream"
}

func isSupportedImageMIME(mimeType string) bool {
	switch strings.ToLower(mimeType) {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
		return true
	default:
		return false
	}
}

func isTextAttachment(name, mimeType string, raw []byte) bool {
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	textExtensions := map[string]bool{
		"txt": true, "md": true, "json": true, "js": true, "jsx": true, "ts": true, "tsx": true,
		"css": true, "html": true, "go": true, "py": true, "yaml": true, "yml": true,
		"xml": true, "csv": true, "log": true, "toml": true, "env": true, "gitignore": true,
	}
	if textExtensions[extension] || strings.EqualFold(filepath.Base(name), ".env") || strings.EqualFold(filepath.Base(name), ".gitignore") {
		return utf8.Valid(raw)
	}
	if strings.HasPrefix(strings.ToLower(mimeType), "text/") || strings.Contains(strings.ToLower(mimeType), "json") || strings.Contains(strings.ToLower(mimeType), "xml") {
		return utf8.Valid(raw)
	}
	return false
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
