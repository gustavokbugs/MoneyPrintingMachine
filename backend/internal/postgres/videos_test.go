package postgres

import (
	"context"
	"errors"
	"testing"

	"contentfactory/internal/channel"
	"contentfactory/internal/production"
	"contentfactory/internal/publishing"
)

func TestVideoRepository(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ch, _ := channel.NewDraft("alice", "Space", "", "")
	if err := NewChannelRepository(pool).Create(ctx, ch); err != nil {
		t.Fatal(err)
	}
	r := NewVideoRepository(pool)
	v := &production.Video{OwnerID: "alice", ChannelID: ch.ID, IdeaIndex: 2, Title: "Moon", AspectRatio: "9:16", TargetSeconds: 60, Status: production.StatusPlanned, VoiceID: 6}
	if err := r.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	dup := *v
	if err := r.Create(ctx, &dup); !errors.Is(err, production.ErrExists) {
		t.Fatalf("duplicate idea: %v", err)
	}
	v.Script = &production.Script{Sections: []production.Section{{Narration: "hi"}}, Claims: []production.Claim{{Text: "c", Status: "unverified"}}}
	v.Status = production.StatusScriptReady
	if err := r.Save(ctx, v); err != nil {
		t.Fatal(err)
	}
	got, err := r.Get(ctx, "alice", v.ID)
	if err != nil || got.Script == nil || len(got.Script.Claims) != 1 || got.Version != 2 || got.Status != production.StatusScriptReady {
		t.Fatalf("get: %+v %v", got, err)
	}
	stale := *got
	stale.Version = 1
	if err := r.Save(ctx, &stale); !errors.Is(err, production.ErrConflict) {
		t.Fatalf("stale save: %v", err)
	}
	if _, err := r.Get(ctx, "bob", v.ID); !errors.Is(err, production.ErrNotFound) {
		t.Fatalf("other owner: %v", err)
	}
	list, err := r.ListByChannel(ctx, "alice", ch.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %d %v", len(list), err)
	}
}

func TestPublicationRepository(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	ch, _ := channel.NewDraft("alice", "Space", "", "")
	_ = NewChannelRepository(pool).Create(ctx, ch)
	v := &production.Video{OwnerID: "alice", ChannelID: ch.ID, Title: "Moon", AspectRatio: "16:9", TargetSeconds: 60, Status: production.StatusProduced}
	if err := NewVideoRepository(pool).Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	r := NewPublicationRepository(pool)
	p, err := r.Begin(ctx, "alice", v.ID, "youtube", "private")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Begin(ctx, "alice", v.ID, "youtube", "private"); !errors.Is(err, publishing.ErrInProgress) {
		t.Fatalf("concurrent begin: %v", err)
	}
	_ = r.Finish(ctx, p.ID, publishing.StatusFailed, "", "", "boom")
	p2, err := r.Begin(ctx, "alice", v.ID, "youtube", "unlisted")
	if err != nil || p2.ID != p.ID || p2.Privacy != "unlisted" {
		t.Fatalf("retry after failure: %+v %v", p2, err)
	}
	_ = r.Finish(ctx, p.ID, publishing.StatusPublished, "yt1", "https://www.youtube.com/watch?v=yt1", "")
	if _, err := r.Begin(ctx, "alice", v.ID, "youtube", "public"); !errors.Is(err, publishing.ErrAlreadyPublished) {
		t.Fatalf("after success: %v", err)
	}
	list, err := r.ListForVideo(ctx, "alice", v.ID)
	if err != nil || len(list) != 1 || list[0].ExternalID != "yt1" {
		t.Fatalf("list: %+v %v", list, err)
	}
	if other, _ := r.ListForVideo(ctx, "bob", v.ID); len(other) != 0 {
		t.Fatal("publications must be owner-scoped")
	}
}
