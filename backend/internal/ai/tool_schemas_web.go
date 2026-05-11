package ai

func openAIWebTools() []map[string]any {
	return []map[string]any{
		openAIStrictFunctionTool("perseus_search_web", "Search the web using the configured Perseus search provider and return relevant results with URLs. Use for current or external information. Cite source URLs when using results.", webSearchParams()),
		openAIStrictFunctionTool("web_extract", "Extract readable text from one HTTP/HTTPS URL when a search result needs more detail. Use only after search or when the user provides a URL. Some providers may not support extraction.", webExtractParams()),
	}
}

func anthropicWebTools() []map[string]any {
	return []map[string]any{
		anthropicTool("perseus_search_web", "Search the web using the configured Perseus search provider and return relevant results with URLs. Use for current or external information. Cite source URLs when using results.", webSearchParams()),
		anthropicTool("web_extract", "Extract readable text from one HTTP/HTTPS URL when a search result needs more detail. Use only after search or when the user provides a URL. Some providers may not support extraction.", webExtractParams()),
	}
}

func webSearchParams() map[string]any {
	return map[string]any{
		"type":                 "object",
		"required":             []string{"query", "max_results"},
		"additionalProperties": false,
		"properties": map[string]any{
			"query":       map[string]any{"type": "string", "description": "Search query."},
			"max_results": map[string]any{"type": "integer", "description": "Maximum results to return, from 1 to 10.", "default": 5},
		},
	}
}

func webExtractParams() map[string]any {
	return map[string]any{
		"type":                 "object",
		"required":             []string{"url"},
		"additionalProperties": false,
		"properties": map[string]any{
			"url": map[string]any{"type": "string", "description": "HTTP or HTTPS URL to extract."},
		},
	}
}
