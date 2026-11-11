package production

import (
	"math"
	"strings"
)

// Word-triggered sound effects: when the narration says "burning", a
// sizzle plays on that word. Every sound is synthesized in code, so there
// is nothing to license.
const (
	SFXHiss      = "hiss"      // steam, sizzling
	SFXFire      = "fire"      // crackling flames
	SFXSplash    = "splash"    // water, bubbles
	SFXKnock     = "knock"     // door knocks
	SFXZap       = "zap"       // electricity
	SFXShatter   = "shatter"   // breaking glass
	SFXHeartbeat = "heartbeat" // fear, tension
	SFXWind      = "wind"      // cold, wind
	SFXCrunch    = "crunch"    // eating, cracking
	SFXClock     = "clock"     // waiting, time
	SFXPop       = "pop"       // bubbles popping, small surprises
)

// wordSounds maps English and Brazilian Portuguese word stems to effects. A word matches a stem when it
// starts with it, so "burning" and "burnt" match "burn"; a stem ending in
// "$" matches only that whole word ("ran$" must not match "random").
var wordSounds = []struct {
	kind  string
	stems []string
}{
	{SFXHiss, []string{"steam", "sizzl", "hiss", "boil", "fry", "fried", "frying", "scald", "cooking", "cooked", "melt", "vapor", "ferv", "frit", "derret", "chiad", "cozinh"}},
	{SFXFire, []string{"burn", "fire", "flame", "blaze", "lava", "torch", "smok", "ignit", "campfire", "queim", "fogo", "chama", "fumaça", "incênd", "brasa", "fogueira"}},
	{SFXSplash, []string{"water", "splash", "pour", "shower", "rain", "drip", "drown", "swim", "pool", "flood", "wet$", "soaked", "sink", "água", "chuva", "molhad", "piscina", "banho", "afog", "inund", "goteira", "pinga"}},
	{SFXKnock, []string{"knock", "door", "porta", "bate$", "bateu$", "batendo$"}},
	{SFXZap, []string{"electric", "shock", "spark", "zap", "lightning", "volt", "elétric", "eletric", "choque", "faísca", "raio$", "raios$"}},
	{SFXShatter, []string{"shatter", "glass", "smash", "broke", "breaks", "crack", "vidro", "quebr", "estilhaç", "rachad", "racha$"}},
	{SFXImpact, []string{"explod", "explosion", "boom", "bomb", "blast", "crash", "slam", "explod", "explos", "bomba", "estrond", "desab"}},
	{SFXHeartbeat, []string{"heart", "scared", "panic", "nervous", "terrif", "fear", "coração", "medo", "pânico", "nervos"}},
	{SFXWind, []string{"wind", "freez", "frozen", "cold", "ice$", "icy$", "snow", "breeze", "vento", "frio$", "gelo", "gelad", "congel", "neve"}},
	{SFXCrunch, []string{"crunch", "bite", "chew", "eat$", "eats$", "eating$", "ate$", "snack", "morde", "mordid", "mastig", "come$", "comeu$", "comendo$"}},
	{SFXClock, []string{"clock", "tick$", "ticks$", "ticking$", "wait", "minute", "hours", "relógio", "espera$", "esperando$", "minuto"}},
	{SFXWhoosh, []string{"run", "ran$", "jump", "fly", "flew", "throw", "threw", "fall", "fell", "rush", "dash", "zoom", "corre$", "correu$", "correndo$", "pula$", "pulou$", "voa$", "voou$", "voando$", "cai$", "caiu$", "caindo$", "joga$", "jogou$"}},
	{SFXChime, []string{"idea", "realiz", "genius", "ding", "notification", "phone", "ideia", "genial", "percebe"}},
	{SFXPop, []string{"pop$", "pops$", "popped$", "popping$", "bubbl", "poof", "pipoca", "estoura", "bolha"}},
}

// soundFor returns the effect for a spoken word, or "".
func soundFor(word string) string {
	w := normWord(word)
	if len(w) < 3 {
		return ""
	}
	for _, s := range wordSounds {
		for _, stem := range s.stems {
			if exact, ok := strings.CutSuffix(stem, "$"); ok && w == exact || !ok && strings.HasPrefix(w, stem) {
				return s.kind
			}
		}
	}
	return ""
}

// Spacing keeps word effects from crowding the narration: any two are at
// least minEffectGap apart, and the same sound repeats only after
// sameEffectGap.
const (
	minEffectGap  = 1.2
	sameEffectGap = 4.0
)

