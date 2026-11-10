package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"contentfactory/internal/channel"
	"contentfactory/internal/jobs"
	"contentfactory/internal/rss"
	"contentfactory/internal/trends"
)

func TestWatchlistSyncAndClaim(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	chRepo := NewChannelRepository(pool)
	a, _ := channel.NewDraft("alice", "  Deep   Sea ", "", "")
	b, _ := channel.NewDraft("alice", "deep sea", "", "") // same key: one watch
	c, _ := channel.NewDraft("alice", "Space", "", "")
	for _, ch := range []*channel.Channel{a, b, c} {
		if err := chRepo.Create(ctx, ch); err != nil {
			t.Fatal(err)
		}
	}
	w := NewWatchlistRepository(pool)
	if err := w.SyncChannelNiches(ctx); err != nil {
		t.Fatal(err)
	}
	list, err := w.List(ctx, "alice", 50)
	if err != nil || len(list) != 2 {
		t.Fatalf("expected 2 watches, got %d %v", len(list), err)
	}
	for _, it := range list {
		if it.Source != trends.WatchSourceChannel || !it.Active {
			t.Fatalf("unexpected watch %+v", it)
		}
	}

	// Archiving the only "space" channel deactivates its watch.
	c.Status = channel.StatusArchived
	if err := chRepo.Save(ctx, c, nil); err != nil {
		t.Fatal(err)
	}
	if err := w.SyncChannelNiches(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(time.Minute)
	claimed, err := w.ClaimDue(ctx, now, 10, time.Hour)
	if err != nil || len(claimed) != 1 || claimed[0].QueryKey != "deep sea" {
		t.Fatalf("claim: %+v %v", claimed, err)
	}
	if again, _ := w.ClaimDue(ctx, now, 10, time.Hour); len(again) != 0 {
		t.Fatal("a leased watch must not be claimed twice")
	}
	if err := w.MarkMeasured(ctx, claimed[0].ID, now, now.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}

	manual, err := w.AddManual(ctx, trends.Watch{OwnerID: "alice", Query: "Volcanoes", QueryKey: "volcanoes", Region: "US", Language: "en-US"})
	if err != nil || manual.Source != trends.WatchSourceManual {
		t.Fatalf("add manual: %+v %v", manual, err)
	}
	if err := w.Delete(ctx, "bob", manual.ID); !errors.Is(err, trends.ErrWatchNotFound) {
		t.Fatalf("other owner delete: %v", err)
	}
	if err := w.Delete(ctx, "alice", claimed[0].ID); !errors.Is(err, trends.ErrWatchNotFound) {
		t.Fatalf("channel watches must not be deletable: %v", err)
	}
	if err := w.Delete(ctx, "alice", manual.ID); err != nil {
		t.Fatal(err)
	}
}

func TestFeedRepository(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	r := NewFeedRepository(pool)
	f, err := r.Add(ctx, "alice", "https://example.org/feed")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Add(ctx, "alice", "https://example.org/feed"); !errors.Is(err, rss.ErrFeedExists) {
		t.Fatalf("duplicate: %v", err)
	}
	now := time.Now().UTC()
	claimed, err := r.ClaimDue(ctx, now.Add(time.Minute), 10, time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: %+v %v", claimed, err)
	}
	items := []rss.Item{
		{GUIDHash: "1", Title: "Deep Sea mining news", PublishedAt: now.Add(-24 * time.Hour)},
		{GUIDHash: "2", Title: "Deep sea creature", PublishedAt: now.Add(-9 * 24 * time.Hour)},
		{GUIDHash: "3", Title: "Old story about the deep sea", PublishedAt: now.Add(-90 * 24 * time.Hour)},
	}
	o := rss.FetchOutcome{Status: "ok", Title: "Example", ETag: "e", FetchedAt: now, NextFetchAt: now.Add(30 * time.Minute), Items: items}
	if err := r.SaveOutcome(ctx, claimed[0], o); err != nil {
		t.Fatal(err)
	}
	if err := r.SaveOutcome(ctx, claimed[0], o); err != nil {
		t.Fatal("saving the same items twice must be idempotent:", err)
	}
	list, err := r.List(ctx, "alice")
	if err != nil || len(list) != 1 || list[0].ItemCount != 3 || list[0].Title != "Example" || list[0].LastStatus != "ok" {
		t.Fatalf("list: %+v %v", list, err)
	}
	if n, _ := r.CountActiveFeeds(ctx, "alice"); n != 1 {
		t.Fatalf("active feeds = %d", n)
	}
	n, err := r.CountMentions(ctx, "alice", []string{"deep", "sea"}, now.Add(-7*24*time.Hour), now.Add(time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("mentions recent = %d %v", n, err)
	}
	if n, _ := r.CountMentions(ctx, "bob", []string{"deep"}, now.Add(-365*24*time.Hour), now.Add(time.Hour)); n != 0 {
		t.Fatal("mentions must be owner-scoped")
	}
	if pruned, _ := r.PruneItems(ctx, now.Add(-60*24*time.Hour)); pruned != 1 {
		t.Fatalf("pruned = %d", pruned)
	}
	if err := r.Delete(ctx, "bob", f.ID); !errors.Is(err, rss.ErrFeedNotFound) {
		t.Fatalf("other owner delete: %v", err)
	}
	if err := r.Delete(ctx, "alice", f.ID); err != nil {
		t.Fatal(err)
	}
}

func TestPruneFinishedJobs(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	store := NewJobStore(pool)
	enq := jobs.NewEnqueuer(store)
	old, _ := enq.Enqueue(ctx, jobs.NewJob{OwnerID: "alice", Kind: "k"})
	queued, _ := enq.Enqueue(ctx, jobs.NewJob{OwnerID: "alice", Kind: "k"})
	if err := store.Complete(ctx, old.ID, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET updated_at = now() - interval '40 days'`); err != nil {
		t.Fatal(err)
	}
	n, err := store.PruneFinished(ctx, time.Now().Add(-30*24*time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("pruned %d, err %v", n, err)
	}
	if _, err := store.Get(ctx, "alice", queued.ID); err != nil {
		t.Fatal("queued jobs must never be pruned")
	}
}
