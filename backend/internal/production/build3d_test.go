package production

import (
	"context"
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

func TestScenePromptListsEveryStage(t *testing.T) {
	sc := Scenes3D[0]
	p := scenePrompt(sc)
	if !strings.Contains(p, "EXACTLY 10 sections") || !strings.Contains(p, "10. "+sc.Stages[9]) {
		t.Errorf("prompt = %s", p)
	}
	gen := &scriptedGen{}
	svc, _ := newTestService(t, gen, nil, Tools{})
	_, _ = svc.GenerateScript(context.Background(), testChannel(), &Video{TargetSeconds: 60, VisualStyle: StyleBuild3D, StoryFormat: FormatFixChain})
	if len(gen.requests) == 0 || !strings.Contains(gen.requests[0].Messages[1].Content, sc.Stages[0]) {
		t.Fatal("3D build scripts must be written to the scene's stages")
	}
	if !strings.Contains(string(gen.requests[0].Schema), `"minItems":10`) {
		t.Error("the schema must ask for one section per stage")
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
