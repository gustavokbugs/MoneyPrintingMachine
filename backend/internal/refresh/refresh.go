// Package refresh keeps trend data current: it re-measures watched queries
// and polls RSS feeds. The work runs as a durable background job; a ticker
// only enqueues that job, once per time slot, using an idempotency key so
// restarts and concurrent schedulers never duplicate a run.
package refresh

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"contentfactory/internal/jobs"
	"contentfactory/internal/rss"
	"contentfactory/internal/trends"
)

// KindTrendRefresh is the job kind for one refresh run.
const KindTrendRefresh = "trends.refresh"

// FeedsPerRun bounds feed fetches per run.
const FeedsPerRun = 20

// TrendRefresher re-measures due watches.
type TrendRefresher interface {
	RunOnce(ctx context.Context) (trends.RunReport, error)
}

// FeedPoller fetches due feeds.
type FeedPoller interface {
	FetchDue(ctx context.Context, limit int) (rss.FetchReport, error)
}

// JobPruner deletes old finished jobs.
type JobPruner interface {
	PruneFinished(ctx context.Context, before time.Time) (int64, error)
}

// JobRetention is how long finished jobs are kept.
const JobRetention = 30 * 24 * time.Hour

// Result is the job result.
type Result struct {
	Measured     int    `json:"measured"`
	Failed       int    `json:"failed"`
	Stopped      string `json:"stopped,omitempty"`
	FeedsFetched int    `json:"feedsFetched"`
	FeedsFailed  int    `json:"feedsFailed"`
	ItemsPruned  int64  `json:"itemsPruned"`
	JobsPruned   int64  `json:"jobsPruned"`
}

// Handler returns the job handler. Feeds are polled even when trend
// measurement fails, and vice versa; the job fails only if both fail.
// Pruning old jobs is best effort and never fails the run.
func Handler(tr TrendRefresher, feeds FeedPoller, pruner JobPruner) jobs.Handler {
	return func(ctx context.Context, _ jobs.Job) (any, error) {
		var res Result
		rep, trendErr := tr.RunOnce(ctx)
		res.Measured, res.Failed, res.Stopped = rep.Measured, rep.Failed, rep.Stopped
		frep, feedErr := feeds.FetchDue(ctx, FeedsPerRun)
		res.FeedsFetched, res.FeedsFailed, res.ItemsPruned = frep.Fetched, frep.Failed, frep.Pruned
		if pruner != nil {
			if n, err := pruner.PruneFinished(ctx, time.Now().Add(-JobRetention)); err == nil {
				res.JobsPruned = n
			}
		}
		if trendErr != nil && feedErr != nil {
			return nil, fmt.Errorf("trend refresh: %v; feed polling: %w", trendErr, feedErr)
		}
		if trendErr != nil {
			res.Stopped = "trend refresh failed: " + trendErr.Error()
		}
		if feedErr != nil && res.Stopped == "" {
			res.Stopped = "feed polling failed: " + feedErr.Error()
		}
		return res, nil
	}
}

// Enqueuer creates jobs.
type Enqueuer interface {
	Enqueue(ctx context.Context, j jobs.NewJob) (jobs.Job, error)
}

// SlotKey returns the idempotency key for the time slot containing t.
func SlotKey(t time.Time, interval time.Duration) string {
	return fmt.Sprintf("%s:%d", KindTrendRefresh, t.UTC().Truncate(interval).Unix())
}

// Schedule enqueues one refresh job per interval until ctx is canceled.
// The first job is enqueued immediately.
func Schedule(ctx context.Context, enq Enqueuer, ownerID string, interval time.Duration, log *slog.Logger) {
	enqueue := func() {
		key := SlotKey(time.Now(), interval)
		if _, err := enq.Enqueue(ctx, jobs.NewJob{OwnerID: ownerID, Kind: KindTrendRefresh, Input: map[string]string{}, IdempotencyKey: key, MaxAttempts: 2}); err != nil && ctx.Err() == nil {
			log.Error("enqueue trend refresh", "error", err)
		}
	}
	enqueue()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			enqueue()
		}
	}
}
