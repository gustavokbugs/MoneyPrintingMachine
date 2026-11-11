package production

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildTimingMapsSectionsToStages(t *testing.T) {
	sc := Scenes3D[0]
	// Hook, one section per stage, outro: 3 s each.
	segs := make([]Timed, len(sc.Stages)+2)
	for i := range segs {
		segs[i] = Timed{Duration: 3}
	}
	b := buildTiming(segs, sc)
	if b.Total != float64(3*len(segs)) || len(b.Stages) != len(sc.Stages)+1 || b.Stages[0] != 0 {
		t.Fatalf("timing = %+v", b)
	}
	for k := 1; k <= len(sc.Stages); k++ {
		if want := float64(3*k) + 0.2; b.Stages[k] != want {
			t.Errorf("stage %d starts at %v, want %v (section %d)", k, b.Stages[k], want, k)
		}
	}
	if b.Outro != float64(3*(len(segs)-1)) {
		t.Errorf("outro = %v", b.Outro)
	}
	// Fewer sections than stages: stages still play in order.
	short := buildTiming([]Timed{{Duration: 2}, {Duration: 2}, {Duration: 2}, {Duration: 2}}, sc)
	for k := 1; k < len(short.Stages); k++ {
		if short.Stages[k] < short.Stages[k-1] {
			t.Fatalf("stages out of order: %v", short.Stages)
		}
	}
}

func TestSceneScriptRewritesTheReference(t *testing.T) {
	sc := Scenes3D[0]
	ref := sc.Reference
	if len(ref.Lines) != len(sc.Stages) {
		t.Fatalf("the reference needs one line per stage: %d lines, %d stages", len(ref.Lines), len(sc.Stages))
	}
	lines := make([]string, len(ref.Lines))
	for i := range lines {
		lines[i] = fmt.Sprintf(`{"narration":"Fresh take number %d on this stage of the build.","onScreenText":"Step %d"}`, i+1, i+1)
	}
	good := `{"hook":"A new hook.","outro":"Bye now.","lines":[` + strings.Join(lines, ",") + `]}`
	svc, _ := newTestService(t, &scriptedGen{responses: []string{good}}, nil, Tools{})
	v := &Video{Title: "Dams", TargetSeconds: 60, VisualStyle: StyleBuild3D, StoryFormat: FormatFixChain}
	got, err := svc.GenerateScript(context.Background(), testChannel(), v)
	if err != nil {
		t.Fatal(err)
	}
	if got.Hook != "A new hook." || got.Sections[3].Narration != "Fresh take number 4 on this stage of the build." || got.Sections[3].OnScreenText != "Step 4" {
		t.Errorf("rewrite not used: %+v", got)
	}
	if len(got.Claims) != len(ref.Claims) || got.Claims[0].Status != ClaimUnverified {
		t.Errorf("claims = %+v (the reference claims still need review)", got.Claims)
	}

	// A broken rewrite (repeated lines) or a model failure falls back to
	// the reference, which keeps the scene and the words in step.
	bad := `{"hook":"h","outro":"o","lines":[` + strings.Repeat(`{"narration":"Same line again and again here.","onScreenText":"x"},`, len(ref.Lines)-1) + `{"narration":"Same line again and again here.","onScreenText":"x"}]}`
	for _, gen := range []*scriptedGen{{responses: []string{bad}}, {}} {
		svc, _ := newTestService(t, gen, nil, Tools{})
		got, err := svc.GenerateScript(context.Background(), testChannel(), v)
		if err != nil || got.Hook != ref.Hook || got.Sections[0].Narration != ref.Lines[0].Narration || got.Provenance.Model != "reference" {
			t.Errorf("fallback = %+v, %v", got, err)
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
	segs := []Timed{{Duration: 3, Text: "You stack stones."}, {Duration: 3, Text: "More."}, {Duration: 3, Text: "Done."}}
	in := ProjectInput{Width: 1080, Height: 1920, Style: StyleBuild3D, Scenes: PlanScenes(segs, nil, "Q"), Captions: WordCaptions(segs),
		Build: buildTiming(segs, Scenes3D[0]), ThreeJS: three}
	dir := filepath.Join(root, "project")
	if err := WriteProject(dir, in, gsap, fonts); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	html := string(b)
	for _, want := range []string{`<canvas id="gl3d"`, `"stages":[0,`, `assets/three.js`, `assets/scene.js`, `window.Scene3D`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
	wrapped, _ := os.ReadFile(filepath.Join(dir, "assets", "three.js"))
	if !strings.HasPrefix(string(wrapped), "window.THREE = {};") {
		t.Error("three.js must be wrapped as a browser global")
	}
	scene, _ := os.ReadFile(filepath.Join(dir, "assets", "scene.js"))
	if !strings.Contains(string(scene), "window.Scene3D") {
		t.Error("the dam scene was not written")
	}
	in.ThreeJS = ""
	if err := WriteProject(filepath.Join(root, "p2"), in, gsap, fonts); err == nil {
		t.Error("a 3D build without three.js must fail clearly")
	}
}

func TestStageMarksFollowTheScene(t *testing.T) {
	sc := Scenes3D[0]
	plans := make([]popPlan, len(sc.Stages)+2)
	for i := range plans {
		plans[i] = popPlan{Keyword: "fix", Problem: "oops", Query: "q"}
	}
	plans[2].Problem = "" // the model found no problem word on a problem stage
	got := stageMarks(plans, sc)
	if got[0].Keyword != "" || got[len(got)-1].Problem != "" {
		t.Error("the hook and outro get no colors")
	}
	if got[1].Keyword != "" || got[1].Problem != "" {
		t.Errorf("stage 1 (stones) is neither problem nor fix: %+v", got[1])
	}
	if got[2].Problem != "fix" || got[2].Keyword != "" {
		t.Errorf("stage 2 is a problem; fall back to the key word: %+v", got[2])
	}
	if got[3].Keyword != "fix" || got[3].Problem != "" {
		t.Errorf("stage 3 is a fix: %+v", got[3])
	}
}
