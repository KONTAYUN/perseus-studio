package store

// NormalizeSystemMessageMode defaults to the most portable OpenAI message layout.
func NormalizeSystemMessageMode(mode string) string {
	if mode == "multiple" {
		return mode
	}
	return "single"
}
