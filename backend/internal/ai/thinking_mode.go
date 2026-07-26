package ai

import "strings"

const (
	ThinkingModeLight    = "light"
	ThinkingModeStandard = "standard"
	ThinkingModeDeep     = "deep"
	ThinkingModeExpert   = "expert"
)

func normalizeThinkingMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ThinkingModeLight:
		return ThinkingModeLight
	case ThinkingModeDeep:
		return ThinkingModeDeep
	case ThinkingModeExpert:
		return ThinkingModeExpert
	default:
		return ThinkingModeStandard
	}
}
