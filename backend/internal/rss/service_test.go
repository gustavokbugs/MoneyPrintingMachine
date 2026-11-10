package rss

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type memStore struct {
	feeds    []FeedRecord
	outcomes map[int64]FetchOutcome
	items    []Item
	pruned   time.Time
}

func (m *memStore) Add(_ context.Context, owner, url string) (FeedRecord, error) {
	for _, f := range m.feeds {
		if f.OwnerID == owner && f.URL == url {
			return FeedRecord{}, ErrFeedExists
		}
	}
	f := FeedRecord{ID: int64(len(m.feeds) + 1), OwnerID: owner, URL: url, Active: true}
	m.feeds = append(m.feeds, f)
	return f, nil
}
func (m *memStore) Count(_ context.Context, owner string) (int, error) {
	n := 0
	for _, f := range m.feeds {
		if f.OwnerID == owner {
			n++
		}
	}
	return n, nil
}
func (m *memStore) List(context.Context, string) ([]FeedRecord, error) { return m.feeds, nil }
func (m *memStore) Delete(context.Context, string, int64) error        { return nil }
func (m *memStore) ClaimDue(context.Context, time.Time, int, time.Duration) ([]FeedRecord, error) {
	return m.feeds, nil
}
func (m *memStore) SaveOutcome(_ context.Context, f FeedRecord, o FetchOutcome) error {
	m.outcomes[f.ID] = o
	m.items = append(m.items, o.Items...)
	return nil
}
func (m *memStore) CountMentions(_ context.Context, _ string, tokens []string, from, to time.Time) (int, error) {
	n := 0
	for _, it := range m.items {
		if it.PublishedAt.Before(from) || !it.PublishedAt.Before(to) {
			continue
		}
		all := true
		for _, tok := range tokens {
			if !strings.Contains(strings.ToLower(it.Title), tok) {
				all = false
			}
		}
		if all {
			n++
		}
	}
	return n, nil
}
func (m *memStore) CountActiveFeeds(context.Context, string) (int, error) { return len(m.feeds), nil }
func (m *memStore) PruneItems(_ context.Context, before time.Time) (int64, error) {
	m.pruned = before
	return 0, nil
}

type fakeFetcher map[string]struct {
	res FetchResult
	err error
}

func (f fakeFetcher) Fetch(_ context.Context, url, _, _ string) (FetchResult, error) {
	r := f[url]
	return r.res, r.err
}

var now = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

func item(title string, daysAgo int) Item {
	return Item{GUIDHash: title, Title: title, PublishedAt: now.AddDate(0, 0, -daysAgo)}
}

func TestFetchDueAndMentions(t *testing.T) {
	store := &memStore{outcomes: map[int64]FetchOutcome{}}
	svc := NewService(store, fakeFetcher{
		"https://ok.example/feed": {res: FetchResult{ETag: "e1", Feed: &Feed{Title: "OK", Items: []Item{
			item("Deep sea mining news", 1), item("Deep sea creature found", 3), item("Unrelated", 2),
			item("Deep sea expedition", 10), {GUIDHash: "f", Title: "Deep sea future", PublishedAt: now.Add(48 * time.Hour)},
		}}}},
		"https://same.example/feed": {res: FetchResult{NotModified: true}},
		"https://down.example/feed": {err: errors.New("HTTP 503")},
	})
	svc.now = func() time.Time { return now }
	for _, u := range []string{"https://ok.example/feed", "https://same.example/feed", "https://down.example/feed"} {
		if _, err := svc.AddFeed(context.Background(), "alice", u); err != nil {
			t.Fatal(err)
		}
	}

	rep, err := svc.FetchDue(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Fetched != 2 || rep.Failed != 1 {
		t.Fatalf("report: %+v", rep)
	}
	if o := store.outcomes[1]; o.Status != "ok" || o.ETag != "e1" || o.Title != "OK" || len(o.Items) != 4 {
		t.Fatalf("ok outcome: %+v", o)
	}
	if o := store.outcomes[3]; o.Status != "error" || !o.NextFetchAt.Equal(now.Add(2*time.Hour)) {
		t.Fatalf("error outcome must back off: %+v", o)
	}
	if !store.pruned.Equal(now.Add(-60 * 24 * time.Hour)) {
		t.Fatalf("prune cutoff: %v", store.pruned)
	}

	st, err := svc.Mentions(context.Background(), "alice", "Deep Sea")
	if err != nil {
		t.Fatal(err)
	}
	if st.Recent7d != 2 || st.Previous7d != 1 || st.MomentumPercent == nil || *st.MomentumPercent != 100 || st.ActiveFeeds != 3 {
		t.Fatalf("mentions: %+v", st)
	}
	st, _ = svc.Mentions(context.Background(), "alice", "mining")
	if st.Recent7d != 1 || st.MomentumPercent != nil {
		t.Fatalf("momentum without a baseline must be nil: %+v", st)
	}
}

func TestAddFeedValidation(t *testing.T) {
	store := &memStore{outcomes: map[int64]FetchOutcome{}}
	svc := NewService(store, fakeFetcher{})
	if _, err := svc.AddFeed(context.Background(), "alice", "file:///etc/passwd"); !errors.Is(err, ErrInvalidURL) {
		t.Fatalf("expected invalid URL, got %v", err)
	}
	for i := range MaxFeedsPerOwner {
		store.feeds = append(store.feeds, FeedRecord{ID: int64(i + 1), OwnerID: "alice"})
	}
	if _, err := svc.AddFeed(context.Background(), "alice", "https://x.example/feed"); !errors.Is(err, ErrTooManyFeeds) {
		t.Fatalf("expected feed limit, got %v", err)
	}
}

func TestMentionsWithoutFeeds(t *testing.T) {
	svc := NewService(&memStore{outcomes: map[int64]FetchOutcome{}}, fakeFetcher{})
	st, err := svc.Mentions(context.Background(), "alice", "anything")
	if err != nil || st.ActiveFeeds != 0 || st.Recent7d != 0 {
		t.Fatalf("unexpected: %+v %v", st, err)
	}
}
