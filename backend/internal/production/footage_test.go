package production

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"contentfactory/internal/stock"
)

type fakeFootage struct {
	results  map[string][]stock.Clip
	err      error
	searches []string
	// limitAfter rate-limits every search after this many (0 = never).
	limitAfter int
	// failDownload makes downloads of these clip IDs fail with the error.
	failDownload map[int64]error
}

func (f *fakeFootage) Available() bool { return true }
func (f *fakeFootage) Search(_ context.Context, q, orientation string, _, _, _ int) ([]stock.Clip, error) {
	f.searches = append(f.searches, orientation+":"+q)
	if f.err != nil {
		return nil, f.err
	}
	if f.limitAfter > 0 && len(f.searches) > f.limitAfter {
		return nil, stock.ErrRateLimited
	}
	return f.results[q], nil
}
func (f *fakeFootage) Download(_ context.Context, c stock.Clip, dir string) (string, error) {
	if err := f.failDownload[c.ID]; err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/clip-%d.mp4", dir, c.ID), nil
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
	args := strings.Join(shotArgs(FootageShot{Path: "c.mp4", Offset: 1, Length: 3.5}, 0, 1080, 1920, "s.mp4"), " ")
	for _, want := range []string{"-stream_loop -1 -ss 1.000 -t 3.500 -i c.mp4 -an", "force_original_aspect_ratio=increase,crop=1080:1920", "fps=30", "format=yuv420p", "s.mp4"} {
		if !strings.Contains(args, want) {
			t.Errorf("args missing %q: %s", want, args)
		}
	}
}

func TestShotArgsPanPhotos(t *testing.T) {
	shot := FootageShot{Clip: stock.Clip{Image: true}, Path: "p.jpg", Length: 4}
	even := strings.Join(shotArgs(shot, 0, 1080, 1920, "s.mp4"), " ")
	odd := strings.Join(shotArgs(shot, 1, 1080, 1920, "s.mp4"), " ")
	for _, want := range []string{"-loop 1 -framerate 30 -t 4.000 -i p.jpg", "scale=1242:2208:force_original_aspect_ratio=increase,crop=1080:1920:x='(iw-ow)*t/4.000'"} {
		if !strings.Contains(even, want) {
			t.Errorf("missing %q: %s", want, even)
		}
	}
	if !strings.Contains(odd, "x='(iw-ow)*(1-t/4.000)'") {
		t.Errorf("odd shots should pan the other way: %s", odd)
	}
}

func TestFootageLabel(t *testing.T) {
	if FootageLabel([]stock.Clip{{Provider: stock.ProviderCommons}}) != "footage:commons" ||
		FootageLabel([]stock.Clip{{Provider: stock.ProviderPexels}, {Provider: stock.ProviderCommons}}) != "footage:mixed" ||
		FootageLabel([]stock.Clip{{Provider: stock.ProviderPexels}}) != "footage:pexels" {
		t.Error("labels wrong")
	}
}

