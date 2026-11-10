package launchpad

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"contentfactory/internal/channel"
	"contentfactory/internal/rss"
	"contentfactory/internal/trends"
	"contentfactory/internal/youtube"
)

type fakeMeasurer struct {
	available bool
	failOn    map[string]error
	requests  []trends.Request
}

func (f *fakeMeasurer) Available() bool { return f.available }
func (f *fakeMeasurer) Measure(_ context.Context, r trends.Request) (trends.Result, error) {
	f.requests = append(f.requests, r)
	if err := f.failOn[r.Query]; err != nil {
		return trends.Result{}, err
	}
	demand, comp := 42.0, 17.0
	return trends.Result{
		Snapshot: trends.Snapshot{CollectedAt: time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC), Metrics: trends.Metrics{SampleSize: 50}},
		Scores:   trends.Scores{Demand: &demand, Competition: &comp},
	}, nil
}

func threeNiches(t *testing.T) string {
	return mustJSON(t, map[string]any{"niches": []any{
		nicheJSON("Alpha", "high", "low"), nicheJSON("Beta", "medium", "medium"), nicheJSON("Gamma", "low", "high"),
	}})
}

func youtubeSource(r *NicheReport) DataSourceStatus {
	for _, ds := range r.DataSources {
		if ds.Name == youtubeSourceName {
			return ds
		}
	}
	return DataSourceStatus{}
}

