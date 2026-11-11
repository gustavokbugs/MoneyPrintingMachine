package launchpad

import (
	"context"
	"errors"
	"strings"
	"testing"

	"contentfactory/internal/channel"
)

func readyChannel() *channel.Channel {
	ch := strategyFixture()
	ch.Name = ""
	ch.Identity = &channel.Identity{NameSuggestions: []channel.NameSuggestion{
		{Name: "Taken One", Availability: channel.AvailabilityHandleTaken},
		{Name: "Fresh Orbit", Availability: channel.AvailabilityNotChecked},
	}}
	ch.Blueprint = &channel.Blueprint{}
	return ch
}

func TestAutopilotOnlyFillsMissingSteps(t *testing.T) {
	g := &scriptedGen{}
	store := &fakeStore{ch: readyChannel()}
	res, err := fixedEngine(g).Autopilot(context.Background(), store, "alice", "ch1")
	if err != nil {
		t.Fatal(err)
	}
	if g.calls != 0 {
		t.Errorf("model called %d times for steps that already exist", g.calls)
	}
	if store.ch.Name != "Fresh Orbit" {
		t.Errorf("name = %q, want the first suggestion not known to be taken", store.ch.Name)
	}
	if strings.Join(res.Generated, ",") != "name" || len(res.Notes) != 1 || !strings.Contains(res.Notes[0], "not checked") {
		t.Errorf("result = %+v", res)
	}
	// A second run changes nothing.
	again, err := fixedEngine(g).Autopilot(context.Background(), store, "alice", "ch1")
	if err != nil || len(again.Generated) != 0 {
		t.Fatalf("rerun = %+v, %v", again, err)
	}
}

func TestAutopilotRejectsArchivedAndUnknownChannels(t *testing.T) {
	ch := readyChannel()
	ch.Status = channel.StatusArchived
	_, err := fixedEngine(&scriptedGen{}).Autopilot(context.Background(), &fakeStore{ch: ch}, "alice", "ch1")
	var ve *channel.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("archived: err = %v", err)
	}
	_, err = fixedEngine(&scriptedGen{}).Autopilot(context.Background(), &fakeStore{ch: readyChannel()}, "mallory", "ch1")
	if !errors.Is(err, channel.ErrNotFound) {
		t.Fatalf("other owner: err = %v", err)
	}
}

func TestAutopilotFailsWithoutUsableName(t *testing.T) {
	ch := readyChannel()
	ch.Identity.NameSuggestions = []channel.NameSuggestion{{Name: "X", Availability: channel.AvailabilityInvalidHandle}}
	_, err := fixedEngine(&scriptedGen{}).Autopilot(context.Background(), &fakeStore{ch: ch}, "alice", "ch1")
	if !errors.Is(err, ErrPrecondition) {
		t.Fatalf("err = %v", err)
	}
}

func TestPickNamePrefersFreeHandles(t *testing.T) {
	name, note, ok := pickName([]channel.NameSuggestion{
		{Name: "A", Availability: channel.AvailabilityNotChecked},
		{Name: "B", Handle: "b", Availability: channel.AvailabilityNoChannel},
	})
	if !ok || name != "B" || !strings.Contains(note, "@b") {
		t.Fatalf("pickName = %q %q %v", name, note, ok)
	}
}

func TestBestNicheHighestScoreFirstOnTies(t *testing.T) {
	best, ok := bestNiche([]Niche{{Name: "a", HeuristicScore: 60}, {Name: "b", HeuristicScore: 80}, {Name: "c", HeuristicScore: 80}, {Name: "", HeuristicScore: 99}})
	if !ok || best.Name != "b" {
		t.Fatalf("best = %+v", best)
	}
	if _, ok := bestNiche(nil); ok {
		t.Error("no niches should report false")
	}
}
