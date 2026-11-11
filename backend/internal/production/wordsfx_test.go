package production

import "testing"

func TestSoundFor(t *testing.T) {
	cases := map[string]string{
		"burning,": SFXFire, "Steam": SFXHiss, "shower": SFXSplash, "knocks": SFXKnock,
		"electricity": SFXZap, "glass!": SFXShatter, "exploded": SFXImpact, "heart": SFXHeartbeat,
		"the": "", "a": "", "computer": "", "random": "", "popular": "", "ticket": "", "ran": SFXWhoosh, "pop": SFXPop,
	}
	for w, want := range cases {
		if got := soundFor(w); got != want {
			t.Errorf("soundFor(%q) = %q, want %q", w, got, want)
		}
	}
}

func TestPlanWordEffectsSpacing(t *testing.T) {
	segs := []Timed{
		{Duration: 10, Text: "Steam pours out. Burning walls.", Words: []float64{0, 0.4, 0.8, 1.6, 2.0}},
		{Duration: 10, Text: "More steam and fire everywhere.", Words: []float64{0, 0.5, 0.9, 1.3, 2.4}},
	}
	cues := PlanWordEffects(segs)
	// "Steam" at 0; "pours" and "Burning" are under 3 s later; "steam" at
	// 10.5 repeats too soon (12 s); "fire" at 11.3 plays.
	want := []SFXCue{{At: 0, Kind: SFXHiss, Word: "steam"}, {At: 11.3, Kind: SFXFire, Word: "fire"}}
	if len(cues) != 2 || cues[0] != want[0] || cues[1] != want[1] {
		t.Fatalf("cues = %+v", cues)
	}
}

func TestPlanWordEffectsCap(t *testing.T) {
	// 24 s allows at most 4 effects, even with a sound word every 3 s.
	text := "fire rain glass boom wind clock heart knock"
	words := []float64{0, 3, 6, 9, 12, 15, 18, 21}
	if cues := PlanWordEffects([]Timed{{Duration: 24, Text: text, Words: words}}); len(cues) != 4 {
		t.Fatalf("cues = %d, want 4", len(cues))
	}
}

func TestWordEffectsRender(t *testing.T) {
	r := newRNG("t")
	for _, kind := range []string{SFXHiss, SFXFire, SFXSplash, SFXKnock, SFXZap, SFXShatter, SFXHeartbeat, SFXWind, SFXCrunch, SFXClock, SFXPop} {
		tr := NewTrack(4)
		if !renderWordEffect(tr, r, SFXCue{At: 0.5, Kind: kind}) {
			t.Errorf("%s not rendered", kind)
			continue
		}
		if p := tr.Peak(); p < 0.02 || p > 3 {
			t.Errorf("%s peak = %v", kind, p)
		}
	}
	if renderWordEffect(NewTrack(1), r, SFXCue{Kind: "nope"}) {
		t.Error("unknown kinds must be reported")
	}
	// A cue near the end must not write past the track.
	tr := NewTrack(1)
	renderWordEffect(tr, r, SFXCue{At: 0.9, Kind: SFXWind})
}

func TestPOVHasNoMusic(t *testing.T) {
	if !musicOff(FormatPOV) || musicOff(FormatEscalation) || musicOff("") {
		t.Error("only the POV story drops the music")
	}
}
