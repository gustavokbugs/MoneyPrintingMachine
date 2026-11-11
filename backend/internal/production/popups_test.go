package production

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"contentfactory/internal/stock"
)

// fakeImages adds photo search to fakeFootage.
type fakeImages struct {
	fakeFootage
	images map[string][]stock.Clip
	err    error
}

func (f *fakeImages) SearchImages(_ context.Context, q string, _, _, _ int) ([]stock.Clip, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.images[q], nil
}

func TestKeywordTime(t *testing.T) {
	seg := Timed{Duration: 4, Text: "You prank your friends.", Words: []float64{0, 0.3, 0.8, 1.2}}
	if got := keywordTime(seg, "friends"); got != 1.2 {
		t.Errorf("measured = %v, want 1.2", got)
	}
	est := Timed{Duration: 4, Text: "You prank your friends."}
	if got := keywordTime(est, "your"); got != 2 {
		t.Errorf("estimated = %v, want 2", got)
	}
	if got := keywordTime(est, "virus"); got != 0.1 {
		t.Errorf("missing word = %v", got)
	}
}

func TestPopMomentsAvoidOverlap(t *testing.T) {
	segs := []Timed{{Duration: 2, Text: "a b c d"}, {Duration: 0.6, Text: "x"}, {Duration: 3, Text: "y"}}
	got := popMoments(segs, []popPlan{{Keyword: "a", Query: "qa"}, {Keyword: "x", Query: "qx"}, {Keyword: "y", Query: "qy"}}, nil)
	// Pop 2 starts 0.6 s before pop 3, so trimming leaves it too short.
	if len(got) != 2 || got[0].Query != "qa" || got[0].End != popSeconds || got[1].Query != "qy" {
		t.Fatalf("moments = %+v", got)
	}
}

func TestEverySoundGetsAPicture(t *testing.T) {
	segs := []Timed{{Duration: 4, Text: "Your house is burning down fast"}, {Duration: 4, Text: "then it explodes"}}
	plans := []popPlan{{Keyword: "house", Query: "house"}, {Keyword: "then", Query: "then"}}
	sounds := []SFXCue{{At: 0.05, Kind: SFXImpact}, {At: 2, Kind: SFXFire, Word: "burning"}, {At: 6.5, Kind: SFXImpact, Word: "explodes"}}
	got := popMoments(segs, plans, sounds)
	// Scene effects (no word) get no picture; "house" (estimated at
	// 0.67 s) stays, "then" at 4 s is far enough from the explosion too.
	var queries []string
	for _, m := range got {
		queries = append(queries, m.Query)
	}
	if strings.Join(queries, ",") != "house,campfire,then,explosion" {
		t.Fatalf("queries = %v", queries)
	}
	// A key word within a second of a sound loses to the sound.
	got = popMoments(segs, []popPlan{{Keyword: "burning", Query: "house"}, {}}, sounds)
	if len(got) != 2 || got[0].Query != "campfire" {
		t.Fatalf("near moments = %+v", got)
	}
	if q := sfxQuery(SFXCue{Kind: SFXHiss, Word: "melted"}); q != "melted" {
		t.Errorf("hiss query = %q", q)
	}
}

