package production

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"math"
	"os"
)

// The soundtrack (music bed and sound effects) is synthesized in code, so
// every sound is original: there is no third-party audio, license, or
// Content ID claim to track. Rendering is deterministic for a given seed.

// AudioRate is the sample rate of generated audio.
const AudioRate = 48000

// Track is a stereo buffer of float samples in [-1, 1].
type Track struct{ L, R []float64 }

// NewTrack returns a silent track of the given length.
func NewTrack(seconds float64) *Track {
	n := int(math.Ceil(math.Max(seconds, 0) * AudioRate))
	return &Track{L: make([]float64, n), R: make([]float64, n)}
}

// Seconds returns the track length.
func (t *Track) Seconds() float64 { return float64(len(t.L)) / AudioRate }

func (t *Track) add(i int, l, r float64) {
	if i >= 0 && i < len(t.L) {
		t.L[i] += l
		t.R[i] += r
	}
}

// Peak returns the largest absolute sample value.
func (t *Track) Peak() float64 {
	p := 0.0
	for i := range t.L {
		p = math.Max(p, math.Max(math.Abs(t.L[i]), math.Abs(t.R[i])))
	}
	return p
}

// normalize scales the track so its peak equals target (no-op when silent).
func (t *Track) normalize(target float64) {
	p := t.Peak()
	if p == 0 {
		return
	}
	g := target / p
	for i := range t.L {
		t.L[i] *= g
		t.R[i] *= g
	}
}

