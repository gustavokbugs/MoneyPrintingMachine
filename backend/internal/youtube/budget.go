package youtube

import (
	"context"
	"fmt"
	"time"
)

// QuotaProvider names YouTube Data API usage in the quota table.
const QuotaProvider = "youtube_data_api"

// UsageStore records quota usage per provider and day.
type UsageStore interface {
	// Reserve adds units for the day if the total stays within limit and
	// reports whether the reservation was made.
	Reserve(ctx context.Context, provider string, day time.Time, units, limit int) (bool, error)
	Used(ctx context.Context, provider string, day time.Time) (int, error)
}

// DailyBudget enforces a local daily quota budget. YouTube quotas reset at
// midnight Pacific time, so days are computed in America/Los_Angeles.
type DailyBudget struct {
	store UsageStore
	limit int
	loc   *time.Location
	now   func() time.Time
}

// NewDailyBudget builds a DailyBudget with the given daily unit limit.
func NewDailyBudget(store UsageStore, limit int) (*DailyBudget, error) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return nil, fmt.Errorf("load Pacific time zone: %w", err)
	}
	return &DailyBudget{store: store, limit: limit, loc: loc, now: time.Now}, nil
}

func (b *DailyBudget) today() time.Time {
	t := b.now().In(b.loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// Reserve implements Quota.
func (b *DailyBudget) Reserve(ctx context.Context, units int) error {
	ok, err := b.store.Reserve(ctx, QuotaProvider, b.today(), units, b.limit)
	if err != nil {
		return fmt.Errorf("reserve quota: %w", err)
	}
	if !ok {
		return ErrQuotaBudget
	}
	return nil
}

// Usage reports today's usage and the limit.
func (b *DailyBudget) Usage(ctx context.Context) (used, limit int, err error) {
	used, err = b.store.Used(ctx, QuotaProvider, b.today())
	return used, b.limit, err
}
