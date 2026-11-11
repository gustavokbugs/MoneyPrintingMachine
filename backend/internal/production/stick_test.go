package production

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanStickUsesModelWithinFixedLists(t *testing.T) {
	gen := &scriptedGen{responses: []string{`{"segments":[{"pose":"point","icon":"bulb","label":"the light bulb!!"},{"pose":"facepalm","icon":"fire","label":"Way too much smoke in here"}]}`}}
	svc, _ := newTestService(t, gen, nil, Tools{})
	segs := []Timed{{Duration: 2, Text: "You want light.", Words: []float64{0, 0.4, 0.9}}, {Duration: 3, Text: "Smoke everywhere."}}
	d := svc.planStick(context.Background(), segs)
	if d.Total != 5 || len(d.Beats) != 2 {
		t.Fatalf("plan = %+v", d)
	}
	if b := d.Beats[0]; b.Pose != "point" || b.Icon != "bulb" || b.Label != "The light bulb" || b.At != 0.1 {
		t.Errorf("beat 1 = %+v", b)
	}
	if b := d.Beats[1]; b.Pose != "facepalm" || b.Label != "Way too much" || b.At != 2.1 {
		t.Errorf("beat 2 = %+v (labels keep at most 3 words)", b)
	}
	// Word times: measured for segment 1, spread over segment 2.
	want := []float64{0, 0.4, 0.9, 2, 3.5}
	if len(d.Words) != len(want) {
		t.Fatalf("words = %v", d.Words)
	}
	for i := range want {
		if d.Words[i] != want[i] {
			t.Errorf("word %d at %v, want %v", i, d.Words[i], want[i])
		}
	}
}

func TestPlanStickFallback(t *testing.T) {
	svc, _ := newTestService(t, &scriptedGen{}, nil, Tools{})
	d := svc.planStick(context.Background(), []Timed{{Duration: 2, Text: "The river gives power."}, {Duration: 2, Text: "Nothing obvious."}})
	if d.Beats[0].Icon != "bolt" || d.Beats[0].Label != "Power" || d.Beats[1].Icon != "question" || d.Beats[1].Label == "" {
		t.Errorf("fallback = %+v", d.Beats)
	}
	if oneOf("dance", StickPoses, "idle") != "idle" {
		t.Error("unknown poses must be replaced")
	}
}

func TestStickListsMatchTheScene(t *testing.T) {
	js := string(sceneStickJS)
	for _, p := range StickPoses {
		if !strings.Contains(js, "    "+p+": ") && !strings.Contains(js, "    "+p+":  ") && !strings.Contains(js, p+": {") {
			t.Errorf("scene_stick.js has no pose %q", p)
		}
	}
	for _, ic := range StickIcons {
		if !strings.Contains(js, "    "+ic+": [") {
			t.Errorf("scene_stick.js has no icon %q", ic)
		}
	}
}

func TestWriteProjectStick(t *testing.T) {
	root := t.TempDir()
	gsap := filepath.Join(root, "gsap.min.js")
	_ = os.WriteFile(gsap, []byte("/*gsap*/"), 0o644)
	fonts := filepath.Join(root, "fontsource")
	for _, f := range projectFonts {
		p := filepath.Join(fonts, f.pkg, "files", f.file)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte("font"), 0o644)
	}
	segs := []Timed{{Duration: 3, Text: "You want light."}}
	in := ProjectInput{Width: 1080, Height: 1920, Style: StyleStick, Scenes: PlanScenes(segs, nil, "Q"), Captions: WordCaptions(segs),
		Stick: &StickData{Total: 3, Beats: []StickBeat{{At: 0.1, Pose: "point", Icon: "bulb", Label: "</script><b>x"}}}}
	dir := filepath.Join(root, "project")
	if err := WriteProject(dir, in, gsap, fonts); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	html := string(b)
	for _, want := range []string{`<svg id="stick"`, `"pose":"point"`, `assets/scene.js`, `window.Scene2D`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(html, "</script><b>") {
		t.Error("labels must be escaped inside the script")
	}
	if strings.Contains(html, `id="hook"`) {
		t.Error("the stick explainer has no hook card")
	}
	in.Stick = nil
	if err := WriteProject(filepath.Join(root, "p2"), in, gsap, fonts); err == nil {
		t.Error("a stick explainer without a storyboard must fail clearly")
	}
}
