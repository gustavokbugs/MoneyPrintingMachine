package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"contentfactory/internal/channel"
	"contentfactory/internal/jobs"
	"contentfactory/internal/launchpad"
)

// testPool connects to TEST_DATABASE_URL, creates an isolated schema, and
// applies migrations. Tests are skipped when the variable is not set.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}
	ctx := context.Background()
	schema := fmt.Sprintf("test_%d", time.Now().UnixNano())

	admin, err := Connect(ctx, url, 2)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

func TestMigrateIsIdempotent(t *testing.T) {
	pool := testPool(t)
	applied, err := Migrate(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 0 {
		t.Fatalf("expected no migrations on second run, got %v", applied)
	}
}

func TestChannelRepository(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewChannelRepository(pool)

	c, _ := channel.NewDraft("alice", "Space", "", "")
	if err := repo.Create(ctx, c); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.Get(ctx, "bob", c.ID); !errors.Is(err, channel.ErrNotFound) {
		t.Fatalf("expected not found for other owner, got %v", err)
	}
	if _, err := repo.Get(ctx, "alice", "not-a-uuid"); !errors.Is(err, channel.ErrNotFound) {
		t.Fatalf("expected not found for malformed id, got %v", err)
	}

	c.Strategy = &channel.Strategy{TargetAudience: "Curious adults"}
	ev := &channel.StatusEvent{From: channel.StatusDraft, To: channel.StatusBranding}
	c.Status = channel.StatusBranding
	if err := repo.Save(ctx, c, ev); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, "alice", c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != channel.StatusBranding || got.Strategy == nil || got.Strategy.TargetAudience != "Curious adults" || got.Version != 2 {
		t.Fatalf("unexpected stored channel: %+v", got)
	}

	stale := *got
	stale.Version = 1
	if err := repo.Save(ctx, &stale, nil); !errors.Is(err, channel.ErrConflict) {
		t.Fatalf("expected conflict for stale version, got %v", err)
	}
	other := *got
	other.OwnerID = "bob"
	if err := repo.Save(ctx, &other, nil); !errors.Is(err, channel.ErrNotFound) {
		t.Fatalf("expected not found saving as other owner, got %v", err)
	}

	var events int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM channel_status_events WHERE channel_id = $1`, c.ID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("expected 1 status event, got %d", events)
	}
}

func TestChannelListPagination(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewChannelRepository(pool)
	for i := range 5 {
		c, _ := channel.NewDraft("alice", fmt.Sprintf("Niche %d", i), "", "")
		if err := repo.Create(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	other, _ := channel.NewDraft("bob", "Other", "", "")
	_ = repo.Create(ctx, other)

	seen := map[string]bool{}
	cursor := ""
	for pages := 0; pages < 10; pages++ {
		items, next, err := repo.List(ctx, "alice", channel.Page{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range items {
			if it.OwnerID != "alice" || seen[it.ID] {
				t.Fatalf("unexpected item %+v", it)
			}
			seen[it.ID] = true
		}
		if next == "" {
			break
		}
		cursor = next
	}
	if len(seen) != 5 {
		t.Fatalf("expected 5 channels across pages, got %d", len(seen))
	}
	if _, _, err := repo.List(ctx, "alice", channel.Page{Limit: 2, Cursor: "garbage"}); err == nil {
		t.Fatal("expected error for invalid cursor")
	}
}

func TestJobStoreLifecycle(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	store := NewJobStore(pool)
	enq := jobs.NewEnqueuer(store)

	j1, err := enq.Enqueue(ctx, jobs.NewJob{OwnerID: "alice", Kind: "k", Input: map[string]string{"q": "x"}, IdempotencyKey: "abc"})
	if err != nil {
		t.Fatal(err)
	}
	j2, err := enq.Enqueue(ctx, jobs.NewJob{OwnerID: "alice", Kind: "k", Input: map[string]string{"q": "y"}, IdempotencyKey: "abc"})
	if err != nil {
		t.Fatal(err)
	}
	if j1.ID != j2.ID {
		t.Fatalf("idempotency key did not deduplicate: %s vs %s", j1.ID, j2.ID)
	}
	if _, err := store.Get(ctx, "bob", j1.ID); !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("expected not found for other owner, got %v", err)
	}

	claimed, err := store.Claim(ctx, []string{"k"}, time.Minute)
	if err != nil || claimed == nil || claimed.ID != j1.ID || claimed.Attempts != 1 {
		t.Fatalf("claim: job=%+v err=%v", claimed, err)
	}
	if again, err := store.Claim(ctx, []string{"k"}, time.Minute); err != nil || again != nil {
		t.Fatalf("expected no second claim while leased, got %+v err=%v", again, err)
	}

	past := time.Now().Add(-time.Second)
	if err := store.Fail(ctx, j1.ID, "temporary", &past); err != nil {
		t.Fatal(err)
	}
	claimed, err = store.Claim(ctx, []string{"k"}, time.Minute)
	if err != nil || claimed == nil || claimed.Attempts != 2 {
		t.Fatalf("expected retry claim, got %+v err=%v", claimed, err)
	}
	if err := store.Complete(ctx, j1.ID, json.RawMessage(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	done, err := store.Get(ctx, "alice", j1.ID)
	if err != nil || done.Status != jobs.StatusSucceeded || string(done.Result) != `{"ok": true}` {
		t.Fatalf("unexpected final job: %+v err=%v", done, err)
	}
}

func TestJobStoreReclaimsExpiredLease(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	store := NewJobStore(pool)
	j, _ := jobs.NewEnqueuer(store).Enqueue(ctx, jobs.NewJob{OwnerID: "alice", Kind: "k"})
	if _, err := store.Claim(ctx, []string{"k"}, time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET locked_until = now() - interval '1 second' WHERE id = $1`, j.ID); err != nil {
		t.Fatal(err)
	}
	re, err := store.Claim(ctx, []string{"k"}, time.Minute)
	if err != nil || re == nil || re.ID != j.ID {
		t.Fatalf("expected expired lease to be reclaimed, got %+v err=%v", re, err)
	}
}

func TestSavedNiches(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewSavedNicheRepository(pool)
	n, err := repo.Create(ctx, "alice", "Space History", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, "alice", "space history", "", nil); !errors.Is(err, launchpad.ErrSavedNicheExists) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
	if err := repo.Delete(ctx, "bob", n.ID); !errors.Is(err, launchpad.ErrSavedNicheNotFound) {
		t.Fatalf("expected not found deleting as other owner, got %v", err)
	}
	if err := repo.Delete(ctx, "alice", n.ID); err != nil {
		t.Fatal(err)
	}
}
