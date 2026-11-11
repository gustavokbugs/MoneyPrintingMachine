package production

import (
	"context"
	"errors"
	"strings"
	"testing"

	"contentfactory/internal/channel"
)

func TestSetStoryFormat(t *testing.T) {
	svc, store := newTestService(t, &scriptedGen{}, nil, Tools{})
	v, err := svc.CreateFromIdea(context.Background(), "alice", "ch1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if v.StoryFormat != FormatExplainer {
		t.Errorf("default format = %q", v.StoryFormat)
	}
	var ve *channel.ValidationError
	if _, err := svc.SetStoryFormat(context.Background(), "alice", v.ID, "rant"); !errors.As(err, &ve) {
		t.Errorf("unknown format: %v", err)
	}
	got, err := svc.SetStoryFormat(context.Background(), "alice", v.ID, FormatEscalation)
	if err != nil || got.StoryFormat != FormatEscalation || got.TargetSeconds != ShortFormatSeconds {
		t.Fatalf("set: %+v %v", got, err)
	}
	// Explainer keeps the current length.
	if got, err = svc.SetStoryFormat(context.Background(), "alice", v.ID, FormatExplainer); err != nil || got.TargetSeconds != ShortFormatSeconds {
		t.Fatalf("explainer: %+v %v", got, err)
	}
	stored := store.items[v.ID]
	stored.Status = StatusProducing
	store.items[v.ID] = stored
	var se *StateError
	if _, err := svc.SetStoryFormat(context.Background(), "alice", v.ID, FormatPOV); !errors.As(err, &se) {
		t.Errorf("while producing: %v", err)
	}
	if _, err := svc.SetStoryFormat(context.Background(), "mallory", v.ID, FormatPOV); !errors.Is(err, ErrNotFound) {
		t.Errorf("other owner: %v", err)
	}
}

func TestShortFormatShapesThePrompt(t *testing.T) {
	gen := &scriptedGen{}
	svc, _ := newTestService(t, gen, nil, Tools{})
	v := &Video{IdeaIndex: 0, TargetSeconds: ShortFormatSeconds, StoryFormat: FormatEscalation, DialogueMode: DialogueNarration}
	_, _ = svc.GenerateScript(context.Background(), testChannel(), v) // the fake model has no answers
	if len(gen.requests) == 0 {
		t.Fatal("no request sent")
	}
	req := gen.requests[0]
	if !strings.Contains(req.Messages[0].Content, "viral") || !strings.Contains(req.Messages[1].Content, "absurd escalation") {
		t.Errorf("prompt does not use the escalation format:\n%s", req.Messages[1].Content)
	}
	if strings.Contains(req.Messages[1].Content, "state what the viewer will learn") {
		t.Error("the explainer hook rule leaked into a short format")
	}

	gen2 := &scriptedGen{}
	svc2, _ := newTestService(t, gen2, nil, Tools{})
	_, _ = svc2.GenerateScript(context.Background(), testChannel(), &Video{TargetSeconds: 120, StoryFormat: FormatExplainer})
	if len(gen2.requests) == 0 || strings.Contains(gen2.requests[0].Messages[0].Content, "viral") {
		t.Error("explainer must keep the original prompt")
	}
}
