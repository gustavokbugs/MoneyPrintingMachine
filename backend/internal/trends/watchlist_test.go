package trends

import (
	"context"
	"errors"
	"testing"
	"time"

	"contentfactory/internal/youtube"
)

type memWatches struct {
	watches  []Watch
	synced   int
	measured map[int64]time.Time
	failed   map[int64]time.Time
}

func (m *memWatches) SyncChannelNiches(context.Context) error { m.synced++; return nil }
func (m *memWatches) ClaimDue(_ context.Context, now time.Time, limit int, lease time.Duration) ([]Watch, error) {
	var out []Watch
	for i := range m.watches {
		if len(out) == limit {
			break
		}
		if m.watches[i].Active && !m.watches[i].NextDueAt.After(now) {
			m.watches[i].NextDueAt = now.Add(lease)
			out = append(out, m.watches[i])
		}
	}
	return out, nil
}
func (m *memWatches) MarkMeasured(_ context.Context, id int64, _, next time.Time) error {
	m.measured[id] = next
	return nil
}
func (m *memWatches) MarkFailed(_ context.Context, id int64, _ string, retry time.Time) error {
	m.failed[id] = retry
	return nil
}
func (m *memWatches) List(context.Context, string, int) ([]Watch, error)  { return m.watches, nil }
func (m *memWatches) AddManual(_ context.Context, w Watch) (Watch, error) { return w, nil }
func (m *memWatches) Delete(context.Context, string, int64) error         { return nil }

type scriptedMeasurer struct {
	available bool
	errs      map[string]error
	calls     []string
}

func (s *scriptedMeasurer) Available() bool { return s.available }
func (s *scriptedMeasurer) Measure(_ context.Context, r Request) (Result, error) {
	s.calls = append(s.calls, r.Query)
	return Result{}, s.errs[r.Query]
}

func watchesFixture(now time.Time) *memWatches {
	return &memWatches{
		measured: map[int64]time.Time{}, failed: map[int64]time.Time{},
		watches: []Watch{
			{ID: 1, OwnerID: "alice", Query: "space", Active: true, NextDueAt: now.Add(-time.Hour)},
			{ID: 2, OwnerID: "alice", Query: "broken", Active: true, NextDueAt: now.Add(-time.Hour)},
			{ID: 3, OwnerID: "alice", Query: "quota", Active: true, NextDueAt: now.Add(-time.Hour)},
			{ID: 4, OwnerID: "alice", Query: "later", Active: true, NextDueAt: now.Add(-time.Hour)},
			{ID: 5, OwnerID: "alice", Query: "future", Active: true, NextDueAt: now.Add(time.Hour)},
			{ID: 6, OwnerID: "alice", Query: "inactive", Active: false, NextDueAt: now.Add(-time.Hour)},
		},
	}
}

func TestRefresherRunOnce(t *testing.T) {
	now := time.Date(2026, 11, 10, 20, 0, 0, 0, time.UTC) // 12:00 Pacific
	store := watchesFixture(now)
	m := &scriptedMeasurer{available: true, errs: map[string]error{
		"broken": errors.New("timeout"),
		"quota":  youtube.ErrQuotaBudget,
	}}
	r := NewRefresher(store, m)
	r.now = func() time.Time { return now }

	rep, err := r.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if store.synced != 1 {
		t.Fatal("channel niches must be synced every run")
	}
	if rep.Measured != 1 || rep.Failed != 1 || rep.Stopped == "" {
		t.Fatalf("unexpected report: %+v", rep)
	}
	if got := store.measured[1]; !got.Equal(now.Add(24 * time.Hour)) {
		t.Fatalf("next due = %v", got)
	}
	if got := store.failed[2]; !got.Equal(now.Add(time.Hour)) {
		t.Fatalf("failed retry = %v", got)
	}
	// Quota exhaustion defers this and the remaining claimed watch to the
	// next Pacific midnight (2026-11-11 08:00 UTC) plus 5 minutes.
	reset := time.Date(2026, 11, 11, 8, 5, 0, 0, time.UTC)
	if !store.failed[3].Equal(reset) || !store.failed[4].Equal(reset) {
		t.Fatalf("quota deferral: %v %v", store.failed[3], store.failed[4])
	}
	for _, q := range m.calls {
		if q == "later" || q == "future" || q == "inactive" {
			t.Fatalf("unexpected measurement of %q", q)
		}
	}
}

func TestRefresherWithoutAPIKey(t *testing.T) {
	store := watchesFixture(time.Now())
	rep, err := NewRefresher(store, &scriptedMeasurer{available: false}).RunOnce(context.Background())
	if err != nil || rep.Stopped == "" || store.synced != 1 {
		t.Fatalf("rep=%+v err=%v synced=%d", rep, err, store.synced)
	}
}

func TestValidateWatch(t *testing.T) {
	w, err := ValidateWatch(Watch{Query: "  Deep   Sea ", Region: "gb"})
	if err != nil || w.Query != "Deep Sea" || w.QueryKey != "deep sea" || w.Region != "GB" || w.Language != "en-US" || w.Source != WatchSourceManual {
		t.Fatalf("unexpected: %+v %v", w, err)
	}
	for _, bad := range []Watch{{Query: ""}, {Query: "x", Region: "USA"}} {
		if _, err := ValidateWatch(bad); !errors.Is(err, ErrInvalidQuery) {
			t.Errorf("%+v: expected invalid, got %v", bad, err)
		}
	}
}
