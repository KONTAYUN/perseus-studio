package store

import (
	"context"
	"database/sql"
	"strings"
)

func (s *UsageStore) Summary(ctx context.Context, query UsageQuery) (UsageSummary, error) {
	filters, args := usageFilters(query)
	where := ""
	if len(filters) > 0 {
		where = "WHERE " + strings.Join(filters, " AND ")
	}
	userFilters, userArgs := usageFiltersWithPrefix(query, "e")
	userWhere := ""
	if len(userFilters) > 0 {
		userWhere = "WHERE " + strings.Join(userFilters, " AND ")
	}
	totals, err := s.aggregateRow(ctx, `
SELECT COUNT(*),
  SUM(CASE WHEN status = 'succeeded' THEN 1 ELSE 0 END),
  SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END),
  COALESCE(SUM(input_tokens), 0),
  COALESCE(SUM(output_tokens), 0),
  COALESCE(SUM(cache_creation_tokens), 0),
  COALESCE(SUM(cache_read_tokens), 0),
  COALESCE(SUM(total_tokens), 0),
  COALESCE(SUM(estimated_cost_micro), 0),
  CASE WHEN COUNT(DISTINCT NULLIF(currency, '')) = 1 THEN MAX(NULLIF(currency, '')) ELSE '' END
FROM llm_usage_events `+where, args...)
	if err != nil {
		return UsageSummary{}, err
	}
	byUser, err := s.aggregateByUser(ctx, userWhere, userArgs)
	if err != nil {
		return UsageSummary{}, err
	}
	byModel, err := s.aggregateByModel(ctx, where, args)
	if err != nil {
		return UsageSummary{}, err
	}
	return UsageSummary{Totals: totals, ByUser: byUser, ByModel: byModel}, nil
}

func (s *UsageStore) aggregateRow(ctx context.Context, query string, args ...any) (UsageAggregate, error) {
	row := s.db.QueryRowContext(ctx, query, args...)
	var item UsageAggregate
	var currency sql.NullString
	err := row.Scan(&item.RequestCount, &item.SuccessCount, &item.FailedCount, &item.InputTokens, &item.OutputTokens,
		&item.CacheCreationTokens, &item.CacheReadTokens, &item.TotalTokens, &item.EstimatedCostMicro, &currency)
	if currency.Valid {
		item.Currency = currency.String
	}
	return item, err
}

func (s *UsageStore) aggregateByUser(ctx context.Context, where string, args []any) ([]UsageUserAggregate, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT e.user_id, COALESCE(u.username, ''), COALESCE(u.role, ''), COALESCE(u.disabled, 0),
  COUNT(*) AS request_count,
  SUM(CASE WHEN e.status = 'succeeded' THEN 1 ELSE 0 END) AS success_count,
  SUM(CASE WHEN e.status = 'failed' THEN 1 ELSE 0 END) AS failed_count,
  COALESCE(SUM(e.input_tokens), 0),
  COALESCE(SUM(e.output_tokens), 0),
  COALESCE(SUM(e.cache_creation_tokens), 0),
  COALESCE(SUM(e.cache_read_tokens), 0),
  COALESCE(SUM(e.total_tokens), 0) AS total_tokens,
  COALESCE(SUM(e.estimated_cost_micro), 0) AS estimated_cost_micro,
  CASE WHEN COUNT(DISTINCT NULLIF(e.currency, '')) = 1 THEN MAX(NULLIF(e.currency, '')) ELSE '' END
FROM llm_usage_events e
LEFT JOIN users u ON u.id = e.user_id
`+where+`
GROUP BY e.user_id
ORDER BY total_tokens DESC, request_count DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []UsageUserAggregate
	for rows.Next() {
		var item UsageUserAggregate
		var disabled int
		var currency sql.NullString
		err := rows.Scan(&item.UserID, &item.Username, &item.Role, &disabled, &item.RequestCount, &item.SuccessCount, &item.FailedCount,
			&item.InputTokens, &item.OutputTokens, &item.CacheCreationTokens, &item.CacheReadTokens, &item.TotalTokens, &item.EstimatedCostMicro, &currency)
		if err != nil {
			return nil, err
		}
		item.Disabled = disabled != 0
		if currency.Valid {
			item.Currency = currency.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *UsageStore) aggregateByModel(ctx context.Context, where string, args []any) ([]UsageModelAggregate, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT model_provider_id, provider_name, protocol, model_id,
  COUNT(*) AS request_count,
  SUM(CASE WHEN status = 'succeeded' THEN 1 ELSE 0 END) AS success_count,
  SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) AS failed_count,
  COALESCE(SUM(input_tokens), 0),
  COALESCE(SUM(output_tokens), 0),
  COALESCE(SUM(cache_creation_tokens), 0),
  COALESCE(SUM(cache_read_tokens), 0),
  COALESCE(SUM(total_tokens), 0) AS total_tokens,
  COALESCE(SUM(estimated_cost_micro), 0) AS estimated_cost_micro,
  CASE WHEN COUNT(DISTINCT NULLIF(currency, '')) = 1 THEN MAX(NULLIF(currency, '')) ELSE '' END
FROM llm_usage_events
`+where+`
GROUP BY model_provider_id, model_id
ORDER BY total_tokens DESC, request_count DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []UsageModelAggregate
	for rows.Next() {
		var item UsageModelAggregate
		var currency sql.NullString
		err := rows.Scan(&item.ModelProviderID, &item.ProviderName, &item.Protocol, &item.ModelID,
			&item.RequestCount, &item.SuccessCount, &item.FailedCount,
			&item.InputTokens, &item.OutputTokens, &item.CacheCreationTokens, &item.CacheReadTokens, &item.TotalTokens, &item.EstimatedCostMicro, &currency)
		if err != nil {
			return nil, err
		}
		if currency.Valid {
			item.Currency = currency.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func usageFilters(query UsageQuery) ([]string, []any) {
	return usageFiltersWithPrefix(query, "")
}

func usageFiltersWithPrefix(query UsageQuery, prefix string) ([]string, []any) {
	if strings.TrimSpace(prefix) != "" && !strings.HasSuffix(prefix, ".") {
		prefix += "."
	}
	var filters []string
	var args []any
	if !query.From.IsZero() {
		filters = append(filters, prefix+"created_at >= ?")
		args = append(args, query.From.UTC())
	}
	if !query.To.IsZero() {
		filters = append(filters, prefix+"created_at < ?")
		args = append(args, query.To.UTC())
	}
	if strings.TrimSpace(query.UserID) != "" {
		filters = append(filters, prefix+"user_id = ?")
		args = append(args, strings.TrimSpace(query.UserID))
	}
	if strings.TrimSpace(query.ModelProviderID) != "" {
		filters = append(filters, prefix+"model_provider_id = ?")
		args = append(args, strings.TrimSpace(query.ModelProviderID))
	}
	if strings.TrimSpace(query.ModelID) != "" {
		filters = append(filters, prefix+"model_id = ?")
		args = append(args, strings.TrimSpace(query.ModelID))
	}
	if strings.TrimSpace(query.Stage) != "" {
		filters = append(filters, prefix+"stage = ?")
		args = append(args, strings.TrimSpace(query.Stage))
	}
	return filters, args
}
