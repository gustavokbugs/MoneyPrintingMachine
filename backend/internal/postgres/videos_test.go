package postgres

import (
	"context"
	"errors"
	"testing"

	"contentfactory/internal/channel"
	"contentfactory/internal/production"
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