func TestPickClipTreatsPhotosAsAnyLength(t *testing.T) {
	pool := []stock.Clip{{Provider: "pexels", ID: 1, Duration: 1}, {Provider: "commons", ID: 1, Image: true}}
	c, ok := pickClip(pool, map[string]bool{}, 4)
	if !ok || c.Provider != "commons" {
		t.Fatalf("picked %+v", c)
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

// A segment whose queries find nothing borrows clips found for other
// segments instead of failing the whole video.
func TestPlanFootageBorrowsForEmptySegments(t *testing.T) {
	f := &fakeFootage{results: map[string][]stock.Clip{
		"petri dish": {{ID: 1, Duration: 10}},
	}}
	svc, _ := newTestService(t, &scriptedGen{}, nil, Tools{})
	svc.WithFootage(f)
	segs := []Timed{{Duration: 3}, {Duration: 3}, {Duration: 3}}
	queries := [][]string{{"1920s london lab bench"}, {"petri dish"}, {"bacteria"}}
	shots, err := svc.planFootage(context.Background(), segs, queries, 1080, 1920, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(shots) != 3 {
		t.Fatalf("shots = %d, want 3", len(shots))
	}
	if shots[1].Clip.ID != 1 {
		t.Errorf("segment 2 should use its own query, got clip %d", shots[1].Clip.ID)
	}
	if shots[0].Clip.ID == 0 || shots[2].Clip.ID == 0 {
		t.Errorf("segments without results must still get a clip: %+v", shots)
	}
}

// A rate limit midway keeps the clips already found for the video.
func TestPlanFootageKeepsResultsAfterRateLimit(t *testing.T) {
	f := &fakeFootage{results: map[string][]stock.Clip{"mold": {{ID: 1, Duration: 10}, {ID: 2, Duration: 10}}}, limitAfter: 1}
	svc, _ := newTestService(t, &scriptedGen{}, nil, Tools{})
	svc.WithFootage(f)
	shots, err := svc.planFootage(context.Background(), []Timed{{Duration: 3}, {Duration: 3}}, [][]string{{"mold"}, {"war"}}, 1080, 1920, t.TempDir())
	if err != nil || len(shots) != 2 {
		t.Fatalf("shots=%d err=%v, want both segments covered", len(shots), err)
	}
	if len(f.searches) != 2 {
		t.Errorf("searches = %v, want searching to stop at the first rate limit", f.searches)
	}
}

// Failed downloads try other clips, then reuse downloaded ones; only a
// video with no downloadable clip fails.
func TestPlanFootageSurvivesFailedDownloads(t *testing.T) {
	clips := []stock.Clip{{ID: 1, Duration: 10}, {ID: 2, Duration: 10}, {ID: 3, Duration: 10}}
	f := &fakeFootage{results: map[string][]stock.Clip{"mold": clips}, failDownload: map[int64]error{
		1: fmt.Errorf("download clip 1: %w", stock.ErrRateLimited), 3: errors.New("HTTP 404")}}
	svc, _ := newTestService(t, &scriptedGen{}, nil, Tools{})
	svc.WithFootage(f)
	shots, err := svc.planFootage(context.Background(), []Timed{{Duration: 3}, {Duration: 3}, {Duration: 3}}, [][]string{{"mold"}, {"mold"}, {"mold"}}, 1080, 1920, t.TempDir())
	if err != nil || len(shots) != 3 {
		t.Fatalf("shots=%d err=%v", len(shots), err)
	}
	for i, s := range shots {
		if s.Clip.ID != 2 || s.Path == "" {
			t.Errorf("shot %d = clip %d %q, want the one downloadable clip", i, s.Clip.ID, s.Path)
		}
	}
	f.failDownload[2] = errors.New("HTTP 500")
	if _, err := svc.planFootage(context.Background(), []Timed{{Duration: 3}}, [][]string{{"mold"}}, 1080, 1920, t.TempDir()); err == nil {
		t.Fatal("expected an error when no clip downloads")
	}
}

func TestPlanFootageSkipsBlockedClips(t *testing.T) {
	f := &fakeFootage{results: map[string][]stock.Clip{"mold": {
		{Provider: stock.ProviderCommons, ID: 1, Duration: 10}, {Provider: stock.ProviderCommons, ID: 2, Duration: 10}}}}
	svc, _ := newTestService(t, &scriptedGen{}, nil, Tools{})
	svc.WithFootage(f)
	list := "# misread symbols\nCOMMONS:1  # cake mold\n\n"
	if err := os.WriteFile(filepath.Join(svc.mediaDir, BlocklistFile), []byte(list), 0o644); err != nil {
		t.Fatal(err)
	}
	shots, err := svc.planFootage(context.Background(), []Timed{{Duration: 3}, {Duration: 3}}, [][]string{{"mold"}, {"mold"}}, 1080, 1920, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range shots {
		if s.Clip.ID == 1 {
			t.Fatalf("blocked clip used: %+v", shots)
		}
	}
}

// Once downloads are rate-limited, clips saved by earlier renders are used.
func TestPlanFootageUsesCachedClipsWhenRateLimited(t *testing.T) {
	limited := fmt.Errorf("download: %w", stock.ErrRateLimited)
	clip := func(id int64) stock.Clip {
		return stock.Clip{Provider: stock.ProviderCommons, ID: id, Image: true, Duration: 10, FileURL: fmt.Sprintf("https://upload.wikimedia.org/%d.jpg", id)}
	}
	pool := []stock.Clip{clip(1), clip(2), clip(3), clip(4), clip(5)}
	f := &fakeFootage{results: map[string][]stock.Clip{"mold": pool}, failDownload: map[int64]error{1: limited, 2: limited, 3: limited, 4: limited, 5: limited}}
	svc, _ := newTestService(t, &scriptedGen{}, nil, Tools{})
	svc.WithFootage(f)
	cache := t.TempDir()
	if err := os.WriteFile(filepath.Join(cache, "commons-5-0.jpg"), []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	if p, ok := stock.Cached(clip(5), cache); !ok || p == "" {
		t.Fatalf("cache naming changed: %q %v", p, ok)
	}
	shots, err := svc.planFootage(context.Background(), []Timed{{Duration: 3}, {Duration: 3}}, [][]string{{"mold"}, {"mold"}}, 1080, 1920, cache)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range shots {
		if s.Clip.ID != 5 {
			t.Errorf("shot uses clip %d, want the cached clip 5", s.Clip.ID)
		}
	}
}
