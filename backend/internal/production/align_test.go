package production

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
)

const sampleRecognition = `num threads: 2
Elapsed seconds: 0.118 s
{"lang": "", "text": "The moon is 1200 km away.", "ys_log_probs": [-0.1, -0.000000-0.2], "timestamps": [0.16, 0.32, 0.48, 0.64, 0.96, 1.40, 1.60, 1.70], "tokens":[" The", " mo", "on", " is", " twelve", " hundred", " km", " away."], "words": []}
`

func TestParseRecognitionJoinsSubwordTokens(t *testing.T) {
	got, err := parseRecognition([]byte(sampleRecognition))
	if err != nil {
		t.Fatal(err)
	}
	want := []heardWord{{"The", 0.16}, {"moon", 0.32}, {"is", 0.64}, {"twelve", 0.96}, {"hundred", 1.40}, {"km", 1.60}, {"away.", 1.70}}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("word %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if _, err := parseRecognition([]byte("no json here")); err == nil {
		t.Error("expected an error without a result line")
	}
	if _, err := parseRecognition([]byte(`{"tokens":["a"],"timestamps":[]}`)); err == nil {
		t.Error("expected an error for mismatched arrays")
	}
}

func TestAlignWordsMatchesAndInterpolates(t *testing.T) {
	heard, _ := parseRecognition([]byte(sampleRecognition))
	// "1,200" is heard as two words; it is interpolated between neighbors.
	got := alignWords([]string{"The", "Moon", "is", "1,200", "km", "away."}, heard, 2.5)
	want := []float64{0.16, 0.32, 0.64, 1.12, 1.60, 1.70}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Fatalf("times = %v, want %v", got, want)
		}
	}
	// Nothing heard: spread evenly, never decreasing.
	even := alignWords([]string{"a", "b", "c", "d"}, nil, 4)
	if even[0] != 0 || even[3] != 3 {
		t.Errorf("even spread = %v", even)
	}
	// Trailing unmatched words spread over the rest of the audio.
	tail := alignWords([]string{"The", "zzz", "yyy"}, heard[:1], 3)
	if !(tail[1] > tail[0] && tail[2] > tail[1] && tail[2] < 3) {
		t.Errorf("tail = %v", tail)
	}
}

type fakeRunner struct {
	out   []byte
	err   error
	calls int
}

func (f *fakeRunner) Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	f.calls++
	return f.out, f.err
}

func TestAlignerCachesAndReportsErrors(t *testing.T) {
	dir := t.TempDir()
	wav := filepath.Join(dir, "abc.wav")
	_ = os.WriteFile(wav, []byte("RIFF"), 0o644)
	r := &fakeRunner{out: []byte(sampleRecognition)}
	a := Aligner{Exe: "asr", ModelDir: dir, Runner: r}
	text := "The moon is 1,200 km away."
	first, err := a.WordTimes(context.Background(), wav, text, 2.5)
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.WordTimes(context.Background(), wav, text, 2.5)
	if err != nil || r.calls != 1 {
		t.Fatalf("second call err=%v runs=%d, want a cache hit", err, r.calls)
	}
	if len(first) != 6 || first[5] != second[5] {
		t.Errorf("cached times differ: %v vs %v", first, second)
	}
	failing := Aligner{Exe: "asr", ModelDir: dir, Runner: &fakeRunner{err: errors.New("boom")}}
	if _, err := failing.WordTimes(context.Background(), filepath.Join(dir, "other.wav"), text, 2.5); err == nil {
		t.Error("expected the runner error")
	}
	if (Aligner{Exe: filepath.Join(dir, "missing.exe"), ModelDir: dir}).Available() {
		t.Error("aligner without tools reported available")
	}
}

func TestCuesUseMeasuredWordTimes(t *testing.T) {
	segs := []Timed{
		{Duration: 3, Text: "one two three four", Words: []float64{0.2, 0.4, 2.0, 2.5}},
		{Duration: 2, Text: "five six"},
	}
	cues := Cues(segs, 2)
	if len(cues) != 3 {
		t.Fatalf("cues = %+v", cues)
	}
	// The first chunk starts at 0 (covers leading silence); the second at
	// the measured start of "three"; segment two is estimated.
	if cues[0].Start != 0 || cues[0].End != 2.0 || cues[1].Start != 2.0 || cues[1].End != 3 || cues[2].Start != 3 || cues[2].End != 5 {
		t.Errorf("cues = %+v", cues)
	}
	track := CaptionTrack(segs, 2)
	if track[0].Words[1].At != 0.4 || track[1].Words[1].At != 2.5 {
		t.Errorf("word times = %+v", track)
	}
}
