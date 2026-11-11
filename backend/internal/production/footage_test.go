package production

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"contentfactory/internal/stock"
)

type fakeFootage struct {
	results  map[string][]stock.Clip
	err      error
	searches []string
}

func (f *fakeFootage) Available() bool { return true }
func (f *fakeFootage) Search(_ context.Context, q, orientation string, _, _, _ int) ([]stock.Clip, error) {
	f.searches = append(f.searches, orientation+":"+q)
	if f.err != nil {
		return nil, f.err
	}
	return f.results[q], nil
}
func (f *fakeFootage) Download(_ context.Context, c stock.Clip, dir string) (string, error) {
	return dir + "/clip.mp4", nil
}

func TestShotLengthsAddUp(t *testing.T) {
	for _, d := range []float64{0.8, 3.5, 7.2, 12.9} {
		sum := 0.0
		for _, l := range shotLengths(d) {
			sum += l
			if l > ShotSeconds*1.5 {
				t.Errorf("d=%v: shot %v too long", d, l)
			}
		}
		if math.Abs(sum-d) > 1e-9 {
			t.Errorf("d=%v: shots sum to %v", d, sum)
		}
	}
	if shotLengths(0) != nil {
		t.Error("zero duration should give no shots")
	}
}

func TestCleanAndFallbackQueries(t *testing.T) {
	if got := cleanQuery("  Moon's SURFACE, close-up!! at night in the dark sky "); got != "moon s surface close up at" {
		t.Errorf("cleanQuery = %q", got)
	}
	if got := fallbackQuery(Timed{Text: "Tides move. More text."}, "space"); got != "tides move" {
		t.Errorf("fallback = %q", got)
	}
	if got := fallbackQuery(Timed{}, ""); got != "nature landscape" {
		t.Errorf("empty fallback = %q", got)
	}
}

func TestFootageQueriesUseModelThenFallback(t *testing.T) {
	gen := &scriptedGen{responses: []string{`{"segments":[{"queries":["Full Moon rising","ocean tides"]},{"queries":["!!"]}]}`}}
	svc, _ := newTestService(t, gen, nil, Tools{})
	got := svc.footageQueries(context.Background(), []Timed{{Text: "The Moon drifts."}, {Title: "Tidal push", Text: "x"}}, "space")
	if strings.Join(got[0], "|") != "full moon rising|ocean tides|the moon drifts" || strings.Join(got[1], "|") != "tidal push" {
		t.Fatalf("queries = %q", got)
	}
	// A failing model still yields one query per segment.
	svc2, _ := newTestService(t, &scriptedGen{}, nil, Tools{})
	if q := svc2.footageQueries(context.Background(), []Timed{{Text: "Waves crash."}}, "ocean"); len(q) != 1 || q[0][0] != "waves crash" {
		t.Fatalf("fallback queries = %q", q)
	}
}

func TestPlanFootageAvoidsReuseAndCoversDuration(t *testing.T) {
	clips := []stock.Clip{{ID: 1, Duration: 10}, {ID: 2, Duration: 2}, {ID: 3, Duration: 8}}
	f := &fakeFootage{results: map[string][]stock.Clip{"moon": clips}}
	svc, _ := newTestService(t, &scriptedGen{}, nil, Tools{})
	svc.WithFootage(f)
	segs := []Timed{{Duration: 7}, {Duration: 3.5}}
	shots, err := svc.planFootage(context.Background(), segs, [][]string{{"moon"}, {"moon"}}, 1080, 1920, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(shots) != 3 {
		t.Fatalf("shots = %d, want 3", len(shots))
	}
	// Long-enough clips first, without reuse while unused clips remain.
	if shots[0].Clip.ID != 1 || shots[1].Clip.ID != 3 || shots[2].Clip.ID != 2 {
		t.Errorf("clip order = %d %d %d", shots[0].Clip.ID, shots[1].Clip.ID, shots[2].Clip.ID)
	}
	total := 0.0
	for _, s := range shots {
		total += s.Length
		if s.Offset < 0 || (s.Clip.Duration > s.Length && s.Offset+s.Length > s.Clip.Duration) {
			t.Errorf("shot runs past its clip: %+v", s)
		}
	}
	if math.Abs(total-(10.5+0.6)) > 1e-9 {
		t.Errorf("total = %v, want the narration plus the tail pad", total)
	}
	if len(f.searches) != 1 || f.searches[0] != "portrait:moon" {
		t.Errorf("searches = %v (repeated queries must be cached)", f.searches)
	}
	if credits := FootageCredits(shots); len(credits) != 3 {
		t.Errorf("credits = %+v", credits)
	}
}

func TestPlanFootageErrors(t *testing.T) {
	svc, _ := newTestService(t, &scriptedGen{}, nil, Tools{})
	svc.WithFootage(&fakeFootage{err: stock.ErrRateLimited})
	if _, err := svc.planFootage(context.Background(), []Timed{{Duration: 3}}, [][]string{{"x"}}, 1920, 1080, t.TempDir()); !errors.Is(err, stock.ErrRateLimited) {
		t.Fatalf("rate limit: %v", err)
	}
	svc.WithFootage(&fakeFootage{results: map[string][]stock.Clip{}})
	if _, err := svc.planFootage(context.Background(), []Timed{{Duration: 3}}, [][]string{{"x"}}, 1920, 1080, t.TempDir()); err == nil {
		t.Fatal("expected an error when nothing is found")
	}
}

func TestShotArgsFillFrame(t *testing.T) {
	args := strings.Join(shotArgs(FootageShot{Path: "c.mp4", Offset: 1, Length: 3.5}, 1080, 1920, "s.mp4"), " ")
	for _, want := range []string{"-stream_loop -1 -ss 1.000 -t 3.500 -i c.mp4 -an", "force_original_aspect_ratio=increase,crop=1080:1920", "fps=30", "s.mp4"} {
		if !strings.Contains(args, want) {
			t.Errorf("args missing %q: %s", want, args)
		}
	}
}

func TestBuildWordASS(t *testing.T) {
	ass := BuildWordASS([]CaptionChunk{
		{Start: 0, End: 0.5, Words: []CaptionWord{{Text: "{\\b1}Hi"}}},
		{Start: 0.5, End: 1, Words: []CaptionWord{{Text: "1,200"}}, Hot: true},
	}, 1080, 1920)
	if strings.Contains(ass, "{\\b1}Hi") {
		t.Error("override tags in caption text were not escaped")
	}
	for _, want := range []string{"Style: Word,Inter,135", `\pos(540,921)`, `\c&H004DE1FF&`, "Dialogue: 0,0:00:00.50,0:00:01.00,Word"} {
		if !strings.Contains(ass, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestPlanEffectsFootageWhooshesOnCuts(t *testing.T) {
	scenes := []Scene{{Start: 0, Duration: 3}, {Kind: SceneNumber, Start: 3, Duration: 4}, {Start: 7, Duration: 2}}
	cues := PlanEffects(scenes, StyleFootage)
	if len(cues) != 3 || cues[0].Kind != SFXImpact || cues[1].Kind != SFXWhoosh || cues[2].Kind != SFXWhoosh {
		t.Fatalf("cues = %+v (no counter ticks without counters)", cues)
	}
}