// PlanWordEffects places an effect on each word that names a sound, at the
// moment it is spoken (measured word times when available).
func PlanWordEffects(segs []Timed) []SFXCue {
	var cues []SFXCue
	last := map[string]float64{}
	prev := -10.0
	offset := 0.0
	for _, seg := range segs {
		words := strings.Fields(seg.Text)
		for i, w := range words {
			kind := soundFor(w)
			if kind == "" {
				continue
			}
			at := offset + seg.Duration*float64(i)/float64(max(len(words), 1))
			if i < len(seg.Words) {
				at = offset + seg.Words[i]
			}
			if at-prev < minEffectGap {
				continue
			}
			if t, ok := last[kind]; ok && at-t < sameEffectGap {
				continue
			}
			cues = append(cues, SFXCue{At: at, Kind: kind, Word: normWord(w)})
			last[kind], prev = at, at
		}
		offset += seg.Duration
	}
	return cues
}

// soundQueries are photo searches that show what a sound effect is; the
// kinds left out search the spoken word itself ("melted", "running").
var soundQueries = map[string]string{
	SFXFire: "campfire", SFXSplash: "water splash", SFXKnock: "wooden door", SFXZap: "electric sparks",
	SFXShatter: "broken glass", SFXImpact: "explosion", SFXHeartbeat: "human heart", SFXWind: "wind blowing trees",
	SFXClock: "clock", SFXPop: "popcorn",
}

// sfxQuery is the photo search for a word effect's pop-up picture.
func sfxQuery(c SFXCue) string {
	if c.Word == "" {
		return ""
	}
	if q, ok := soundQueries[c.Kind]; ok {
		return q
	}
	return c.Word
}

// renderWordEffect synthesizes one word effect; it reports false for kinds
// it does not know (the scene effects are rendered by RenderEffects).
func renderWordEffect(t *Track, r *rng, c SFXCue) bool {
	switch c.Kind {
	case SFXHiss:
		hiss(t, r, c.At, 1.3, 0.8)
	case SFXFire:
		crackle(t, r, c.At, 1.6, 0.8)
	case SFXSplash:
		splash(t, r, c.At, 0.9)
	case SFXKnock:
		for k := 0; k < 3; k++ {
			knock(t, r, c.At+float64(k)*0.17, 0.9)
		}
	case SFXZap:
		zap(t, r, c.At, 0.6, 0.45)
	case SFXShatter:
		shatter(t, r, c.At, 0.8)
	case SFXHeartbeat:
		for k := 0; k < 2; k++ {
			knockAt(t, c.At+float64(k)*0.8, 55, 0.12, 0.8)
			knockAt(t, c.At+float64(k)*0.8+0.16, 50, 0.1, 0.6)
		}
	case SFXWind:
		wind(t, r, c.At, 2.2, 0.8)
	case SFXCrunch:
		for k := 0; k < 7; k++ {
			hiss(t, r, c.At+float64(k)*0.035+r.float()*0.02, 0.03, 0.8)
		}
	case SFXClock:
		for k := 0; k < 4; k++ {
			f := 2100.0
			if k%2 == 1 {
				f = 1600
			}
			blip(t, c.At+float64(k)*0.5, f, 0.015, 0.45)
		}
	case SFXPop:
		// A burst, like popcorn: quick pops at random moments.
		for k := 0; k < 9; k++ {
			pop(t, c.At+r.float()*1.1, 0.5+r.float()*0.4)
		}
	default:
		return false
	}
	return true
}

// hiss is high-passed noise with a fast attack and a long tail: "tsss".
func hiss(t *Track, r *rng, at, dur, gain float64) {
	i0 := int(at * AudioRate)
	lp := newOnePole(2500)
	n := int(dur * AudioRate)
	for k := 0; k < n; k++ {
		s := float64(k) / AudioRate
		x := r.noise()
		high := x - lp.step(x)
		env := math.Min(1, s/0.015) * math.Exp(-s*2.2/dur) * (0.85 + 0.15*math.Sin(2*math.Pi*23*s))
		v := high * env * gain
		t.add(i0+k, v*(0.9+0.1*r.float()), v)
	}
}

