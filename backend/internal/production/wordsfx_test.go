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
		{Duration: 4, Text: "Steam pours out. Burning walls.", Words: []float64{0, 0.4, 0.8, 1.6, 2.0}},
		{Duration: 4, Text: "More steam and fire everywhere.", Words: []float64{0, 0.5, 0.9, 1.3, 2.4}},
	}
	cues := PlanWordEffects(segs)
	// "Steam" at 0; "pours" (splash) at 0.4 is too close; "Burning" at 1.6;
	// segment 2: "steam" at 4.5 may repeat (4.5 s later), so "fire" at 5.3
	// is too close to it.
	want := []SFXCue{{At: 0, Kind: SFXHiss}, {At: 1.6, Kind: SFXFire}, {At: 4.5, Kind: SFXHiss}}
	if len(cues) != 3 || cues[0] != want[0] || cues[1] != want[1] || cues[2] != want[2] {
		t.Fatalf("cues = %+v", cues)
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
