package youtube

import (
	"context"
	"errors"
	"testing"
	"time"
)

type memUsage map[string]int

func (m memUsage) Reserve(_ context.Context, p string, day time.Time, units, limit int) (bool, error) {
	k := p + day.Format("2006-01-02")
	if m[k]+units > limit {
		return false, nil
	}
	m[k] += units
	return true, nil
}

func (m memUsage) Used(_ context.Context, p string, day time.Time) (int, error) {
	return m[p+day.Format("2006-01-02")], nil
}

func TestDailyBudget(t *testing.T) {
	store := memUsage{}
	b, err := NewDailyBudget(store, 150)
	if err != nil {
		t.Fatal(err)
	}
	// 2026-11-11 05:00 UTC is still 2026-11-10 in Pacific time.
	b.now = func() time.Time { return time.Date(2026, 11, 11, 5, 0, 0, 0, time.UTC) }
	ctx := context.Background()
	if err := b.Reserve(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if err := b.Reserve(ctx, 100); !errors.Is(err, ErrQuotaBudget) {
		t.Fatalf("expected budget error, got %v", err)
	}
	if used, limit, _ := b.Usage(ctx); used != 100 || limit != 150 {
		t.Fatalf("usage = %d/%d", used, limit)
	}
	if store["youtube_data_api2026-11-10"] != 100 {
		t.Fatalf("usage recorded on wrong day: %v", store)
	}
	// After midnight Pacific the budget resets.
	b.now = func() time.Time { return time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC) }
	if err := b.Reserve(ctx, 100); err != nil {
		t.Fatalf("expected new day to have budget, got %v", err)
	}
}
