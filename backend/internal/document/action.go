package document

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

// Action is explicit user authorization, persisted with the user message.
// Runtime permissions are enforced independently of the generated prompt.
type Action struct {
	Type        string  `json:"type"`
	Path        string  `json:"path"`
	BaseHash    string  `json:"base_hash"`
	Profile     string  `json:"profile_version"`
	Diagnostics []Issue `json:"diagnostics"`
}

func (a *Action) Validate() error {
	if a == nil {
		return nil
	}
	hash, err := hex.DecodeString(a.BaseHash)
	if a.Type != "format_repair" || !IsMarkdown(a.Path) || path.Clean(a.Path) != a.Path || strings.HasPrefix(a.Path, "/") || strings.HasPrefix(a.Path, "../") || strings.ContainsAny(a.Path, "\\:\x00") || a.Profile != Version || err != nil || len(hash) != 32 || len(a.Diagnostics) == 0 || len(a.Diagnostics) > 20 {
		return fmt.Errorf("invalid document repair authorization")
	}
	for _, issue := range a.Diagnostics {
		if len(issue.Code) > 100 || len(issue.Message) > 1000 {
			return fmt.Errorf("document diagnostic is too long")
		}
	}
	return nil
}
func (a *Action) MetaJSON() string {
	if a == nil {
		return ""
	}
	raw, _ := json.Marshal(map[string]any{"document_action": a})
	return string(raw)
}
func ActionFromMeta(raw string) (*Action, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var meta struct {
		Action *Action `json:"document_action"`
	}
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return nil, err
	}
	return meta.Action, meta.Action.Validate()
}
func (a *Action) Allows(tool string) bool {
	return a == nil || tool == "read_workspace_text" || tool == "apply_workspace_patch" || tool == "attempt_completion" || tool == "ask_followup_question"
}
func (a *Action) Prompt() string {
	raw, _ := json.Marshal(a)
	return "用户已同意检查这一个文稿的格式。授权和诊断：" + string(raw) + "\n先读取目标文稿，再用 apply_workspace_patch 提交最小格式调整候选；不得更改正文、数值、链接、代码、公式及特殊载荷。候选最多两次。工具返回 staged 表示仅已暂存建议，尚未修改文件；不得声称已应用。只有工具明确返回 applied 才能说明已应用。不能确定时询问用户或结束，不得绕圈、联网、转交后台或操作其他文件。"
}
