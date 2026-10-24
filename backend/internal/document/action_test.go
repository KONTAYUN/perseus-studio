package document

import (
	"strings"
	"testing"
)

func TestRepairAuthorizationCannotBroadenToolScope(t *testing.T) {
	a := &Action{Type: "format_repair", Path: "目录/a.md", BaseHash: strings.Repeat("a", 64), Profile: Version, Diagnostics: []Issue{{Code: "roundtrip", Message: "测试"}}}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"run_workspace_command", "start_background_task", "manage_workspace_path", "web_extract", "load_perseus_toolset", "save_workspace_file", "read_workspace_files"} {
		if a.Allows(tool) {
			t.Fatal(tool)
		}
	}
	for _, tool := range []string{"read_workspace_text", "apply_workspace_patch", "attempt_completion", "ask_followup_question"} {
		if !a.Allows(tool) {
			t.Fatal(tool)
		}
	}
	restored, err := ActionFromMeta(a.MetaJSON())
	if err != nil || restored.MetaJSON() != a.MetaJSON() {
		t.Fatal(err)
	}
	for _, path := range []string{"../a.md", "/a.md", "a\\b.md", "a.txt", "E:/a.md"} {
		copy := *a
		copy.Path = path
		if copy.Validate() == nil {
			t.Fatal(path)
		}
	}
}
func TestRepairProjectionDistinguishesProseValuesLinksCodeAndHTML(t *testing.T) {
	for _, pair := range [][2]string{{"正文 123", "正文 456"}, {"[链接](https://a.test)", "[链接](https://b.test)"}, {"```\na\n```", "```\nb\n```"}, {"<div>x</div>", "<div>y</div>"}, {"[r]: https://a.test", "[r]: https://b.test"}, {"$a$", "$b$"}} {
		if RepairEquivalent([]byte(pair[0]), []byte(pair[1])) {
			t.Fatal(pair)
		}
	}
	if !RepairEquivalent([]byte("**正文**\n"), []byte("__正文__\n\n")) {
		t.Fatal("format-only normalization rejected")
	}
}
