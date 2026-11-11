package production

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func rms(t *Track, from, to float64) float64 {
	i0, i1 := int(from*AudioRate), min(len(t.L), int(to*AudioRate))
	s := 0.0
	for i := i0; i < i1; i++ {
		s += t.L[i]*t.L[i] + t.R[i]*t.R[i]
	}
	return math.Sqrt(s / float64(max(1, 2*(i1-i0))))
}

func TestComposeMusicIsDeterministicAndBounded(t *testing.T) {
	for _, style := range []string{StyleMinimal, StyleDocumentary, StyleBold, StyleTech} {
		a := ComposeMusic(MoodFor(style), 6, "video-1")
		b := ComposeMusic(MoodFor(style), 6, "video-1")
		if a.Seconds() < 5.99 || a.Seconds() > 6.01 {
			t.Fatalf("%s: length %v", style, a.Seconds())
		}
		if p := a.Peak(); p > 0.71 || p < 0.5 {
			t.Fatalf("%s: peak %v", style, p)
		}
		if rms(a, 2, 4) < 0.02 {
			t.Fatalf("%s: music is nearly silent", style)
		}
		for i := 0; i < len(a.L); i += 997 {
			if a.L[i] != b.L[i] {
				t.Fatalf("%s: same seed rendered differently", style)
			}
		}
	}
	x := ComposeMusic(MoodFor(StyleTech), 4, "seed-a")
	y := ComposeMusic(MoodFor(StyleTech), 4, "seed-b")
	same := true
	for i := 0; i < len(x.L); i += 211 {
		if x.L[i] != y.L[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("different seeds should vary the music")
	}
}

func TestPlanEffectsFollowsScenes(t *testing.T) {
	scenes := PlanScenes([]Timed{
		{Duration: 3, Text: "Hook."},
		{Duration: 4, Title: "3.8 cm per year", Text: "x"},
		{Duration: 5, Title: "Tidal Pull", Text: "y"},
		{Duration: 3, Text: "Bye."},
	}, nil, "Quiet Orbit")
	cues := PlanEffects(scenes)
	count := map[string]int{}
	for _, c := range cues {
		count[c.Kind]++
		if c.Kind == SFXTick && (c.At < 3.2 || c.At > 3.2+1.81) {
			t.Fatalf("tick outside the counter: %v", c.At)
		}
	}
	if count[SFXImpact] != 1 || count[SFXWhoosh] != 2 || count[SFXTick] != 14 || count[SFXChime] != 1 || count[SFXRiser] != 1 {
		t.Fatalf("cues = %v", count)
	}
}

func TestRenderEffectsPlacesSound(t *testing.T) {
	tr := RenderEffects([]SFXCue{{At: 2, Kind: SFXWhoosh}}, 4, "v")
	if rms(tr, 0, 1.5) != 0 {
		t.Fatal("sound before the cue")
	}
	if rms(tr, 1.7, 2.3) < 0.01 {
		t.Fatal("whoosh missing around its cue")
	}
	silent := RenderEffects(nil, 2, "v")
	if silent.Peak() != 0 {
		t.Fatal("no cues should render silence")
	}
}

func TestWriteWAVHeader(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.wav")
	tr := NewTrack(0.5)
	tr.L[0], tr.R[0] = 2, -2 // clipped on write
	if err := tr.WriteWAV(p); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" || binary.LittleEndian.Uint32(b[24:28]) != AudioRate || binary.LittleEndian.Uint16(b[22:24]) != 2 {
		t.Fatalf("bad header % x", b[:44])
	}
	if len(b) != 44+len(tr.L)*4 {
		t.Fatalf("size %d", len(b))
	}
	if int16(binary.LittleEndian.Uint16(b[44:46])) != 32767 || int16(binary.LittleEndian.Uint16(b[46:48])) != -32767 {
		t.Fatal("samples not clipped")
	}
}

func TestFFmpegArgsMixesSoundtrack(t *testing.T) {
	args := FFmpegArgs("visual.mp4", AudioMix{Voice: []string{"a.wav", "b.wav"}, Music: "music.wav", Effects: "effects.wav"}, 10, 1080, 1920, "#000000", "c.ass", "f", "o.mp4")
	joined := strings.Join(args, " ")
	for _, want := range []string{"-i music.wav", "-i effects.wav", "[3:a]volume=0.30[bed]", "sidechaincompress", "[voice][music][4:a]amix=inputs=3:duration=first", "loudnorm=I=-14"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %s", want, joined)
		}
	}
	voiceOnly := strings.Join(FFmpegArgs("", AudioMix{Voice: []string{"a.wav"}}, 3, 1080, 1920, "#000000", "c.ass", "f", "o.mp4"), " ")
	if strings.Contains(voiceOnly, "sidechain") || !strings.Contains(voiceOnly, "[voice];[voice]amix=inputs=1") {
		t.Fatalf("voice-only mix: %s", voiceOnly)
	}
}
