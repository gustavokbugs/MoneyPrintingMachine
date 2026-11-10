package refresh

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"contentfactory/internal/jobs"
	"contentfactory/internal/rss"
	"contentfactory/internal/trends"
)

type fakeTrends struct {
	rep trends.RunReport
	err error
}

func (f fakeTrends) RunOnce(context.Context) (trends.RunReport, error) { return f.rep, f.err }

type fakeFeeds struct {
	rep rss.FetchReport
	err error
}

func (f fakeFeeds) FetchDue(context.Context, int) (rss.FetchReport, error) { return f.rep, f.err }

type fakePruner struct{ before time.Time }

func (f *fakePruner) PruneFinished(_ context.Context, before time.Time) (int64, error) {
	f.before = before
	return 4, nil
}

func TestHandler(t *testing.T) {
	pruner := &fakePruner{}
	ok := Handler(fakeTrends{rep: trends.RunReport{Measured: 2}}, fakeFeeds{rep: rss.FetchReport{Fetched: 3, Pruned: 7}}, pruner)
	res, err := ok(context.Background(), jobs.Job{})
	if err != nil {
		t.Fatal(err)
	}
	if r := res.(Result); r.Measured != 2 || r.FeedsFetched != 3 || r.ItemsPruned != 7 || r.JobsPruned != 4 || r.Stopped != "" {
		t.Fatalf("unexpected result: %+v", r)
	}

	if d := time.Since(pruner.before); d < JobRetention || d > JobRetention+time.Minute {
		t.Fatalf("prune cutoff off by %v", d-JobRetention)
	}

	partial := Handler(fakeTrends{err: errors.New("db")}, fakeFeeds{rep: rss.FetchReport{Fetched: 1}}, nil)
	res, err = partial(context.Background(), jobs.Job{})
	if err != nil || res.(Result).FeedsFetched != 1 || res.(Result).Stopped == "" {
		t.Fatalf("a failing half must not fail the job: %+v %v", res, err)
	}

	both := Handler(fakeTrends{err: errors.New("db")}, fakeFeeds{err: errors.New("db")}, nil)
	if _, err := both(context.Background(), jobs.Job{}); err == nil {
		t.Fatal("expected error when both halves fail")
	}
}

func TestSlotKey(t *testing.T) {
	a := time.Date(2026, 11, 10, 12, 1, 0, 0, time.UTC)
	b := time.Date(2026, 11, 10, 12, 14, 59, 0, time.UTC)
	c := time.Date(2026, 11, 10, 12, 15, 0, 0, time.UTC)
	if SlotKey(a, 15*time.Minute) != SlotKey(b, 15*time.Minute) {
		t.Fatal("times in one slot must share a key")
	}
	if SlotKey(a, 15*time.Minute) == SlotKey(c, 15*time.Minute) {
		t.Fatal("different slots must have different keys")
	}
}

type recordingEnqueuer struct {
	mu   sync.Mutex
	jobs []jobs.NewJob
	got  chan struct{}
}

func (r *recordingEnqueuer) Enqueue(_ context.Context, j jobs.NewJob) (jobs.Job, error) {
	r.mu.Lock()
	r.jobs = append(r.jobs, j)
	r.mu.Unlock()
	r.got <- struct{}{}
	return jobs.Job{}, nil
}

func TestScheduleEnqueuesImmediatelyAndStops(t *testing.T) {
	enq := &recordingEnqueuer{got: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Schedule(ctx, enq, "local", time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil)))
		close(done)
	}()
	select {
	case <-enq.got:
	case <-time.After(5 * time.Second):
		t.Fatal("first job was not enqueued")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("scheduler did not stop")
	}
	enq.mu.Lock()
	defer enq.mu.Unlock()
	if len(enq.jobs) != 1 || enq.jobs[0].Kind != KindTrendRefresh || enq.jobs[0].IdempotencyKey == "" || enq.jobs[0].OwnerID != "local" {
		t.Fatalf("unexpected jobs: %+v", enq.jobs)
	}
}
