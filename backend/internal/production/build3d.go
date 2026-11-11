package production

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The "3D build" style plays a hand-built 3D scene in stages (one stage
// per narration section), like watching something get built. Scenes are
// written by hand; the script is written to fit the scene's stages, so
// the picture always matches the words.

//go:embed scene_dam.js
var sceneDamJS []byte

// Scene3D is a hand-built scene and the stages it plays, in order.
type Scene3D struct {
	ID     string
	Name   string
	Intro  string   // what the hook shows (stage 0)
	Stages []string // what each narration section must describe
	js     []byte
}

// Scenes3D lists the available scenes.
var Scenes3D = []Scene3D{{
	ID: "dam", Name: "River to hydropower",
	Intro: "A river flows through a grassy valley.",
	Stages: []string{
		"Stones are dropped across the river to block it.",
		"Problem: water leaks through the gaps between the stones.",
		"Fix: clay is packed into the gaps and the leaks stop.",
		"A lake grows behind the wall.",
		"Problem: the lake gets too full and water pours over the top.",
		"The overflow washes the wall away and the lake drains.",
		"Fix: a thick stone dam, wider at the bottom, is built layer by layer, and the lake refills.",
		"Fix: a spillway channel at the side carries the extra water safely down.",
		"The falling water spins a water wheel.",
		"The wheel drives a generator; wires carry electricity to a house and its lights turn on.",
	},
	js: sceneDamJS,
}}

// sceneFor returns the scene a video plays (one scene so far).
func sceneFor(*Video) Scene3D { return Scenes3D[0] }

// BuildData tells the scene when each stage starts.
type BuildData struct {
	Total  float64   `json:"total"`
	Stages []float64 `json:"stages"`
	Outro  float64   `json:"outro"`
	scene  Scene3D
}

// buildTiming maps narration segments (hook, sections, outro) to stages:
// the hook plays stage 0, section i plays stage i+1, and the outro pulls
// the camera back. With a different number of sections, stages are
// spread over them in order.
func buildTiming(segs []Timed, sc Scene3D) *BuildData {
	starts := make([]float64, len(segs))
	total := 0.0
	for i, s := range segs {
		starts[i] = total
		total += s.Duration
	}
	d := &BuildData{Total: total, Outro: total, scene: sc}
	if len(segs) == 0 {
		return d
	}
	d.Stages = append(d.Stages, 0)
	sections := len(segs) - 2 // hook and outro around the sections
	if sections < 1 {
		sections = max(1, len(segs)-1)
	}
	for k := 1; k <= len(sc.Stages); k++ {
		idx := 1 + (k-1)*sections/len(sc.Stages)
		d.Stages = append(d.Stages, starts[min(idx, len(segs)-1)]+0.2)
	}
	if len(segs) >= 3 {
		d.Outro = starts[len(segs)-1]
	}
	return d
}

// scenePrompt is the script instruction that ties sections to stages.
func scenePrompt(sc Scene3D) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\nThis video plays over a 3D animation in stages. The hook plays over: %s\nWrite EXACTLY %d sections, in this order; section N narrates stage N and nothing else:\n", sc.Intro, len(sc.Stages))
	for i, s := range sc.Stages {
		fmt.Fprintf(&b, "%d. %s\n", i+1, s)
	}
	b.WriteString("Make the problems and fixes feel like a chain the viewer is doing (\"you\"). Keep every real fact true.")
	return b.String()
}

// writeBuildAssets adds three.js (wrapped as a browser global) and the
// scene script to a render project's assets.
func writeBuildAssets(assetsDir, threeCJS string, sc Scene3D) error {
	src, err := os.ReadFile(threeCJS)
	if err != nil {
		return fmt.Errorf("read three.js: %w", err)
	}
	wrapped := append([]byte("window.THREE = {};\n(function (exports) {\n"), src...)
	wrapped = append(wrapped, []byte("\n})(window.THREE);\n")...)
	if err := os.WriteFile(filepath.Join(assetsDir, "three.js"), wrapped, 0o644); err != nil {
		return fmt.Errorf("write three.js: %w", err)
	}
	return os.WriteFile(filepath.Join(assetsDir, "scene.js"), sc.js, 0o644)
}
