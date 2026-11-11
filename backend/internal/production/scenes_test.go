package production

import (
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestResolveStyle(t *testing.T) {
	cases := map[[2]string]string{
		{"auto", "Space and Astronomy"}:     StyleTech,
		{"auto", "Ancient History Stories"}: StyleDocumentary,
		{"auto", "Personal Finance"}:        StyleBold,
		{"auto", "Cooking"}:                 StyleMinimal,
		{"documentary", "Space"}:            StyleDocumentary,
		{"unknown", "Space"}:                StyleTech,
		{"", "Psychology of everyday tech"}: StyleTech,
	}
	for in, want := range cases {
		if got := ResolveStyle(in[0], in[1]); got != want {
			t.Errorf("ResolveStyle(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}

func TestParseNumber(t *testing.T) {
	v, d, p, s, ok := parseNumber("3.8 cm per year")
	if !ok || v != 3.8 || d != 1 || p != "" || s != " cm per year" {
		t.Fatalf("got %v %d %q %q %v", v, d, p, s, ok)
	}
	v, d, p, s, ok = parseNumber("Over 1,200 satellites")
	if !ok || v != 1200 || d != 0 || p != "Over " || s != " satellites" {
		t.Fatalf("got %v %d %q %q %v", v, d, p, s, ok)
	}
	if _, _, _, _, ok := parseNumber("A slow escape"); ok {
		t.Fatal("text without numbers is not a number scene")
	}
}

func TestPlanScenes(t *testing.T) {
	segs := []Timed{
		{Duration: 3, Text: "The Moon is leaving us. Here is why."},
		{Duration: 10, Title: "3.8 cm per year", Text: "..."},
		{Duration: 8, Title: "Tidal Pull", Text: "..."},
		{Duration: 4, Text: "Subscribe for more."},
	}
	sc := PlanScenes(segs, []string{"", "The drift", "", ""}, "Quiet Orbit")
	kinds := []string{sc[0].Kind, sc[1].Kind, sc[2].Kind, sc[3].Kind}
	if strings.Join(kinds, ",") != "title,number,statement,outro" {
		t.Fatalf("kinds = %v", kinds)
	}
	if sc[0].Headline != "The Moon is leaving us." || sc[1].Kicker != "The drift" || sc[3].Headline != "Quiet Orbit" {
		t.Fatalf("scenes = %+v", sc)
	}
	if sc[2].Start != 13 || sc[3].Start != 21 {
		t.Fatalf("starts: %v %v", sc[2].Start, sc[3].Start)
	}
}

func TestWriteProjectEscapesContent(t *testing.T) {
	root := t.TempDir()
	gsap := filepath.Join(root, "gsap.min.js")
	_ = os.WriteFile(gsap, []byte("/*gsap*/"), 0o644)
	fonts := filepath.Join(root, "fontsource")
	for _, f := range projectFonts {
		p := filepath.Join(fonts, f.pkg, "files", f.file)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte("font"), 0o644)
	}
	dir := filepath.Join(root, "project")
	in := ProjectInput{
		Width: 1080, Height: 1920, Style: StyleTech,
		Palette:   Style{Background: "#0F172A", Text: "#F8FAFC", Accent: "red;}</style><script>alert(1)</script>"},
		Secondary: "#1E3A8A",
		Scenes: PlanScenes([]Timed{
			{Duration: 3, Title: `<img src=x onerror=alert(1)>`, Text: "x"},
			{Duration: 2, Title: "42 </script><script>alert(2)</script>", Text: "y"},
		}, nil, "Quiet Orbit"),
	}
	if err := WriteProject(dir, in, gsap, fonts); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(b)
	for _, bad := range []string{"<img src=x", "alert(1)</script>", "<script>alert(2)", "red;}"} {
		if strings.Contains(html, bad) {
			t.Errorf("unescaped content %q in project", bad)
		}
	}
	for _, want := range []string{`data-composition-id="main"`, `data-duration="5.000"`, `--accent: #60A5FA`, `class="style-tech"`, `window.__timelines["main"]`, `"kind":"number"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "assets", "gsap.min.js")); err != nil {
		t.Fatal("gsap not vendored into the project")
	}
}

func TestEmphasisIndex(t *testing.T) {
	cases := []struct {
		words []string
		want  int
	}{
		{strings.Fields("Over 1,200 satellites launched"), 1},
		{strings.Fields("Why lasers matter so much"), 1}, // first of the longest words
		{strings.Fields("It is so"), -1},
	}
	for _, c := range cases {
		if got := emphasisIndex(c.words); got != c.want {
			t.Errorf("emphasisIndex(%v) = %d, want %d", c.words, got, c.want)
		}
	}
}

func TestCaptionTrackTimesWords(t *testing.T) {
	chunks := CaptionTrack([]Timed{{Duration: 4, Text: "One two three four five six"}}, 3)
	if len(chunks) != 2 || len(chunks[0].Words) != 3 {
		t.Fatalf("chunks = %+v", chunks)
	}
	prev := -1.0
	for _, c := range chunks {
		for _, w := range c.Words {
			if w.At < c.Start || w.At >= c.End || w.At <= prev {
				t.Fatalf("word %q at %v outside [%v, %v) or not increasing", w.Text, w.At, c.Start, c.End)
			}
			prev = w.At
		}
	}
	if chunks[1].End < 3.99 || chunks[1].End > 4.01 {
		t.Fatalf("last caption ends at %v", chunks[1].End)
	}
}

func TestWriteProjectRendersCaptionsSafely(t *testing.T) {
	root := t.TempDir()
	gsap := filepath.Join(root, "gsap.min.js")
	_ = os.WriteFile(gsap, []byte("/*gsap*/"), 0o644)
	fonts := filepath.Join(root, "fontsource")
	for _, f := range projectFonts {
		p := filepath.Join(fonts, f.pkg, "files", f.file)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte("font"), 0o644)
	}
	segs := []Timed{{Duration: 3, Title: "Why lasers matter", Text: `Lasers <b>hit</b> the "Moon".`}}
	dir := filepath.Join(root, "project")
	in := ProjectInput{Width: 1080, Height: 1920, Style: StyleBold, Scenes: PlanScenes(segs, nil, "Q"), Captions: CaptionTrack(segs, 3), Seed: 42}
	if err := WriteProject(dir, in, gsap, fonts); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	html := string(b)
	if !regexp.MustCompile(`var seed = +42 *;`).MatchString(html) {
		t.Error("seed missing")
	}
	if strings.Contains(html, "<b>hit</b>") {
		t.Fatal("caption text not escaped")
	}
	for _, want := range []string{`id="c0w0"`, `class="word em"`, `"words":[0`, `class="captions clip"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestWordCaptionsShowOneWordAndPairShortOnes(t *testing.T) {
	segs := []Timed{{Duration: 4, Text: "It is 1,200 kilometers away."}, {Duration: 2, Text: "Wow. A dog"}}
	got := WordCaptions(segs)
	var texts []string
	for _, c := range got {
		var ws []string
		for _, w := range c.Words {
			ws = append(ws, w.Text)
		}
		texts = append(texts, strings.Join(ws, " "))
	}
	// "It is" pairs; "1,200" is hot; "Wow." ends a sentence so it stays alone;
	// "A dog" pairs within the second segment.
	want := []string{"It is", "1,200", "kilometers", "away.", "Wow.", "A dog"}
	if strings.Join(texts, "|") != strings.Join(want, "|") {
		t.Fatalf("captions = %q, want %q", texts, want)
	}
	if !got[1].Hot || !got[2].Hot || got[0].Hot {
		t.Errorf("hot flags wrong: %+v", got)
	}
	for i := 1; i < len(got); i++ {
		if got[i].Start < got[i-1].End-1e-9 {
			t.Errorf("caption %d overlaps the previous one", i)
		}
	}
	if end := got[len(got)-1].End; math.Abs(end-6) > 1e-6 {
		t.Errorf("captions end at %v, want 6", end)
	}
}

func TestCaptionFontFitsLongWords(t *testing.T) {
	short := captionFontUnits(CaptionChunk{Words: []CaptionWord{{Text: "yes"}}})
	long := captionFontUnits(CaptionChunk{Words: []CaptionWord{{Text: "electromagnetically"}}})
	if short != 12.5 {
		t.Errorf("short word size = %v, want the 12.5 cap", short)
	}
	if long*0.62*19 > 90.01 {
		t.Errorf("long word overflows: %v units", long)
	}
}

func TestWriteProjectGameplayStory(t *testing.T) {
	root := t.TempDir()
	gsap := filepath.Join(root, "gsap.min.js")
	_ = os.WriteFile(gsap, []byte("/*gsap*/"), 0o644)
	fonts := filepath.Join(root, "fontsource")
	for _, f := range projectFonts {
		p := filepath.Join(fonts, f.pkg, "files", f.file)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte("font"), 0o644)
	}
	segs := []Timed{{Duration: 3, Title: "<i>Hook</i>", Text: `Lasers <b>hit</b> it.`}, {Duration: 3, Text: "Then 42 more."}}
	dir := filepath.Join(root, "project")
	in := ProjectInput{Width: 1080, Height: 1920, Style: StyleGameplay, ChannelName: "<script>x</script>",
		Scenes: PlanScenes(segs, nil, "Q"), Captions: WordCaptions(segs), Seed: 7}
	if err := WriteProject(dir, in, gsap, fonts); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	html := string(b)
	for _, bad := range []string{"<b>hit</b>", "<i>Hook</i>", "<script>x</script>"} {
		if strings.Contains(html, bad) {
			t.Errorf("unescaped %q", bad)
		}
	}
	for _, want := range []string{`<canvas id="game"`, `id="hook"`, `class="cap hot"`, `font-size: calc(var(--u) * 12.50)`, `window.__timelines["main"]`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(html, `class="style-`) {
		t.Error("gameplay story used the scene template")
	}
}