func TestPlanImagePops(t *testing.T) {
	gen := &scriptedGen{responses: []string{`{"segments":[{"keyword":"Friends","problem":"prank","query":"friends laughing"},{"keyword":"nope","problem":"absent","query":"computer virus"}]}`}}
	svc, _ := newTestService(t, gen, nil, Tools{})
	img := func(id int64) stock.Clip {
		return stock.Clip{Provider: stock.ProviderCommons, ID: id, Image: true, FileURL: fmt.Sprintf("https://upload.wikimedia.org/%d.jpg", id)}
	}
	f := &fakeImages{images: map[string][]stock.Clip{
		"friends laughing": {img(1), img(2)},
		"computer virus":   {img(1), {Provider: stock.ProviderPexels, ID: 9}, img(3)},
	}}
	svc.WithFootage(f)
	segs := []Timed{{Duration: 5, Text: "You prank your friends."}, {Duration: 5, Text: "Congrats, a virus."}}
	plans := svc.planPops(context.Background(), segs)
	if plans[0].Problem != "prank" || plans[1].Problem != "" || plans[1].Keyword != "congrats" {
		t.Errorf("plans = %+v (problem words must come from the text)", plans)
	}
	pops, err := svc.planImagePops(context.Background(), segs, plans, nil, 1080, 1920)
	if err != nil || len(pops) != 2 {
		t.Fatalf("pops=%+v err=%v", pops, err)
	}
	// Segment 2's keyword is not in its text, so the longest word is used;
	// clip 1 is already used and clip 9 is a video, so clip 3 is picked.
	if pops[0].Clip.ID != 1 || pops[1].Clip.ID != 3 {
		t.Errorf("clips = %d, %d", pops[0].Clip.ID, pops[1].Clip.ID)
	}
	// "friends" is word 4 of 4 (estimated 3.75 s); pop 1 is trimmed to end
	// before "congrats" (word 1 of segment 2, at 5 s).
	if pops[0].Start != 3.75 || pops[1].Start != 5 || pops[0].End != 4.95 {
		t.Errorf("pops = %+v", pops)
	}
	if len(PopCredits(pops)) != 2 {
		t.Error("every pop needs a credit")
	}

	// A provider error with nothing found is reported; no searcher, no pops.
	f.err = fmt.Errorf("search: %w", stock.ErrRateLimited)
	if _, err := svc.planImagePops(context.Background(), segs, plans, nil, 1080, 1920); !errors.Is(err, stock.ErrRateLimited) {
		t.Errorf("err = %v", err)
	}
	svc.WithFootage(&fakeFootage{})
	if pops, err := svc.planImagePops(context.Background(), segs, plans, nil, 1080, 1920); pops != nil || err != nil {
		t.Errorf("without image search: %v %v", pops, err)
	}
}

func TestWriteProjectIncludesPops(t *testing.T) {
	root := t.TempDir()
	gsap := filepath.Join(root, "gsap.min.js")
	_ = os.WriteFile(gsap, []byte("/*gsap*/"), 0o644)
	fonts := filepath.Join(root, "fontsource")
	for _, f := range projectFonts {
		p := filepath.Join(fonts, f.pkg, "files", f.file)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte("font"), 0o644)
	}
	photo := filepath.Join(root, "photo.jpg")
	_ = os.WriteFile(photo, []byte("jpeg"), 0o644)
	segs := []Timed{{Duration: 3, Text: "You prank your friends."}}
	in := ProjectInput{Width: 1080, Height: 1920, Style: StyleGameplay, Scenes: PlanScenes(segs, nil, "Q"), Captions: WordCaptions(segs),
		Pops: []ImagePop{{Path: photo, Start: 1, End: 2.8}, {Path: filepath.Join(root, "evil.svg"), Start: 3, End: 4}}}
	dir := filepath.Join(root, "project")
	if err := WriteProject(dir, in, gsap, fonts); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	html := string(b)
	if !strings.Contains(html, `<img src="assets/pop-0.jpg"`) || strings.Contains(html, "pop-1") {
		t.Errorf("pops not written as expected")
	}
	if _, err := os.Stat(filepath.Join(dir, "assets", "pop-0.jpg")); err != nil {
		t.Error("pop image not copied into the project")
	}
}

func TestMarkCaptions(t *testing.T) {
	chunks := []CaptionChunk{{Start: 0, End: 0.5, Hot: true}, {Start: 0.5, End: 1}, {Start: 1, End: 1.5}}
	got := MarkCaptions(chunks, []float64{0.52}, []float64{1.0})
	if got[0].Hot || got[0].Problem {
		t.Error("old highlights must be cleared")
	}
	if !got[1].Hot || got[1].Problem || !got[2].Problem || got[2].Hot {
		t.Errorf("marks = %+v", got)
	}
	if !chunks[0].Hot {
		t.Error("MarkCaptions must not modify its input")
	}
	fixes, problems := captionMarks([]Timed{{Duration: 2, Text: "it cracks so clay"}, {Duration: 2, Text: "done"}},
		[]popPlan{{Keyword: "clay", Problem: "cracks"}, {Keyword: "done"}})
	if len(fixes) != 2 || fixes[0] != 1.5 || fixes[1] != 2 || len(problems) != 1 || problems[0] != 0.5 {
		t.Errorf("fixes=%v problems=%v", fixes, problems)
	}
}
