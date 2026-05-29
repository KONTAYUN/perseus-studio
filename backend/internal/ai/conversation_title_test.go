package ai

import (
	"strings"
	"testing"
)

func TestNormalizeGeneratedConversationTitle(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "strips label and punctuation", in: "标题：项目部署压测分析。", want: "项目部署压测分析"},
		{name: "uses first line", in: "民国克苏鲁设定\n这是解释", want: "民国克苏鲁设定"},
		{name: "strips quotes", in: "“Code Server 部署”", want: "Code Server 部署"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeGeneratedConversationTitle(tt.in); got != tt.want {
				t.Fatalf("normalizeGeneratedConversationTitle(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBuildConversationTitlePromptIncludesAssistantContext(t *testing.T) {
	prompt := buildConversationTitlePrompt("请帮我部署 New API 到 Docker", "下面是 Docker Compose 和反代配置步骤。")
	if !containsAll(prompt, []string{"User message:", "Assistant response:", "部署 New API", "Docker Compose"}) {
		t.Fatalf("prompt missing expected context: %s", prompt)
	}
}

func containsAll(value string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