func TestDiscoverWithMeasurements(t *testing.T) {
	m := &fakeMeasurer{available: true, failOn: map[string]error{"Beta": errors.New("timeout")}}
	e := fixedEngine(&scriptedGen{responses: []string{threeNiches(t)}}).WithMeasurer(m)
	report, err := e.DiscoverNiches(context.Background(), DiscoverInput{OwnerID: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Niche{}
	for _, n := range report.Niches {
		byName[n.Name] = n
	}
	if a := byName["Alpha"].Measured; a.DemandScore == nil || *a.DemandScore != 42 || a.Metrics == nil || len(a.Sources) != 1 {
		t.Fatalf("Alpha not measured: %+v", a)
	}
	if b := byName["Beta"].Measured; b.DemandScore != nil || b.Metrics != nil {
		t.Fatalf("failed measurement must stay empty: %+v", b)
	}
	if byName["Alpha"].Measured.SearchInterest != nil {
		t.Fatal("search interest has no source and must stay nil")
	}
	ds := youtubeSource(report)
	if ds.Status != "connected" || !strings.Contains(ds.Note, "Measured 2 of 3") || !strings.Contains(ds.Note, "1 measurement(s) failed") {
		t.Fatalf("unexpected source status: %+v", ds)
	}
	if m.requests[0].OwnerID != "alice" || m.requests[0].Region != "US" || m.requests[0].Language != "en-US" {
		t.Fatalf("unexpected request: %+v", m.requests[0])
	}
}

func TestDiscoverStopsOnQuotaBudget(t *testing.T) {
	m := &fakeMeasurer{available: true, failOn: map[string]error{"Alpha": youtube.ErrQuotaBudget}}
	e := fixedEngine(&scriptedGen{responses: []string{threeNiches(t)}}).WithMeasurer(m)
	report, err := e.DiscoverNiches(context.Background(), DiscoverInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.requests) != 1 {
		t.Fatalf("measuring must stop after the budget is reached, got %d requests", len(m.requests))
	}
	if ds := youtubeSource(report); ds.Status != "unavailable" || !strings.Contains(ds.Note, "quota") {
		t.Fatalf("unexpected source status: %+v", ds)
	}
}

func TestDiscoverWithoutMeasurer(t *testing.T) {
	e := fixedEngine(&scriptedGen{responses: []string{threeNiches(t)}}).WithMeasurer(&fakeMeasurer{available: false})
	report, err := e.DiscoverNiches(context.Background(), DiscoverInput{})
	if err != nil {
		t.Fatal(err)
	}
	if ds := youtubeSource(report); ds.Status != "not_configured" {
		t.Fatalf("unexpected source status: %+v", ds)
	}
}

func TestHandleFor(t *testing.T) {
	for in, want := range map[string]string{
		"Quiet Orbit":    "quietorbit",
		"Copper & Stone": "copperstone",
		"Ça Va":          "ava",
		"X":              "",
		"An Extremely Long Channel Name That Exceeds": "",
	} {
		if got := HandleFor(in); got != want {
			t.Errorf("HandleFor(%q) = %q, want %q", in, got, want)
		}
	}
}

type fakeHandles struct {
	taken map[string]bool
	errOn string
	calls int
}

func (f *fakeHandles) HasAPIKey() bool { return true }
func (f *fakeHandles) ChannelByHandle(_ context.Context, h string) (*youtube.ChannelInfo, error) {
	f.calls++
	if h == f.errOn {
		return nil, youtube.ErrQuotaBudget
	}
	if f.taken[h] {
		return &youtube.ChannelInfo{ID: "UC" + h}, nil
	}
	return nil, nil
}

type identityStore struct {
	fakeStore
	saved *channel.Identity
}

func (s *identityStore) SetIdentity(_ context.Context, _, _ string, i channel.Identity) (*channel.Channel, error) {
	s.saved = &i
	c := *s.ch
	c.Identity = &i
	return &c, nil
}

func TestCheckNames(t *testing.T) {
	ch := strategyFixture()
	ch.Identity = &channel.Identity{NameSuggestions: []channel.NameSuggestion{
		{Name: "Quiet Orbit"}, {Name: "Far Light"}, {Name: "X"}, {Name: "Iron Moon"},
	}}
	store := &identityStore{fakeStore: fakeStore{ch: ch}}
	h := &fakeHandles{taken: map[string]bool{"quietorbit": true}, errOn: "ironmoon"}
	e := fixedEngine(&scriptedGen{}).WithHandleLookup(h)

	_, err := e.CheckNames(context.Background(), store, "alice", "ch1")
	if !errors.Is(err, youtube.ErrQuotaBudget) {
		t.Fatalf("expected partial result with quota error, got %v", err)
	}
	n := store.saved.NameSuggestions
	want := []string{channel.AvailabilityHandleTaken, channel.AvailabilityNoChannel, channel.AvailabilityInvalidHandle, ""}
	for i, w := range want {
		if n[i].Availability != w {
			t.Errorf("%s: availability %q, want %q", n[i].Name, n[i].Availability, w)
		}
	}
	if n[0].Handle != "quietorbit" || n[0].CheckedAt == nil || n[3].CheckedAt != nil {
		t.Fatalf("unexpected check details: %+v", n)
	}
	if h.calls != 3 {
		t.Fatalf("invalid handles must not call the API: %d calls", h.calls)
	}

	if _, err := fixedEngine(&scriptedGen{}).CheckNames(context.Background(), store, "alice", "ch1"); !errors.Is(err, ErrPrecondition) {
		t.Fatalf("expected precondition error without lookup, got %v", err)
	}
}

type fakeNews struct{ feeds int }

func (f fakeNews) Mentions(_ context.Context, _, query string) (rss.MentionStats, error) {
	if query == "Beta" {
		return rss.MentionStats{}, errors.New("db down")
	}
	pct := 50.0
	return rss.MentionStats{Recent7d: 3, Previous7d: 2, MomentumPercent: &pct, ActiveFeeds: f.feeds}, nil
}

func TestDiscoverWithNewsAndMeasurements(t *testing.T) {
	e := fixedEngine(&scriptedGen{responses: []string{threeNiches(t)}}).
		WithMeasurer(&fakeMeasurer{available: true}).
		WithNews(fakeNews{feeds: 2})
	report, err := e.DiscoverNiches(context.Background(), DiscoverInput{OwnerID: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range report.Niches {
		if n.Name == "Beta" {
			if n.Measured.News != nil {
				t.Fatal("failed news count must stay empty")
			}
			continue
		}
		if n.Measured.News == nil || n.Measured.News.Recent7d != 3 || n.Measured.DemandScore == nil {
			t.Fatalf("%s: expected both YouTube and news data, got %+v", n.Name, n.Measured)
		}
		if strings.Join(n.Measured.Sources, ",") != "youtube_data_api,rss" {
			t.Fatalf("%s: sources = %v", n.Name, n.Measured.Sources)
		}
	}
	var news DataSourceStatus
	for _, ds := range report.DataSources {
		if ds.Name == newsSourceName {
			news = ds
		}
	}
	if news.Status != "connected" || !strings.Contains(news.Note, "2 feed(s)") || !strings.Contains(news.Note, "1 count(s) failed") {
		t.Fatalf("news source: %+v", news)
	}
}

func TestDiscoverNewsWithoutFeeds(t *testing.T) {
	e := fixedEngine(&scriptedGen{responses: []string{threeNiches(t)}}).WithNews(fakeNews{feeds: 0})
	report, _ := e.DiscoverNiches(context.Background(), DiscoverInput{})
	for _, ds := range report.DataSources {
		if ds.Name == newsSourceName && ds.Status != "not_configured" {
			t.Fatalf("expected not_configured, got %+v", ds)
		}
	}
	for _, n := range report.Niches {
		if n.Measured.News != nil {
			t.Fatal("no feeds means no news data")
		}
	}
}
