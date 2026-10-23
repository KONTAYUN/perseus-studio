package ai

import (
	"context"
	"sync/atomic"
	"time"

	"webide-backend/internal/failure"
)

type runRequestBudgetKey struct{}
type runRequestBudget struct {
	limit int64
	used  atomic.Int64
}

func withRunRequestBudget(ctx context.Context, opts RunOptions) (context.Context, context.CancelFunc) {
	limit := opts.MaxProviderRequests
	if limit <= 0 {
		limit = envInt("PERSEUS_RUN_MAX_PROVIDER_REQUESTS", 192)
		if opts.TaskID != "" {
			limit = 384
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 2048 {
		limit = 2048
	}
	duration := opts.MaxRunDuration
	if duration <= 0 {
		duration = 20 * time.Minute
		if opts.TaskID != "" {
			duration = 2 * time.Hour
		}
	}
	ctx = context.WithValue(ctx, runRequestBudgetKey{}, &runRequestBudget{limit: int64(limit)})
	return context.WithTimeoutCause(ctx, duration, failure.New("run_deadline_exceeded", "本轮执行时间预算已耗尽，请核验已有结果后继续", context.DeadlineExceeded))
}

func admitProviderRequest(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return context.Cause(ctx)
	}
	budget, _ := ctx.Value(runRequestBudgetKey{}).(*runRequestBudget)
	if budget != nil && budget.used.Add(1) > budget.limit {
		return failure.New("run_request_budget_exhausted", "本轮模型请求预算已耗尽；不会继续重试或回退，请核验已有结果后继续", nil)
	}
	return nil
}