// WriteWAV stores the track as 16-bit PCM stereo.
func (t *Track) WriteWAV(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create wav: %w", err)
	}
	w := bufio.NewWriterSize(f, 1<<16)
	n := len(t.L)
	dataSize := uint32(n * 4)
	hdr := []any{
		[4]byte{'R', 'I', 'F', 'F'}, 36 + dataSize, [4]byte{'W', 'A', 'V', 'E'},
		[4]byte{'f', 'm', 't', ' '}, uint32(16), uint16(1), uint16(2), uint32(AudioRate), uint32(AudioRate * 4), uint16(4), uint16(16),
		[4]byte{'d', 'a', 't', 'a'}, dataSize,
	}
	for _, v := range hdr {
		if err := binary.Write(w, binary.LittleEndian, v); err != nil {
			f.Close()
			return fmt.Errorf("write wav header: %w", err)
		}
	}
	buf := make([]byte, 4)
	for i := 0; i < n; i++ {
		binary.LittleEndian.PutUint16(buf[0:], uint16(pcm16(t.L[i])))
		binary.LittleEndian.PutUint16(buf[2:], uint16(pcm16(t.R[i])))
		if _, err := w.Write(buf); err != nil {
			f.Close()
			return fmt.Errorf("write wav: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return fmt.Errorf("write wav: %w", err)
	}
	return f.Close()
}

func pcm16(v float64) int16 {
	v = math.Max(-1, math.Min(1, v))
	return int16(math.Round(v * 32767))
}

// rng is a small deterministic generator (xorshift64*).
type rng struct{ s uint64 }

func newRNG(seed string) *rng {
	h := fnv.New64a()
	_, _ = h.Write([]byte(seed))
	s := h.Sum64()
	if s == 0 {
		s = 0x9E3779B97F4A7C15
	}
	return &rng{s: s}
}

func (r *rng) next() uint64 {
	r.s ^= r.s >> 12
	r.s ^= r.s << 25
	r.s ^= r.s >> 27
	return r.s * 2685821657736338717
}

// float returns a value in [0, 1).
func (r *rng) float() float64 { return float64(r.next()>>11) / (1 << 53) }

// noise returns a value in [-1, 1).
func (r *rng) noise() float64 { return r.float()*2 - 1 }

func (r *rng) intn(n int) int { return int(r.next() % uint64(n)) }

func midiHz(note float64) float64 { return 440 * math.Pow(2, (note-69)/12) }

// Mood describes a music bed.
type Mood struct {
	Name        string
	BPM         float64
	Chords      [][]int // MIDI notes per bar
	Cutoff      float64 // pad brightness (Hz)
	Arp         int     // arpeggio notes per beat (0 = none)
	Kick        int     // kicks per bar (0 = none)
	Hats        bool
	Drone       bool
	Description string
}

var moods = map[string]Mood{
	StyleMinimal: {Name: "calm", BPM: 84, Cutoff: 1400, Arp: 2, Description: "calm, warm pads with a soft arpeggio",
		Chords: [][]int{{48, 52, 55, 59}, {45, 48, 52, 55}, {41, 45, 48, 52}, {43, 47, 50, 55}}},
	StyleDocumentary: {Name: "cinematic", BPM: 66, Cutoff: 900, Drone: true, Description: "slow, dark cinematic strings and drone",
		Chords: [][]int{{45, 52, 57, 60}, {41, 48, 53, 57}, {48, 52, 55, 60}, {43, 50, 55, 59}}},
	StyleBold: {Name: "driving", BPM: 100, Cutoff: 2200, Arp: 2, Kick: 4, Hats: true, Description: "upbeat, driving pulse",
		Chords: [][]int{{50, 53, 57, 62}, {46, 50, 53, 58}, {41, 48, 53, 57}, {48, 52, 55, 60}}},
	StyleFootage: {Name: "cinematic pulse", BPM: 90, Cutoff: 1500, Arp: 2, Kick: 2, Drone: true, Description: "cinematic pads over a steady low pulse",
		Chords: [][]int{{45, 52, 57, 60}, {41, 48, 53, 57}, {48, 52, 55, 60}, {43, 50, 55, 59}}},
	StyleGameplay: {Name: "lofi", BPM: 78, Cutoff: 1100, Arp: 1, Kick: 2, Hats: true, Description: "laid-back lo-fi chords with a soft beat",
		Chords: [][]int{{50, 53, 57, 64}, {43, 53, 57, 59}, {48, 52, 55, 62}, {45, 52, 55, 60}}},
	StyleTech: {Name: "pulse", BPM: 110, Cutoff: 2600, Arp: 4, Kick: 2, Hats: true, Description: "electronic pulse with fast arpeggios",
		Chords: [][]int{{52, 55, 59, 64}, {48, 52, 55, 60}, {43, 50, 55, 59}, {50, 54, 57, 62}}},
}

// MoodFor returns the music mood matching a resolved visual style.
func MoodFor(style string) Mood {
	if m, ok := moods[style]; ok {
		return m
	}
	return moods[StyleMinimal]
}

// onePole is a simple low-pass filter.
type onePole struct{ a, y float64 }

func newOnePole(cutoff float64) *onePole {
	return &onePole{a: 1 - math.Exp(-2*math.Pi*cutoff/AudioRate)}
}

func (f *onePole) step(x float64) float64 {
	f.y += f.a * (x - f.y)
	return f.y
}

// softSaw is a band-limited sawtooth built from a few harmonics.
func softSaw(phase float64, harmonics int) float64 {
	s := 0.0
	for k := 1; k <= harmonics; k++ {
		s += math.Sin(phase*float64(k)) / float64(k)
	}
	return s * 0.55
}

// ComposeMusic renders an original music bed of the given length. The seed
// varies key and chord order so channels and videos do not all sound alike.
func ComposeMusic(m Mood, seconds float64, seed string) *Track {
	r := newRNG(seed)
	tr := NewTrack(seconds)
	n := len(tr.L)
	if n == 0 {
		return tr
	}
	transpose := float64(r.intn(5) - 2)
	rot := r.intn(len(m.Chords))
	beat := 60 / m.BPM
	bar := beat * 4
	chordAt := func(b int) []int { return m.Chords[(b+rot)%len(m.Chords)] }
	bars := int(math.Ceil(seconds/bar)) + 1

	pad := NewTrack(seconds)
	// Pads: two slightly detuned voices per note, panned apart, crossfaded
	// between chords.
	for b := 0; b < bars; b++ {
		start := float64(b)*bar - 0.25
		length := bar + 0.9
		i0 := int(math.Max(0, start*AudioRate))
		i1 := min(n, int((start+length)*AudioRate))
		for _, note := range chordAt(b) {
			f := midiHz(float64(note) + transpose)
			lp := []*onePole{newOnePole(m.Cutoff), newOnePole(m.Cutoff)}
			for v, det := range []float64{0.9965, 1.0035} {
				phase := r.float() * 2 * math.Pi
				pan := 0.3 + 0.4*float64(v)
				w := 2 * math.Pi * f * det / AudioRate
				for i := i0; i < i1; i++ {
					t := float64(i)/AudioRate - start
					env := math.Min(1, t/0.7) * math.Min(1, (length-t)/0.9)
					s := lp[v].step(softSaw(phase, 5)) * env * 0.08
					phase += w
					pad.add(i, s*(1-pan), s*pan)
				}
			}
		}
	}

	// Bass: root an octave below the chord, sustained or pulsing with kicks.
	for b := 0; b < bars; b++ {
		root := float64(chordAt(b)[0]) - 12 + transpose
		f := midiHz(root)
		hits := 1
		if m.Kick > 0 {
			hits = m.Kick * 2
		}
		step := bar / float64(hits)
		for h := 0; h < hits; h++ {
			start := float64(b)*bar + float64(h)*step
			i0 := int(start * AudioRate)
			i1 := min(n, int((start+step)*AudioRate))
			for i := max(0, i0); i < i1; i++ {
				t := float64(i)/AudioRate - start
				env := math.Min(1, t/0.02) * math.Exp(-t*1.2/step) * math.Min(1, (step-t)/0.03)
				ph := 2 * math.Pi * f * t
				s := (math.Sin(ph) + 0.25*math.Sin(2*ph)) * env * 0.16
				tr.add(i, s, s)
			}
		}
	}

	// Arpeggio: plucked chord tones an octave up with a ping-pong echo.
	arp := NewTrack(seconds)
	if m.Arp > 0 {
		step := beat / float64(m.Arp)
		pattern := []int{0, 2, 1, 3, 2, 1}
		count := int(seconds / step)
		for k := 0; k < count; k++ {
			start := float64(k) * step
			chord := chordAt(int(start / bar))
			note := float64(chord[pattern[k%len(pattern)]%len(chord)]) + 12 + transpose
			f := midiHz(note)
			accent := 0.7
			if k%m.Arp == 0 {
				accent = 1
			}
			i0 := int(start * AudioRate)
			i1 := min(n, i0+int(0.45*AudioRate))
			for i := i0; i < i1; i++ {
				t := float64(i)/AudioRate - start
				ph := 2 * math.Pi * f * t
				s := (math.Sin(ph) + 0.18*math.Sin(3*ph)) * math.Exp(-t*9) * math.Min(1, t/0.004) * 0.055 * accent
				arp.add(i, s, s)
			}
		}
		pingPong(arp, step*3/2, 0.38)
	}

	// Drone for cinematic moods: a low fifth that swells slowly.
	if m.Drone {
		f := midiHz(float64(chordAt(0)[0]) - 12 + transpose)
		for i := 0; i < n; i++ {
			t := float64(i) / AudioRate
			swell := 0.6 + 0.4*math.Sin(2*math.Pi*t/(bar*2))
			s := (math.Sin(2*math.Pi*f*t) + 0.5*math.Sin(2*math.Pi*f*1.5*t)) * swell * 0.05
			tr.add(i, s, s)
		}
	}

	// Drums: soft kick and closed hats.
	if m.Kick > 0 {
		kstep := bar / float64(m.Kick)
		for t0 := 0.0; t0 < seconds; t0 += kstep {
			kick(tr, t0, 0.32)
		}
	}
	if m.Hats {
		for t0 := beat / 2; t0 < seconds; t0 += beat {
			hat(tr, r, t0, 0.05)
		}
	}

	reverb(pad, 0.3)
	reverb(arp, 0.25)
	for i := 0; i < n; i++ {
		tr.L[i] += pad.L[i] + arp.L[i]
		tr.R[i] += pad.R[i] + arp.R[i]
	}
	fade(tr, 2, 3)
	for i := range tr.L {
		tr.L[i] = math.Tanh(tr.L[i] * 1.2)
		tr.R[i] = math.Tanh(tr.R[i] * 1.2)
	}
	tr.normalize(0.7)
	return tr
}

func fade(t *Track, in, out float64) {
	n := len(t.L)
	for i := 0; i < n; i++ {
		s := float64(i) / AudioRate
		g := math.Min(1, s/in) * math.Min(1, (float64(n-i)/AudioRate)/out)
		t.L[i] *= g
		t.R[i] *= g
	}
}

func kick(t *Track, at, gain float64) {
	i0 := int(at * AudioRate)
	phase := 0.0
	for k := 0; k < int(0.35*AudioRate); k++ {
		s := float64(k) / AudioRate
		f := 45 + 75*math.Exp(-s*30)
		phase += 2 * math.Pi * f / AudioRate
		v := math.Sin(phase) * math.Exp(-s*9) * gain
		t.add(i0+k, v, v)
	}
}

func hat(t *Track, r *rng, at, gain float64) {
	i0 := int(at * AudioRate)
	prev := 0.0
	for k := 0; k < int(0.05*AudioRate); k++ {
		x := r.noise()
		hp := x - prev // crude high-pass
		prev = x
		v := hp * math.Exp(-float64(k)/AudioRate*70) * gain
		t.add(i0+k, v*0.8, v)
	}
}

// pingPong adds an echo alternating between channels.
func pingPong(t *Track, delay, feedback float64) {
	d := int(delay * AudioRate)
	if d <= 0 {
		return
	}
	for i := d; i < len(t.L); i++ {
		l, r := t.L[i-d], t.R[i-d]
		t.L[i] += r * feedback
		t.R[i] += l * feedback
	}
}

// reverb is a small Schroeder reverb (parallel combs, serial all-passes).
func reverb(t *Track, wet float64) {
	for ch, buf := range [][]float64{t.L, t.R} {
		spread := ch * 23
		combs := []int{1557 + spread, 1617 + spread, 1491 + spread, 1422 + spread}
		out := make([]float64, len(buf))
		for _, c := range combs {
			d := c * AudioRate / 44100
			line := make([]float64, d)
			lp := 0.0
			for i := range buf {
				y := line[i%d]
				lp = y*0.7 + lp*0.3
				line[i%d] = buf[i] + lp*0.8
				out[i] += y * 0.25
			}
		}
		for _, a := range []int{556 + spread, 225 + spread} {
			d := a * AudioRate / 44100
			line := make([]float64, d)
			for i := range out {
				y := line[i%d]
				x := out[i]
				line[i%d] = x + y*0.5
				out[i] = y - x*0.5
			}
		}
		for i := range buf {
			buf[i] = buf[i]*(1-wet) + out[i]*wet
		}
	}
}

// Sound effect kinds placed on the timeline.
const (
	SFXWhoosh = "whoosh"
	SFXImpact = "impact"
	SFXTick   = "tick"
	SFXChime  = "chime"
	SFXRiser  = "riser"
)

// Cue marks a sound effect at a time in seconds.
type SFXCue struct {
	At   float64
	Kind string
}

// PlanEffects places effects on scene changes: an impact on the opening
// title, a whoosh into each new scene, ticks while counters run, and a
// chime on the outro. Times match the scene animations in scenes_html.go.
// The gameplay story has no scene cuts, only the hook card and the closing
// follow prompt (story_html.go).
func PlanEffects(scenes []Scene, style string) []SFXCue {
	if style == StyleFootage {
		// Footage cuts between segments: a whoosh into each new one.
		var cues []SFXCue
		for i, s := range scenes {
			if i == 0 {
				cues = append(cues, SFXCue{At: 0.05, Kind: SFXImpact})
			} else {
				cues = append(cues, SFXCue{At: s.Start - 0.12, Kind: SFXWhoosh})
			}
		}
		return cues
	}
	if style == StyleGameplay {
		if len(scenes) == 0 {
			return nil
		}
		cues := []SFXCue{{At: 0.05, Kind: SFXImpact}}
		last := scenes[len(scenes)-1]
		if total := last.Start + last.Duration; total > 4 {
			cues = append(cues, SFXCue{At: total - 3 + 0.05, Kind: SFXChime})
		}
		return cues
	}
	var cues []SFXCue
	for i, s := range scenes {
		switch {
		case i == 0:
			cues = append(cues, SFXCue{At: 0.05, Kind: SFXImpact})
		case s.Kind == SceneOutro:
			cues = append(cues, SFXCue{At: s.Start - 0.6, Kind: SFXRiser}, SFXCue{At: s.Start + 0.15, Kind: SFXChime})
			continue
		default:
			cues = append(cues, SFXCue{At: s.Start - 0.12, Kind: SFXWhoosh})
		}
		if s.Kind == SceneNumber {
			// The counter runs from start+0.2 for min(1.8, duration/2) with a
			// power2.out ease: ticks bunch up early and slow down.
			d := math.Min(1.8, s.Duration*0.5)
			for k := 1; k <= 14; k++ {
				p := float64(k) / 14
				// Inverse of power2.out (1-(1-x)^2): time where progress = p.
				x := 1 - math.Sqrt(1-p)
				cues = append(cues, SFXCue{At: s.Start + 0.2 + x*d, Kind: SFXTick})
			}
		}
	}
	return cues
}

// RenderEffects synthesizes the effects track.
func RenderEffects(cues []SFXCue, seconds float64, seed string) *Track {
	r := newRNG(seed + ":sfx")
	tr := NewTrack(seconds)
	for _, c := range cues {
		switch c.Kind {
		case SFXWhoosh:
			sweep(tr, r, c.At-0.35, 0.7, 400, 3200, 0.5)
		case SFXRiser:
			sweep(tr, r, c.At-1.0, 1.4, 300, 5000, 0.28)
		case SFXImpact:
			boom(tr, r, c.At, 0.8)
		case SFXTick:
			blip(tr, c.At, 2400, 0.012, 0.18)
		case SFXChime:
			bell(tr, c.At, 880, 0.3)
		}
	}
	return tr
}

// sweep is filtered noise whose band moves from f0 to f1 and back down,
// shaped to peak in the middle (a whoosh or riser).
func sweep(t *Track, r *rng, at, dur, f0, f1, gain float64) {
	i0 := int(at * AudioRate)
	n := int(dur * AudioRate)
	low, band := 0.0, 0.0
	for k := 0; k < n; k++ {
		p := float64(k) / float64(n)
		f := f0 + (f1-f0)*math.Sin(math.Pi*p*0.85)
		fc := 2 * math.Sin(math.Pi*f/AudioRate)
		x := r.noise()
		low += fc * band
		high := x - low - 0.6*band
		band += fc * high
		env := math.Pow(math.Sin(math.Pi*p), 2)
		v := band * env * gain
		pan := 0.5 + 0.35*math.Sin(2*math.Pi*p)
		t.add(i0+k, v*(1-pan)*1.4, v*pan*1.4)
	}
}

func boom(t *Track, r *rng, at, gain float64) {
	i0 := int(at * AudioRate)
	phase := 0.0
	lp := newOnePole(500)
	for k := 0; k < int(1.6*AudioRate); k++ {
		s := float64(k) / AudioRate
		f := 32 + 60*math.Exp(-s*8)
		phase += 2 * math.Pi * f / AudioRate
		body := math.Sin(phase) * math.Exp(-s*2.6)
		crack := lp.step(r.noise()) * math.Exp(-s*18) * 1.5
		v := (body + crack) * gain * math.Min(1, s/0.003)
		t.add(i0+k, v, v)
	}
}

func blip(t *Track, at, f, dur, gain float64) {
	i0 := int(at * AudioRate)
	for k := 0; k < int(dur*AudioRate); k++ {
		s := float64(k) / AudioRate
		v := math.Sin(2*math.Pi*f*s) * math.Exp(-s/dur*5) * gain
		t.add(i0+k, v, v)
	}
}

func bell(t *Track, at, f, gain float64) {
	i0 := int(at * AudioRate)
	partials := []struct{ ratio, amp, decay float64 }{{1, 1, 1.6}, {2.76, 0.45, 2.6}, {5.4, 0.25, 4}, {8.93, 0.12, 6}}
	for k := 0; k < int(2.5*AudioRate); k++ {
		s := float64(k) / AudioRate
		v := 0.0
		for _, p := range partials {
			v += math.Sin(2*math.Pi*f*p.ratio*s) * p.amp * math.Exp(-s*p.decay)
		}
		v *= gain * math.Min(1, s/0.002)
		t.add(i0+k, v*0.9, v)
	}
}
