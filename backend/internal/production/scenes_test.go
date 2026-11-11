package production

import (
	"os"
	"path/filepath"
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
