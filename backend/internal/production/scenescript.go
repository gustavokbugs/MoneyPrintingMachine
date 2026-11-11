package production

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"contentfactory/internal/channel"
	"contentfactory/internal/llm"
)

// SceneScript is a hand-checked reference script for a 3D scene: one line
// per stage, plus the facts it states. Small local models cannot plan a
// ten-step chain, but they can rewrite one, so 3D build scripts are fresh
// rewrites of this reference, and the reference itself is the fallback.
type SceneScript struct {
	Hook   string
	Lines  []SceneLine // one per stage, in order
	Outro  string
	Claims []Claim
}

// SceneLine is the narration and caption title for one stage.
type SceneLine struct{ Narration, OnScreen string }

// damScript is the reference for the dam scene.
var damScript = SceneScript{
	Hook: "You live by a river, and you want power for your house. Let's build it from scratch.",
	Lines: []SceneLine{
		{"First, you drop big stones across the river to block it. Done? Not quite.", "Stones"},
		{"The water squeezes right through the gaps between the stones.", "Gaps"},
		{"So you pack the gaps with clay. Sticky, heavy, watertight. The leaks stop.", "Clay"},
		{"Now the water piles up behind your wall. Congrats, you made a lake.", "A lake"},
		{"But the lake keeps filling, and the water starts pouring over the top.", "Overflow"},
		{"It digs at the wall from both sides, and the whole thing washes away.", "Gone"},
		{"So you build a real dam out of cut stone, thick at the bottom, where the water pushes hardest.", "A real dam"},
		{"Then you add a spillway, a side channel that lets extra water out before it reaches the top.", "Spillway"},
		{"That falling water is wasted, so you put a water wheel under it, and it spins.", "Water wheel"},
		{"Hook the wheel to a generator, run wires to your house, and the lights come on.", "Power"},
	},
	Outro: "Congrats. You just built a hydroelectric dam. Follow for more.",
	Claims: []Claim{
		{Text: "Water pressure increases with depth, so dams are built thicker at the bottom", SourceHint: "hydraulic engineering references on hydrostatic pressure and dam design"},
		{Text: "Spillways release excess water so it does not flow over the top of a dam", SourceHint: "engineering references on dam spillways"},
		{Text: "A water wheel or turbine connected to a generator turns flowing water into electricity", SourceHint: "physics and energy references on hydroelectric power"},
	},
}

// Script returns the reference as a script with unverified claims.
func (r SceneScript) Script() Script {
	sc := Script{Hook: r.Hook, Outro: r.Outro}
	for _, l := range r.Lines {
		sc.Sections = append(sc.Sections, Section{Heading: l.OnScreen, Narration: l.Narration, OnScreenText: l.OnScreen})
	}
	for _, c := range r.Claims {
		c.Status = ClaimUnverified
		sc.Claims = append(sc.Claims, c)
	}
	return sc
}

const rewriteSystemPrompt = `You rewrite narration for a short 3D animation where each line plays over one stage of the animation.
Rules:
- Rewrite EVERY line in fresh words, in the same order, with the same meaning and the same facts. One output line per input line, never merge, skip, or add lines.
- Keep it punchy and spoken to "you": short sentences, everyday words, a little humor.
- Keep the "But..." lines as problems and the "So you..." lines as fixes.
- Never add numbers, dates, names, or facts that are not in the reference.
- Write a new hook (one or two sentences) that makes people keep watching.
- Respond only with JSON matching the schema.`

type rewriteOutput struct {
	Hook  string `json:"hook"`
	Lines []struct {
		Narration string `json:"narration"`
		OnScreen  string `json:"onScreenText"`
	} `json:"lines"`
	Outro string `json:"outro"`
}

func rewriteSchema(n int) json.RawMessage {
	str := map[string]any{"type": "string"}
	s := map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"hook", "lines", "outro"},
		"properties": map[string]any{
			"hook": str, "outro": str,
			"lines": map[string]any{"type": "array", "minItems": n, "maxItems": n,
				"items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"narration", "onScreenText"},
					"properties": map[string]any{"narration": str, "onScreenText": str}}},
		},
	}
	raw, _ := json.Marshal(s)
	return raw
}

// rewriteUsable rejects rewrites that would break the scene: empty or
// overlong lines, or lines repeated within the script.
func rewriteUsable(out rewriteOutput, ref SceneScript) bool {
	if len(out.Lines) != len(ref.Lines) || strings.TrimSpace(out.Hook) == "" {
		return false
	}
	seen := map[string]bool{}
	for i, l := range out.Lines {
		n := strings.ToLower(strings.Join(strings.Fields(l.Narration), " "))
		words := len(strings.Fields(n))
		refWords := len(strings.Fields(ref.Lines[i].Narration))
		if words < 4 || words > refWords*2+6 || seen[n] {
			return false
		}
		seen[n] = true
	}
	return true
}

// generateSceneScript writes a 3D build script as a fresh rewrite of the
// scene's reference, falling back to the reference when the model fails.
func (s *Service) generateSceneScript(ctx context.Context, ch *channel.Channel, v *Video, sc Scene3D) (*Script, error) {
	ref := sc.Reference
	var b strings.Builder
	fmt.Fprintf(&b, "Video title: %s\nReference hook: %s\n", v.Title, ref.Hook)
	for i, l := range ref.Lines {
		fmt.Fprintf(&b, "Line %d (stage: %s): %s\n", i+1, sc.Stages[i], l.Narration)
	}
	fmt.Fprintf(&b, "Reference outro: %s\n", ref.Outro)
	if lang := languageName(ch.Language); lang != "American English" {
		fmt.Fprintf(&b, "Write everything in %s.\n", lang)
	}
	var out rewriteOutput
	err := s.writer().GenerateJSON(ctx, llm.Request{
		Messages: []llm.Message{{Role: "system", Content: rewriteSystemPrompt}, {Role: "user", Content: b.String()}},
		Schema:   rewriteSchema(len(ref.Lines)), Temperature: 0.7, MaxTokens: 1600,
	}, &out)
	script := ref.Script()
	model := "reference"
	if err == nil && rewriteUsable(out, ref) {
		clean := func(x string) string { return strings.Join(strings.Fields(x), " ") }
		script.Hook = clean(out.Hook)
		if o := clean(out.Outro); o != "" {
			script.Outro = o
		}
		for i, l := range out.Lines {
			script.Sections[i].Narration, _ = stripSpeakerLabel(clean(l.Narration))
			if on := clean(l.OnScreen); on != "" && len(on) <= 40 {
				script.Sections[i].OnScreenText, script.Sections[i].Heading = on, on
			}
		}
		model = s.writer().Model()
	}
	script.Provenance = channel.Provenance{Generator: "ollama", Model: model, GeneratedAt: s.now().UTC()}
	if err := script.Validate(); err != nil {
		script = ref.Script() // the reference always validates
	}
	script.EstimatedSeconds = int(float64(script.WordCount()) / WordsPerSecond / paceOf(v.StoryFormat))
	return &script, nil
}
