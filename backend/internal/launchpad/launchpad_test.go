package launchpad

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"contentfactory/internal/channel"
	"contentfactory/internal/jobs"
	"contentfactory/internal/llm"
)

// scriptedGen returns pre-recorded responses in order, or computes them
// with respond when set.
type scriptedGen struct {
	responses []string
	respond   func(call int, req llm.Request) string
	calls     int
	prompts   []string
}

func (g *scriptedGen) Model() string { return "test-model" }

func (g *scriptedGen) GenerateJSON(_ context.Context, req llm.Request, out any) error {
	g.calls++
	g.prompts = append(g.prompts, req.Messages[len(req.Messages)-1].Content)
	var body string
	switch {
	case g.respond != nil:
		body = g.respond(g.calls, req)
	case g.calls <= len(g.responses):
		body = g.responses[g.calls-1]
	default:
		return fmt.Errorf("%w: no scripted response", llm.ErrUnavailable)
	}
	return llm.DecodeStrict(body, out)
}

func fixedEngine(g Generator) *Engine {
	e := NewEngine(g)
	e.now = func() time.Time { return time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC) }
	return e
}

func nicheJSON(name, audience, competition string) map[string]any {
	return map[string]any{
		"name": name, "description": "d", "subNiches": []string{"a", "b"},
		"contentOpportunities": []string{"x", "y"}, "suggestedFormats": []string{"f"},
		"rationale": "r", "audiencePotential": audience, "monetizationPotential": "medium",
		"competitionEstimate": competition, "productionDifficulty": "medium",
		"sourceAvailability": "high", "originalityPotential": "medium",
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestHeuristicScore(t *testing.T) {
	best := Heuristics{RatingHigh, RatingHigh, RatingLow, RatingLow, RatingHigh, RatingHigh}
	worst := Heuristics{RatingLow, RatingLow, RatingHigh, RatingHigh, RatingLow, RatingLow}
	if got := HeuristicScore(best); got != 100 {
		t.Errorf("best score = %d, want 100", got)
	}
	if got := HeuristicScore(worst); got != 0 {
		t.Errorf("worst score = %d, want 0", got)
	}
	mid := Heuristics{RatingMedium, RatingMedium, RatingMedium, RatingMedium, RatingMedium, RatingMedium}
	if got := HeuristicScore(mid); got <= 0 || got >= 100 {
		t.Errorf("mid score = %d, want strictly between 0 and 100", got)
	}
}

func TestDiscoverNichesRanksAndResamples(t *testing.T) {
	invalid := mustJSON(t, map[string]any{"niches": []any{nicheJSON("A", "huge", "low"), nicheJSON("B", "low", "low"), nicheJSON("C", "low", "low")}})
	valid := mustJSON(t, map[string]any{"niches": []any{
		nicheJSON("Low One", "low", "high"),
		nicheJSON("High One", "high", "low"),
		nicheJSON("high one", "high", "low"), // duplicate name, different case
		nicheJSON("Mid One", "medium", "medium"),
	}})
	g := &scriptedGen{responses: []string{invalid, valid}}
	report, err := fixedEngine(g).DiscoverNiches(context.Background(), DiscoverInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.calls != 2 {
		t.Fatalf("expected a resample after invalid output, got %d calls", g.calls)
	}
	if len(report.Niches) != 3 {
		t.Fatalf("expected duplicates removed, got %d niches", len(report.Niches))
	}
	if report.Niches[0].Name != "High One" || report.Niches[2].Name != "Low One" {
		t.Fatalf("unexpected ranking: %s, %s, %s", report.Niches[0].Name, report.Niches[1].Name, report.Niches[2].Name)
	}
	for _, n := range report.Niches {
		if n.Measured.TrendScore != nil || n.Measured.CompetitionScore != nil || n.Measured.SearchInterest != nil {
			t.Fatalf("measured scores must stay empty without a data source: %+v", n.Measured)
		}
	}
	for _, ds := range report.DataSources {
		if ds.Status == "connected" {
			t.Fatalf("no data source should report connected: %+v", ds)
		}
	}
}

func TestDiscoverNichesCustomQueryStaysFirst(t *testing.T) {
	valid := mustJSON(t, map[string]any{"niches": []any{
		nicheJSON("Vintage Watches", "low", "high"),
		nicheJSON("Watch Restoration", "high", "low"),
		nicheJSON("Horology History", "medium", "medium"),
	}})
	g := &scriptedGen{responses: []string{valid}}
	report, err := fixedEngine(g).DiscoverNiches(context.Background(), DiscoverInput{Query: "  Vintage   Watches "})
	if err != nil {
		t.Fatal(err)
	}
	if report.Query != "Vintage Watches" || report.Niches[0].Name != "Vintage Watches" {
		t.Fatalf("custom niche not first: query=%q first=%q", report.Query, report.Niches[0].Name)
	}
	if !strings.Contains(g.prompts[0], "<niche>Vintage Watches</niche>") {
		t.Fatalf("query not delimited in prompt")
	}
}

func TestDiscoverNichesRejectsLongQuery(t *testing.T) {
	_, err := fixedEngine(&scriptedGen{}).DiscoverNiches(context.Background(), DiscoverInput{Query: strings.Repeat("x", MaxQueryLength+1)})
	if !errors.Is(err, ErrPrecondition) {
		t.Fatalf("expected precondition error, got %v", err)
	}
}

func TestDiscoverNichesGivesUpAfterBoundedAttempts(t *testing.T) {
	g := &scriptedGen{respond: func(int, llm.Request) string { return "not json" }}
	_, err := fixedEngine(g).DiscoverNiches(context.Background(), DiscoverInput{})
	if !errors.Is(err, llm.ErrInvalidOutput) {
		t.Fatalf("expected invalid output error, got %v", err)
	}
	if g.calls != maxSampleAttempts {
		t.Fatalf("expected %d attempts, got %d", maxSampleAttempts, g.calls)
	}
}

func TestFilterNames(t *testing.T) {
	in := []channel.NameSuggestion{
		{Name: "Quiet Orbit"},
		{Name: "quiet  orbit"},   // duplicate
		{Name: "Space Hub"},      // generic token
		{Name: "AI Explained"},   // generic tokens
		{Name: "<script>"},       // malformed
		{Name: "X"},              // too short
		{Name: "Copper & Stone"}, // allowed punctuation
	}
	got := FilterNames(in)
	if len(got) != 2 || got[0].Name != "Quiet Orbit" || got[1].Name != "Copper & Stone" {
		t.Fatalf("unexpected names: %+v", got)
	}
	for _, n := range got {
		if n.Availability != "not_checked" {
			t.Fatalf("availability must be not_checked, got %q", n.Availability)
		}
	}
}

func TestContrastAndPalette(t *testing.T) {
	r, ok := ContrastRatio("#000000", "#FFFFFF")
	if !ok || r < 20.9 || r > 21.1 {
		t.Fatalf("black/white contrast = %v", r)
	}
	if _, ok := ContrastRatio("black", "#FFFFFF"); ok {
		t.Fatal("expected invalid color to be rejected")
	}
	p := EnsureReadablePalette(channel.Palette{Background: "#101820", Text: "#222222"})
	if p.Text != "#FAFAFA" {
		t.Fatalf("expected light text on dark background, got %s", p.Text)
	}
	p = EnsureReadablePalette(channel.Palette{Background: "#F4F1EA", Text: "#DDDDDD"})
	if p.Text != "#111111" {
		t.Fatalf("expected dark text on light background, got %s", p.Text)
	}
	keep := EnsureReadablePalette(channel.Palette{Background: "#FFFFFF", Text: "#1A1A1A"})
	if keep.Text != "#1A1A1A" {
		t.Fatalf("readable text color should be kept, got %s", keep.Text)
	}
}

func identityResponse(names []string) map[string]any {
	ns := make([]any, 0, len(names))
	for _, n := range names {
		ns = append(ns, map[string]any{"name": n, "rationale": "r"})
	}
	return map[string]any{
		"names": ns, "shortBio": "Short bio.", "fullDescription": "Full description.",
		"positioningStatement": "p", "tagline": "t", "keywords": []string{"k1", "k2", "k3", "k4", "k5"},
		"primaryColor": "#1d3557", "secondaryColor": "#457b9d", "accentColor": "#e63946",
		"backgroundColor": "#f1faee", "textColor": "#f1faee", "headingFont": "Oswald", "bodyFont": "Inter",
		"typographyNotes": "n", "avatarConcept": "a", "thumbnailStyle": "t", "captionStyle": "c",
		"motionGuidelines": "m", "introOutroGuidelines": "i",
	}
}

func strategyFixture() *channel.Channel {
	return &channel.Channel{
		ID: "ch1", OwnerID: "alice", Niche: "Space", Language: "en-US", GeoTarget: "US", Name: "Quiet Orbit",
		Strategy: &channel.Strategy{
			TargetAudience: "Adults", Positioning: "p", EditorialIdentity: "e", ContentPillars: []string{"missions"},
			PublishingFrequency: channel.PublishingSchedule{VideosPerWeek: 2, Days: []string{"Tuesday", "Friday"}, LocalTime: "17:00", TimeZone: "America/New_York"},
		},
	}
}

func TestGenerateIdentity(t *testing.T) {
	names := []string{"Quiet Orbit", "Far Light", "Slow Comet", "Pale Signal", "Iron Moon", "Deep Field",
		"Night Ledger", "Long Horizon", "Cold Star", "Red Dust", "Outer Ring", "Glass Planet", "Silent Probe", "Dark Tide"}
	g := &scriptedGen{responses: []string{mustJSON(t, identityResponse(names))}}
	idn, err := fixedEngine(g).GenerateIdentity(context.Background(), strategyFixture())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(idn.NameSuggestions) != len(names) {
		t.Fatalf("expected %d names, got %d", len(names), len(idn.NameSuggestions))
	}
	if idn.Palette.Primary != "#1D3557" {
		t.Fatalf("expected uppercase hex, got %s", idn.Palette.Primary)
	}
	if idn.Palette.Text == idn.Palette.Background {
		t.Fatal("unreadable text color was not corrected")
	}
	if idn.Provenance.Model != "test-model" {
		t.Fatalf("missing provenance: %+v", idn.Provenance)
	}
}

func TestGenerateIdentityRejectsTooFewUsableNames(t *testing.T) {
	generic := []string{"Space Hub", "Space Zone", "Star Vault", "Orbit Nexus", "Cosmic Realm", "Galaxy Quest",
		"Space Daily", "Space TV", "Star World", "Moon Nation", "Planet Central", "Quiet Orbit", "Far Light", "Iron Moon"}
	resp := mustJSON(t, identityResponse(generic))
	g := &scriptedGen{respond: func(int, llm.Request) string { return resp }}
	_, err := fixedEngine(g).GenerateIdentity(context.Background(), strategyFixture())
	if !errors.Is(err, llm.ErrInvalidOutput) {
		t.Fatalf("expected invalid output error, got %v", err)
	}
}

func TestGenerateIdentityRequiresStrategy(t *testing.T) {
	ch := strategyFixture()
	ch.Strategy = nil
	_, err := fixedEngine(&scriptedGen{}).GenerateIdentity(context.Background(), ch)
	if !errors.Is(err, ErrPrecondition) {
		t.Fatalf("expected precondition error, got %v", err)
	}
}

func TestIsNearDuplicate(t *testing.T) {
	existing := []channel.ContentIdea{{Topic: "Voyager golden record", WorkingTitle: "The Secret Story of the Voyager Golden Record"}}
	if !IsNearDuplicate(channel.ContentIdea{Topic: "Other", WorkingTitle: "Secret Story of Voyager's Golden Record"}, existing) {
		t.Error("reworded title should be a near-duplicate")
	}
	if !IsNearDuplicate(channel.ContentIdea{Topic: "voyager golden record", WorkingTitle: "Completely different"}, existing) {
		t.Error("same topic should be a near-duplicate")
	}
	if IsNearDuplicate(channel.ContentIdea{Topic: "Apollo 13", WorkingTitle: "Apollo 13's Carbon Dioxide Fix"}, existing) {
		t.Error("distinct idea flagged as duplicate")
	}
}

func TestBuildCalendar(t *testing.T) {
	sched := channel.PublishingSchedule{Days: []string{"Tuesday", "Friday"}, LocalTime: "17:00", TimeZone: "America/New_York"}
	// 2026-11-10 is a Tuesday; scheduling starts the next day.
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	cal := BuildCalendar(4, sched, now)
	want := []string{"2026-11-13", "2026-11-17", "2026-11-20", "2026-11-24"}
	if len(cal) != len(want) {
		t.Fatalf("got %d entries", len(cal))
	}
	for i, e := range cal {
		if e.Date != want[i] || e.IdeaIndex != i || e.LocalTime != "17:00" {
			t.Errorf("entry %d = %+v, want date %s", i, e, want[i])
		}
	}
	if BuildCalendar(3, channel.PublishingSchedule{}, now) != nil {
		t.Error("expected no calendar without publishing days")
	}
}

var ideaWords = []string{"Apollo", "Voyager", "Hubble", "Cassini", "Juno", "Kepler", "Gemini", "Mercury", "Skylab", "Mir",
	"Pioneer", "Viking", "Galileo", "Rosetta", "Dawn", "Spitzer", "Chandra", "Ulysses", "Magellan", "Huygens",
	"Ariel", "Luna", "Vostok", "Salyut", "Giotto", "Hayabusa", "Akatsuki", "Phoenix", "Opportunity", "Spirit",
	"Curiosity", "Perseverance", "Ingenuity", "Tianwen", "Chandrayaan"}

func TestGenerateBlueprint(t *testing.T) {
	plan := map[string]any{
		"priorityTopics": []string{"t1", "t2", "t3", "t4", "t5", "t6", "t7", "t8", "t9", "t10"},
		"series": []any{
			map[string]any{"name": "s1", "description": "d", "cadence": "weekly"},
			map[string]any{"name": "s2", "description": "d", "cadence": "weekly"},
			map[string]any{"name": "s3", "description": "d", "cadence": "weekly"},
			map[string]any{"name": "s4", "description": "d", "cadence": "weekly"},
			map[string]any{"name": "s5", "description": "d", "cadence": "weekly"},
		},
		"formats": []any{
			map[string]any{"name": "Deep Dive", "description": "d", "aspectRatio": "16:9", "durationSeconds": 600},
			map[string]any{"name": "Short", "description": "d", "aspectRatio": "9:16", "durationSeconds": 45},
			map[string]any{"name": "Explainer", "description": "d", "aspectRatio": "16:9", "durationSeconds": 300},
		},
		"hooks": []string{"h1", "h2", "h3", "h4", "h5", "h6"},
	}
	next := 0
	g := &scriptedGen{respond: func(call int, _ llm.Request) string {
		if call == 1 {
			return mustJSON(t, plan)
		}
		ideas := make([]any, 0, 10)
		for range 10 {
			w := ideaWords[next%len(ideaWords)]
			// Every third idea of the first batch repeats an earlier title to
			// exercise duplicate filtering.
			if call == 2 && next%3 == 2 {
				w = ideaWords[0]
			} else {
				next++
			}
			ideas = append(ideas, map[string]any{
				"topic": w + " mission", "workingTitle": "The " + w + " Story", "targetAudience": "a",
				"angle": "a", "whyInteresting": "w", "researchSources": []string{"NASA archives"},
				"originalityNotes": "o", "suggestedDurationSeconds": 9999, "format": "Short",
			})
		}
		return mustJSON(t, map[string]any{"ideas": ideas})
	}}
	bp, err := fixedEngine(g).GenerateBlueprint(context.Background(), strategyFixture())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bp.Ideas) != targetIdeas || len(bp.Calendar) != targetIdeas {
		t.Fatalf("ideas=%d calendar=%d, want %d", len(bp.Ideas), len(bp.Calendar), targetIdeas)
	}
	for i, a := range bp.Ideas {
		if a.SuggestedDurationSecs != 3600 {
			t.Fatalf("duration not clamped: %d", a.SuggestedDurationSecs)
		}
		if IsNearDuplicate(a, bp.Ideas[:i]) {
			t.Fatalf("duplicate idea kept: %s", a.WorkingTitle)
		}
	}
	if g.calls < 5 {
		t.Fatalf("expected extra batch after duplicates were dropped, got %d calls", g.calls)
	}
	if !strings.Contains(g.prompts[2], "The Apollo Story") {
		t.Fatal("later batches must include already planned titles")
	}
}

func TestGenerateBlueprintFailsWhenIdeasRepeat(t *testing.T) {
	plan := `{"priorityTopics":["a","b","c","d","e","f","g","h","i","j"],"series":[],"formats":[{"name":"Short","description":"d","aspectRatio":"9:16","durationSeconds":45}],"hooks":[]}`
	g := &scriptedGen{respond: func(call int, _ llm.Request) string {
		if call == 1 {
			return plan
		}
		ideas := make([]any, 0, 10)
		for range 10 {
			ideas = append(ideas, map[string]any{
				"topic": "same", "workingTitle": "Same Title", "targetAudience": "a", "angle": "a",
				"whyInteresting": "w", "researchSources": []string{"s"}, "originalityNotes": "o",
				"suggestedDurationSeconds": 60, "format": "Short",
			})
		}
		return mustJSON(t, map[string]any{"ideas": ideas})
	}}
	_, err := fixedEngine(g).GenerateBlueprint(context.Background(), strategyFixture())
	if !errors.Is(err, llm.ErrInvalidOutput) {
		t.Fatalf("expected invalid output error, got %v", err)
	}
	if g.calls != 1+maxIdeaBatches {
		t.Fatalf("expected %d calls, got %d", 1+maxIdeaBatches, g.calls)
	}
}

type fakeStore struct {
	ch       *channel.Channel
	strategy *channel.Strategy
}

func (f *fakeStore) Get(_ context.Context, owner, id string) (*channel.Channel, error) {
	if f.ch == nil || f.ch.OwnerID != owner || f.ch.ID != id {
		return nil, channel.ErrNotFound
	}
	c := *f.ch
	return &c, nil
}
func (f *fakeStore) SetStrategy(_ context.Context, _, _ string, s channel.Strategy) (*channel.Channel, error) {
	f.strategy = &s
	return f.ch, nil
}
func (f *fakeStore) SetIdentity(context.Context, string, string, channel.Identity) (*channel.Channel, error) {
	return f.ch, nil
}
func (f *fakeStore) SetBlueprint(context.Context, string, string, channel.Blueprint) (*channel.Channel, error) {
	return f.ch, nil
}

func TestStrategyJobHandler(t *testing.T) {
	resp := mustJSON(t, map[string]any{
		"targetAudience": "Curious adults 25-45", "audienceGeography": "United States",
		"positioning": "p", "editorialIdentity": "e", "contentPillars": []string{"a", "b", "c"},
		"videoFormats": []any{
			map[string]any{"name": "Long", "description": "d", "aspectRatio": "16:9", "durationSeconds": 600},
			map[string]any{"name": "Short", "description": "d", "aspectRatio": "9:16", "durationSeconds": 45},
		},
		"publishingDays":       []string{"Friday", "Tuesday", "Friday"},
		"publishingLocalTime":  "17:30",
		"competitorReferences": []string{},
		"monetizationOptions":  []string{"Ads", "Sponsorships"},
	})
	store := &fakeStore{ch: &channel.Channel{ID: "ch1", OwnerID: "alice", Niche: "Space", Language: "en-US", GeoTarget: "US"}}
	h := fixedEngine(&scriptedGen{responses: []string{resp}}).Handlers(store)[KindGenerateStrategy]

	res, err := h(context.Background(), jobs.Job{OwnerID: "alice", ChannelID: "ch1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.(ChannelJobResult).ChannelID != "ch1" {
		t.Fatalf("unexpected result %+v", res)
	}
	pf := store.strategy.PublishingFrequency
	if pf.TimeZone != "America/New_York" || pf.VideosPerWeek != 2 || strings.Join(pf.Days, ",") != "Tuesday,Friday" {
		t.Fatalf("unexpected schedule: %+v", pf)
	}
	if store.strategy.CompetitorNote == "" {
		t.Fatal("competitor references must carry an unverified note")
	}

	_, err = h(context.Background(), jobs.Job{OwnerID: "bob", ChannelID: "ch1"})
	if !jobs.IsPermanent(err) || !errors.Is(err, channel.ErrNotFound) {
		t.Fatalf("expected permanent not-found for other owner, got %v", err)
	}
}

func TestClassify(t *testing.T) {
	if jobs.IsPermanent(classify(fmt.Errorf("x: %w", llm.ErrUnavailable))) {
		t.Error("unavailable model should be retryable")
	}
	if jobs.IsPermanent(classify(errors.New("connection reset"))) {
		t.Error("unknown errors should be retryable")
	}
	if !jobs.IsPermanent(classify(&channel.ValidationError{Field: "f", Message: "m"})) {
		t.Error("validation errors should be permanent")
	}
	if !jobs.IsPermanent(classify(fmt.Errorf("%w: missing", ErrPrecondition))) {
		t.Error("precondition errors should be permanent")
	}
}
func (f *fakeStore) UpdateSettings(_ context.Context, _, _ string, u channel.SettingsUpdate) (*channel.Channel, error) {
	if u.Name != nil {
		f.ch.Name = *u.Name
	}
	if u.Niche != nil {
		f.ch.Niche = *u.Niche
	}
	return f.ch, nil
}

func TestGenerateBlueprintAcceptsAPartialPlan(t *testing.T) {
	plan := map[string]any{
		"priorityTopics": []string{"t1", "t2", "t3"},
		"series":         []any{map[string]any{"name": "s1", "description": "d", "cadence": "weekly"}},
		"formats":        []any{map[string]any{"name": "Short", "description": "d", "aspectRatio": "9:16", "durationSeconds": 45}},
		"hooks":          []string{"h1"},
	}
	// Each batch adds 4 new ideas and repeats the rest: 20 after 5 batches,
	// below the target of 30 but above the minimum.
	next := 0
	g := &scriptedGen{respond: func(call int, _ llm.Request) string {
		if call == 1 {
			return mustJSON(t, plan)
		}
		ideas := make([]any, 0, 10)
		for k := range 10 {
			w := ideaWords[0]
			if k < 4 {
				w = ideaWords[next%len(ideaWords)]
				next++
			}
			ideas = append(ideas, map[string]any{
				"topic": w + " mission", "workingTitle": "The " + w + " Story", "targetAudience": "a",
				"angle": "a", "whyInteresting": "w", "researchSources": []string{"archives"},
				"originalityNotes": "o", "suggestedDurationSeconds": 60, "format": "Short",
			})
		}
		return mustJSON(t, map[string]any{"ideas": ideas})
	}}
	bp, err := fixedEngine(g).GenerateBlueprint(context.Background(), strategyFixture())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n := len(bp.Ideas); n < minIdeas || n >= targetIdeas {
		t.Fatalf("ideas = %d, want a partial plan between %d and %d", n, minIdeas, targetIdeas)
	}
}