// crackle is a low fire rumble with random sharp pops on top.
func crackle(t *Track, r *rng, at, dur, gain float64) {
	i0 := int(at * AudioRate)
	lp := newOnePole(600)
	n := int(dur * AudioRate)
	for k := 0; k < n; k++ {
		p := float64(k) / float64(n)
		env := math.Sin(math.Pi*math.Min(1, p*1.3)) * gain
		v := lp.step(r.noise()) * 0.6 * env
		t.add(i0+k, v, v)
	}
	for c := 0; c < int(dur*28); c++ {
		start := i0 + r.intn(n)
		amp := (0.4 + r.float()*0.8) * gain
		pan := r.float()
		for k := 0; k < int(0.004*AudioRate); k++ {
			v := r.noise() * amp * math.Exp(-float64(k)/(0.0012*AudioRate))
			t.add(start+k, v*(1-pan)*1.6, v*pan*1.6)
		}
	}
}

// splash is a noise burst followed by rising bubble chirps.
func splash(t *Track, r *rng, at, gain float64) {
	hiss(t, r, at, 0.35, gain*0.7)
	for b := 0; b < 22; b++ {
		start := at + 0.05 + r.float()*0.7
		f0 := 300 + r.float()*500
		i0 := int(start * AudioRate)
		phase := 0.0
		for k := 0; k < int(0.04*AudioRate); k++ {
			s := float64(k) / AudioRate
			phase += 2 * math.Pi * (f0 + f0*1.5*s/0.04) / AudioRate
			v := math.Sin(phase) * math.Exp(-s*60) * gain * 0.6
			t.add(i0+k, v, v)
		}
	}
}

// knock is a short wooden thump.
func knock(t *Track, r *rng, at, gain float64) {
	knockAt(t, at, 140, 0.07, gain)
	hiss(t, r, at, 0.02, gain*0.3)
}

// knockAt is a decaying low sine: a thump (or a heartbeat at low pitch).
func knockAt(t *Track, at, f, dur, gain float64) {
	i0 := int(at * AudioRate)
	for k := 0; k < int(dur*3*AudioRate); k++ {
		s := float64(k) / AudioRate
		v := math.Sin(2*math.Pi*f*s*(1+0.5*math.Exp(-s*40))) * math.Exp(-s/dur*3) * gain * math.Min(1, s/0.002)
		t.add(i0+k, v, v)
	}
}

// zap is a harsh electric buzz with crackles.
func zap(t *Track, r *rng, at, dur, gain float64) {
	i0 := int(at * AudioRate)
	n := int(dur * AudioRate)
	for k := 0; k < n; k++ {
		s := float64(k) / AudioRate
		buzz := math.Copysign(1, math.Sin(2*math.Pi*120*s)) * 0.5
		if r.float() < 0.02 {
			buzz += r.noise() * 2
		}
		env := math.Min(1, s/0.005) * math.Exp(-s*3/dur) * (0.6 + 0.4*math.Abs(math.Sin(2*math.Pi*17*s)))
		v := buzz * env * gain
		t.add(i0+k, v, v)
	}
}

// shatter is a bright noise burst with glassy pings.
func shatter(t *Track, r *rng, at, gain float64) {
	hiss(t, r, at, 0.25, gain)
	for p := 0; p < 9; p++ {
		blip(t, at+r.float()*0.5, 2500+r.float()*4000, 0.08+r.float()*0.15, gain*0.35)
	}
}

// wind is low-passed noise whose brightness swells and falls.
func wind(t *Track, r *rng, at, dur, gain float64) {
	i0 := int(at * AudioRate)
	n := int(dur * AudioRate)
	low, band := 0.0, 0.0
	for k := 0; k < n; k++ {
		p := float64(k) / float64(n)
		f := 300 + 500*math.Sin(math.Pi*p) + 120*math.Sin(2*math.Pi*p*3)
		fc := 2 * math.Sin(math.Pi*f/AudioRate)
		low += fc * band
		high := r.noise() - low - 0.3*band
		band += fc * high
		v := band * math.Pow(math.Sin(math.Pi*p), 1.5) * gain
		t.add(i0+k, v*(1-0.3*p), v*(0.7+0.3*p))
	}
}

// pop is a quick falling sine blip.
func pop(t *Track, at, gain float64) {
	i0 := int(at * AudioRate)
	phase := 0.0
	for k := 0; k < int(0.06*AudioRate); k++ {
		s := float64(k) / AudioRate
		phase += 2 * math.Pi * (900 - 6000*s) / AudioRate
		v := math.Sin(phase) * math.Exp(-s*50) * gain
		t.add(i0+k, v, v)
	}
}
