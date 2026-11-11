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
	// Reference is the hand-checked script the model rewrites.
	Reference SceneScript
	js        []byte
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
		"Problem: the overflow washes the wall away and the lake drains.",
		"Fix: a thick stone dam, wider at the bottom, is built layer by layer, and the lake refills.",
		"Fix: a spillway channel at the side carries the extra water safely down.",
		"The falling water spins a water wheel.",
		"The wheel drives a generator; wires carry electricity to a house and its lights turn on.",
	},
	Reference: damScript,
	js:        sceneDamJS,
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

// stageMarks keeps the caption colors in step with the scene: red only on
// "Problem:" stages, yellow only on "Fix:" stages, nothing elsewhere.
// plans line up with segments: the hook, one per stage, then the outro.
func stageMarks(plans []popPlan, sc Scene3D) []popPlan {
	out := make([]popPlan, len(plans))
	for i, p := range plans {
		out[i] = popPlan{Query: p.Query}
		if i < 1 || i > len(sc.Stages) {
			continue
		}
		switch stage := sc.Stages[i-1]; {
		case strings.HasPrefix(stage, "Problem:"):
			out[i].Problem = p.Problem
			if out[i].Problem == "" {
				out[i].Problem = p.Keyword
			}
		case strings.HasPrefix(stage, "Fix:"):
			out[i].Keyword = p.Keyword
		}
	}
	return out
}
