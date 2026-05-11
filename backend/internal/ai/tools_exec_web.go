package ai

import (
	"context"
	"encoding/json"
	"fmt"
)

func (s *Service) execWebSearch(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	var args struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在联网搜索...", "", fmt.Errorf("invalid tool arguments")
	}
	results, provider, err := s.webSearch(ctx, userID, args.Query, args.MaxResults)
	if err != nil {
		return "", "", "正在联网搜索...", "", err
	}
	payload := map[string]any{"provider": provider, "query": args.Query, "results": results}
	summary := fmt.Sprintf("web search provider=%s query=%s results=%d", provider, args.Query, len(results))
	return mustJSON(payload), summary, "正在联网搜索...", "", nil
}

func (s *Service) execWebExtract(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在读取网页...", "", fmt.Errorf("invalid tool arguments")
	}
	result, err := s.webExtract(ctx, userID, args.URL)
	if err != nil {
		return "", "", "正在读取网页...", args.URL, err
	}
	summary := fmt.Sprintf("web extract provider=%s url=%s chars=%d truncated=%t", result.Provider, result.URL, len([]rune(result.Content)), result.Truncated)
	return mustJSON(result), summary, "正在读取网页...", result.URL, nil
}
