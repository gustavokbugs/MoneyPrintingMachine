package production

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanBuildUsesModelWithinFixedLists(t *testing.T) {
	gen := &scriptedGen{responses: []string{`{"segments":[{"piece":"rocks","action":"drop","problem":"leak","problemWord":"gaps"},{"piece":"clay","action":"grow","problem":"none","problemWord":""}]}`}}
	svc, _ := newTestService(t, gen, nil, Tools{})
	segs := []Timed{{Duration: 4, Text: "Stones stop the water, but gaps leak.", Words: []float64{0, 0.5, 0.9, 1.2, 1.8, 2.2, 2.6}}, {Duration: 3, Text: "So you add clay."}}
	b := svc.planBuild(context.Background(), segs, 9)
	if b.Total != 7 || b.Seed != 9 || len(b.Steps) != 2 {
		t.Fatalf("build = %+v", b)
	}
	s0, s1 := b.Steps[0], b.Steps[1]
	if s0.Piece != "rocks" || s0.Problem != "leak" || s0.At != 0.3 || s0.ProblemAt != 2.2 {
		t.Errorf("step 1 = %+v (the leak shows on the word 'gaps')", s0)
	}
	if s1.Piece != "clay" || s1.At != 4.25 || s1.Problem != "none" {
		t.Errorf("step 2 = %+v", s1)
	}
}

func TestPlanBuildFallsBackToWords(t *testing.T) {
	svc, _ := newTestService(t, &scriptedGen{}, nil, Tools{})
	b := svc.planBuild(context.Background(), []Timed{{Duration: 3, Text: "You light a fire."}, {Duration: 3, Text: "Nothing here."}}, 1)
	if b.Steps[0].Piece != "fire" || b.Steps[1].Piece != "crate" || b.Steps[0].Problem != "none" {
		t.Errorf("fallback = %+v", b.Steps)
	}
	if oneOf("bomb", BuildPieces, "crate") != "crate" {
		t.Error("unknown pieces must be replaced")
	}
}

func TestBuildPiecesMatchTheScene(t *testing.T) {
	js := string(build3dJS)
	for _, p := range BuildPieces {
		if !strings.Contains(js, "    "+p+"() {") {
			t.Errorf("build3d.js has no builder for %q", p)
		}
	}
	for _, a := range BuildActions[:5] {
		if !strings.Contains(js, `case "`+a+`"`) && a != "grow" {
			t.Errorf("build3d.js does not handle action %q", a)
		}
	}
}

func TestWriteProjectBuild3D(t *testing.T) {
	root := t.TempDir()
	gsap := filepath.Join(root, "gsap.min.js")
	_ = os.WriteFile(gsap, []byte("/*gsap*/"), 0o644)
	fonts := filepath.Join(root, "fontsource")
	for _, f := range projectFonts {
		p := filepath.Join(fonts, f.pkg, "files", f.file)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte("font"), 0o644)
	}
	three := filepath.Join(root, "three.cjs")
	_ = os.WriteFile(three, []byte("exports.REVISION = '181';"), 0o644)
	segs := []Timed{{Duration: 3, Text: "You stack stones."}}
	in := ProjectInput{Width: 1080, Height: 1920, Style: StyleBuild3D, Scenes: PlanScenes(segs, nil, "Q"), Captions: WordCaptions(segs),
		Build: &BuildData{Total: 3, Seed: 1, Steps: []BuildStep{{Piece: "rocks", Action: "drop", Problem: "none", At: 0.3, ProblemAt: 1}}}, ThreeJS: three}
	dir := filepath.Join(root, "project")
	if err := WriteProject(dir, in, gsap, fonts); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	html := string(b)
	for _, want := range []string{`<canvas id="gl3d"`, `"piece":"rocks"`, `assets/three.js`, `window.Build3D`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
	wrapped, _ := os.ReadFile(filepath.Join(dir, "assets", "three.js"))
	if !strings.HasPrefix(string(wrapped), "window.THREE = {};") {
		t.Error("three.js must be wrapped as a browser global")
	}
	if _, err := os.Stat(filepath.Join(dir, "assets", "build3d.js")); err != nil {
		t.Error("build3d.js not written")
	}
	in.ThreeJS = ""
	if err := WriteProject(filepath.Join(root, "p2"), in, gsap, fonts); err == nil {
		t.Error("a 3D build without three.js must fail clearly")
	}
}
