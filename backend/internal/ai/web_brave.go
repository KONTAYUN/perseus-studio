package ai

import (
	"context"
	"fmt"
	"net/url"
)

func (s *Service) braveSearch(ctx context.Context, apiKey, query string, maxResults int) ([]WebSearchResult, error) {
	endpoint := "https://api.search.brave.com/res/v1/web/search?q=" + url.QueryEscape(query) + fmt.Sprintf("&count=%d", maxResults)
	headers := map[string]string{
		"X-Subscription-Token": apiKey,
		"Accept":               "application/json",
	}
	var data struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
				Age         string `json:"age"`
			} `json:"results"`
		} `json:"web"`
		Error any `json:"error"`
	}
	if err := s.getJSON(ctx, endpoint, headers, &data); err != nil {
		return nil, fmt.Errorf("brave search failed: %w", err)
	}
	if data.Error != nil {
		return nil, fmt.Errorf("brave search failed: %v", data.Error)
	}
	results := make([]WebSearchResult, 0, len(data.Web.Results))
	for _, item := range data.Web.Results {
		snippet := truncateRunes(item.Description, 1000)
		results = append(results, WebSearchResult{
			Title:         item.Title,
			URL:           item.URL,
			Snippet:       snippet,
			Content:       snippet,
			Provider:      "brave",
			PublishedDate: item.Age,
		})
	}
	return results, nil
}
