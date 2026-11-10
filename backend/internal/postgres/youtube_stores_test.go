package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"contentfactory/internal/channel"
	"contentfactory/internal/connections"
	"contentfactory/internal/trends"
)

func TestQuotaStoreReserve(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	q := NewQuotaStore(pool)
	day := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	for i, want := range []bool{true, true, false} {
		ok, err := q.Reserve(ctx, "yt", day, 100, 250)
		if err != nil || ok != want {
			t.Fatalf("reserve %d: ok=%v err=%v, want %v", i, ok, err, want)
		}
	}
	if used, _ := q.Used(ctx, "yt", day); used != 200 {
		t.Fatalf("used = %d", used)
	}
	if ok, _ := q.Reserve(ctx, "yt", day.AddDate(0, 0, 1), 300, 250); ok {
		t.Fatal("a single reservation above the limit must fail")
	}
}

func TestTrendSnapshotRepository(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	r := NewTrendSnapshotRepository(pool)
	base := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	for i := range 3 {
		s := &trends.Snapshot{Source: "yt", Query: "Space", QueryKey: "space", Region: "US", Language: "en-US",
			Metrics: trends.Metrics{SampleSize: i}, CollectedAt: base.Add(time.Duration(i) * 24 * time.Hour)}
		if err := r.Save(ctx, "alice", s); err != nil {
			t.Fatal(err)
		}
	}
	latest, err := r.Latest(ctx, "alice", "yt", "space", "US", "en-US", base.Add(36*time.Hour))
	if err != nil || latest == nil || latest.Metrics.SampleSize != 1 {
		t.Fatalf("latest before cutoff: %+v %v", latest, err)
	}
	if none, _ := r.Latest(ctx, "bob", "yt", "space", "US", "en-US", base.Add(72*time.Hour)); none != nil {
		t.Fatal("snapshots must be owner-scoped")
	}
	hist, err := r.History(ctx, "alice", "yt", "space", "US", "en-US", 2)
	if err != nil || len(hist) != 2 || hist[0].Metrics.SampleSize != 2 {
		t.Fatalf("history: %+v %v", hist, err)
	}
}

func TestConnectionStore(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	chRepo := NewChannelRepository(pool)
	c1, _ := channel.NewDraft("alice", "Space", "", "")
	c2, _ := channel.NewDraft("alice", "History", "", "")
	_ = chRepo.Create(ctx, c1)
	_ = chRepo.Create(ctx, c2)
	s := NewConnectionStore(pool)
	now := time.Now()

	if err := s.SaveState(ctx, connections.State{StateHash: "h1", OwnerID: "alice", ChannelID: c1.ID, Platform: "youtube",
		VerifierEnc: []byte("v"), ExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	st, err := s.ConsumeState(ctx, "h1", now)
	if err != nil || st.ChannelID != c1.ID {
		t.Fatalf("consume: %+v %v", st, err)
	}
	if _, err := s.ConsumeState(ctx, "h1", now); !errors.Is(err, connections.ErrStateInvalid) {
		t.Fatalf("state must be single-use, got %v", err)
	}
	_ = s.SaveState(ctx, connections.State{StateHash: "h2", OwnerID: "alice", ChannelID: c1.ID, Platform: "youtube",
		VerifierEnc: []byte("v"), ExpiresAt: now.Add(time.Minute)})
	if _, err := s.ConsumeState(ctx, "h2", now.Add(2*time.Minute)); !errors.Is(err, connections.ErrStateInvalid) {
		t.Fatalf("expired state must be rejected, got %v", err)
	}

	rec := &connections.Record{OwnerID: "alice", ChannelID: c1.ID, Platform: "youtube", Status: "verified",
		ExternalID: "UC1", ExternalName: "One", CanPublish: true, RefreshTokenEnc: []byte("enc"),
		GrantedScopes: []string{"a", "b"}}
	if err := s.Upsert(ctx, rec); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, "alice", c1.ID, "youtube")
	if err != nil || got == nil || got.ExternalID != "UC1" || len(got.GrantedScopes) != 2 || string(got.RefreshTokenEnc) != "enc" {
		t.Fatalf("get: %+v %v", got, err)
	}
	n, _ := NewPlatformConnectionRepository(pool).CountVerified(ctx, "alice", c1.ID)
	if n != 1 {
		t.Fatalf("verified count = %d", n)
	}

	dup := &connections.Record{OwnerID: "alice", ChannelID: c2.ID, Platform: "youtube", Status: "verified", ExternalID: "UC1"}
	if err := s.Upsert(ctx, dup); !errors.Is(err, connections.ErrExternalInUse) {
		t.Fatalf("expected external in use, got %v", err)
	}
	other := &connections.Record{OwnerID: "mallory", ChannelID: c1.ID, Platform: "youtube", Status: "error"}
	if err := s.Upsert(ctx, other); !errors.Is(err, channel.ErrNotFound) {
		t.Fatalf("another owner must not overwrite the connection, got %v", err)
	}
	if err := s.Delete(ctx, "alice", c1.ID, "youtube"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get(ctx, "alice", c1.ID, "youtube"); got != nil {
		t.Fatal("connection not deleted")
	}
}

func TestVerifiedPlatforms(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	chRepo := NewChannelRepository(pool)
	c1, _ := channel.NewDraft("alice", "Space", "", "")
	c2, _ := channel.NewDraft("alice", "History", "", "")
	_ = chRepo.Create(ctx, c1)
	_ = chRepo.Create(ctx, c2)
	s := NewConnectionStore(pool)
	_ = s.Upsert(ctx, &connections.Record{OwnerID: "alice", ChannelID: c1.ID, Platform: "youtube", Status: "verified", ExternalID: "UC1"})
	_ = s.Upsert(ctx, &connections.Record{OwnerID: "alice", ChannelID: c2.ID, Platform: "youtube", Status: "authorized"})
	got, err := NewPlatformConnectionRepository(pool).VerifiedPlatforms(ctx, "alice", []string{c1.ID, c2.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got[c1.ID]) != 1 || got[c1.ID][0] != "youtube" || len(got[c2.ID]) != 0 {
		t.Fatalf("unexpected verified platforms: %v", got)
	}
}
