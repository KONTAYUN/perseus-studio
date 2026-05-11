package ai

import (
	"context"
	"fmt"
)

func (s *Service) tavilySearch(ctx context.Context, apiKey, query string, maxResults int) ([]WebSearchResult, error) {
	payload := map[string]any{
		"api_key":     apiKey,
		"query":       query,
		"max_results": maxResults,
	}
	var data struct {
		Results []struct {
			Title         string `json:"title"`
			URL           string `json:"url"`
			Content       string `json:"content"`
			RawContent    string `json:"raw_content"`
			PublishedDate string `json:"published_date"`
		} `json:"results"`
		Error string `json:"error"`
	}
	if err := s.postJSON(ctx, "https://api.tavily.com/search", payload, nil, &data); err != nil {
		return nil, fmt.Errorf("tavily search failed: %w", err)
	}
	if data.Error != "" {
		return nil, fmt.Errorf("tavily search failed: %s", data.Error)
	}
	results := make([]WebSearchResult, 0, len(data.Results))
	for _, item := range data.Results {
		content := truncateRunes(firstNonEmpty(item.Content, item.RawContent), 1000)
		results = append(results, WebSearchResult{
			Title:         item.Title,
			URL:           item.URL,
			Snippet:       content,
			Content:       content,
			Provider:      "tavily",
			PublishedDate: item.PublishedDate,
		})
	}
	return results, nil
}

func (s *Service) tavilyExtract(ctx context.Context, apiKey, rawURL string) (WebExtractResult, error) {
	payload := map[string]any{
		"api_key": apiKey,
		"urls":    []string{rawURL},
	}
	var data struct {
		Results []struct {
			URL        string `json:"url"`
			Title      string `json:"title"`
			Content    string `json:"content"`
			RawContent string `json:"raw_content"`
		} `json:"results"`
		Error string `json:"error"`
	}
	if err := s.postJSON(ctx, "https://api.tavily.com/extract", payload, nil, &data); err != nil {
		return WebExtractResult{}, fmt.Errorf("tavily extract failed: %w", err)
	}
	if data.Error != "" {
		return WebExtractResult{}, fmt.Errorf("tavily extract failed: %s", data.Error)
	}
	if len(data.Results) == 0 {
		return WebExtractResult{}, fmt.Errorf("tavily extract returned no content")
	}
	content, truncated := truncateRunesWithFlag(firstNonEmpty(data.Results[0].Content, data.Results[0].RawContent), 20000)
	return WebExtractResult{
		URL:       firstNonEmpty(data.Results[0].URL, rawURL),
		Title:     data.Results[0].Title,
		Content:   content,
		Truncated: truncated,
		Provider:  "tavily",
	}, nil
}
