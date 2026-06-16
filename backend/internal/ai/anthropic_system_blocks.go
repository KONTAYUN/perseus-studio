package ai

import (
	"os"
	"strings"
)

const actionManifestAnchorPrefix = "Perseus current action manifest. This is environment-provided action state, not user input. Follow it for the next action.\n"

type AnthropicSystemBlockBuilder struct{}

func (AnthropicSystemBlockBuilder) Build(system string, cacheEnabled bool) any {
	system = strings.TrimSpace(system)
	if system == "" {
		return ""
	}
	if !cacheEnabled {
		return system
	}
	stable, dynamic := splitAnthropicStableSystem(system)
	if stable == "" {
		stable = system
		dynamic = ""
	}
	blocks := []map[string]any{{
		"type":          "text",
		"text":          stable,
		"cache_control": map[string]any{"type": "ephemeral"},
	}}
	if dynamic != "" {
		blocks = append(blocks, map[string]any{
			"type": "text",
			"text": dynamic,
		})
	}
	return blocks
}

func splitAnthropicStableSystem(system string) (string, string) {
	index := strings.LastIndex(system, actionManifestAnchorPrefix)
	if index < 0 {
		return strings.TrimSpace(system), ""
	}
	return strings.TrimSpace(system[:index]), strings.TrimSpace(system[index:])
}

func anthropicPromptCacheEnabled() bool {
	value := strings.TrimSpace(os.Getenv("PERSEUS_ANTHROPIC_PROMPT_CACHE_ENABLED"))
	if value == "" {
		return true
	}
	switch strings.ToLower(value) {
	case "0", "false", "no", "off", "disabled":
		return false
	default:
		return true
	}
}
