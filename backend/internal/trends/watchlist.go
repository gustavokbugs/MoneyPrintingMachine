package trends

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"contentfactory/internal/youtube"
)

// Watch sources.
const (
	WatchSourceChannel = "channel"
	WatchSourceManual  = "manual"
)

// ErrWatchNotFound is returned when a watch does not exist for the owner.
var ErrWatchNotFound = errors.New("watched query not found")

// Watch is a query re-measured on a schedule.
type Watch struct {
	ID             int64      `json:"id"`
	OwnerID        string     `json:"-"`
	Query          string     `json:"query"`
	QueryKey       string     `json:"queryKey"`
	Region         string     `json:"region"`
	Language       string     `json:"language"`
	Source         string     `json:"source"`
	Active         bool       `json:"active"`
	NextDueAt      time.Time  `json:"nextDueAt"`
	LastMeasuredAt *time.Time `json:"lastMeasuredAt"`
	LastError      string     `json:"lastError"`
	CreatedAt      time.Time  `json:"createdAt"`
}

// WatchStore persists the watchlist.
type WatchStore interface {
	// SyncChannelNiches adds a watch for every non-archived channel niche
	// and deactivates channel watches whose niche is no longer used.
	SyncChannelNiches(ctx context.Context) error
	// ClaimDue leases up to limit due watches by pushing their due time
	// forward by lease, so a crashed run retries them later.
	ClaimDue(ctx context.Context, now time.Time, limit int, lease time.Duration) ([]Watch, error)
	MarkMeasured(ctx context.Context, id int64, at, next time.Time) error
	MarkFailed(ctx context.Context, id int64, msg string, retryAt time.Time) error
	List(ctx context.Context, ownerID string, limit int) ([]Watch, error)
	AddManual(ctx context.Context, w Watch) (Watch, error)
	Delete(ctx context.Context, ownerID string, id int64) error
}

// Measurer takes one measurement.
type Measurer interface {
	Available() bool
	Measure(ctx context.Context, r Request) (Result, error)
}

// Refresher re-measures due watches within the quota budget.
type Refresher struct {
	store    WatchStore
	measurer Measurer
	now      func() time.Time
	// Interval between measurements of the same query. It must exceed
	// MinMomentumGap so each refresh can compute momentum.
	Interval time.Duration
	// PerRun bounds measurements per run (about 102 quota units each).
	PerRun int
	Lease  time.Duration
}

// NewRefresher builds a Refresher with daily refreshes and at most five
// measurements per run.
func NewRefresher(store WatchStore, m Measurer) *Refresher {
	return &Refresher{store: store, measurer: m, now: time.Now, Interval: 24 * time.Hour, PerRun: 5, Lease: 15 * time.Minute}
}

// RunReport summarizes one refresh run.
type RunReport struct {
	Measured int
	Failed   int
	Stopped  string
}

// nextQuotaReset returns the next midnight Pacific time, when YouTube
// quotas reset, plus a small margin.
func nextQuotaReset(now time.Time) time.Time {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return now.Add(24 * time.Hour)
	}
	p := now.In(loc)
	midnight := time.Date(p.Year(), p.Month(), p.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	return midnight.Add(5 * time.Minute)
}

// RunOnce syncs channel niches into the watchlist and measures due watches.
func (r *Refresher) RunOnce(ctx context.Context) (RunReport, error) {
	var rep RunReport
	if err := r.store.SyncChannelNiches(ctx); err != nil {
		return rep, fmt.Errorf("sync channel niches: %w", err)
	}
	if r.measurer == nil || !r.measurer.Available() {
		rep.Stopped = "YouTube Data API is not configured"
		return rep, nil
	}
	now := r.now().UTC()
	due, err := r.store.ClaimDue(ctx, now, r.PerRun, r.Lease)
	if err != nil {
		return rep, fmt.Errorf("claim due watches: %w", err)
	}
	for i, w := range due {
		if ctx.Err() != nil {
			return rep, ctx.Err()
		}
		_, err := r.measurer.Measure(ctx, Request{OwnerID: w.OwnerID, Query: w.Query, Region: w.Region, Language: w.Language})
		switch {
		case err == nil:
			rep.Measured++
			if err := r.store.MarkMeasured(ctx, w.ID, now, now.Add(r.Interval)); err != nil {
				return rep, fmt.Errorf("mark measured: %w", err)
			}
		case errors.Is(err, youtube.ErrQuotaBudget) || errors.Is(err, youtube.ErrQuotaExceeded):
			// Defer this and all remaining claimed watches to the reset.
			reset := nextQuotaReset(now)
			for _, rest := range due[i:] {
				_ = r.store.MarkFailed(ctx, rest.ID, "Daily YouTube quota reached; retrying after the reset.", reset)
			}
			rep.Stopped = "daily YouTube quota reached"
			return rep, nil
		default:
			rep.Failed++
			msg := err.Error()
			if len(msg) > 500 {
				msg = msg[:500]
			}
			if err := r.store.MarkFailed(ctx, w.ID, msg, now.Add(time.Hour)); err != nil {
				return rep, fmt.Errorf("mark failed: %w", err)
			}
		}
	}
	return rep, nil
}

// ValidateWatch normalizes and validates a manual watch.
func ValidateWatch(w Watch) (Watch, error) {
	w.Query = strings.Join(strings.Fields(w.Query), " ")
	if w.Query == "" || len(w.Query) > 200 {
		return Watch{}, ErrInvalidQuery
	}
	w.QueryKey = QueryKey(w.Query)
	w.Region = strings.ToUpper(strings.TrimSpace(w.Region))
	if w.Region == "" {
		w.Region = "US"
	}
	if len(w.Region) != 2 {
		return Watch{}, fmt.Errorf("%w: region must be a two-letter country code", ErrInvalidQuery)
	}
	if w.Language == "" {
		w.Language = "en-US"
	}
	w.Source = WatchSourceManual
	return w, nil
}
